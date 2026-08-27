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
- **TypeScript**: add `@types/three` to devDependencies and keep `skipLibCheck: true` in `tsconfig.json` so library typings never block the build (a missing `@types/three` triggers `TS7016: could not find a declaration file for 'three'`). Type nullable refs (e.g. a player mesh) as `Mesh | null` and null-check before use rather than churning the same file against `TS18047: possibly null`.
- This is a **single WebGL view on one `<canvas>`** — there are NO routes or pages. Do not add a router, do not split into multiple pages, and do not treat a missing page/route as a defect.
- The entry point (`index.html`, `src/main.tsx`) is already correct — never create, rename, copy, or grep-verify `main.*`. Verify by the canvas actually rendering, not by inspecting the served HTML's `<script src>`.
