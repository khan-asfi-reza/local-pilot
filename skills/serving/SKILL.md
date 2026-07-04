---
name: serving
description: Run and verify a web server or app in ANY language or framework (FastAPI, Flask, Django, Express, Next.js, PHP, Laravel, Rails, Sinatra, Spring Boot, .NET, Go, Rust, Phoenix, static). Use whenever the task needs a server started or a running endpoint checked. Never start a server with shell_run — it blocks; use the serve tool, then curl.
---

# Running and verifying a server (any language / framework)

A server does not exit on its own, so it must NEVER be launched with shell_run — that blocks until the timeout and fails the task. Use the `serve` tool: it starts the server in the background, waits until its port is open, and stops it automatically when the task ends.

## 1. Install dependencies FIRST — but only if the project declares some

A program that uses only the standard library needs NO install; skip this step for it. Install only when the matching manifest file exists:

- Python (`requirements.txt` or `pyproject.toml`): make a local virtualenv so it works on managed systems — `python3 -m venv .venv` then `.venv/bin/pip install -r requirements.txt` (or `.venv/bin/pip install .`). Afterwards use `.venv/bin/python`, `.venv/bin/uvicorn`, etc. — NOT bare `pip`/`python3`, which may be blocked as "externally-managed".
- Node (`package.json`): `npm install`.
- PHP (`composer.json`): `composer install`.
- Ruby (`Gemfile`): `bundle install`.
- Java (`pom.xml` / `build.gradle`): `mvn -q package -DskipTests` / `gradle build`.
- Rust (`Cargo.toml`): `cargo build`.
- .NET (`*.csproj`): `dotnet restore`.
- Elixir (`mix.exs`): `mix deps.get`.
- Go (`go.mod`): usually nothing; `go mod download` if needed.

## 2. Start the server with the `serve` tool

Give it the command and the port it listens on. Pick the row for the framework:

Python
- FastAPI: `.venv/bin/uvicorn main:app --port 8000` (module path to the app, e.g. `backend.main:app`), port 8000.
- Flask: `.venv/bin/flask --app main run --port 8000`, port 8000.
- Django (dev): `.venv/bin/python manage.py runserver 8000`, port 8000.
- Django (prod): `.venv/bin/gunicorn <project>.wsgi --bind 127.0.0.1:8000`, port 8000.
- Static files (no deps): `python3 -m http.server 8000`, port 8000.

Node / JS
- Express or plain: `node server.js`, port from the code (often 3000).
- npm script: `npm start` or `npm run dev`, port from the app.
- Next.js: `npm run dev`, port 3000.

PHP
- Built-in server: `php -S localhost:8000`, port 8000.
- Laravel: `php artisan serve --port 8000`, port 8000.
- Symfony: `symfony server:start` or `php -S localhost:8000 -t public`, port 8000.

Ruby
- Rails: `bin/rails server -p 3000`, port 3000.
- Rack / Sinatra: `rackup -p 9292`, port 9292.

JVM / .NET / others
- Spring Boot: `mvn spring-boot:run` (or `java -jar target/app.jar`), port 8080.
- .NET: `dotnet run`, port 5000.
- Go: `go run .` (only if it is a server), port from the code.
- Rust (axum/actix): `cargo run`, port from the code.
- Elixir / Phoenix: `mix phx.server`, port 4000.

If your framework is not listed, use its documented run command with the `serve` tool the same way — the tool is language-agnostic.

## 3. Confirm it came up

Read the serve result. `ready:true` means the port opened. If `ready:false`, read `logs` for the startup error (missing dependency, wrong module path, port in use, import error), fix it, and serve again. Do not proceed until it is ready.

## 4. Verify real behavior with curl (shell_run)

- GET: `curl -s http://localhost:8000/` or a route like `curl -s http://localhost:8000/todos`.
- POST: `curl -s -X POST http://localhost:8000/todos -H "Content-Type: application/json" -d '{"title":"x"}'`.
Confirm the response matches what the task expects.

## 5. Finish

Only after a real request returned the correct response. In your final reply give the user the exact command to start the server themselves.

## Rules
- Never run a server, watcher, or `--reload`/`--watch` process with shell_run. Use serve.
- Install dependencies before serving — but only if the project has a manifest; stdlib apps need none.
- Keep the port consistent between the serve command and the curl checks.
- The serve tool stops the server for you at the end; do not kill it yourself.
