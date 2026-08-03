# OpenBazaar — Backend PRD (Lite)

**Scope:** REST + SSE API for a 100% Cash-on-Delivery marketplace. Direct Buy + Live Auction.
**Stack:** Go (or Node.js) single service, PostgreSQL, Redis. NGINX in front.
**Out of scope (deferred):** Kafka, read replicas, multi-node autoscaling, admin analytics.

---

## 1. Roles

| Role | Browse | Bid | Buy Now | List | Moderate |
|---|---|---|---|---|---|
| Guest | yes | no | no | no | no |
| Buyer | yes | yes | yes | no | no |
| Seller | yes | yes | yes | yes | no |
| Admin | yes | no | no | moderate | yes |

Seller = buyer who has completed one listing; single `users` row, no separate table.

---

## 2. Auth

- Signup: name, email, phone, password. Phone unique and required (CoD depends on it).
- `argon2id` password hashing. JWT access token (15 min) + refresh token (30 d, rotated).
- Phone verified once at signup via SMS OTP → sets `users.is_verified`.
- Rate limit: 5 auth attempts / min / IP.

---

## 3. Listing Engine

Validation enforced server-side; client checks are advisory only.

| Field | Rule |
|---|---|
| images | 3–10, converted to WebP on upload, primary cropped 1:1 |
| video | optional, 1 max, ≤30 s, ≤50 MB |
| title | 15–100 chars |
| category | must be a leaf of the 3-tier `categories` tree |
| condition | `NEW` \| `LIKE_NEW` \| `GOOD` \| `FAIR` \| `FOR_PARTS` |
| sale_type | `FIXED` \| `AUCTION` \| `HYBRID` |
| defects | mandatory free-text disclosure, non-empty |
| location | city, area, postal code |

Price rules by `sale_type`:
- `FIXED` → `fixed_price` required, auction fields null.
- `AUCTION` → `starting_bid`, `bid_increment_step`, `auction_end_time` required. `reserve_price` optional.
- `HYBRID` → both sets required; a Buy-Now purchase closes the auction immediately and voids open bids.

Media stored on object storage (S3-compatible); DB holds keys only.

---

## 4. Auction Engine

- **Min next bid** = `current_highest_bid + bid_increment_step` (or `starting_bid` if no bids).
- **Atomicity:** every bid runs inside a Redis Lua script keyed `auction:{item_id}` — read current high, validate, write, release. DB insert follows in the same request; on DB failure the Redis value is rolled back.
- **Proxy bidding:** buyer submits `max_proxy_amount`. System places the minimum winning bid and re-bids automatically when outbid, up to the cap. Ties resolve to the earlier `created_at`.
- **Anti-sniping:** bid inside the final 3 min → `auction_end_time += 3 min`. No cap on extensions.
- **Close:** a scheduled sweeper runs every 10 s over `idx_items_status_end_time`.
  - `highest_bid >= reserve_price` (or no reserve) → item `SOLD`, create order `PENDING_OTP`.
  - else → item `EXPIRED`, no order.
- **Live updates:** `GET /items/{id}/stream` (SSE). Events: `bid`, `extended`, `closed`. Redis pub/sub fans out to connected clients. Heartbeat comment every 20 s.

Self-bidding rejected. Bidders with `cod_reliability_score < 75` rejected.

---

## 5. CoD Order Lifecycle

```
Buy Now / Auction won
        v
  PENDING_OTP  --- OTP fail or 15 min timeout ---> REFUSED_CANCELLED (item back to ACTIVE)
        v (OTP verified)
   CONFIRMED
        v (seller hands to courier)
  DISPATCHED
        v
  +----------------------+---------------------------+
  v                                                  v
DELIVERED_PAID (score +2)              REFUSED_CANCELLED (score -25)
remit to seller                        item stays SOLD, no relist
```

- OTP: 6 digits, sent by SMS/WhatsApp, 15 min TTL, 3 attempts, stored in `orders.otp_code`.
- Only the courier webhook (signed, HMAC) may set `DELIVERED_PAID` / `REFUSED_CANCELLED`.
- Reliability score clamped to [0, 100]. `< 75` blocks placing new CoD orders.
- Status transitions are whitelisted; any other pair returns `409`.

---

## 6. API Surface

```
POST   /auth/signup                 POST   /auth/login
POST   /auth/otp/verify             POST   /auth/refresh

GET    /categories                  # full tree, cached 1 h
GET    /items                       # q, category, condition, sale_type, min/max price,
                                    # sort=ending_soon|newest|price, cursor paginated
GET    /items/{id}
POST   /items                       # seller
PATCH  /items/{id}                  # seller, only while DRAFT or ACTIVE with 0 bids
POST   /items/{id}/media

GET    /items/{id}/stream           # SSE
POST   /items/{id}/bids             # {amount} or {max_proxy_amount}
GET    /items/{id}/bids             # public history, bidder names masked

POST   /orders                      # Buy Now
GET    /orders                      # role-scoped: as buyer or as seller
GET    /orders/{id}
POST   /orders/{id}/otp/verify
POST   /orders/{id}/dispatch        # seller
POST   /webhooks/courier            # HMAC signed
```

Errors: `{ "error": { "code": "BID_TOO_LOW", "message": "...", "meta": {...} } }`.
Codes the frontend must handle: `BID_TOO_LOW`, `AUCTION_ENDED`, `SELF_BID`, `SCORE_TOO_LOW`, `OTP_INVALID`, `OTP_EXPIRED`, `ITEM_NOT_AVAILABLE`, `INVALID_TRANSITION`.

---

## 7. Caching & Performance

- Item detail cached in Redis 60 s, invalidated on bid/close.
- Category tree cached 1 h.
- Search hits Postgres directly with `idx_items_category` + `idx_items_status_end_time`; add a trigram index on `title` for `q`.
- Targets: p95 read `< 150 ms`, p95 bid write `< 250 ms`, SSE fan-out `< 500 ms`.

---

## 8. Security

- NGINX rate limits: 5 bids/sec/IP, 60 reads/sec/IP.
- TLS 1.3 only. `argon2id` hashing.
- Flag for manual review: >20 bids/min from one user, or two accounts sharing a device fingerprint bidding on the same item.
- All mutations audit-logged (actor, action, entity, before/after).

---

## 9. Database Schema

Unchanged from the original PRD.

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE users (
    user_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    full_name VARCHAR(100) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    phone_number VARCHAR(20) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    cod_reliability_score DECIMAL(5,2) DEFAULT 100.00,
    is_verified BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE categories (
    category_id SERIAL PRIMARY KEY,
    parent_id INT REFERENCES categories(category_id) ON DELETE SET NULL,
    name VARCHAR(100) NOT NULL,
    slug VARCHAR(120) UNIQUE NOT NULL
);

CREATE TABLE items (
    item_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    seller_id UUID NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    title VARCHAR(150) NOT NULL,
    description TEXT NOT NULL,
    category_id INT NOT NULL REFERENCES categories(category_id),
    condition_rating VARCHAR(30) NOT NULL CHECK (condition_rating IN ('NEW', 'LIKE_NEW', 'GOOD', 'FAIR', 'FOR_PARTS')),
    sale_type VARCHAR(20) NOT NULL CHECK (sale_type IN ('FIXED', 'AUCTION', 'HYBRID')),
    fixed_price DECIMAL(12,2),
    starting_bid DECIMAL(12,2),
    reserve_price DECIMAL(12,2),
    current_highest_bid DECIMAL(12,2) DEFAULT 0.00,
    bid_increment_step DECIMAL(10,2) DEFAULT 1.00,
    auction_start_time TIMESTAMP WITH TIME ZONE,
    auction_end_time TIMESTAMP WITH TIME ZONE,
    status VARCHAR(20) DEFAULT 'ACTIVE' CHECK (status IN ('DRAFT', 'ACTIVE', 'SOLD', 'EXPIRED', 'CANCELLED')),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE bids (
    bid_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    item_id UUID NOT NULL REFERENCES items(item_id) ON DELETE CASCADE,
    bidder_id UUID NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    bid_amount DECIMAL(12,2) NOT NULL,
    is_auto_bid BOOLEAN DEFAULT FALSE,
    max_proxy_amount DECIMAL(12,2),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_bids_item_id_amount ON bids(item_id, bid_amount DESC);
CREATE INDEX idx_items_status_end_time ON items(status, auction_end_time);
CREATE INDEX idx_items_category ON items(category_id);

CREATE TABLE orders (
    order_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    item_id UUID NOT NULL REFERENCES items(item_id) ON DELETE RESTRICT,
    buyer_id UUID NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    seller_id UUID NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    final_amount DECIMAL(12,2) NOT NULL,
    payment_method VARCHAR(20) DEFAULT 'COD',
    order_status VARCHAR(30) DEFAULT 'PENDING_OTP'
        CHECK (order_status IN ('PENDING_OTP', 'CONFIRMED', 'DISPATCHED', 'DELIVERED_PAID', 'REFUSED_CANCELLED')),
    shipping_address TEXT NOT NULL,
    shipping_phone VARCHAR(20) NOT NULL,
    courier_tracking_id VARCHAR(100),
    otp_code VARCHAR(6),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

---

## 10. Build Order

1. Auth + users + OTP
2. Categories seed + items CRUD + media upload
3. Search / listing read paths + caching
4. Bids + Redis Lua atomic bid + SSE
5. Proxy bidding + anti-sniping + close sweeper
6. Orders + OTP verify + dispatch + courier webhook + reliability score
7. Rate limits, audit log, fraud flags
