---
name: express
description: Express (Node) API conventions.
internal: true
---
# Express
- `const app = express()`; group routes with `express.Router()`. Use `express.json()` middleware for bodies. Order matters: middleware before routes, error handler last (4-arg).
- Keep deps in `package.json`. Don't block the event loop; use async handlers with try/catch or a wrapper.
- Verify: `npm run migrate` (if a DB), start via the serve tool (not blocking shell_run), then curl `/health` and your endpoints.

## The scaffold already gives you these — use them, don't fight them
- **Env is loaded** (`src/index.ts` starts with `import './env'`). Just read `process.env.DATABASE_URL`, `process.env.PORT`, etc. Do NOT add another dotenv setup. The API runs on **port 3001**; don't change it.
- **DB pool**: `import { pool } from './db'`. Use `pool.query('… $1', [v])`.
- **CORS is already enabled** (open, for the dev proxy). Don't set specific origins.

## Auth is already built — if `src/auth.ts` exists, use it
When the spec needs authentication, the scaffold provides `src/auth.ts` (JWT + bcrypt) and `migrations/000_auth.sql` (the `users` table), and `POST /api/auth/register`, `POST /api/auth/login`, `GET /api/auth/me` are already mounted in `src/index.ts`. So:
- **Never** recreate the `users` table, a register/login route, JWT signing, or password hashing.
- To protect a route, `import { requireAuth, requireRole, AuthRequest } from './auth'` and add `requireAuth` as middleware; the user is on `req.user` (`{ id, email, role }`).
- Feature tables reference the user with a `user_id INTEGER REFERENCES users(id)` column. Need an extra column on users? add it with `ALTER TABLE users ADD COLUMN ...` in your migration — don't redefine the table.

## Seed real demo data (empty lists = broken product)
Add a `seed` npm script (or `src/seed.ts`) that inserts demo rows with `pool.query(...)` and is idempotent. The scaffold's data must be REAL so the UI renders content, not empty states. Seed the tables your features list (users via the auth flow or a direct insert, plus each feature table).

## Never break these
1. **Schema lives in `migrations/NNN_name.sql`** as plain SQL; it applies with `npm run migrate` (a runner is already in `src/migrate.ts`). Do NOT create tables ad-hoc in JS, and do NOT leave `.sql` files unrun.
2. **Every model/service/controller MUST match the migration schema** — identical table and column names. Decide the schema once and keep code and SQL in sync (a query for a column the migration never created is a runtime 500).
3. **Mount every router** in `src/index.ts`: `app.use('/api/<name>', <name>Router)`. A router file you never mount is dead code and the frontend gets 404s.
4. **The frontend calls you at `/api/...`** through the Vite proxy — keep that prefix on your routes.
5. **Every package you `import` must be in `package.json` dependencies** (e.g. `jsonwebtoken`, `bcrypt`, `zod`). An unlisted import crashes the server with "Cannot find module".
6. **Prefer `write_file` (whole file) over `edit_file`.** A partial insert mid-file easily breaks it; if you edit, keep it minimal and re-read after to confirm it still parses.
7. **Services must really query the database** via `pool.query(...)` and return the rows. Do NOT return mock objects, empty arrays, or leave `// In real implementation…` stubs — an endpoint that returns fake/empty data is a broken feature. Every GET reads real rows; every POST/PUT/DELETE writes real rows and returns the result.
