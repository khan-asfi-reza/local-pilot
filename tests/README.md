# Tests

The Local Pilot (Shamsu) test suite. Two languages, one directory.

```
tests/
  go/       black-box tests for the Go harness (package systemtest)
  python/   pytest suite for the FastAPI backend and the Telegram bot
  sandbox/  scratch space for eval runs (git-ignored, not part of this suite)
```

Everything here is black box: the Go tests import the harness packages the way
the terminal and the server do, and the Python tests drive the HTTP API the way
the browser and the bot do. Nothing reaches into private state, so a test that
fails here is a failure a real caller would hit.

## Running

From the repository root:

```sh
# Go: the black-box suite (about a second)
go test ./tests/go

# Go: everything, including the in-package unit tests
go test ./...

# Python: backend, Telegram bridge, and bot rendering
backend/.venv/bin/python -m pytest tests/python -q
```

## Coverage

```sh
# Go, measured against every harness package
go test ./tests/go -coverpkg=./harness/... -coverprofile=/tmp/cover.out
go tool cover -func=/tmp/cover.out | tail -1

# Python (the project venv has no coverage.py, so this uses the stdlib tracer)
backend/.venv/bin/python tests/python/coverage_report.py
```

## What is not covered here

- Model quality. Whether a 4B model actually solves a task is measured by the
  eval harness (`pilot eval`, manifests in `eval/checks/`), not by unit tests.
- The terminal UI and the tree-sitter code graph, which have their own
  in-package tests under `terminal/` and `harness/graph/`.
- The App Builder gateway and the PTY terminal service, which need a real
  browser and a real pseudo-terminal.

## Notes for anyone adding tests

- Never touch the developer's real data: the Go tests point `appdir` at a temp
  home, and the Python suite pins `HOME` and `DATABASE_URL` to a temp directory
  in `conftest.py`.
- No test may need a running model. Model calls go to a stub HTTP server that
  speaks the OpenAI-compatible wire format (`stubModel` in Go, `FakeHarness` in
  Python).
- Name a test after the behaviour it protects, not the function it calls.
