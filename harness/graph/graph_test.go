package graph

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTree writes files (rel path -> content) under a fresh temp dir.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestBuildGoGraph(t *testing.T) {
	if !Enabled() {
		t.Skip("tree-sitter disabled (nots build)")
	}
	dir := writeTree(t, map[string]string{
		"main.go": `package main

func helper() string { return "hi" }

func main() {
	_ = helper()
	_ = helper()
}
`,
	})
	g, err := Build(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !g.DefinesInFile("main.go", "helper") || !g.DefinesInFile("main.go", "main") {
		t.Fatalf("expected helper and main symbols, got nodes=%d", len(g.Nodes))
	}
	// main calls helper → a calls edge from main to helper must exist.
	callers := g.Query("callers", "helper", "", 10)
	found := false
	for _, r := range callers {
		if r.Name == "main" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected main to be a caller of helper, got %+v", callers)
	}
}

func TestBuildPythonGraphAndImports(t *testing.T) {
	if !Enabled() {
		t.Skip("tree-sitter disabled")
	}
	dir := writeTree(t, map[string]string{
		"db.py": "def connect():\n    return 1\n",
		"app.py": `from db import connect


def main():
    connect()
`,
	})
	g, err := Build(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !g.DefinesInFile("db.py", "connect") {
		t.Fatal("expected connect defined in db.py")
	}
	// app.py imports db.py
	imps := g.Query("imports", "", "app.py", 10)
	if len(imps) == 0 || imps[0].File != "db.py" {
		t.Errorf("expected app.py to import db.py, got %+v", imps)
	}
	// digest must render and be bounded
	d := g.Digest(6000, nil)
	if !strings.Contains(d, "app.py") || len(d) > 6000 {
		t.Errorf("digest unexpected (len=%d): %s", len(d), d)
	}
}

func TestIncrementalReuse(t *testing.T) {
	if !Enabled() {
		t.Skip("tree-sitter disabled")
	}
	dir := writeTree(t, map[string]string{
		"a.go": "package main\nfunc A() {}\n",
	})
	g1, err := Build(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Add a second file; rebuild with prev — a.go must be reused (same hash).
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\nfunc B() { A() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g2, err := Build(context.Background(), dir, g1)
	if err != nil {
		t.Fatal(err)
	}
	if !g2.DefinesInFile("a.go", "A") || !g2.DefinesInFile("b.go", "B") {
		t.Fatal("expected A and B after incremental build")
	}
	callers := g2.Query("callers", "A", "", 10)
	if len(callers) == 0 {
		t.Errorf("expected B to call A across files, got %+v", callers)
	}
}
