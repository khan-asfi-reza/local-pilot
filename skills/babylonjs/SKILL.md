---
name: babylonjs
description: Babylon.js 3D game engine conventions.
internal: true
---
# Babylon.js
- Create an `Engine` on a canvas, then a `Scene`. Add a camera (e.g. `ArcRotateCamera`) and a light before meshes, or nothing renders.
- Run with `engine.runRenderLoop(() => scene.render())`; call `engine.resize()` on window resize.
- Use built-in `MeshBuilder`, materials, and the physics plugin for games. Load from CDN for a single-file preview or `@babylonjs/core` as a dep.
