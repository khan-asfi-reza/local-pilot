package tools

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
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

// A pre-existing listener on the expected port must not be reported as "our
// server is up": that is how a generated Vite app on 5173 got mistaken for the
// Local Pilot UI answering on the same port.
func TestWaitForServerIgnoresAPreExistingListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port

	var log bytes.Buffer
	if _, ready := waitForServer(&log, busy, 700*time.Millisecond, true); ready {
		t.Error("waitForServer reported ready for a port another process already held")
	}

	// Once the server names its own port in its banner, that port is trusted.
	log.WriteString("  ➜  Local:   http://localhost:" + strconv.Itoa(busy) + "/\n")
	if got, ready := waitForServer(&log, busy, 700*time.Millisecond, true); !ready || got != busy {
		t.Errorf("waitForServer after a banner = (%d, %v), want (%d, true)", got, ready, busy)
	}
}

func TestWaitForServerStillWorksOnAFreePort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	var log bytes.Buffer
	if got, ready := waitForServer(&log, port, time.Second, false); !ready || got != port {
		t.Errorf("waitForServer = (%d, %v), want (%d, true)", got, ready, port)
	}
}
