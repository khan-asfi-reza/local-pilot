package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"harness/harness/appdir"
	"harness/harness/events"
	"harness/harness/model"
)

// auditLog appends a JSONL record of raw model I/O, tool calls, and results to
// <datadir>/logs/harness-YYYY-MM-DD.jsonl, so every chat and tool call is kept
// for inspection and debugging. It is safe for concurrent use and across the
// terminal and web-server processes (append-only, one file per day).
type auditLog struct {
	mu  sync.Mutex
	dir string
}

func newAuditLog() *auditLog {
	dir := filepath.Join(appdir.Dir(), "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &auditLog{}
	}
	return &auditLog{dir: dir}
}

func (l *auditLog) write(rec map[string]any) {
	if l == nil || l.dir == "" {
		return
	}
	rec["ts"] = time.Now().Format(time.RFC3339Nano)
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	path := filepath.Join(l.dir, "harness-"+time.Now().Format("2006-01-02")+".jsonl")
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

// turn records the start of a request: the incoming chat, model, and mode.
func (l *auditLog) turn(activeModel, mode, workDir string, msgs []model.Message) {
	l.write(map[string]any{"event": "turn", "model": activeModel, "mode": mode, "work_dir": workDir, "messages": msgs})
}

// request records the full prompt sent to the model for one round.
func (l *auditLog) request(msgs []model.Message, defs []model.ToolDef) {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Function.Name
	}
	l.write(map[string]any{"event": "model_request", "messages": msgs, "tools": names})
}

// response records the raw model reply for one round.
func (l *auditLog) response(content string, toolCalls []model.ToolCall, tokens int) {
	l.write(map[string]any{"event": "model_response", "content": content, "tool_calls": toolCalls, "tokens": tokens})
}

// event records a streamed event (tool calls, results, final text, errors),
// skipping the token-usage pings.
func (l *auditLog) event(ev events.Event) {
	if ev.Type == "usage" {
		return
	}
	l.write(map[string]any{
		"event": ev.Type, "tool": ev.Tool, "info": ev.Info,
		"content": ev.Content, "data": ev.Data, "message": ev.Message,
	})
}
