---
name: csharp
description: C# / .NET project conventions.
internal: true
---
# C# / .NET
- Work within the `.csproj`; add packages with `dotnet add package`.
- Use namespaces matching folders, PascalCase for types/methods, `async`/`await` for I/O.
- Verify: `dotnet build` then `dotnet test`. Fix build errors before finishing.
