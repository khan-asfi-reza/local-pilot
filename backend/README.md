# Backend

## Install

```bash
python -m pip install -r requirements.txt
```

## Run

1. Start the Python mock harness with `python -m uvicorn mock_harness:app --host 0.0.0.0 --port 8001`.
2. Start the backend with `python -m uvicorn main:app --host 0.0.0.0 --port 6000`.

If you later want the Go harness instead, set `HARNESS_URL=http://localhost:9000/run` and start it with `go run ./harness/server --port 9000`.

## Environment

- `PORT` default `6000`
- `HARNESS_URL` default `http://localhost:8001/run`
- `DATABASE_URL` default `sqlite:///./local-pilot.db`

The backend exposes thread persistence and an SSE turn endpoint for the web client.
