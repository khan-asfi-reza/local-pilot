---
name: tailwind
description: Tailwind CSS conventions.
internal: true
---
# Tailwind CSS
- Style with utility classes in markup; avoid custom CSS unless a utility can't express it.
- Extend design tokens in `tailwind.config.js` `theme.extend`, not inline arbitrary values everywhere.
- Ensure `content` globs in the config cover your files, or classes get purged.
- For dark mode use the config's strategy (`class` or `media`). Keep spacing/color scales consistent.
