---
name: nestjs
description: NestJS backend conventions.
internal: true
---
# NestJS
- Structure by modules: `@Module` wiring controllers + providers. `@Controller` for routes, `@Injectable` services, DI via constructors.
- DTOs with class-validator; use pipes/guards/interceptors rather than ad-hoc code in controllers.
- Keep deps in `package.json`, strict TypeScript. Verify: `npm run build` then `npm test`.
