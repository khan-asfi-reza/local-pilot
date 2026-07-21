---
name: nextjs
description: Next.js app conventions.
internal: true
---
# Next.js
- Detect the router: App Router (`app/`) vs Pages Router (`pages/`). Follow whichever the project uses.
- App Router: components are Server Components by default; add `"use client"` only when you need hooks/browser APIs. Use `app/page.tsx`, `layout.tsx`, route folders.
- Data fetching in Server Components / route handlers; don't fetch in client components unnecessarily.
- Keep deps in `package.json`; use built-in `next/link`, `next/image`.
- Verify: `npm run build`. Fix type and build errors before finishing.
