package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogCallWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	SetLogDir(dir)
	defer SetLogDir("")
	logCall("chat", "qwen", []Message{{Role: "user", Content: "hi"}}, "hello", []ToolCall{{Function: FunctionCall{Name: "write_file"}}}, 42, time.Now(), nil)
	logCall("constrained", "qwen", []Message{{Role: "system", Content: "x"}}, `{"a":1}`, nil, 7, time.Now(), nil)
	raw, err := os.ReadFile(filepath.Join(dir, "llm.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 log lines, got %d", len(lines))
	}
	var e callEntry
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("bad jsonl: %v", err)
	}
	if e.Kind != "chat" || e.Model != "qwen" || e.Response != "hello" || len(e.Request) != 1 || len(e.ToolCalls) != 1 || e.Tokens != 42 {
		t.Errorf("bad entry: %+v", e)
	}
}
