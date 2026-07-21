---
name: java
description: Java project conventions.
internal: true
---
# Java
- Use the existing build tool: Maven (`pom.xml`) or Gradle (`build.gradle`). Add deps there.
- One public class per file, matching the filename; keep the package/directory in sync.
- Verify: `mvn -q compile` or `gradle build`, then run tests. Fix compile errors first.
