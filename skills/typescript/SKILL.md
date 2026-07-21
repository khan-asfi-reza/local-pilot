---
name: typescript
description: TypeScript project conventions.
internal: true
---
# TypeScript
- Use ES modules and explicit types on exported functions. Avoid `any`.
- Dependencies go in `package.json`; keep `tsconfig.json` `strict: true`.
- Import with correct extensions per the module setting; don't invent paths.
- Verify: `npx tsc --noEmit` (type-check), and `npm run build`/`npm test` if defined. Report real output.
