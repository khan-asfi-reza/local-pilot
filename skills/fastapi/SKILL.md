---
name: fastapi
description: FastAPI (Python) API conventions.
internal: true
---
# FastAPI
- `app = FastAPI()`; path operations with `@app.get/post`. Pydantic models for request/response bodies. Type hints drive validation + docs.
- Dependencies via `Depends`. Keep `fastapi` + `uvicorn` in requirements.
- Verify: `python -c "import main"`; for a live check use the serve tool (`uvicorn main:app`) then curl — never a blocking shell_run.

## The scaffold already gives you these — use them, don't fight them
- **Everything is an `app/` package**: `app/main.py`, `app/db.py`, `app/auth.py`. Put your feature modules in `app/` too and import with `from app.db import ...`, `from app.auth import ...`. The server runs as `uvicorn app.main:app` **from the `backend/` directory**, so imports are rooted at `app.` — always `from app.models.doctor import Doctor`, NEVER `from backend.app...` (there is no `backend` package at runtime; that import crashes startup with `ModuleNotFoundError: No module named 'backend'`).
- **Routers auto-mount — do NOT edit `app/main.py`.** Any module you create under `app/routes/`, `app/routers/`, or `app/api/` that defines a module-level `router = APIRouter(...)` is discovered and mounted automatically under `/api`. So: create `app/routes/doctors.py` with `router = APIRouter(prefix="/doctors")` and it serves at `/api/doctors` — you do not import it anywhere, you do not touch `main.py`. (Setting `prefix="/api/doctors"` also works; it won't be double-prefixed.)
- **DB + schema**: `from app.db import Base, get_db`. Every model subclasses `Base`; every route takes `db: Session = Depends(get_db)`. Tables are created automatically on startup by `init_db()` — do NOT write migrations or call `create_all`. Just define models on `Base`; the auto-mount imports your module so its tables register.
- **CORS is enabled** and a `/health` route exists. The API runs on **port 8000**; don't change it. The frontend calls you at `/api/...` through the Vite proxy.

## Auth is already built — if `app/auth.py` exists, use it
When the spec needs authentication, `app/auth.py` provides a `User` model + table, `POST /api/auth/register`, `POST /api/auth/login`, `GET /api/auth/me` (already included in `app/main.py`), and a `get_current_user` dependency. So:
- **Never** recreate the users table, a register/login route, JWT, or password hashing. Do NOT add your own `app/api/auth.py` — it already exists.
- To protect a route: `from app.auth import get_current_user, User` and add `user: User = Depends(get_current_user)`.
- Reference the user with `user_id = Column(Integer, ForeignKey("users.id"))`.

## Seed real demo data (empty lists = broken product)
Provide `app/seed.py` with a `run_seed()` and `if __name__ == "__main__": run_seed()`, so the UI shows real content. It MUST:
- open a real session: `db = SessionLocal()` — NEVER `db = get_db()` (that is a generator).
- import every model first (`from app.auth import User`, each `app.models.*`) so relationships resolve.
- insert rows AND `db.commit()` (without commit nothing persists), then verify a non-zero count.
- be idempotent (skip if already seeded).

## Every route must actually work when called
- An APIRouter list route must return real rows from the DB, not a stub. Define `router` in an `app/routes/<name>.py` module (it auto-mounts under `/api`) — an endpoint that raises 500s is a broken feature. Boot the app and GET each endpoint; fix until they return data (or 401 if auth-guarded). Use the exact `/api/...` paths from the project map so the frontend's calls resolve.
