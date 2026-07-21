---
name: remix
description: Remix (React Router) app conventions.
internal: true
---
# Remix
- Routes in `app/routes`. Export `loader` (server GET data), `action` (mutations), and the default component. Use `useLoaderData`.
- Forms post to `action` with `<Form>`; progressive enhancement first. Keep server-only code in `loader`/`action`.
- Keep deps in `package.json`. Verify: `npm run build`.
