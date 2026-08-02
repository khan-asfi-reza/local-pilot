# OpenBazaar — Web Cash-on-Delivery Auction Marketplace

Build the complete backend for **OpenBazaar**, a web marketplace where users list
items for **Fixed price**, **Auction**, or **Hybrid**, bid in real time, and pay
**Cash on Delivery (CoD)**. Implement it as a runnable reference service using
**FastAPI** with the standard-library **sqlite3** (the production target is
PostgreSQL/Redis but deliver a working sqlite implementation), plus a simple
**single-file HTML/JS frontend**. Everything must run and its test suite must pass.

## 1. Roles & permissions
- Guest: browse & search only.
- Registered Buyer: browse, place bids, Buy-Now (CoD).
- Registered Seller: everything a buyer can, plus create listings.
- Admin: moderate/audit.
Every user has a `cod_reliability_score` starting at 100.00.

## 2. Files
- `backend/main.py` — FastAPI app, all endpoints, SSE stream.
- `backend/db.py` — sqlite schema + connection + data access.
- `backend/logic.py` — pure functions for auction/CoD rules (no I/O; unit-testable).
- `backend/requirements.txt` — pin `fastapi`, `uvicorn`.
- `backend/test_api.py` — a plain script (no pytest) using FastAPI `TestClient`
  that exercises every rule below and prints `all tests passed`.
- `frontend/index.html` — a self-contained page (React via CDN or vanilla JS)
  that lists items, shows a product detail with a live bid box, and can place a
  bid and Buy-Now. It fetches the API.

## 3. Data model (sqlite tables)
- `users(id, full_name, email UNIQUE, phone UNIQUE, password_hash, cod_reliability_score REAL DEFAULT 100.0, is_verified INT DEFAULT 0)`
- `categories(id, parent_id, name, slug UNIQUE)` — supports a 3-tier tree.
- `items(id, seller_id, title, description, category_id, condition_rating['NEW'|'LIKE_NEW'|'GOOD'|'FAIR'|'FOR_PARTS'], sale_type['FIXED'|'AUCTION'|'HYBRID'], fixed_price, starting_bid, reserve_price, current_highest_bid REAL DEFAULT 0, bid_increment REAL DEFAULT 1, auction_start_ts, auction_end_ts, status['DRAFT'|'ACTIVE'|'SOLD'|'EXPIRED'|'CANCELLED'] DEFAULT 'ACTIVE')`
- `bids(id, item_id, bidder_id, bid_amount, is_auto_bid INT DEFAULT 0, max_proxy_amount)`
- `orders(id, item_id, buyer_id, seller_id, final_amount, payment_method DEFAULT 'COD', order_status['PENDING_OTP'|'CONFIRMED'|'DISPATCHED'|'DELIVERED_PAID'|'REFUSED_CANCELLED'] DEFAULT 'PENDING_OTP', shipping_address, shipping_phone, otp_code, courier_tracking_id)`

## 4. Endpoints
- `POST /users` — register (score starts 100).
- `POST /categories` — create a category (optional parent_id for the tree).
- `GET /categories` — the category tree.
- `POST /items` — create a listing (validate required fields per sale_type:
  FIXED needs fixed_price; AUCTION needs starting_bid, bid_increment,
  reserve_price, auction_end_ts; title 15–100 chars).
- `GET /items` — search/list, filterable by `category_id` and `status`.
- `GET /items/{id}` — detail incl. current_highest_bid, status, time left.
- `POST /items/{id}/bid` — body `{bidder_id, amount, max_proxy?}`. Applies the
  bidding rules (section 5). Returns the new current_highest_bid.
- `GET /items/{id}/stream` — **Server-Sent Events**: streams the item's current
  bid state to connected clients (emit an event when the bid changes).
- `POST /items/{id}/buy` — Buy-Now on a FIXED/HYBRID item at fixed_price: creates
  a PENDING_OTP order.
- `POST /items/{id}/close` — conclude an auction whose `auction_end_ts` has passed.
- `POST /orders/{id}/otp` — body `{otp}`: CONFIRMED on match, else unchanged.
- `POST /orders/{id}/dispatch` — mark DISPATCHED (only from CONFIRMED).
- `POST /orders/{id}/deliver` — body `{paid: bool}`: DELIVERED_PAID (buyer score
  +2, funds remitted) or REFUSED_CANCELLED (buyer score −25, item back to ACTIVE).

## 5. Business rules (in `logic.py`, verified by `test_api.py`)
1. **Minimum valid bid** = `current_highest_bid + bid_increment` (or
   `starting_bid` for the first bid). Lower bids are rejected with HTTP 400.
2. **Proxy auto-bidding**: a bid with `max_proxy` keeps the bidder in the lead by
   auto-raising to the minimum needed to beat challengers, never exceeding
   `max_proxy`. When two proxies collide, the higher max wins at the loser's max +
   increment.
3. **Anti-sniping**: a valid bid within the final **180 seconds** of the auction
   extends `auction_end_ts` by **+180 seconds**.
4. **Conclusion** (`close`): if the highest bid `>= reserve_price`, item → SOLD
   and a PENDING_OTP order is created for the top bidder at the winning amount;
   otherwise item → EXPIRED and no order.
5. **CoD lifecycle & reliability**: order starts PENDING_OTP → correct OTP →
   CONFIRMED → dispatch → DISPATCHED → deliver(paid) → DELIVERED_PAID (buyer
   score +2) OR deliver(refused) → REFUSED_CANCELLED (buyer score −25 and item
   returns to ACTIVE). A score below 75 suspends the buyer's CoD privilege
   (Buy-Now/bid returns 403 for that user).
6. **Security**: hash passwords (hashlib is fine for the reference impl); reject a
   bid on an inactive/expired item.

## 6. Verify
`python backend/test_api.py` must print `all tests passed`, covering: registration;
category tree; listing creation validation; first-bid-below-start rejected; a
valid raise; proxy auto-bid taking and holding the lead; anti-sniping extending
the end time; conclusion creating an order when reserve is met and expiring when
not; the full OTP→dispatch→deliver lifecycle with both paid and refused outcomes
and the correct score changes; and CoD suspension below score 75.
