# local-pilot

A local, offline-first coding assistant. It runs a small language model on your
own machine and lets that model actually work in your project: search files,
read and edit code, run shell commands, start servers, scaffold whole projects,
and verify its own work, all without sending a single byte off the device.

Normal use needs no internet. The only tools that reach the network are
`web_search` and `search_images`, and both are opt-in.

There are four ways to drive it, all on one agent core:

| Surface | Command | What it is |
|---------|---------|------------|
| **Terminal** | `pilot code` | Interactive TUI coding agent |
| **Headless** | `pilot run` | One-shot task, NDJSON events out |
| **Browser** | `pilot web` | Chat, a VS Code-style **Code IDE**, and an **App Builder** with live preview |
| **Telegram** | (part of `pilot web`) | Drive a project from your phone, text or voice |

---

## Requirements

| Requirement | Version / notes |
|-------------|-----------------|
| **Go**      | 1.25 or newer (builds the pilot binary) |
| **ollama**  | Installed and on your `PATH` ([ollama.com](https://ollama.com)). `pilot start` will install and launch it for you. |
| **Disk**    | ~3 GB for the default `qwen3.5:4b` model (one-time download) |
| **RAM**     | 8 GB works for the 4B default; 16 GB or more for 7B and up |
| **OS**      | macOS, Linux, or Windows |
| **Python**  | 3.10+ — only for `pilot web` (backend) and the Telegram bridge |
| **Node**    | 18+ — only for `pilot web` (frontend) |
| **ffmpeg**  | Only for Telegram voice notes |

No API keys. No accounts. Nothing to configure to get started.

---

## Installation

### 1. Get the code

```bash
git clone <your-repo-url> local-pilot
cd local-pilot
```

### 2. Build the binary

The `./pilot` launcher builds on demand, so this step is optional, but you can
build up front:

```bash
go build -o bin/pilot ./cmd/pilot   # the single pilot binary
go build ./...                      # or compile everything
```

`bin/pilot` is the whole CLI; the launcher just builds it and forwards your
arguments. Use the launcher for your OS:

| OS | Launcher | Example |
|----|----------|---------|
| macOS / Linux | `./pilot` (bash) | `./pilot start` |
| Windows (cmd) | `pilot.cmd` | `pilot.cmd start` |
| Windows (PowerShell) | `pilot.ps1` | `.\pilot.ps1 start` |

The commands below use `./pilot`; substitute your launcher on Windows (or just
run `bin\pilot.exe` directly). The agent runs its own shell commands through
`cmd /C` on Windows and `sh -c` elsewhere, independent of your terminal.

> **Code graph and CGO.** The tree-sitter code graph behind `query_graph` is
> gated on the `cgo` build tag. With CGO on (the default on macOS/Linux with a C
> toolchain) you get the real graph; with `CGO_ENABLED=0` — or on a Windows box
> with no compiler — the package falls back to a pure-Go stub and everything
> still builds. No flag to remember either way.

### 3. Bring up ollama and the default model

```bash
./pilot start
```

`pilot start` is the one-time setup. It:

1. installs ollama if it is missing (with your confirmation), then starts it,
2. sizes the ollama context window to your hardware (RAM/VRAM, Apple Silicon),
3. shows a menu of models to pick from (or type any ollama model name); press
   Enter for the default,
4. pulls the model if it is not installed (`qwen3.5:4b`, ~3 GB), applying the
   tool-call template and context window where the family needs it,
5. sets it as the default and confirms it is ready.

It is idempotent. Run it any time to make sure everything is up. If your default
model lives on another machine, `pilot start` notices and does *not* start a
local ollama.

The model menu is driven by the `suggested` list in `models.json`:

```
Choose a model:
  1. qwen3.5:4b        (default)
  2. qwen2.5-coder:7b
  3. qwen2.5-coder:14b
  4. qwen2.5-coder:32b
  5. Enter an ollama model name
```

### Data directory

Config and skills live in a per-user data directory, seeded from the built-in
defaults on first run and reused after (so pilot runs from anywhere, and your
added models, installed skills, and prompt edits persist):

| OS | Location |
|----|----------|
| macOS | `~/.localpilot` |
| Linux | `$XDG_DATA_HOME/localpilot` or `~/.local/share/localpilot` |
| Windows | `%LOCALAPPDATA%\localpilot` |

It mirrors the repo layout: `models/models.json`, `models/prompt.json`,
`skills/` (defaults, refreshed on upgrade), `skills_local/` (yours, never
touched), and the SQLite database the web backend uses. Delete it to reset to
defaults; `pilot start` recreates it.

### 4. Start working

```bash
# interactive coding agent in a project of your choice
./pilot code --dir /path/to/your/project

# ...or a single task headless (auto mode, exits when done)
./pilot run --dir /path/to/your/project --task "Add a README and a smoke test."

# ...or the browser stack (chat + Code IDE + App Builder + Telegram bridge)
./pilot web
```

### The `pilot` commands

| Command | What it does |
|---------|--------------|
| `./pilot start`                       | Install/start ollama, size the context, pick a model, get ready |
| `./pilot stop`                        | Stop the ollama server |
| `./pilot web`                         | Run the browser stack and open it in your browser |
| `./pilot models add <model> [--host URL]` | Add a model (local, or from another ollama server) |
| `./pilot models list`                 | List configured models, their server, and the default |
| `./pilot models set-default [name]`   | Choose the default build model |
| `./pilot models set-default-planner [name]` | Choose the model used for planning/decomposition |
| `./pilot models remove <name>`        | Unregister a model |
| `./pilot skill add <source>`          | Install a local skill (`owner/repo[/path]`, git URL, or path) |
| `./pilot skill list` / `remove <name>`| Manage installed local skills |
| `./pilot context [tokens\|auto]`      | Show or set the ollama context window (restarts ollama) |
| `./pilot code [--dir X]`              | Open the interactive terminal UI |
| `./pilot run --dir X --task "..."`    | Run one task to completion, headless |
| `./pilot eval`                        | Run the PRD ladder and report a pass rate |

`pilot run` flags: `--task-file F`, `--max-steps N`, `--format ndjson|human`,
`--mode auto|ask|plan`, `--model NAME`, `--planner NAME`, `--grounding FILE`,
`--skills DIR`, `--config PATH`.

### Interactive commands (inside `pilot code`)

| Command | Action |
|---------|--------|
| `/plan`, `/ask`, `/auto` | Switch tool mode |
| `/model [name]`          | List models (with ready status) or switch the active one |
| `/cwd`                   | Show the working directory |
| `/clear`                 | Clear the conversation |
| `/help`                  | Show help |
| `/quit`, `/exit`         | Leave |

---

## Overview

You point local-pilot at a project directory and give it a task, either
interactively or as a one-shot. The model then drives a **ReAct loop**: it calls
one tool at a time, reads the result, and keeps going until the work is done and
verified. It does not just print code for you to paste; it writes the files,
installs the dependencies, runs the tests, and reports the real output.

The permission **mode** decides how much freedom it has: `plan` is read-only and
produces a plan, `ask` pauses for your approval before any change, and `auto`
runs everything. You set the mode; the model can never loosen it.

Big tasks do not go through the plain loop. When a request is large enough, the
agent **decomposes** it into a dependency graph of sub-tasks, scaffolds the
project deterministically (real generators, real installs, for nine language
stacks), builds the sub-tasks in parallel against a shared API contract, and
then runs a **boot-and-run evaluator** that installs, migrates, boots, probes,
and repairs the result. That is what makes a 4B model produce an app that
actually starts.

---

## Architecture

```
   terminal (TUI / run)     browser (chat · Code IDE · App Builder)     Telegram
            |                              |                              |
            |                       backend :8182 (FastAPI)  <------------+
            |                              |
            |                    harness-server :9000 (Go, thin HTTP)
            |                              |
            +--------> harness core <------+
                            |
                  ReAct loop + orchestrator + tools
                            |
                  model router --> ollama :11434 (local or LAN)
```

- **Harness core** (`harness/`, Go): the stateless agent loop, the tool set, the
  context manager, the top-down orchestrator, the tree-sitter code graph, the
  deterministic language scaffolder, and a URL-based model router. It is used two
  ways: imported directly by the terminal client, and run behind a thin HTTP
  server (`harness/server`) for the web and Telegram paths.
- **Backend** (`backend/`, Python/FastAPI): thread persistence, project registry,
  the Code IDE file/agent API, the App Builder, the models API, profile and
  Telegram settings — all on SQLite in the data directory. It streams agent
  events to clients over SSE and PTY terminals over websockets.
- **Frontend** (`frontend/`, React + Vite + Tailwind): the chat UI, the
  CodeMirror-based Code IDE with a file tree, diff review and an embedded
  terminal, and the App Builder with a live isolated preview.
- **Telegram bridge** (`telegram/`, Python): a thin I/O layer — Telegram
  messages and Whisper voice transcription only. All auth, project selection,
  and agent runs happen in the backend.
- **Model**: served by [ollama](https://ollama.com) over its OpenAI-compatible
  API, by default on `localhost:11434`. The default is `qwen3.5:4b`; the router
  is URL-based, so a model on another machine is just another registry entry.

### Ports

| Service | Port |
|---------|------|
| ollama | 11434 |
| harness HTTP server | 9000 |
| backend (FastAPI) | 8182 |
| frontend (Vite) | 5173 |
| App Builder preview gateway | 6969 |

`pilot web` frees these (except ollama's), starts every service, and opens the
browser.

### Tools

| Tool | What it does |
|------|--------------|
| `search`, `list_dir` | Discovery (ripgrep-backed search) |
| `read_file`, `write_file`, `edit_file` | Read and edit; edits are anchor-based, never by line number |
| `read_document` | Pull text out of PDF / DOCX / PPTX / XLSX |
| `shell_run` | Run a command in the working directory and wait |
| `serve` | Start a background server and wait for its port |
| `code_run` | Run a snippet in an isolated sandbox |
| `npm_install`, `install_deps` | Install dependencies for the detected stack |
| `query_graph` | Query the tree-sitter code graph (definitions, references, ranked files) |
| `search_images` | Real, hotlinkable photo URLs so the model never invents a broken `<img src>` |
| `web_search` | The one general-purpose network tool |
| `load_skill` | Load a skill on demand |

`shell_run` refuses to launch blocking servers (use `serve`) and refuses
commands that reach outside the working directory without approval. Every
executing tool returns structured fields — `exit_code`, `stdout`, `stderr`,
`command` — never one blob, which is what keeps the debug loop legible to a
small model.

`search_images` uses the Pexels API when a key is present (`PEXELS_API_KEY` or
`~/.localpilot/pexels_key`) and falls back to seeded Lorem Picsum photos when
there is none, so it always returns usable URLs.

### Skills

Skills are short procedures in `skills/<name>/SKILL.md` that the model loads on
demand when a task matches. Some are auto-injected by the loop: `serving` when a
server command is attempted, `dependencies` when an install fails, `debug` on the
first tool failure. Beyond those three, there is a skill per stack — React,
Next.js, Vue, Svelte, SvelteKit, Astro, Remix, Angular, HTMX, Tailwind, Express,
NestJS, FastAPI, Flask, Django, Rails, Laravel, Spring Boot, React Native,
Flutter, Go, Rust, Python, TypeScript, JavaScript, Java, C#, C++, PHP, Ruby, Lua,
Three.js, Babylon.js, PixiJS, Phaser, Kaplay, plus `app-builder`, `frontend-ui`
and `webgame`.

Install your own with `pilot skill add <source>` (or `npx pilot-skill add`).
Installed skills live in `skills_local/`, which upgrades never overwrite; a local
skill shadows a default one of the same name.

---

## Adding a model

Some small models do not emit tool calls in a format ollama can parse out of the
box. `pilot models add` handles that: it pulls the base model, rewrites the
Modelfile template where the family needs it (for qwen2.5-coder, swapping
`<tool_call>` tags for `[tool_call]`, which that family emits reliably), bakes in
the context window, picks a tool-calling mode by size (very small models use the
grammar-JSON path, larger ones use native calls), and registers the result.

```bash
./pilot models add qwen2.5-coder:14b   # creates and registers qwen2.5-coder:14b-tools
```

Then switch to it from inside the TUI with `/model`, or with
`pilot models set-default`.

### Planner vs build model

Decomposition and planning can run on a different (usually larger) model than the
one that writes the code:

```bash
./pilot models set-default-planner qwen2.5-coder:32b-tools
```

A registry entry with no `host` means localhost — so if you want planning to run
on a remote box, point `default_planner` at the entry that carries that host, not
at a bare model name.

### Using another machine's ollama server

If another computer on your network runs ollama, register one of its models with
`--host`. pilot verifies the model exists there and stores the server URL; it
does not download anything locally.

```bash
# add a model served by a machine at 192.168.1.50
./pilot models add qwen2.5-coder:32b-tools --host 192.168.1.50:11434
# or a full URL
./pilot models add llama3.1:70b --host http://192.168.1.50:11434
```

The host is saved per model in `models.json` (`"host": "http://192.168.1.50:11434"`),
so switching to that model automatically routes requests to that server. You can
register models from several machines and switch between them. `pilot models list`
shows each model's server and whether it is reachable:

```
Configured models:
➤ qwen3.5:4b                  ready  local
  qwen2.5-coder:32b-tools     ready  http://192.168.1.50:11434
```

The remote machine must have ollama reachable on the network (start it with
`OLLAMA_HOST=0.0.0.0 ollama serve` so it listens beyond localhost).

Models are declared in `models/models.json`:

```json
{
  "context_tokens": 24000,
  "default": "qwen3.5:4b",
  "suggested": ["qwen3.5:4b", "qwen2.5-coder:7b", "qwen2.5-coder:14b", "qwen2.5-coder:32b"],
  "models": [
    { "name": "qwen3.5:4b", "port": 11434, "tool_mode": "native" },
    { "name": "qwen2.5-coder:7b-tools", "port": 11434, "tool_mode": "native", "base": "qwen2.5-coder:7b" }
  ]
}
```

The system prompt lives in `models/prompt.json` and is reloaded on every run, so
you can tune the agent's behavior there with no rebuild.

---

## The browser stack

```bash
./pilot web
```

This starts the harness server, the backend, the frontend, and (if present) the
Telegram bridge, then opens `http://localhost:5173`. It binds `0.0.0.0`, so other
machines on your LAN can reach the UI.

- **Chat** — plain conversation with the model, threads persisted in SQLite.
- **Code IDE** — open any local project: file tree, CodeMirror editor with
  VS Code icons and themes, an embedded PTY terminal, and an agent panel where
  `ask` mode shows you a diff to approve before anything is written.
- **App Builder** — describe an app, watch it get generated, and see it running
  in an isolated live preview served through the gateway on :6969. Projects can
  be renamed, re-run, stopped, and exported as a zip.
- **Settings** — models (add, pull, activate, remove), profile, and the Telegram
  bot token.

### Telegram

Set a bot token from [@BotFather](https://t.me/BotFather) in **Settings →
Telegram**, then **Connect Telegram** and tap the deep link. Pick a project with
`/projects` (or `/chat` for no project) and send text or a voice note. Only
linked chats can control the pilot, and linking requires a code minted in the
local web UI. See [`telegram/README.md`](telegram/README.md) for the full command
list.

### The safety boundary

The web path is not simply the terminal path with a browser on top:

- The harness server exposes only the **safe** tool set by default. `full_access`
  — the file, shell, and serve tools the Code IDE needs — is granted **server
  side**, never by the caller asking nicely.
- The harness grants it only to loopback callers that send no `Origin` header
  (i.e. not a browser), which blocks the CSRF and DNS-rebinding paths that can
  otherwise reach a loopback listener from a victim's browser.
- The backend applies its own, separate rule: localhost plus a same-site
  `Sec-Fetch-Site`. The two rules are deliberately different and are not unified.
- Modes are enforced in tool dispatch, not by the model. `plan` refuses every
  mutating tool, `ask` pauses for approval, `auto` runs through. The model cannot
  escalate.

---

## Repository layout

```
cmd/pilot/       Go: the pilot binary (start / web / models / skill / context / code / run / eval)
terminal/        Go: the terminal client (interactive TUI and headless run)
harness/
  agent/         the ReAct loop, context manager, prompt, grounding, memory, repo map
  orchestrator/  decomposition DAG, API contract, scaffold, provision, verify, repair
  tools/         the tool set
  graph/         tree-sitter code graph (cgo-gated, pure-Go stub otherwise)
  lang/          deterministic scaffolders and API generators for nine stacks
  model/         config, registry, router, client, call log
  server/        the thin HTTP wrapper
  projects/      the unified project registry
  events/        the shared NDJSON event contract
  appdir/        the per-user data directory
backend/         Python/FastAPI: threads, Code IDE, App Builder, models, profile, Telegram
frontend/        React + Vite + Tailwind: chat, Code IDE, App Builder, settings
telegram/        Python: the Telegram bridge (I/O and voice transcription only)
builder_gateway/ Node: the App Builder's isolated preview proxy
packages/        npm package `pilot-skill` (the skill installer)
models/          models.json (registry) and prompt.json (the system prompt)
skills/          on-demand skills, one folder per stack or procedure
eval/            PRDs, acceptance-check manifests, fixtures, and the eval runner
tests/           go/ and python/ black-box suites; sandbox/ is the throwaway work dir
docs/            project documentation, interface contracts, and the testing report
bin/             compiled binaries (gitignored)
pilot            the ./pilot launcher script
AGENTS.md        design notes and architecture rules for contributors
```

> The Go module is named `harness` internally; **local-pilot** is the product name.

---

## Development

The Go workspace builds and tests as a whole:

```bash
go build ./...                 # build everything
go test ./...                  # run all tests, including in-package unit tests
go test ./tests/go             # the Go black-box suite (about a second)
gofmt -l . && go vet ./...     # lint
```

The Python suites drive the HTTP API the way the browser and the bot do:

```bash
backend/.venv/bin/python -m pytest tests/python -q
```

Both suites are black box, and neither needs a running model: model calls go to a
stub that speaks the OpenAI-compatible wire format. See
[`tests/README.md`](tests/README.md) for coverage commands and the rules for
adding tests.

You can also run the pieces directly, bypassing the launcher:

```bash
go run ./cmd/pilot code --dir .                      # interactive TUI
go run ./cmd/pilot run --dir . --task "..."          # headless one-shot
go run ./harness/server --port 9000                  # headless HTTP wrapper
```

> **Rebuilding the harness server.** `pilot web` prefers the prebuilt
> `bin/harness-server` over `go run`. After editing harness code, rebuild it —
> `go build -o bin/harness-server ./harness/server` — and restart, or your change
> will not be live.

### Evaluation

Tests answer "is the harness safe and correct"; evaluation answers "can the model
actually build the thing". The second question is scored, not asserted:

```bash
./pilot eval                       # the whole PRD ladder, small to large
./pilot eval --only todo-cli       # one PRD
./pilot eval --keep                # keep the sandboxes for debugging
```

Each PRD in `eval/prds/` pairs with an acceptance manifest in `eval/checks/`;
runs happen in throwaway sandboxes under `tests/sandbox/eval` and reports land in
`eval/reports/`. `eval/cmplx/` holds the larger multi-service PRDs.

The full methodology, test inventory, and results are in
[`docs/testing-report.md`](docs/testing-report.md).

---

## Design principles

- **Fully offline by default.** It must work with no internet; only `web_search`
  and `search_images` reach out.
- **The harness is stateless.** It stores nothing between requests. History lives
  in the client — on disk for the terminal, in SQLite for the web and Telegram.
- **One core, many entry points.** The same agent code is imported by the
  terminal and served to the browser and the bot; logic is never duplicated.
- **The model cannot escalate its own permissions.** Only you change the mode,
  and `full_access` is decided server side.
- **Deterministic where it can be.** Scaffolding, dependency installation, and
  the boot-and-run evaluator are real code, not model output — the model is asked
  only for the parts that genuinely need judgement.

For the full design and the rules contributors follow, see
[`AGENTS.md`](AGENTS.md) and the docs in [`docs/`](docs/).
