---
name: pixijs
description: PixiJS 2D WebGL rendering conventions.
internal: true
---
# PixiJS
- Create `const app = new PIXI.Application()`; (v8: `await app.init({...})`), then append `app.canvas`. Build a scene graph of `Container`/`Sprite`.
- Load textures with `Assets.load`. Animate via `app.ticker.add((ticker) => ...)` using `ticker.deltaTime`.
- Load from CDN for a single-file preview, or keep `pixi.js` in `package.json`. Match the installed major version's API (v7 vs v8 differ).
