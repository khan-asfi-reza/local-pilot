# ShipFast — Logistics & Shipment Tracking (PRD)
Stack: Go (Gin) backend, React + Vite frontend, PostgreSQL (pgx or database/sql).

## 1. Overview
ShipFast manages shipments from creation to delivery, with live tracking events. Responsive web.

## 2. Roles & Auth
- Merchant: create shipments, view status.
- Driver: pick up, update location/status, mark delivered.
- Admin: assign drivers.
JWT auth, hashed passwords, role-checked write endpoints.

## 3. Shipments
Shipment: tracking_number (generated), merchant, origin, destination, weight_kg, status (created/assigned/in_transit/out_for_delivery/delivered/failed), assigned_driver, cost. CRUD + assign-driver + status transitions with validation.

## 4. Tracking Events
Each status change appends a tracking_event (shipment, status, note, location, timestamp). Endpoint returns a shipment's ordered event history — the tracking timeline.

## 5. Drivers & Routes
Driver: name, vehicle, phone, active. List a driver's assigned shipments (their route) ordered by created time.

## 6. Real-Time
A Server-Sent Events endpoint streams tracking updates for a shipment so the tracking page updates live.

## 7. UI/UX
Sticky nav, shipments table with status filters, a create-shipment form, a shipment detail page with a tracking timeline + live updates, and a driver route view with status-update buttons. Modern responsive design, status badges, loading/empty states. Controls call the real API.

## 8. Data (PostgreSQL)
users, shipments, tracking_events, drivers. SQL migrations run before boot. Indexes on tracking_number + shipment status. Seed a few shipments with events + a driver.

## 9. Acceptance
- Signup/login works.
- Merchant creates a shipment (gets a tracking number); admin assigns a driver; driver advances status and events append.
- Tracking timeline shows the ordered events; the SSE stream pushes updates.
- Go builds (go build ./...); server boots; endpoints return real data; frontend builds and renders it.
