---
name: go
description: Go project conventions.
internal: true
---
# Go
- Package layout: `main` for commands, library packages otherwise. Keep `go.mod` accurate; run `go mod tidy` after adding imports.
- Handle every error explicitly. Short one-line doc comments on exported funcs.
- Verify: `go build ./...` then `go test ./...`. Fix every compile error before finishing.
