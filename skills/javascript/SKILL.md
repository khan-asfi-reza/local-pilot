---
name: javascript
description: JavaScript / Node project conventions.
internal: true
---
# JavaScript (Node)
- Use ES modules (`import`) when `package.json` has `"type": "module"`, else CommonJS (`require`). Match the existing file.
- Declare deps in `package.json`. Don't assume a global is installed.
- Verify: `node <file>` for a script, or `npm test` / `npm run build` if defined. Report real output.
