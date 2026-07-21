---
name: sveltekit
description: SvelteKit app conventions.
internal: true
---
# SvelteKit
- File-based routing in `src/routes`: `+page.svelte` (UI), `+page.(js|ts)` and `+page.server.ts` (load), `+server.ts` (endpoints), `+layout.svelte`.
- Load data in `load` functions, not in components. Use `$app/*` modules for navigation/stores.
- Keep deps in `package.json`. Verify: `npm run build` (svelte-kit sync + vite build).
