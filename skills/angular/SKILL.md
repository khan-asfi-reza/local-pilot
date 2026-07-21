---
name: angular
description: Angular app conventions.
internal: true
---
# Angular
- Use the Angular CLI conventions: components/services in their folders, `@Component`/`@Injectable` decorators, DI via constructors.
- Prefer standalone components if the project uses them; otherwise register in a module.
- Keep deps in `package.json`; strict TypeScript.
- Verify: `ng build` (or `npm run build`). Fix template + type errors before finishing.
