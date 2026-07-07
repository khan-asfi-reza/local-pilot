# AGENTS.md

## Project overview

Project Harness is a local, offline-first LLM assistant. A stateless agent core (the harness) runs a ReAct loop over a small local model served by llama.cpp. The core is used two ways: imported as a library by a terminal coding client, and run as a server behind a FastAPI web backend with a React frontend. Everything runs on one machine and no data leaves the device. Normal chat needs no internet; only the web_search tool does.

The full harness design and workflow live in docs/harness_context.md. Read that before working on anything in /harness or /terminal.

## Tech stack

- Harness and terminal client: Go 1.22+ (working assumption; if the harness moves to Rust, replace only the harness command block below).
- Backend: Python 3.11+, FastAPI, uvicorn, httpx, pydantic v2.
- Frontend: React 18 with Vite, Tailwind CSS.
- Inference: model slots (planner, coder) are filled by backends reached by URL over the OpenAI-compatible format. Version 1 is on-device only: every URL is localhost (llama.cpp llama-server). The router is URL-based so off-device backends (a LAN machine, an internet provider) can be added later without harness changes, but that is post-v1. When two models run on-device, llama-swap sits in front for swapping.
- Models (v1, on-device): Option A is two models, planner xLAM-2-3b-fc-r (Qwen3-4B-Instruct-2507 or SmolLM3-3B as fallback) plus coder Qwen2.5-Coder-3B, both Q4_K_M, needing swap. Option B is one dual-purpose model good at both tool calling and coding (Qwen3-4B-Instruct-2507), filling both roles with no swap. Pick per hardware behavior; the router makes it a config change.

## Repository layout

- /harness   Go: agent loop, tools, context manager, model router, server wrapper
- /terminal  Go: CLI client, on-disk sessions, model-switch display
- /backend   Python/FastAPI: stateless relay, safe tool set only
- /frontend  React/Tailwind: chat UI, holds conversation history
- /skills    SKILL.md folders, loaded by progressive disclosure
- /models    GGUF model files (gitignored); the v1 model catalog is whatever is found here
- /tests     test suites; /tests/sandbox is the throwaway working directory tests run against
- /docs      project documentation, interface contracts, harness_context.md

## Commands

Harness and terminal (Go):
- Build: `go build ./...`
- Test all: `go test ./...`
- Single test: `go test ./harness -run TestName`
- Lint: `gofmt -l . && go vet ./...`
- Run harness server: `go run ./harness/server --port 9000`
- Run terminal client in a repo: `go run ./terminal --dir .`

Backend (Python, run from /backend with the venv active):
- Setup: `python -m venv venv && source venv/bin/activate && pip install -r requirements.txt`
- Run: `uvicorn main:app --port 8000 --reload`
- Run mock harness for local dev: `uvicorn mock_harness:app --port 9000 --reload`
- Test: `pytest tests -v`

Frontend (React, run from /frontend):
- Setup: `npm install`
- Dev server: `npm run dev` (serves on http://localhost:5173)
- Build: `npm run build`
- Lint: `npm run lint`

Inference (v1, on-device, backends by localhost URL):
- Two-model (Option A): run each model on its own port and llama-swap in front:
  `llama-server -m models/xlam-2-3b-fc-r-q4_k_m.gguf --port 8080 -c 8192`
  `llama-server -m models/qwen2.5-coder-3b-q4_k_m.gguf --port 8081 -c 8192`
- One dual-purpose model (Option B): one server, both roles point at it, no swap:
  `llama-server -m models/qwen3-4b-instruct-q4_k_m.gguf --port 8080 -c 8192`
- The router requests models by role name. Off-device URLs are a post-v1 addition; do not build for them yet, but do not hardcode against them either.

## Architecture rules (do not violate)

- The harness is stateless per request. It stores nothing between calls. Every request carries the full message list, the allowed tool list, and the working directory. History lives in the client: on disk for the terminal, in frontend state for the web.
- One harness codebase, two entry points: a library (imported by /terminal) and a thin server (/harness/server, called by /backend). Shared logic lives in the core and is never duplicated in the server.
- The backend passes only the safe tool set: code_run (sandbox) and web_search. It must never enable file_read, file_write, shell_run, or any code-intelligence tool. Those are terminal-only. This is the core safety boundary.
- Tool calls use the model's native function-calling format (the tools field on the chat request), never a custom text protocol. Constrain the tool-call arguments to the tool's JSON schema with a grammar so they are always valid. Do not grammar-constrain the whole assistant turn; that would suppress the reasoning step.
- The planner does not know the file layout in advance. It must locate a file with search, a symbol query, or list_dir before reading or editing it, and never guess a path.
- Edits to existing files go through edit_file by anchor text (old_text/new_text), never by line number, because small models miscount lines. The harness resolves each anchor to an exact position (old_text must match exactly once, else it errors and writes nothing) and returns a positional diff (hunks with real line numbers and add/remove/context ops) for the terminal to render and for ask-mode confirmation. write_file is only for new files or full rewrites.
- Modes (plan, ask, auto) are a permission policy enforced in dispatch, not by the model, and travel with the request like allowed_tools. Plan refuses all mutating tools and the agent produces a plan and stops; ask pauses on any mutating tool for user approval via the confirm and approve events; auto runs everything. Classify each tool as read-only or mutating (shell_run is always mutating). The model can never escalate its own mode from stricter to looser; only the human does, by Tab or an explicit request. Modes are terminal-only; the web path relies on its sandboxed tool set instead.
- The coder model is reached as a tool (write_code), not as a peer. It receives a tight spec, returns code, and never touches the loop or the conversation.
- Models come from a model source. In v1 the only source is the local models/ directory, scanned for GGUF files; that set is the catalog. Roles bind to a model by name; the user can rebind a role from the terminal with /model. Access models only through the source interface (list, resolve), never by hardcoding a path, so a Downloader source can be added post-v1 without touching the router, loop, or CLI. The downloader itself is not built in v1.
- Model routing is by role name (planner or coder) through a URL-based router. Version 1 points every role at a localhost llama-server; the two roles may share one model (dual-purpose) or use two models with llama-swap. Do not hardcode ports or assume a fixed model. The router is URL-based so LAN or internet endpoints can be added post-v1 with no harness change; do not build that support in v1.
- Version 1 is fully offline: it must work with no internet. Where a model runs will become a privacy axis when off-device backends land later; it stays separate from where a tool runs, which is the execution-safety boundary that already applies.

## Canonical tools

- Discovery: search (ripgrep, universal text), list_dir, and later the code-intelligence tools.
- Read and edit: read_file, write_file (new files or full rewrite), edit_file (anchor-based edits to existing files; the model gives old_text/new_text, the harness derives exact line positions and returns a git-style diff to render).
- Execution: shell_run, code_run (sandbox only).
- Delegation: write_code (routes to the coder model).
- External: web_search.
- Extension: MCP tools, load_skill.

Code intelligence is tiered and added after the basic loop works: ripgrep is the floor, tree-sitter adds structural symbol lookup with no running server, and LSP (get_diagnostics, find_references, find_symbol) is the semantic top layer. Add get_diagnostics first; feed its output into the debug loop.

## Event contract (the shared language)

Every layer speaks the same newline-delimited JSON events. Do not add or change shapes without updating /docs first.
- `{"type":"text","content":"..."}`
- `{"type":"tool_call","tool":"...","info":"..."}`
- `{"type":"tool_result","tool":"...","info":"..."}`
- `{"type":"confirm","id":"...","tool":"...","summary":"...","diff":{...}}`  (ask mode: harness pauses for approval; edits include a positional diff)
- `{"type":"approve","id":"...","decision":"yes|no|always"}`  (client's reply to a confirm)
- `{"type":"error","message":"..."}`
- `{"type":"done"}`

## Tool results must be structured

Any executing tool (shell_run, test runner, code_run) returns separate fields, not one blob: `exit_code`, `stdout`, `stderr`, and the `command` run. Truncate long output from the middle, keeping the head and tail. This is what makes the debug loop legible to a small model.

## Code style

- Go: gofmt clean, no unformatted code merged. Wrap errors with context, never ignore them. Small packages organized by responsibility.
- Python: type hints on every signature, async handlers by default, pydantic v2 for all request and response shapes, ruff for lint.
- React: function components and hooks only, no class components. Do not use HTML `<form>` tags in components; use onClick and onKeyDown. Tailwind utility classes, no separate CSS files.
- Do not use em dashes or en dashes in code, comments, docs, or generated output.

## Testing

- Tests live in /tests. Tests that exercise the file, shell, or code tools run against /tests/sandbox, a throwaway directory: point the harness working_directory at /tests/sandbox so no test ever touches the real repo. Treat its contents as disposable, set it up fresh per test, and clean it after. It is gitignored except for a .gitkeep.
- Every harness tool has a unit test that runs it in isolation, using /tests/sandbox as its working directory where it needs a filesystem.
- The agent loop is tested with a fake model that returns scripted tool calls, so the loop is verified without a real LLM.
- The backend is tested against the mock harness, not a live model.
- Verify a change by running the exact command that exercises it. Do not report a task done without a passing run.

## Security

- Never enable file, shell, or code-intelligence tools on the web path.
- code_run executes only inside its isolated execution sandbox, with no host filesystem or network access. This is a separate thing from the /tests/sandbox directory, which is just a throwaway working folder for tests.
- Shell and file tools operate only within the working directory passed per request. Do not escape it.
- Never commit model files (*.gguf), the /models directory, or any keys. They are gitignored.

## Conventions

- Commit format: `<area>: <short imperative>`, for example `harness: add grammar builder`. Areas: harness, terminal, backend, frontend, skills, docs.
- Branch naming: `<area>/<short-topic>`.
- Keep this file focused and under about 200 lines. If guidance for one package grows large, add a nested AGENTS.md inside that package; the nearest file wins.
