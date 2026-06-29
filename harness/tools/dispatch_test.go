package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"harness/harness/events"
	"harness/harness/model"
)

// editCall builds an edit_file tool call that replaces "a - b" with "a + b".
func editCall(path string) model.ToolCall {
	args, _ := json.Marshal(map[string]any{
		"path": path,
		"edits": []map[string]string{
			{"old_text": "a - b", "new_text": "a + b"},
		},
	})
	return model.ToolCall{
		ID:       "call_0",
		Type:     "function",
		Function: model.FunctionCall{Name: "edit_file", Arguments: string(args)},
	}
}

// TestFlexibleEditReindents checks that an anchor whose indentation differs from
// the file still matches and the replacement is re-indented to the file's style.
func TestFlexibleEditReindents(t *testing.T) {
	content := "def add(a, b):\n\treturn a - b\n" // file uses a tab
	out, err := applyEdits(content, []editSpec{
		{OldText: "    return a - b", NewText: "    return a + b"}, // model used spaces
	})
	if err != nil {
		t.Fatalf("flexible edit failed: %v", err)
	}
	if !strings.Contains(out, "\treturn a + b") {
		t.Fatalf("expected tab-reindented result, got %q", out)
	}
}

// setup writes a small file with the anchor text into a throwaway directory and
// returns the directory, the file name, and the dispatch environment.
func setup(t *testing.T) (string, string, Env) {
	t.Helper()
	dir := t.TempDir()
	name := "hello.py"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("def add(a, b):\n    return a - b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, name, Env{Ctx: context.Background(), WorkDir: dir}
}

func readBody(t *testing.T, dir, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestPlanModeRefusesMutation checks that a mutating tool is refused in plan mode
// and the file is left untouched.
func TestPlanModeRefusesMutation(t *testing.T) {
	reg := NewRegistry(nil)
	dir, name, env := setup(t)

	result, _ := reg.Dispatch(editCall(name), reg.Names(), ModePlan, env, nil)
	if !strings.Contains(result, "Plan mode is read-only") {
		t.Fatalf("expected plan-mode refusal, got %s", result)
	}
	if got := readBody(t, dir, name); strings.Contains(got, "a + b") {
		t.Fatalf("file was modified in plan mode: %s", got)
	}
}

// TestAskModeDeclineDoesNotRun checks that declining a confirmation stops the
// action.
func TestAskModeDeclineDoesNotRun(t *testing.T) {
	reg := NewRegistry(nil)
	dir, name, env := setup(t)

	confirm := func(tool, summary string, diff *events.Diff) (Decision, string) { return Decline, "" }
	result, _ := reg.Dispatch(editCall(name), reg.Names(), ModeAsk, env, confirm)
	if !strings.Contains(result, "declined") {
		t.Fatalf("expected decline message, got %s", result)
	}
	if got := readBody(t, dir, name); strings.Contains(got, "a + b") {
		t.Fatalf("file was modified after decline: %s", got)
	}
}

// TestAskModeApproveRuns checks that approving a confirmation applies the edit
// and produces a diff.
func TestAskModeApproveRuns(t *testing.T) {
	reg := NewRegistry(nil)
	dir, name, env := setup(t)

	var sawDiff bool
	confirm := func(tool, summary string, diff *events.Diff) (Decision, string) {
		sawDiff = diff != nil && diff.Added == 1 && diff.Removed == 1
		return Approve, ""
	}
	result, diff := reg.Dispatch(editCall(name), reg.Names(), ModeAsk, env, confirm)
	if strings.Contains(result, "error") {
		t.Fatalf("unexpected error: %s", result)
	}
	if !sawDiff {
		t.Fatalf("confirmation did not receive the expected diff")
	}
	if diff == nil {
		t.Fatalf("dispatch returned no applied diff")
	}
	if got := readBody(t, dir, name); !strings.Contains(got, "a + b") {
		t.Fatalf("file was not modified after approve: %s", got)
	}
}

// TestAutoModeRunsWithoutConfirm checks that auto mode never asks: a mutating
// tool runs straight through, and the confirm callback is not called.
func TestAutoModeRunsWithoutConfirm(t *testing.T) {
	reg := NewRegistry(nil)
	dir, name, env := setup(t)

	confirm := func(tool, summary string, diff *events.Diff) (Decision, string) {
		t.Fatal("confirm was called in auto mode")
		return Decline, ""
	}
	result, _ := reg.Dispatch(editCall(name), reg.Names(), ModeAuto, env, confirm)
	if strings.Contains(result, "error") {
		t.Fatalf("unexpected error: %s", result)
	}
	if got := readBody(t, dir, name); !strings.Contains(got, "a + b") {
		t.Fatalf("edit was not applied in auto mode: %s", got)
	}
}

// TestReadBeforeEditRequired checks that changing an existing file the model has
// not read this turn is refused, and allowed once it reads the file.
func TestReadBeforeEditRequired(t *testing.T) {
	reg := NewRegistry(nil)
	dir, name, env := setup(t)
	env.Seen = map[string]bool{}

	result, _ := reg.Dispatch(editCall(name), reg.Names(), ModeAuto, env, nil)
	if !strings.Contains(result, "Read the file first") {
		t.Fatalf("expected read-first refusal, got %s", result)
	}

	readArgs, _ := json.Marshal(map[string]any{"path": name})
	readTC := model.ToolCall{ID: "r", Type: "function", Function: model.FunctionCall{Name: "read_file", Arguments: string(readArgs)}}
	reg.Dispatch(readTC, reg.Names(), ModeAuto, env, nil)

	result2, _ := reg.Dispatch(editCall(name), reg.Names(), ModeAuto, env, nil)
	if strings.Contains(result2, "Read the file first") {
		t.Fatalf("edit still refused after read: %s", result2)
	}
	if got := readBody(t, dir, name); !strings.Contains(got, "a + b") {
		t.Fatalf("edit not applied after read: %s", got)
	}
}

// TestAllowedGate checks that a tool absent from the allowed set is refused
// before the mode gate is ever consulted.
func TestAllowedGate(t *testing.T) {
	reg := NewRegistry(nil)
	_, name, env := setup(t)

	result, _ := reg.Dispatch(editCall(name), []string{"read_file"}, ModeAuto, env, nil)
	if !strings.Contains(result, "not allowed") {
		t.Fatalf("expected not-allowed refusal, got %s", result)
	}
}
