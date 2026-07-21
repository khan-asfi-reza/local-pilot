---
name: fastapi
description: FastAPI (Python) API conventions.
internal: true
---
# FastAPI
- `app = FastAPI()`; path operations with `@app.get/post`. Pydantic models for request/response bodies. Type hints drive validation + docs.
- Dependencies via `Depends`. Keep `fastapi` + `uvicorn` in requirements.
- Verify: `python -c "import main"`; for a live check use the serve tool (`uvicorn main:app`) then curl — never a blocking shell_run.
