---
name: lua
description: Lua project conventions.
internal: true
---
# Lua
- 1-based indexing; `local` for all variables unless a global is intended.
- Use `require` with module paths that match the file layout.
- Verify: `lua <file>` or `luajit <file>`; run the project's test runner (busted) if present.
