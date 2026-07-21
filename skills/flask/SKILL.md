---
name: flask
description: Flask (Python) app conventions.
internal: true
---
# Flask
- `app = Flask(__name__)`; routes with `@app.route`. Return strings/`jsonify`/templates. Blueprints for larger apps.
- Keep `Flask` in requirements. Debug via app.run only through the serve tool, not a blocking shell_run.
- Verify: `python -c "import app"`; live-check with the serve tool then curl.
