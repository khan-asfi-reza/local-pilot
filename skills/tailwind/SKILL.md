---
name: tailwind
description: Tailwind CSS conventions.
internal: true
---
# Tailwind CSS
- Style with utility classes ON THE ELEMENTS: `className="rounded-xl bg-white shadow p-4 hover:shadow-lg"`. This is the default — reach for it first.
- **If you write a semantic class name** (`className="product-card"`), you MUST also write its CSS (in `index.css` under `@layer components` with `@apply`, or plain rules). A class with no rule renders unstyled — the page looks blank/broken. Prefer utilities so this never happens.
- Extend design tokens in `tailwind.config` `theme.extend`, not inline arbitrary values everywhere.
- Ensure `content` globs in the config cover your files, or classes get purged.
- For dark mode use the config's strategy (`class` or `media`). Keep spacing/color scales consistent.
