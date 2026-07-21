---
name: kaplay
description: KAPLAY / Kaboom.js game library conventions.
internal: true
---
# KAPLAY (formerly Kaboom.js)
- Init with `kaplay()` (or `kaboom()`), then compose game objects from components: `add([ sprite("x"), pos(), area(), body() ])`.
- Scenes via `scene("name", () => {...})` + `go("name")`. Input with `onKeyDown`/`onKeyPress`; frame logic in `onUpdate`. Physics/collisions via `area()`/`body()` + `onCollide`.
- Great for a single-file browser preview loaded from CDN. Keep assets small and loaded with `loadSprite`.
