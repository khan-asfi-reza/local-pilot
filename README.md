# local-pilot

A local, offline-first coding assistant. It runs a small language model on your
own machine and lets that model actually work in your project: search files,
read and edit code, run shell commands, start servers, and verify its own work,
all without sending a single byte off the device.

Normal use needs no internet. The only tool that reaches the network is
`web_search`, and it is opt-in.

> **Status: early, terminal-first.** The Go agent core and the terminal client
> work today. The web backend and frontend are planned but not yet built (their
> folders are empty scaffolds). See [Project state](#project-state) below.

---

## Requirements

| Requirement | Version / notes |
|-------------|-----------------|
| **Go**      | 1.25 or newer (builds the pilot binary) |
| **ollama**  | Installed and on your `PATH` ([ollama.com](https://ollama.com)). `pilot start` will launch it for you. |
| **Disk**    | ~5 GB for the default `qwen2.5-coder:7b` model (one-time download) |
| **RAM**     | 8 GB works; 16 GB or more recommended for the 7B model |
| **OS**      | macOS, Linux, or Windows |

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
go build -o bin/pilot ./cmd/pilot   # the single pilot binary (start/add/code/run)
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

### 3. Bring up ollama and the default model

```bash
./pilot start
```

`pilot start` is the one-time setup. It:

1. installs ollama if it is missing (with your confirmation), then starts it,
2. shows a menu of models to pick from (or type any ollama model name); press
   Enter for the default,
3. if the chosen model is not installed, pulls its base (e.g. `qwen2.5-coder:7b`,
   ~5 GB one-time download), applies the tool-call template and context window,
   and creates the local model,
4. sets it as the default and confirms it is ready.

It is idempotent. Run it any time to make sure everything is up.

The model menu is driven by the `suggested` list in `models.json`:

```
Choose a model:
  1. qwen2.5-coder:7b   (default)
  2. qwen2.5-coder:14b
  3. qwen2.5-coder:32b
  4. qwen2.5-coder:3b
  5. Enter an ollama model name
```

### Data directory

Config and skills live in a per-user data directory, seeded from the built-in
defaults on first run and reused after (so pilot runs from anywhere, and your
added models and prompt edits persist):

| OS | Location |
|----|----------|
| macOS | `~/.localpilot` |
| Linux | `$XDG_DATA_HOME/localpilot` or `~/.local/share/localpilot` |
| Windows | `%LOCALAPPDATA%\localpilot` |

It mirrors the repo layout: `models/models.json`, `models/prompt.json`, and
`skills/`. Delete it to reset to defaults; `pilot start` recreates it.

### 4. Start working

```bash
# interactive coding agent in a project of your choice
./pilot code --dir /path/to/your/project

# ...or a single task headless (auto mode, exits when done)
./pilot run --dir /path/to/your/project --task "Add a README and a smoke test."
```

### The `pilot` commands

| Command | What it does |
|---------|--------------|
| `./pilot start`               | Install/start ollama, pick a model, and get ready |
| `./pilot stop`                | Stop the ollama server |
| `./pilot models add <model>`  | Pull a base model, apply the tool-call template, and register it |
| `./pilot models list`         | List installed models and the default |
| `./pilot models set-default`  | Choose the default from installed models (or pass a name) |
| `./pilot code [--dir X]`      | Open the interactive terminal UI |
| `./pilot run --dir X --task "..."` | Run one task to completion, headless (`--task-file F`, `--max-steps N`, `--format ndjson\|human`) |

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

---

## Architecture

```
              +-----------------------------------------------+
              |                 local-pilot                   |
              |                                               |
  you  ------>|  terminal client (TUI / headless run)         |
              |        |                                      |
              |        v                                      |
              |  harness core  -- ReAct loop --> tools        |
              |        |            (search, edit_file,        |
              |        |             shell_run, serve, ...)    |
              |        v                                       |
              |  model router --> ollama (localhost:11434)     |
              +-----------------------------------------------+
```

- **Harness core** (`harness/`, Go): the stateless agent loop, the tool set, the
  context manager, and a URL-based model router. It is used two ways: imported
  directly by the terminal client, and (later) run behind a thin HTTP server for
  the web path.
- **Model**: served by [ollama](https://ollama.com) over its OpenAI-compatible
  API on `localhost:11434`. The default is `qwen2.5-coder:7b-tools`, a local
  variant `pilot` builds for you (see [Adding a model](#adding-a-model)).
- **Tools**: `search`, `list_dir`, `read_file`, `write_file`, `edit_file`
  (anchor-based, never by line number), `shell_run`, `serve` (background
  servers), `code_run` (sandboxed snippets), `web_search`, and `load_skill`.
  `shell_run` refuses to launch blocking servers (use `serve`) and refuses
  commands that reach outside the working directory without approval.
- **Skills**: short procedures in `skills/` (`debug`, `dependencies`, `serving`)
  the model loads on demand when a task matches.

---

## Adding a model

Small local models do not emit tool calls in a format ollama can parse out of
the box. `pilot models add` fixes that: it pulls the base model, for
qwen2.5-coder rewrites its Modelfile template to swap `<tool_call>` tags for
`[tool_call]` (which that family emits reliably; other families are left
untouched), bakes in the context window, picks a tool-calling mode by size
(models `<=` 3B use the grammar-JSON path, larger ones use native calls), and
registers the result as `<base>-tools`.

```bash
./pilot models add qwen2.5-coder:14b   # creates and registers qwen2.5-coder:14b-tools
```

Then switch to it from inside the TUI with `/model qwen2.5-coder:14b-tools`.

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
register models from several machines and switch between them with `/model` (in
the TUI) or `pilot models set-default`. `pilot models list` shows each model's
server and whether it is reachable:

```
Configured models:
➤ qwen2.5-coder:7b-tools      ready  local
  qwen2.5-coder:32b-tools     ready  http://192.168.1.50:11434
```

The remote machine must have ollama reachable on the network (start it with
`OLLAMA_HOST=0.0.0.0 ollama serve` so it listens beyond localhost).

Models are declared in `models/models.json`:

```json
{
  "context_tokens": 24000,
  "default": "qwen2.5-coder:7b-tools",
  "models": [
    { "name": "qwen2.5-coder:7b-tools", "port": 11434, "tool_mode": "native", "base": "qwen2.5-coder:7b", "num_ctx": 32768 }
  ]
}
```

The system prompt lives in `models/prompt.json` and is reloaded on every run, so
you can tune the agent's behavior there with no rebuild.

---

## Repository layout

```
cmd/pilot/     Go: the pilot binary (start / add / code / run)
terminal/      Go: the terminal client package (interactive TUI and headless run)
harness/       Go: agent loop, tools, context manager, model client/router, HTTP server
models/        models.json (registry) and prompt.json (the system prompt)
skills/        on-demand skills: debug, dependencies, serving
tests/         test suites; tests/sandbox is the throwaway working dir
backend/       (planned) Python/FastAPI relay for the web path, not built yet
frontend/      (planned) React/Tailwind chat UI, not built yet
docs/          project documentation and interface contracts
bin/           compiled binaries (gitignored)
pilot          the ./pilot launcher script
AGENTS.md      design notes and architecture rules for contributors
```

> The Go module is named `harness` internally; **local-pilot** is the product name.

---

## Development

The Go workspace builds and tests as a whole:

```bash
go build ./...                 # build everything
go test ./...                  # run all tests
go test ./harness -run TestX   # run one test
gofmt -l . && go vet ./...     # lint
```

You can also run the pieces directly, bypassing the launcher:

```bash
go run ./cmd/pilot code --dir .                     # interactive TUI
go run ./cmd/pilot run --dir . --task "..."          # headless one-shot
go run ./harness/server --port 9000                  # headless HTTP wrapper
```

---

## Project state

**Working today**

- The stateless harness agent core with the full ReAct loop and context
  compaction.
- The complete v1 tool set, with the plan / ask / auto permission model.
- The terminal client: interactive TUI plus the headless `run` command.
- ollama integration with native tool-calling (and a JSON tool-call fallback
  path), the `models.json` registry, and the `pilot` launcher.
- The `debug`, `dependencies`, and `serving` skills.
- The headless HTTP server wrapper (`harness/server`).

**Planned (scaffolded, not yet implemented)**

- **Backend**: a stateless Python/FastAPI relay exposing only the *safe* tool
  set (`code_run`, `web_search`) for the web path. File and shell tools stay
  terminal-only; that split is the core safety boundary.
- **Frontend**: a React/Tailwind chat UI holding conversation history.
- Off-device model backends (LAN or hosted). The router is already URL-based so
  these can be added without touching the loop.

---

## Design principles

- **Fully offline by default.** v1 must work with no internet; only `web_search`
  reaches out.
- **The harness is stateless.** It stores nothing between requests. History
  lives in the client, on disk for the terminal.
- **One core, two entry points.** The same agent code is imported by the
  terminal and served for the web; logic is never duplicated.
- **The model cannot escalate its own permissions.** Only you change the mode.

For the full design and the rules contributors follow, see
[`AGENTS.md`](AGENTS.md) and the docs in [`docs/`](docs/).
