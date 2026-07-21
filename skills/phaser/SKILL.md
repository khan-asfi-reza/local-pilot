---
name: phaser
description: Phaser 3 web game conventions.
internal: true
---
# Phaser 3
- Config: `new Phaser.Game({ type: Phaser.AUTO, width, height, physics, scene })`. Organize logic into Scenes with `preload()`, `create()`, `update(time, delta)`.
- Load assets in `preload` (`this.load.image/spritesheet`); create objects in `create`; move/collide in `update`. Use Arcade physics for simple games (`this.physics.add.*`).
- Load Phaser from CDN for a no-build single-file preview, or as a `package.json` dep in a bundled project.
- Verify a bundled project with `npm run build`; keep the canvas mounted in a container div.
