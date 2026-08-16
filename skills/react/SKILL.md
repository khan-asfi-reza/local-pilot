---
name: react
description: React app conventions.
internal: true
---
# React
- Function components + hooks only. One component per file; PascalCase names.
- Derive state minimally; lift shared state up. Keys on list items. No side effects in render — use `useEffect`.
- Dependencies (`react`, `react-dom`, etc.) go in `package.json`. Use the project's bundler (Vite/CRA); don't add a new one.
- Verify: `npm run build` (or `npm test`). Report real output.

## Never break these (each one blanks or breaks the page)
1. **Exactly ONE `export default` per file.** Two default exports is a fatal parse error → white page.
2. **Never put an object, array, or Date directly in JSX.** `{someDate}` or `{someObj}` throws "Objects are not valid as a React child" and unmounts the whole app. Format first: `{d.toLocaleDateString()}`, `{String(x)}`, `{items.map(...)}`.
3. **Import every component, hook, and icon you reference.** A missing import blanks the page.
4. **Data comes from the generated API client, never hardcoded arrays or hand-written fetch paths.** If `src/lib/api.ts` exists, import its typed functions (e.g. `import { listDoctors, createAppointment } from '../lib/api'`) and call them inside `useEffect`/handlers — it has the EXACT paths + types from the contract, so you never guess a URL or shape. Render loading / error / empty states. A component full of fake data, or one that hardcodes `fetch('/api/..')` when a client function exists, is a failure. (Login: on success `localStorage.setItem('token', token)` — the client attaches it automatically.)
5. **Wire every page into the router.** For each page add a `<Route path=... element={<Page/>}/>` in `App.tsx`, and make sure every `<Link to>`/route target resolves to a real element. An unrouted page is dead code.
6. **Style with Tailwind utility classes directly on elements** — `className="rounded-xl bg-white shadow p-4 hover:shadow-lg"`. Do NOT invent semantic class names like `.product-card` or `.hero` unless you ALSO write their CSS; an unstyled class = an unstyled (blank-looking) page.
7. **Reach the backend at `/api/...`** — the Vite dev proxy is already configured. Never edit `vite.config`, never hardcode `http://localhost:PORT`.
8. **Every package you `import` must be in `package.json` dependencies** (name only is fine). If you import `axios`/`zustand`/etc., add it — an unlisted import crashes the app with "Cannot find module".
9. **Prefer `write_file` (whole file) over `edit_file`.** A partial insert in the middle of a file easily breaks it. If you must edit, keep it minimal and re-read the file after to confirm it still parses.

## Make it beautiful and real (design-first, not stubs)
Build a genuine product, not placeholder pages:
- **Every screen in the spec is a real page** with content — a sticky top nav, a hero/landing on `/` (headline + subtext + primary CTA + a visual, NOT a bare `<Link>`), list/grid pages with cards, a detail page, forms in a modal or panel. A route whose element is just a link or one line of text is a defect.
- **Modern visual polish**: generous spacing, a consistent accent color, rounded cards with soft shadows (`rounded-xl shadow-sm border`), hover states, readable type scale, responsive grid (`grid gap-6 sm:grid-cols-2 lg:grid-cols-3`), and proper loading (skeleton/spinner), empty ("No X yet"), and error states. Use `lucide-react` for icons.
- **Wire it to real data** via the generated client (rule #4) so pages show the seeded content, and every button/form calls the client and reflects the result.
