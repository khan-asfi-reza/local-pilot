---
name: svelte
description: Svelte component conventions.
internal: true
---
# Svelte
- One component per `.svelte` file: `<script>`, markup, `<style>` (scoped). Svelte 5 uses runes (`$state`, `$derived`, `$effect`); older uses `let` + `$:`. Match the project's version.
- Reactivity is assignment-based — reassign to trigger updates. Props via `export let` (or `$props()` in runes).
- Keep deps in `package.json`; build with Vite. Verify: `npm run build`.
