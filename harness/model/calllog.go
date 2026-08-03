package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LLM-call logging: every model call (main loop, planning, children, memory) is
// appended as one JSON line to <dir>/llm.jsonl for later visualization. The dir
// is a process-wide sink set once per run (all children share the work dir).

var (
	logMu  sync.Mutex
	logDir string
)

// SetLogDir points LLM-call logging at dir (creating it). Empty disables it.
func SetLogDir(dir string) {
	logMu.Lock()
	logDir = dir
	logMu.Unlock()
	if dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
}

type callEntry struct {
	TS         string     `json:"ts"`
	Kind       string     `json:"kind"` // chat | constrained
	Model      string     `json:"model"`
	Request    []Message  `json:"request"`
	Response   string     `json:"response,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Tokens     int        `json:"tokens"`
	DurationMS int64      `json:"duration_ms"`
	Error      string     `json:"error,omitempty"`
}

func logCall(kind, model string, req []Message, resp string, tcs []ToolCall, tokens int, start time.Time, callErr error) {
	logMu.Lock()
	dir := logDir
	logMu.Unlock()
	if dir == "" {
		return
	}
	e := callEntry{
		TS: time.Now().Format(time.RFC3339Nano), Kind: kind, Model: model,
		Request: req, Response: resp, ToolCalls: tcs, Tokens: tokens,
		DurationMS: time.Since(start).Milliseconds(),
	}
	if callErr != nil {
		e.Error = callErr.Error()
	}
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	logMu.Lock()
	defer logMu.Unlock()
	f, err := os.OpenFile(filepath.Join(dir, "llm.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}
