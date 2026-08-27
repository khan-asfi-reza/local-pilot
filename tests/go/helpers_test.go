// Package systemtest is the black-box test suite for Local Pilot (Shamsu).
//
// It sits outside every harness package on purpose: each test drives the same
// exported surface the rest of the system (terminal, web backend, Telegram
// bridge) drives, so a test failing here means a real caller would break too.
package systemtest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"harness/harness/model"
	"harness/harness/tools"
)

// tempProject writes a set of files into a throwaway directory and returns it.
func tempProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// call builds one native tool call the dispatcher can run.
func call(t *testing.T, name string, args map[string]any) model.ToolCall {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return model.ToolCall{
		ID:       "call_test",
		Type:     "function",
		Function: model.FunctionCall{Name: name, Arguments: string(raw)},
	}
}

// env is a dispatch environment rooted at dir with read-tracking enabled.
func env(dir string) tools.Env {
	return tools.Env{Ctx: context.Background(), WorkDir: dir, Seen: map[string]bool{}}
}

// body reads a file back out of a project directory.
func body(t *testing.T, dir, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// decode unmarshals a dispatch result string into a map.
func decode(t *testing.T, result string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Fatalf("result is not JSON: %s", result)
	}
	return m
}

// errorOf returns the error message a dispatch result carries, or "".
func errorOf(t *testing.T, result string) string {
	t.Helper()
	m := decode(t, result)
	s, _ := m["error"].(string)
	return s
}

// exists reports whether a path is present under dir.
func exists(dir, rel string) bool {
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	return err == nil
}
