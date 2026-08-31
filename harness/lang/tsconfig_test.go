package lang

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelaxTSBuildGateKeepsRealTypeChecking(t *testing.T) {
	dir := t.TempDir()
	// The shape create-vite ships, comments included (so a JSON parse would fail).
	src := `{
  "compilerOptions": {
    /* Linting */
    "strict": true,
    "verbatimModuleSyntax": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "erasableSyntaxOnly": true,
    "noFallthroughCasesInSwitch": true
  }
}`
	path := filepath.Join(dir, "tsconfig.app.json")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	relaxTSBuildGate(dir)

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, off := range []string{"noUnusedLocals", "noUnusedParameters", "verbatimModuleSyntax", "erasableSyntaxOnly"} {
		if !strings.Contains(got, `"`+off+`": false`) {
			t.Errorf("%s should be off, got:\n%s", off, got)
		}
	}
	if !strings.Contains(got, `"strict": true`) {
		t.Error("strict must stay on: it catches real type errors")
	}
	if !strings.Contains(got, `"noFallthroughCasesInSwitch": true`) {
		t.Error("noFallthroughCasesInSwitch catches a real bug and must stay on")
	}
	if !strings.Contains(got, "/* Linting */") {
		t.Error("the rewrite must preserve the file's comments")
	}
}

func TestRelaxTSBuildGateIgnoresProjectsWithoutTSConfig(t *testing.T) {
	relaxTSBuildGate(t.TempDir()) // must not panic or create files
}
