---
name: astro
description: Astro site conventions.
internal: true
---
# Astro
- `.astro` files: frontmatter (`---` JS/TS) + HTML template. Pages in `src/pages` (file-based routing). Ship zero JS by default.
- Add interactivity with framework islands (`client:load`, `client:visible`) only where needed.
- Keep deps in `package.json`. Verify: `npm run build`.
