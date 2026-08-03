---
name: frontend-ui
description: Build beautiful, fully functional frontend UI — real design, working controls, icon libraries not emojis.
internal: true
---

# Frontend UI

You are building real user-facing UI. It must look like a polished production app
and every control must actually work. A skeleton or a mockup is a failure.

## Icons — use an icon LIBRARY, never emojis
Never use emoji characters (🛒, ✅, 🔔) as icons. Use an icon library:
- React / Next.js: `lucide-react` — `import { ShoppingCart, Search, Bell } from "lucide-react"` and render `<ShoppingCart className="h-5 w-5" />`. Add `lucide-react` to package.json.
- Vue: `lucide-vue-next`. Svelte: `lucide-svelte`. Angular: `lucide-angular`.
- Plain HTML/JS (no bundler): load lucide from a CDN (`<script src="https://unpkg.com/lucide@latest"></script>` then `lucide.createIcons()`), or paste inline `<svg>` icons.
Import every icon you use — a missing import is a blank page.

## Design (make it genuinely good)
- Cohesive modern look: one clear type scale (a big bold heading, comfortable body), generous consistent spacing, a small tasteful palette with ONE accent color, strong readable contrast.
- Cards with rounded corners and soft shadows; clear hover and focus states; a responsive grid (desktop → tablet → mobile).
- Real, believable content — real product names, prices, copy. Never lorem ipsum, never "TODO"/placeholder text.
- Use the project's CSS framework if present (Tailwind, etc.); otherwise write clean, cohesive CSS.

## Functional (every control works)
- Buttons, forms, filters, tabs, search must be wired to real state and the real API — clicking must change what renders (filter from state, POST an action, fetch real data). No dead buttons, no `onClick` that only logs.
- Fetch data from the actual endpoints; show loading and empty states; reflect updates in the UI.
- Import every component/hook you reference so nothing 404s or blanks the page.

Ship UI that a user could actually click through and use.
