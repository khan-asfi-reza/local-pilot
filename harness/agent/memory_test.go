package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnforceBudgetCapsAndPrunes(t *testing.T) {
	dir := t.TempDir()
	// One real file, one that does not exist — the missing one must be pruned.
	os.WriteFile(filepath.Join(dir, "real.go"), []byte("package x\n"), 0o644)

	m := memory{
		Summary:     strings.Repeat("a", maxSummaryBytes+50),
		Stack:       makeStrings(maxStack + 5),
		KeyFiles:    []keyFile{{Path: "real.go", Purpose: "kept"}, {Path: "ghost.go", Purpose: "pruned"}},
		Done:        makeStrings(maxDone + 4), // "0".."18"
		Conventions: makeStrings(maxConventions + 3),
		TODO:        makeStrings(maxTODO + 2),
	}
	enforceBudget(&m, dir)

	if len(m.Summary) != maxSummaryBytes {
		t.Errorf("summary not clamped: got %d want %d", len(m.Summary), maxSummaryBytes)
	}
	if len(m.Stack) != maxStack {
		t.Errorf("stack not capped: %d", len(m.Stack))
	}
	if len(m.KeyFiles) != 1 || m.KeyFiles[0].Path != "real.go" {
		t.Errorf("key files not pruned to existing: %v", m.KeyFiles)
	}
	if len(m.Done) != maxDone {
		t.Fatalf("done not capped: %d", len(m.Done))
	}
	// Done keeps the NEWEST (FIFO drop-oldest): last element is the highest index.
	if m.Done[len(m.Done)-1] != "18" {
		t.Errorf("done should keep newest, tail = %q", m.Done[len(m.Done)-1])
	}
	if len(m.TODO) != maxTODO {
		t.Errorf("todo not capped: %d", len(m.TODO))
	}
}

func TestRenderMarkdownHasFixedHeaders(t *testing.T) {
	md := renderMarkdown(memory{
		Summary:  "A tool.",
		Stack:    []string{"Go"},
		KeyFiles: []keyFile{{Path: "main.go", Purpose: "entry"}},
		Done:     []string{"built it"},
	})
	for _, h := range []string{"## Summary", "## Stack", "## Key files", "## Done", "## Conventions", "## TODO"} {
		if !strings.Contains(md, h) {
			t.Errorf("rendered memory missing header %q", h)
		}
	}
	if !strings.Contains(md, "main.go — entry") {
		t.Errorf("key file not rendered: %s", md)
	}
	if len(md) > maxMemoryBytes {
		t.Errorf("rendered memory exceeds budget: %d", len(md))
	}
}

func TestMemoryPathUnderGitRoot(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	sub := filepath.Join(root, "pkg", "inner")
	os.MkdirAll(sub, 0o755)

	got := memoryPath(sub)
	want := filepath.Join(root, ".pilot", "memory.md")
	if got != want {
		t.Errorf("memoryPath(%q) = %q, want %q", sub, got, want)
	}
}

func TestWriteAndDiscoverMemoryRoundTrip(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".git"), 0o755)

	md := renderMarkdown(memory{Summary: "Round trip.", Stack: []string{"Go"}})
	if err := writeMemory(memoryPath(root), md); err != nil {
		t.Fatalf("writeMemory: %v", err)
	}
	got := discoverMemory(root)
	if !strings.Contains(got, "Round trip.") {
		t.Errorf("discoverMemory did not return written content: %q", got)
	}
}

func makeStrings(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = itoa(i)
	}
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
