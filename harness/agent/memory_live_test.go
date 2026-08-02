package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"harness/harness/model"
)

// TestMemoryMergeLive exercises the real memory-merge call against the running
// model. Gated on PILOT_LIVE=1 so it never runs in normal `go test`.
func TestMemoryMergeLive(t *testing.T) {
	if os.Getenv("PILOT_LIVE") != "1" {
		t.Skip("set PILOT_LIVE=1 and PILOT_CONFIG to run")
	}
	cfg, err := model.LoadConfig(os.Getenv("PILOT_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	a := &Agent{cfg: cfg, router: model.NewRouter(cfg, model.NewClient()), contextTokens: cfg.ContextTokens}

	// First, raw: does the model satisfy the memory schema at all?
	msgs := []model.Message{
		{Role: "system", Content: "Output ONLY the JSON."},
		{Role: "user", Content: "Project is a C program area.c that computes shape areas. Summarize into the memory schema."},
	}
	raw, _, err := a.router.Constrained(context.Background(), msgs, json.RawMessage(memorySchema))
	t.Logf("raw err=%v\nraw=%s", err, raw)
	if err == nil {
		var m memory
		t.Logf("unmarshal err=%v", json.Unmarshal([]byte(raw), &m))
	}

	// Then the full path.
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.WriteFile(filepath.Join(dir, "area.c"), []byte("int main(void){return 0;}\n"), 0o644)
	a.updateMemory(context.Background(), dir, []string{"area.c"}, "remove all comments from area.c")
	got := discoverMemory(dir)
	t.Logf("MEMORY FILE:\n%s", got)
	if got == "" {
		t.Error("memory not written")
	}
}
