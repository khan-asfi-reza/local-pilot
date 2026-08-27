package systemtest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"harness/harness/appdir"
	"harness/harness/projects"
)

// isolateAppDir points the shared data directory at a temp home for one test.
func isolateAppDir(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LOCALAPPDATA", home)
	case "darwin":
		t.Setenv("HOME", home)
	default:
		t.Setenv("XDG_DATA_HOME", home)
	}
	dir := appdir.Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestProjectRegistryIsStablePerPath checks the registry the terminal, the web
// Code IDE and Telegram all share: opening the same folder twice must refresh one
// record, not create a second, or the three surfaces drift onto different ids.
func TestProjectRegistryIsStablePerPath(t *testing.T) {
	isolateAppDir(t)
	work := t.TempDir()

	first, err := projects.Upsert(work, "terminal")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.Name != filepath.Base(work) {
		t.Fatalf("registered record looks wrong: %+v", first)
	}

	second, err := projects.Upsert(work, "web")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Errorf("the same path produced two ids: %q then %q", first.ID, second.ID)
	}
	if second.LastOpened == "" {
		t.Error("re-opening did not refresh last_opened")
	}

	other, err := projects.Upsert(t.TempDir(), "telegram")
	if err != nil {
		t.Fatal(err)
	}
	if other.ID == first.ID {
		t.Error("two different folders share one id")
	}
	if n := len(projects.Load()); n != 2 {
		t.Fatalf("registry holds %d projects, want 2", n)
	}
}

// TestProjectRegistryFileMatchesThePythonSchema checks the on-disk JSON keys the
// FastAPI backend reads. Go and Python own the same file, so a renamed field here
// silently empties the project list in the web UI and the Telegram bot.
func TestProjectRegistryFileMatchesThePythonSchema(t *testing.T) {
	dir := isolateAppDir(t)
	if _, err := projects.Upsert(t.TempDir(), "terminal"); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "projects.json"))
	if err != nil {
		t.Fatalf("registry file not written where the backend looks for it: %v", err)
	}
	var records []map[string]any
	if err := json.Unmarshal(raw, &records); err != nil {
		t.Fatalf("registry is not a JSON array: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	for _, key := range []string{"id", "path", "name", "source", "created_at", "last_opened"} {
		if _, ok := records[0][key]; !ok {
			t.Errorf("registry record is missing the %q field the Python side reads", key)
		}
	}
	if !filepath.IsAbs(records[0]["path"].(string)) {
		t.Error("paths must be absolute so every runtime resolves the same folder")
	}
}

// TestLoadIsEmptyBeforeAnythingIsRegistered checks a fresh install reads cleanly
// rather than failing.
func TestLoadIsEmptyBeforeAnythingIsRegistered(t *testing.T) {
	isolateAppDir(t)
	if got := projects.Load(); len(got) != 0 {
		t.Fatalf("a fresh registry returned %d projects", len(got))
	}
}
