# OpenBazaar — Frontend PRD (Lite)

**Scope:** Responsive web app (desktop / tablet / mobile). No native app, no PWA offline mode.
**Stack:** React + Vite, TanStack Query for server state, Tailwind, native `EventSource` for SSE.
**Consumes:** the API in `op_be.md`. No business rules duplicated here — server is authoritative; client validation is UX only.

---

## 1. Performance Budget

| Metric | Target |
|---|---|
| LCP (4G) | `< 1.2 s` |
| INP | `< 50 ms` |
| CLS | `< 0.05` |
| Initial JS | `< 180 KB` gzipped |

Route-level code splitting. Images served as WebP with explicit `width`/`height` to hold layout. Grid cards use fixed 1:1 aspect boxes.

---

## 2. Routes

| Route | Auth | Purpose |
|---|---|---|
| `/` | guest | Home: hero search, ending-soon rail, category tiles |
| `/search` | guest | Results grid + filters, URL-synced state |
| `/item/:id` | guest | Product detail page (PDP) |
| `/login`, `/signup` | guest | Auth + phone OTP step |
| `/sell/new` | seller | Listing wizard |
| `/me/listings` | seller | Own items, status, bid counts |
| `/me/orders` | buyer | Orders as buyer and as seller, tabbed |
| `/order/:id` | owner | Order detail, OTP entry, tracking |

Guest hitting a gated action → auth modal, not a redirect; the pending action resumes after login.

---

## 3. Home & Search

- Sticky header: logo, search input with debounced (250 ms) autocomplete, category menu, auth/avatar.
- Grid columns: 4 desktop (≥1280), 3 tablet (≥768), 2 mobile.
- Card shows: primary image, title (2-line clamp), condition badge, price or current bid, countdown if auction, location.
- Filters: category tree, condition, sale type, price range, sort (`ending_soon` default on auction views, else `newest`). All filters live in the query string so results are shareable and back-button works.
- Cursor pagination via infinite scroll with a visible "Load more" fallback.
- Skeleton cards on first load; keep previous results visible while refetching.

---

## 4. Product Detail Page

- **Media:** carousel, thumbnail strip, arrow/swipe nav, click to zoom. Video plays inline, muted, no autoplay.
- **Conversion box** (sticky right on desktop, sticky bottom bar on mobile):
  - Live countdown, seconds-precision under 5 min.
  - Current bid + bid count, min next bid shown explicitly.
  - Quick-bid buttons `+$5 / +$10 / +$25` plus a custom amount field.
  - Proxy bid entry behind a "Set max bid" disclosure.
  - `Buy Now via Cash on Delivery` primary CTA for `FIXED` / `HYBRID`.
- **Below the fold:** description, spec key-value grid, **Known Defects** block styled as a warning callout (never collapsed by default), seller card with reliability score, bid history with masked names.
- **Live via SSE** on `/items/{id}/stream`:
  - `bid` → update price, bid count, min next bid; flash the price; append to history.
  - `extended` → update countdown, toast "Auction extended 3 minutes".
  - `closed` → swap conversion box for the outcome state (won / lost / reserve not met).
  - On disconnect: reconnect with exponential backoff (1 s → 30 s cap), show a subtle "reconnecting" dot, refetch item on reconnect to close any gap.

---

## 5. Bidding UX

- Optimistic: show the bid as pending immediately, reconcile on server response.
- Error mapping (from server `error.code`):
  - `BID_TOO_LOW` → inline field error with the corrected minimum, prefilled.
  - `AUCTION_ENDED` → replace box with closed state.
  - `SELF_BID` → inline "You own this listing".
  - `SCORE_TOO_LOW` → modal explaining CoD suspension.
- Outbid while viewing → toast "You've been outbid" + one-tap re-bid at the new minimum.
- Bid button disabled while a request is in flight; double-submit impossible.

---

## 6. Listing Wizard (`/sell/new`)

Four steps, progress bar, autosaved as `DRAFT` after each step.

1. **Media** — drag-drop, 3–10 images, per-file progress, reorder to pick primary, client-side preview crop 1:1. Reject oversize files before upload.
2. **Details** — title with live char counter (15–100), 3-tier category cascader, condition radio group with plain-language help text.
3. **Pricing** — sale type segmented control; the form shows only the fields that type needs. Auction: starting bid, increment, duration preset (1/3/5/7 days) or custom end time, optional reserve.
4. **Disclosure & logistics** — spec key-value rows (add/remove), **required** defects textarea, location fields, weight class.

Final step shows a read-only preview of the PDP card + conversion box before publish.

---

## 7. CoD Order Flow

- After Buy Now / auction win → order page in `PENDING_OTP`.
- **OTP screen:** 6-digit segmented input, autofocus, paste-aware, resend after 60 s cooldown, visible countdown to the 15 min expiry, attempts remaining shown after the first failure.
- Timeout / 3 failures → cancelled state with a "browse similar" link.
- **Status timeline** component for the five statuses; current step highlighted, past steps checked, courier tracking ID and copy button when present.
- Seller view of the same order adds a `Mark dispatched` action, disabled until `CONFIRMED`.
- Reliability score shown on the buyer's profile with a one-line explainer; visible warning banner below 80.

---

## 8. Component Inventory

`Header` · `SearchInput` · `CategoryCascader` · `FilterPanel` · `ItemGrid` · `ItemCard` · `Countdown` · `MediaCarousel` · `ConversionBox` · `QuickBidRow` · `BidHistory` · `SpecGrid` · `DefectCallout` · `SellerCard` · `OtpInput` · `OrderTimeline` · `Toast` · `AuthModal` · `Skeleton`

---

## 9. Accessibility

- Keyboard-operable carousel, cascader, and OTP input; visible focus rings.
- Countdown and live bid updates announced via `aria-live="polite"`, throttled to at most one announcement per 5 s.
- Contrast ≥ 4.5:1. Condition and status conveyed by label, never colour alone.
- All images require alt text; seller-supplied media falls back to the item title.

---

## 10. Build Order

1. Shell, routing, auth modal, API client + error mapping
2. Home + search + filters + `ItemCard` / `ItemGrid`
3. PDP static (media, specs, defects, seller)
4. SSE wiring + `ConversionBox` + bidding UX
5. Listing wizard
6. Order flow: OTP, timeline, dispatch action
7. Perf pass (budgets), a11y pass
