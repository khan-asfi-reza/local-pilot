Build a small full-stack Todo app: a FastAPI backend and a React frontend.

Create these files (and only these):

BACKEND (in a `backend/` folder):
1. `backend/main.py` — a FastAPI app with an in-memory list of todos. Each todo is `{"id": int, "title": str, "done": bool}`. Endpoints:
   - `GET /todos` → return the list of todos.
   - `POST /todos` with JSON body `{"title": "..."}` → create a todo (auto-increment id, done=False), return the created todo.
   - `DELETE /todos/{id}` → remove that todo, return `{"ok": true}`.
   Enable CORS for all origins (use `fastapi.middleware.cors.CORSMiddleware`).
2. `backend/requirements.txt` — list `fastapi` and `uvicorn` and `httpx` (httpx is needed by the test client).
3. `backend/test_api.py` — a plain-Python test (no pytest) using `fastapi.testclient.TestClient` that: GET /todos is empty, POST a todo then GET returns 1 item with the right title and done=False, DELETE it then GET is empty again. Print `all api tests passed` at the end.

FRONTEND (in a `frontend/` folder):
4. `frontend/index.html` — a single-file React app loaded from CDN (React, ReactDOM, and Babel standalone via `<script>` tags from unpkg). It should: fetch the todo list from `http://localhost:8000/todos` on load and render it, provide a text input and an "Add" button that POSTs a new todo, and a "Delete" button next to each todo. Keep it one self-contained HTML file (JSX in a `<script type="text/babel">`).

VERIFY the backend actually works: create/activate nothing, just `pip install -r backend/requirements.txt`, then run `python3 backend/test_api.py` and confirm it prints `all api tests passed`. Do NOT start uvicorn or any server (it blocks). The frontend is a static file — just make sure it is valid HTML with the React CDN scripts. Fix any error before finishing.
