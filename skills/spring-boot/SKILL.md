---
name: spring-boot
description: Spring Boot (Java) conventions.
internal: true
---
# Spring Boot
- Annotation-driven: `@SpringBootApplication`, `@RestController`, `@Service`, `@Repository`; constructor injection. Config in `application.properties`/`yml`.
- Manage deps in Maven/Gradle. Verify: `mvn -q compile` / `gradle build`, run tests. Live check with the serve tool, not a blocking run.
