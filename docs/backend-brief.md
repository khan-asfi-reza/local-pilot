# Backend Agent Brief

You are building the BACKEND for local-pilot, a local offline-first coding
assistant. The Go agent core ("harness") and terminal client already work.
`/backend` is an empty scaffold. Build it. Work ONLY inside `/backend`.

## Read first (before any code)

1. `AGENTS.md` — architecture rules you must not violate
2. `README.md` — product overview + project state
3. `harness/server/main.go` — the upstream you call
4. `harness/events/events.go` — event shapes you receive
5. `harness/model/types.go` — the Message shape you send

## Ports (the chain)

frontend (React, :5173) -> YOUR backend (FastAPI, :6000) -> harness (Go, :9000) -> ollama (:11434) -> model

- ollama: 11434 (managed by `./pilot start`)
- harness HTTP server: 9000 (`go run ./harness/server --port 9000`)
- YOUR backend: 6000 (`PORT` env, default 6000)
- harness URL: `HARNESS_URL` env, default http://localhost:9000

## Upstream contract (how you call the harness)

Harness is STATELESS — remembers nothing. Send full history every turn.

    POST http://localhost:9000/run   Content-Type: application/json
    { "messages": [Message...], "allowed_tools": ["code_run","web_search"], "working_directory": "" }

Message: `{role, content, tool_calls?, tool_call_id?, name?}` (see harness/model/types.go).

Response: 200, `application/x-ndjson`. A STREAM — one JSON event per line, flushed.
Read line by line. Event types:

    {"type":"text","content":"..."}                     assistant prose
    {"type":"tool_call","tool":"...","info":"..."}
    {"type":"tool_result","tool":"...","info":"...","data":"...","diff":{...}}
    {"type":"error","message":"..."}
    {"type":"usage","tokens":N}
    {"type":"done"}                                      turn end; stop reading

"confirm" events exist in the contract but the web path runs AUTO mode with no
confirmation, so you will not receive them. Handle gracefully if you ever do.

## SAFETY BOUNDARY (non-negotiable)

Web path may use ONLY the safe tool set: `code_run` and `web_search`. Never request
or enable file, shell, serve, read/edit, or code-intelligence tools. The harness
enforces this too, but you must never ask for them. File/shell tools are
terminal-only. Do not add a way around this.

## What to build

Python/FastAPI service = the web "client": owns conversation state (threads +
messages), relays each turn to the stateless harness.

Note: AGENTS.md calls the backend a "stateless relay." The harness stays stateless;
the backend adds thread/message PERSISTENCE on top, because "history lives in the
client" and here the backend+DB IS that client. This satisfies the rule, not breaks it.

1. Persistence (SQLite via SQLModel/SQLAlchemy, file-based, minimal schema)
   - `Thread`: id, title, created_at, updated_at
   - `Message`: id, thread_id, role, content, tool_calls, tool_call_id, name,
     created_at — mirrors the harness Message shape so you can rebuild history.

2. Turn flow (the core). On a new user message to a thread:
   a. persist the user message
   b. load the thread's full ordered history, mapped to harness Message shape
   c. POST to harness `/run` with `allowed_tools=["code_run","web_search"]`
   d. stream the NDJSON response line by line: forward each event to the frontend
      in real time, accumulate "text" into the assistant message, record
      tool_call/tool_result for rendering
   e. on "done": persist the assistant message + kept tool events, bump
      thread.updated_at, close the stream
   f. on "error": forward it, persist a sensible error state

3. Streaming to frontend: expose the turn as SSE (`text/event-stream`) for
   EventSource. Forward harness events unchanged (same type names) — one shared
   event language. Stream as it arrives; do not buffer the whole turn.

4. Endpoints
   - `POST   /threads`               create        -> {thread}
   - `GET    /threads`               list          -> [{thread}]
   - `GET    /threads/{id}`          thread + messages
   - `DELETE /threads/{id}`          delete
   - `POST   /threads/{id}/messages` send user msg, STREAM assistant turn (SSE;
     persists on done)
   - `GET    /health`                {ok, harness reachable?}

5. Resilience: if harness (:9000) is down, `/health` reports it and the turn
   endpoint returns a clear error (do not hang). Harness needs ollama + model up
   via `./pilot start`. Handle client disconnect mid-stream without corrupting state.

## Scope + conventions (checked in review)

- Create/edit files ONLY inside `/backend`. Do NOT touch /harness, /terminal, /cmd,
  /models, /skills, /frontend, /docs.
- To change the shared EVENT CONTRACT or `/run` request shape: STOP and flag it, do
  not edit /docs yourself — the contract is shared with terminal + frontend.
- Offline-first: no external network except through the harness. No cloud, no hosted
  DB. SQLite/local files only.
- Root `.gitignore` already covers Python venvs and Node. Never commit a venv,
  node_modules, a secret `.env`, or a local DB file.
- Tests: cover the turn flow against a FAKE harness (stub server emitting scripted
  NDJSON), so relay + persistence are verified without a real model.
- Commits: `backend: <short imperative>`. Branch: `backend/<short-topic>`.
- No em dashes or en dashes in code, comments, or docs.
- Keep `/backend/README.md`: install, run, env vars, startup order
  (`./pilot start` -> harness :9000 -> backend :6000).

## Verify (end-to-end, before calling anything done)

1. `./pilot start`                       # ollama + model (:11434)
2. `go run ./harness/server --port 9000` # upstream (:9000)
3. start backend on :6000
4. `curl /health` -> `{ok:true, harness:true}`
5. create a thread, POST a user message, confirm you SEE the SSE stream of
   text/tool events, and that GET /threads/{id} returns the full persisted convo.

Do not report done without a real passing end-to-end run.

## First steps

1. Read the files above; restate the safety boundary and the `/run` contract in
   your own words.
2. Propose the `/backend` layout (app module, models, harness client, routes) and
   dependency list, then build.
3. Wire the turn flow against a fake harness first (test), then the real server,
   then verify end-to-end.
