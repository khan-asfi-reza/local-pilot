package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRepoMapListsSymbols checks the repo map lists a code file with its
// top-level definitions and skips ignored directories.
func TestRepoMapListsSymbols(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("app.py", "import os\n\ndef add(a, b):\n    return a + b\n\nclass Thing:\n    pass\n")
	write("util.go", "package util\n\nfunc Hello() string { return \"hi\" }\ntype Config struct{}\n")
	write("node_modules/pkg/index.js", "function shouldBeSkipped() {}\n")

	m := buildRepoMap(dir)

	for _, want := range []string{"app.py", "def add(a, b):", "class Thing", "util.go", "func Hello", "type Config"} {
		if !strings.Contains(m, want) {
			t.Fatalf("repo map missing %q:\n%s", want, m)
		}
	}
	if strings.Contains(m, "shouldBeSkipped") || strings.Contains(m, "node_modules") {
		t.Fatalf("repo map did not skip node_modules:\n%s", m)
	}
}
