# Inventra — Warehouse Inventory System (PRD)
Stack: Fastify (Node + TypeScript) backend, React + Vite frontend, PostgreSQL.

## 1. Overview
Inventra tracks products, stock levels across warehouses, purchase orders, and suppliers, with low-stock alerts. Responsive web.

## 2. Roles & Auth
- Manager: full CRUD, approve orders.
- Staff: adjust stock, create orders.
JWT auth, hashed passwords, role checks on approve/delete.

## 3. Products & Stock
Product: sku, name, category, unit_price, reorder_level, image_url. Stock: product, warehouse, quantity. Endpoints to CRUD products, list stock by warehouse, and adjust stock (in/out) writing a stock_movement record.

## 4. Suppliers & Purchase Orders
Supplier: name, email, lead_time_days. PurchaseOrder: supplier, status (draft/approved/received), lines[] (product, qty, unit_cost). Approving an order is manager-only; receiving it increments stock.

## 5. Alerts & Reports
Endpoint lists products at or below reorder_level (low-stock). A summary endpoint returns total SKUs, total stock value, and low-stock count.

## 6. UI/UX
Sticky nav, products table with search + category filter + a low-stock badge, a product form, a stock-adjust modal, a purchase-orders page with an approve action, and a dashboard with the summary tiles. Modern responsive design, loading/empty states. Controls call the real API.

## 7. Data (PostgreSQL)
users, products, warehouses, stock, stock_movements, suppliers, purchase_orders, purchase_order_lines. Migrations before boot. Seed products, a warehouse with stock, a supplier, and one order.

## 8. Acceptance
- Signup/login works.
- CRUD products; adjust stock (movement recorded); create + approve + receive an order (stock increments).
- Low-stock list + dashboard summary return real numbers.
- Fastify boots; endpoints return real data; frontend builds and renders it.
