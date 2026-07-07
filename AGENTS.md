# AGENTS.md

## Project overview

local-pilot is a local, offline-first coding assistant. A stateless agent core (the harness) runs a ReAct loop over one small local model served by ollama. The core is used two ways: imported as a library by a terminal coding client, and (planned) run as a thin HTTP server behind a web backend. Everything runs on one machine and no data leaves the device. Normal use needs no internet; only the web_search tool does.

The Go module is named `harness`; local-pilot is the product name. Background design notes live in docs/harness_context.md, but it predates the ollama pivot — this file is the current source of truth.

## Tech stack

- Harness, terminal client, and pilot launcher: Go 1.25+.
- Inference: ollama, reached over its OpenAI-compatible API at `http://localhost:11434`. The model router is URL-based, so off-device backends can be added later without harness changes; that is post-v1.
- Model: one dual-purpose model does everything (no planner/coder split, no code delegation — the model writes code directly with write_file/edit_file). Default is `qwen2.5-coder:7b-tools`, a local variant built by `pilot` from base `qwen2.5-coder:7b`: its chat template's `<tool_call>` tags are swapped to `[tool_call]` (which the coder model reliably emits, so ollama parses native tool_calls) and `num_ctx` is baked in (32768).
- Backend/Frontend: planned, not built (empty scaffolds). Backend will be Python/FastAPI; frontend React/Tailwind.

## Entry point

`./pilot` is the launcher (a bash wrapper that builds and execs the Go binaries):
- `./pilot start` — ensure ollama is running (starts it if down), ensure the default model from models.json is installed (pull base + apply template/num_ctx + `ollama create` if missing). Idempotent.
- `./pilot add <base>` — pull a base model, apply the same template swap + num_ctx, create `<base>-tools`, register it in models.json.
- `./pilot code [--dir X]` — interactive terminal coding agent.
- `./pilot run --dir X --task "..."` — headless one-shot; `--task-file`, `--max-steps`, `--format ndjson|human`.

## Repository layout

- /harness   Go: agent loop, tools, context manager, model client/router, HTTP server
- /terminal  Go: terminal client (TUI + headless `run`), on-disk sessions, /model switching
- /cmd/pilot Go: the pilot launcher (start / add)
- /skills    SKILL.md folders, loaded on demand (debug, dependencies, serving)
- /models    models.json (registry) and prompt.json (system prompt). No model weights: ollama holds those.
- /tests     test suites; /tests/sandbox is the throwaway working directory
- /backend   (planned) Python/FastAPI relay, safe tool set only — not built
- /frontend  (planned) React/Tailwind chat UI — not built
- /docs      project documentation and interface contracts

## Commands

- Build all: `go build ./...`
- Build binaries: `go build -o bin/pilot ./cmd/pilot` and `go build -o bin/harness ./terminal`
- Test all: `go test ./...` ; single: `go test ./harness -run TestName`
- Lint: `gofmt -l . && go vet ./...`
- Run terminal directly: `go run ./terminal --dir .` ; headless: `go run ./terminal run --dir . --task "..."`
- Run HTTP server: `go run ./harness/server --port 9000`
- Model setup is `./pilot start`; do not launch ollama models by hand in code paths.

## Architecture rules (do not violate)

- The harness is stateless per request. It stores nothing between calls. Every request carries the full message list, the allowed tool list, the mode, and the working directory. History lives in the client: on disk for the terminal.
- One harness codebase, two entry points: a library (imported by /terminal and /cmd/pilot) and a thin server (/harness/server). Shared logic lives in the core and is never duplicated.
- The web/server path passes only the safe tool set: code_run (sandbox) and web_search. It must never enable read/write/edit file tools, shell_run, or serve. Those are terminal-only. This is the core safety boundary.
- Tool calls use the model's native function-calling format (the tools field on the chat request); the harness consumes `message.tool_calls` and feeds each result back as a `role:"tool"` message. A `tool_mode:"json"` grammar-constrained fallback exists for backends that cannot emit native calls, but native is the default and the live path.
- The model does not know the file layout in advance. It must locate a file with search or list_dir before reading or editing it, and never guess a path.
- Edits to existing files go through edit_file by anchor text (old_text/new_text), never by line number. old_text must be non-empty and match exactly once, else the tool errors and writes nothing. It returns a positional diff for rendering and ask-mode confirmation. write_file is only for new files or full rewrites.
- Servers (uvicorn, npm start, php -S, rails server, etc.) are started with the `serve` tool, never shell_run. serve runs them as a background process group, waits for the port, and stops them when the turn ends. shell_run refuses server commands and redirects to serve, and refuses commands that read or write outside the working directory (they require approval; refused when headless).
- Modes (plan, ask, auto) are a permission policy enforced in dispatch, not by the model. Plan refuses all mutating tools (the model produces a plan and stops); ask pauses on any mutating tool for approval; auto runs everything. shell_run and serve are always mutating. The model can never escalate its own mode; only the human does.
- Models are declared in models/models.json (name, base, num_ctx, tool_mode, port). Reach models only through the config/router, never by hardcoding a tag or port. Adding a model is `./pilot add`, not a manual ollama create in code.
- Version 1 is fully offline: it must work with no internet; only web_search reaches out.

## Canonical tools

- Discovery: search (ripgrep), list_dir.
- Read and edit: read_file, write_file (new files / full rewrite), edit_file (anchor-based).
- Execution: shell_run (waits, in-dir), serve (background servers), code_run (isolated sandbox).
- External: web_search.
- Extension: load_skill.

Every executing tool returns structured fields, never one blob: `exit_code`, `stdout`, `stderr`, and the `command`. Truncate long output from the middle, keeping head and tail. This is what makes the debug loop legible to a small model.

## Skills

Skills are `skills/<name>/SKILL.md` (YAML frontmatter: name, description; body is the procedure). The catalog is listed in the prompt; the model loads one with load_skill. Some are auto-injected by the loop when relevant: `serving` when a server command is attempted, `dependencies` when a package install fails on the environment, `debug` on the first tool failure. Current skills: debug, dependencies, serving.

## Prompt and context

- The system prompt is externalized to models/prompt.json (role, rules, per-tool descriptions) and reloaded every run — tune behavior there with no rebuild. defaultPrompt() in harness/agent/prompt.go is the built-in fallback kept in sync with it.
- The conversation is compacted to fit `context_tokens` (models/models.json) before each model call; old bulky tool results are elided, the recent working set kept. `num_ctx` on the ollama model must be large enough to hold it (32768).

## Event contract (the shared language)

Every layer speaks the same newline-delimited JSON events. Do not add or change shapes without updating /docs first.
- `{"type":"text","content":"..."}`
- `{"type":"tool_call","tool":"...","info":"..."}`
- `{"type":"tool_result","tool":"...","info":"...","data":"...","diff":{...}}`
- `{"type":"confirm","id":"...","tool":"...","summary":"...","diff":{...}}`
- `{"type":"error","message":"..."}`
- `{"type":"done"}`
- `{"type":"usage","tokens":N}`

## Code style

- Go: gofmt clean. Wrap errors with context, never ignore them. Small packages by responsibility.
- Comments: a SHORT one-line comment on top-level functions saying what they do. Do NOT comment structs, fields, consts, or vars, and avoid multi-line/inline comment blocks. Keep it lean.
- Do not use em dashes or en dashes in code, comments, or docs.

## Testing

- Tests live in /tests. Tests that exercise file, shell, or serve tools run against /tests/sandbox, a throwaway directory pointed at by the working_directory. Treat its contents as disposable; set up fresh per test; it is gitignored except .gitkeep.
- Every harness tool has a unit test that runs it in isolation.
- The agent loop is tested with a fake model returning scripted tool calls, so the loop is verified without a real LLM.
- Verify a change by running the exact command that exercises it. Do not report a task done without a passing run.

## Security

- Never enable file, shell, or serve tools on the web/server path.
- code_run executes only inside its isolated sandbox, no host filesystem or network. Separate from /tests/sandbox (a throwaway working folder).
- Shell and file tools operate only within the per-request working directory; a command reaching outside it requires approval.
- Never commit secrets. Model weights live in ollama, not the repo.

## Conventions

- Commit format: `<area>: <short imperative>`, e.g. `harness: add serve tool`. Areas: harness, terminal, pilot, models, skills, docs.
- Branch naming: `<area>/<short-topic>`.
- Keep this file focused and under about 200 lines. If guidance for one package grows large, add a nested AGENTS.md inside it; the nearest file wins.
