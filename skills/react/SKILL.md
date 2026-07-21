---
name: react
description: React app conventions.
internal: true
---
# React
- Function components + hooks only. One component per file; PascalCase names.
- Derive state minimally; lift shared state up. Keys on list items. No side effects in render — use `useEffect`.
- Dependencies (`react`, `react-dom`, etc.) go in `package.json`. Use the project's bundler (Vite/CRA); don't add a new one.
- Style with the project's existing approach (CSS modules, Tailwind, styled). Match it, don't introduce a new one.
- Verify: `npm run build` (or `npm test`). Report real output.
