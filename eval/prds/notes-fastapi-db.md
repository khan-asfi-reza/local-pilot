# Notes API with SQLite persistence (FastAPI)

Build a FastAPI notes service whose data survives a restart, using the standard
library `sqlite3` (no ORM).

## Files
- `backend/main.py` — a FastAPI app backed by a SQLite database file
  (`notes.db`) with:
  - `GET /notes` — list all notes.
  - `POST /notes` — body `{"text": "..."}`, insert a note, return it with its id.
  - `DELETE /notes/{id}` — delete a note.
  - The table is created on startup if it does not exist.
- `backend/requirements.txt` — pin `fastapi` and `uvicorn`.
- `backend/test_api.py` — a plain script that: opens a `TestClient`, POSTs a
  note, then simulates a restart by creating a NEW app/`TestClient` against the
  same `notes.db` and confirms the note is still there via `GET /notes`. It
  prints `all api tests passed` on success and exits non-zero on failure.

## Verify
`python backend/test_api.py` prints `all api tests passed`.
