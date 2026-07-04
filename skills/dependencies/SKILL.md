---
name: dependencies
description: Install project dependencies portably in any language, and recover when an install fails (e.g. Python's "externally-managed-environment" / "pip: command not found"). Use whenever you must install packages before building, testing, or serving.
---

# Installing dependencies (any language)

Install only when the project declares dependencies (a manifest file exists). A standard-library-only program needs nothing installed — skip this entirely for it.

## Python — ALWAYS use a virtualenv

Do NOT use bare `pip` or `python3 -m pip` into the system Python: on most machines it fails with `externally-managed-environment` or `pip: command not found`. Instead create a project-local virtualenv (this always works and keeps the project self-contained):

1. `python3 -m venv .venv`
2. `.venv/bin/pip install -r requirements.txt`  (or `.venv/bin/pip install .` for a pyproject.toml)
3. From now on run everything through the venv: `.venv/bin/python <script>.py`, `.venv/bin/uvicorn ...`, `.venv/bin/pytest`, etc. Never fall back to bare `python3`/`pip` for anything that needs the installed packages.

If you already tried bare `pip` and saw `externally-managed-environment` or `command not found`, that is expected — switch to the `.venv` steps above; do not retry bare pip or reach for `pipx`, `--user`, or `--break-system-packages`.

## Other languages

- Node (`package.json`): `npm install` (creates local `node_modules`).
- PHP (`composer.json`): `composer install`.
- Ruby (`Gemfile`): `bundle install`.
- Rust (`Cargo.toml`): `cargo build` (fetches crates).
- Go (`go.mod`): usually automatic; `go mod download` if needed.
- Java (`pom.xml`): `mvn -q package -DskipTests`.

## Rules
- Manifest first: create requirements.txt / package.json / etc. before installing.
- Python: virtualenv every time. Bare pip is not an option here.
- A "command not found" or "externally-managed" error means change approach (venv), NOT retry the same command.
- Keep the venv and node_modules inside the working directory.
