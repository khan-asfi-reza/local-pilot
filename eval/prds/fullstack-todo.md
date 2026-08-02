# Full-stack to-do app (FastAPI + React)

Build a small full-stack to-do application.

## Backend (`backend/`)
- `backend/main.py` — a FastAPI app with an in-memory list of todos and these
  endpoints:
  - `GET /todos` — return all todos as JSON.
  - `POST /todos` — body `{"title": "..."}`, create a todo, return it with an id.
  - `DELETE /todos/{id}` — delete a todo.
  - Enable CORS (allow all origins) so the frontend can call it.
- `backend/requirements.txt` — pin `fastapi` and `uvicorn` (plus `httpx` if your
  test needs it).
- `backend/test_api.py` — a plain script that uses FastAPI's `TestClient` to
  drive the API: GET (empty) → POST one → GET (one) → DELETE → GET (empty). It
  must print `all api tests passed` on success and exit non-zero on failure. It
  is run as `python backend/test_api.py`.

## Frontend (`frontend/`)
- `frontend/index.html` — a single self-contained page that loads React from a
  CDN, fetches `/todos`, renders the list, has a text input + Add button that
  POSTs a new todo, and a Delete button per todo.

## Verify
`python backend/test_api.py` prints `all api tests passed`.
