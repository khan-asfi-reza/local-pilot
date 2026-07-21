---
name: express
description: Express (Node) API conventions.
internal: true
---
# Express
- `const app = express()`; group routes with `express.Router()`. Use `express.json()` middleware for bodies. Order matters: middleware before routes, error handler last (4-arg).
- Keep deps in `package.json`. Don't block the event loop; use async handlers with try/catch or a wrapper.
- Verify: start via the serve tool (not blocking shell_run) then curl an endpoint, or run tests.
