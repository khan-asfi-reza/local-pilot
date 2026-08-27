package systemtest

import (
	"strings"
	"testing"

	"harness/harness/tools"
)

// TestPathsCannotEscapeTheProject checks the file-access boundary: a relative
// climb, an absolute path and a sneaky mixed path are all refused, for reads and
// for writes.
func TestPathsCannotEscapeTheProject(t *testing.T) {
	dir := tempProject(t, map[string]string{"inside.txt": "ok\n"})
	reg := tools.NewRegistry(nil)
	e := env(dir)

	escapes := []string{"../outside.txt", "../../etc/hosts", "/etc/hosts", "sub/../../outside.txt"}
	for _, p := range escapes {
		t.Run(p, func(t *testing.T) {
			out, _ := reg.Dispatch(call(t, "read_file", map[string]any{"path": p}), reg.Names(), tools.ModeAuto, e, nil)
			if !strings.Contains(errorOf(t, out), "escapes the working directory") {
				t.Errorf("read of %q was not blocked: %s", p, out)
			}
			out, _ = reg.Dispatch(call(t, "write_file", map[string]any{"path": p, "content": "pwned"}), reg.Names(), tools.ModeAuto, e, nil)
			if !strings.Contains(errorOf(t, out), "escapes the working directory") {
				t.Errorf("write to %q was not blocked: %s", p, out)
			}
		})
	}

	// A nested path INSIDE the project is fine, and parents are created.
	out, _ := reg.Dispatch(call(t, "write_file", map[string]any{"path": "a/b/c.txt", "content": "deep\n"}), reg.Names(), tools.ModeAuto, e, nil)
	if errorOf(t, out) != "" {
		t.Fatalf("nested write failed: %s", out)
	}
	if body(t, dir, "a/b/c.txt") != "deep\n" {
		t.Fatal("nested write did not land")
	}
}

// TestEditRefusesAmbiguousAndMissingAnchors checks an anchor edit is all-or-
// nothing: a non-unique or absent anchor errors and the file is left alone.
func TestEditRefusesAmbiguousAndMissingAnchors(t *testing.T) {
	original := "x = 1\nx = 1\ny = 2\n"
	dir := tempProject(t, map[string]string{"vals.py": original})
	reg := tools.NewRegistry(nil)
	e := env(dir)
	reg.Dispatch(call(t, "read_file", map[string]any{"path": "vals.py"}), reg.Names(), tools.ModeAuto, e, nil)

	cases := []struct {
		name, old, want string
	}{
		{"ambiguous", "x = 1", "more than once"},
		{"missing", "z = 9", "not found"},
		{"empty", "", "old_text is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := reg.Dispatch(call(t, "edit_file", map[string]any{
				"path":  "vals.py",
				"edits": []map[string]string{{"old_text": tc.old, "new_text": "q = 0"}},
			}), reg.Names(), tools.ModeAuto, e, nil)
			if !strings.Contains(errorOf(t, out), tc.want) {
				t.Errorf("expected %q in the error, got %s", tc.want, out)
			}
			if body(t, dir, "vals.py") != original {
				t.Error("a rejected edit still modified the file")
			}
		})
	}
}

// TestEditToleratesModelIndentation checks the whitespace-flexible fallback: an
// anchor whose indentation differs from the file still matches, and the
// replacement is re-indented to the file's real style. This is the single most
// common small-model edit failure.
func TestEditToleratesModelIndentation(t *testing.T) {
	dir := tempProject(t, map[string]string{"add.py": "def add(a, b):\n\treturn a - b\n"}) // file uses a tab
	reg := tools.NewRegistry(nil)
	e := env(dir)
	reg.Dispatch(call(t, "read_file", map[string]any{"path": "add.py"}), reg.Names(), tools.ModeAuto, e, nil)

	out, _ := reg.Dispatch(call(t, "edit_file", map[string]any{
		"path": "add.py",
		"edits": []map[string]string{
			{"old_text": "    return a - b", "new_text": "    return a + b"}, // model used four spaces
		},
	}), reg.Names(), tools.ModeAuto, e, nil)

	if errorOf(t, out) != "" {
		t.Fatalf("flexible anchor did not match: %s", out)
	}
	if got := body(t, dir, "add.py"); !strings.Contains(got, "\treturn a + b") {
		t.Fatalf("replacement was not re-indented to the file's tabs: %q", got)
	}
}

// TestWriteFileRepairsOverEscapedJSX checks that a model writing the literal
// escape sequences for < > & into a JS/TS file gets real characters on disk,
// while a non-source file is passed through untouched.
func TestWriteFileRepairsOverEscapedJSX(t *testing.T) {
	dir := tempProject(t, nil)
	reg := tools.NewRegistry(nil)
	e := env(dir)
	broken := `export const App = () => <div>hi & bye</div>;`

	reg.Dispatch(call(t, "write_file", map[string]any{"path": "App.jsx", "content": broken}), reg.Names(), tools.ModeAuto, e, nil)
	got := body(t, dir, "App.jsx")
	if strings.Contains(got, `\u00`) {
		t.Fatalf("escape sequences survived into JSX source: %q", got)
	}
	if !strings.Contains(got, "<div>hi & bye</div>") {
		t.Fatalf("JSX was not decoded correctly: %q", got)
	}

	reg.Dispatch(call(t, "write_file", map[string]any{"path": "notes.txt", "content": broken}), reg.Names(), tools.ModeAuto, e, nil)
	if got := body(t, dir, "notes.txt"); got != broken {
		t.Fatalf("a non-source file must pass through unchanged, got %q", got)
	}
}

// TestReadFileRangeAndTruncation checks the line-range read and the large-file
// cap, which is what keeps a single read from blowing the context window.
func TestReadFileRangeAndTruncation(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 200; i++ {
		b.WriteString("line ")
		b.WriteString(strings.Repeat("x", 20))
		b.WriteString("\n")
	}
	huge := strings.Repeat("y", 250_000)
	dir := tempProject(t, map[string]string{"long.txt": b.String(), "huge.txt": huge})
	reg := tools.NewRegistry(nil)
	e := env(dir)

	out, _ := reg.Dispatch(call(t, "read_file", map[string]any{"path": "long.txt", "start_line": 3, "end_line": 5}), reg.Names(), tools.ModeAuto, e, nil)
	res := decode(t, out)
	if n := strings.Count(res["content"].(string), "\n") + 1; n != 3 {
		t.Fatalf("line range returned %d lines, want 3", n)
	}
	if res["truncated"] != false {
		t.Fatal("a small range should not be marked truncated")
	}

	out, _ = reg.Dispatch(call(t, "read_file", map[string]any{"path": "huge.txt"}), reg.Names(), tools.ModeAuto, e, nil)
	res = decode(t, out)
	if res["truncated"] != true {
		t.Fatal("a 250KB file should come back truncated")
	}
	if len(res["content"].(string)) >= len(huge) {
		t.Fatal("truncated read returned the whole file")
	}
}

// TestListDirSkipsBuildOutput checks discovery does not drown the model in
// vendored and generated directories.
func TestListDirSkipsBuildOutput(t *testing.T) {
	dir := tempProject(t, map[string]string{
		"src/app.py":               "x\n",
		"node_modules/left/pad.js": "x\n",
		".git/config":              "x\n",
		"__pycache__/app.pyc":      "x\n",
		"dist/bundle.js":           "x\n",
	})
	reg := tools.NewRegistry(nil)

	out, _ := reg.Dispatch(call(t, "list_dir", map[string]any{"path": ".", "depth": 3}), reg.Names(), tools.ModeAuto, env(dir), nil)
	listing := out
	for _, noisy := range []string{"node_modules", ".git", "__pycache__", "dist"} {
		if strings.Contains(listing, noisy) {
			t.Errorf("list_dir surfaced %q", noisy)
		}
	}
	if !strings.Contains(listing, "app.py") {
		t.Errorf("list_dir lost real source files: %s", listing)
	}
}

// TestSearchFallsBackToLiteral checks that a query which is not a valid regex is
// retried as a literal string instead of failing the tool call.
func TestSearchFallsBackToLiteral(t *testing.T) {
	dir := tempProject(t, map[string]string{"a.py": "total = sum(values[0])\n"})
	reg := tools.NewRegistry(nil)

	// "values[0" is an unclosed character class, so ripgrep rejects it as a regex
	// (exit 2); the harness must retry it as a plain literal string.
	out, _ := reg.Dispatch(call(t, "search", map[string]any{"query": "values[0"}), reg.Names(), tools.ModeAuto, env(dir), nil)
	if errorOf(t, out) != "" {
		t.Fatalf("search errored: %s", out)
	}
	if !strings.Contains(out, "a.py") {
		t.Fatalf("literal fallback found nothing: %s", out)
	}
}
