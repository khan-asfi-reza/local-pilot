package systemtest

import (
	"encoding/json"
	"testing"

	"harness/harness/events"
)

// decodeEvent marshals an event the way the harness streams it (one JSON line)
// and reads it back as a generic map, which is exactly what the web UI and the
// Telegram bridge parse.
func decodeEvent(t *testing.T, ev events.Event) map[string]any {
	t.Helper()
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// TestEventWireShape checks the newline-delimited JSON contract shared by the
// terminal, the web Code IDE and the Telegram bot. Both clients switch on `type`
// and read fixed field names, so a rename here breaks them silently.
func TestEventWireShape(t *testing.T) {
	cases := []struct {
		name     string
		event    events.Event
		wantType string
		wantKeys map[string]any
	}{
		{"text", events.Text("hello"), "text", map[string]any{"content": "hello"}},
		{"reasoning", events.Reasoning("thinking"), "reasoning", map[string]any{"content": "thinking"}},
		{"tool_call", events.ToolCall("write_file", "app.py", `{"path":"app.py"}`), "tool_call",
			map[string]any{"tool": "write_file", "info": "app.py", "data": `{"path":"app.py"}`}},
		{"tool_result", events.ToolResult("write_file", "12 bytes written", "{}", nil), "tool_result",
			map[string]any{"tool": "write_file", "info": "12 bytes written", "data": "{}"}},
		{"confirm", events.Confirm("c1", "shell_run", "run: pytest", nil), "confirm",
			map[string]any{"id": "c1", "tool": "shell_run", "summary": "run: pytest"}},
		{"error", events.Error("backend unreachable"), "error", map[string]any{"message": "backend unreachable"}},
		{"usage", events.Usage(1234), "usage", map[string]any{"tokens": float64(1234)}},
		{"done", events.Done(), "done", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := decodeEvent(t, tc.event)
			if m["type"] != tc.wantType {
				t.Fatalf("type = %v, want %q", m["type"], tc.wantType)
			}
			for k, want := range tc.wantKeys {
				if m[k] != want {
					t.Errorf("field %q = %v, want %v", k, m[k], want)
				}
			}
		})
	}
}

// TestDoneEventCarriesNothingElse checks empty fields are omitted, so a stream of
// text deltas stays small over the wire.
func TestDoneEventCarriesNothingElse(t *testing.T) {
	m := decodeEvent(t, events.Done())
	if len(m) != 1 {
		t.Fatalf("done event carries extra fields: %v", m)
	}
	if got := decodeEvent(t, events.Text("hi")); len(got) != 2 {
		t.Fatalf("text event carries extra fields: %v", got)
	}
}

// TestDiffWireShape checks the diff payload the UI renders and the Telegram
// bridge flattens into +/- lines.
func TestDiffWireShape(t *testing.T) {
	diff := &events.Diff{
		Path: "calc.py", Added: 1, Removed: 1,
		Hunks: []events.Hunk{{
			OldStart: 1, OldCount: 2, NewStart: 1, NewCount: 2,
			Lines: []events.DiffLine{
				{Op: events.OpContext, Old: 1, New: 1, Text: "def add(a, b):"},
				{Op: events.OpRemove, Old: 2, Text: "    return a - b"},
				{Op: events.OpAdd, New: 2, Text: "    return a + b"},
			},
		}},
	}
	m := decodeEvent(t, events.ToolResult("edit_file", "calc.py", "{}", diff))

	d, ok := m["diff"].(map[string]any)
	if !ok {
		t.Fatalf("diff missing from the event: %v", m)
	}
	if d["path"] != "calc.py" || d["added"] != float64(1) || d["removed"] != float64(1) {
		t.Errorf("diff header wrong: %v", d)
	}
	hunks, ok := d["hunks"].([]any)
	if !ok || len(hunks) != 1 {
		t.Fatalf("hunks wrong: %v", d["hunks"])
	}
	lines := hunks[0].(map[string]any)["lines"].([]any)
	if len(lines) != 3 {
		t.Fatalf("expected 3 diff lines, got %d", len(lines))
	}
	ops := []string{}
	for _, ln := range lines {
		ops = append(ops, ln.(map[string]any)["op"].(string))
	}
	if ops[0] != "context" || ops[1] != "remove" || ops[2] != "add" {
		t.Errorf("diff line ops = %v, want context remove add", ops)
	}
}
