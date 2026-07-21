---
name: threejs
description: Three.js 3D / WebGL conventions.
internal: true
---
# Three.js
- Core trio: `Scene`, `PerspectiveCamera`, `WebGLRenderer` (append `renderer.domElement`). Add meshes (`Geometry` + `Material`).
- Animate with `renderer.setAnimationLoop` (or `requestAnimationFrame`); update on each frame. Handle window resize (camera aspect + renderer size).
- Dispose geometries/materials/textures you drop to avoid GPU leaks. Import via ES modules or a CDN importmap for a single-file preview.
- Keep deps (`three`) in `package.json` for bundled projects; verify `npm run build`.
