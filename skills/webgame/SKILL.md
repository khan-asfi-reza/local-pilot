---
name: webgame
description: Vanilla HTML5 canvas game conventions (no engine).
internal: true
---
# HTML5 Canvas Game (vanilla)
- One `index.html` with a `<canvas>` and a script. Get `const ctx = canvas.getContext('2d')`.
- Game loop with `requestAnimationFrame`; use a delta time (ms since last frame) so movement is frame-rate independent — never assume 60fps.
- Structure: `update(dt)` mutates state, `render()` draws. Clear each frame: `ctx.clearRect(0,0,w,h)`.
- Input via `keydown`/`keyup` into a `keys` set (don't act only on keydown for held movement). Handle canvas resize / DPR for crisp rendering.
- Keep it self-contained for the browser preview: inline the script, no build step.
