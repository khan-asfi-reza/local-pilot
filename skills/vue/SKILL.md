---
name: vue
description: Vue 3 app conventions.
internal: true
---
# Vue 3
- Use `<script setup>` SFCs with the Composition API (`ref`, `computed`, `watch`).
- One component per `.vue` file, PascalCase. Props typed; emits declared.
- Follow the project setup (Vite/Nuxt); keep deps in `package.json`.
- Verify: `npm run build`. Report real output.
