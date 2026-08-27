package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseBoundPort(t *testing.T) {
	cases := map[string]int{
		"VITE v7  ready in 300 ms\n  ➜  Local:   http://localhost:5245/": 5245,
		"Uvicorn running on http://127.0.0.1:8360 (Press CTRL+C to quit)":  8360,
		"Server listening on 0.0.0.0:8080":                                8080,
		"listening on port 3001":                                          3001,
		"app running on port 4000":                                        4000,
		"no port here at all":                                             0,
	}
	for in, want := range cases {
		if got := parseBoundPort(in); got != want {
			t.Errorf("parseBoundPort(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestReadEnvPort(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DATABASE_URL=postgres://x\nPORT=8360\nVITE_PORT=5245\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readEnvPort(dir, "PORT"); got != 8360 {
		t.Errorf("readEnvPort PORT = %d, want 8360", got)
	}
	if got := readEnvPort(dir, "VITE_PORT"); got != 5245 {
		t.Errorf("readEnvPort VITE_PORT = %d, want 5245", got)
	}
	if got := readEnvPort(dir, "MISSING"); got != 0 {
		t.Errorf("readEnvPort MISSING = %d, want 0", got)
	}

	// A monorepo backend reads the root .env one level up.
	be := filepath.Join(dir, "backend")
	if err := os.MkdirAll(be, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := readEnvPort(be, "PORT"); got != 8360 {
		t.Errorf("readEnvPort backend PORT (../.env) = %d, want 8360", got)
	}
}

func TestIsFrontendServeCmd(t *testing.T) {
	front := []string{"npm run dev", "vite", "npx vite --host", "next dev", "pnpm dev"}
	back := []string{"node server.js", "npm start", ".venv/bin/uvicorn main:app", "go run ."}
	for _, c := range front {
		if !isFrontendServeCmd(c) {
			t.Errorf("isFrontendServeCmd(%q) = false, want true", c)
		}
	}
	for _, c := range back {
		if isFrontendServeCmd(c) {
			t.Errorf("isFrontendServeCmd(%q) = true, want false", c)
		}
	}
}
