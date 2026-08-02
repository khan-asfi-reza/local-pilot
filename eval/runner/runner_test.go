package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestManifestDefaults(t *testing.T) {
	m := Manifest{}
	if m.threshold() != 0.80 {
		t.Errorf("default threshold = %v, want 0.80", m.threshold())
	}
	if m.retries() != 2 {
		t.Errorf("default retries = %v, want 2", m.retries())
	}
	if weightOf(Check{}) != 1 {
		t.Errorf("default weight = %v, want 1", weightOf(Check{}))
	}
	if weightOf(Check{Weight: 3}) != 3 {
		t.Errorf("explicit weight lost")
	}
}

func TestMutatedSetExcludesToolState(t *testing.T) {
	sb := t.TempDir()
	// seed a baseline file and commit it
	os.WriteFile(filepath.Join(sb, "seed.txt"), []byte("original\n"), 0o644)
	gitInitBaseline(sb)

	// the agent creates one file, modifies the seed, and writes tool state
	os.WriteFile(filepath.Join(sb, "new.txt"), []byte("added\n"), 0o644)
	os.WriteFile(filepath.Join(sb, "seed.txt"), []byte("changed\n"), 0o644)
	os.MkdirAll(filepath.Join(sb, ".pilot"), 0o755)
	os.WriteFile(filepath.Join(sb, ".pilot", "memory.md"), []byte("## Summary\n"), 0o644)

	mutated := mutatedSet(sb)
	if !mutated["new.txt"] || !mutated["seed.txt"] {
		t.Errorf("expected new.txt and seed.txt mutated, got %v", mutated)
	}
	for p := range mutated {
		if p == ".pilot/memory.md" || p == ".pilot" {
			t.Errorf("tool state must be excluded from mutated set, got %q", p)
		}
	}
}

func TestExecChecks(t *testing.T) {
	ctx := context.Background()
	sb := t.TempDir()
	os.WriteFile(filepath.Join(sb, "area.c"), []byte("int main(void){return 0;}\n"), 0o644)
	gitInitBaseline(sb)
	os.WriteFile(filepath.Join(sb, "area.c"), []byte("int main(void){return 1;}\n"), 0o644)
	mutated := mutatedSet(sb)

	// file_exists
	if r := execCheck(ctx, sb, Check{Type: "file_exists", Path: "area.c"}, mutated); !r.Passed {
		t.Error("file_exists area.c should pass")
	}
	if r := execCheck(ctx, sb, Check{Type: "file_absent", Path: "ghost.c"}, mutated); !r.Passed {
		t.Error("file_absent ghost.c should pass")
	}
	// mutated_only: only area.c allowed → passes
	if r := execCheck(ctx, sb, Check{Type: "mutated_only", Allow: []string{"area.c"}}, mutated); !r.Passed {
		t.Errorf("mutated_only [area.c] should pass, observed=%q", r.Observed)
	}
	// mutated_only: allowing something else → area.c is extra → fails
	if r := execCheck(ctx, sb, Check{Type: "mutated_only", Allow: []string{"other.c"}}, mutated); r.Passed {
		t.Error("mutated_only [other.c] should fail because area.c changed")
	}
	// cmd with stdout match
	if r := execCheck(ctx, sb, Check{Type: "cmd", Run: "echo hello", StdoutContains: "hello"}, mutated); !r.Passed {
		t.Error("cmd echo should pass")
	}
	// cmd expecting nonzero via negation
	if r := execCheck(ctx, sb, Check{Type: "cmd", Run: "! grep -q zzz area.c"}, mutated); !r.Passed {
		t.Error("negated grep should pass when pattern absent")
	}
}

func TestScoreAttemptHardFailZeroes(t *testing.T) {
	ctx := context.Background()
	sb := t.TempDir()
	os.WriteFile(filepath.Join(sb, "decoy.txt"), []byte("keep\n"), 0o644)
	gitInitBaseline(sb)
	// agent wrongly modifies the decoy
	os.WriteFile(filepath.Join(sb, "decoy.txt"), []byte("tampered\n"), 0o644)

	m := Manifest{
		Name:          "t",
		PassThreshold: 0.5,
		Checks: []Check{
			{ID: "ok", Type: "cmd", Run: "true", Weight: 5},
			{ID: "decoy", Type: "file_unchanged", Path: "decoy.txt", Weight: 1, Hard: true},
		},
	}
	_, score, passed := scoreAttempt(ctx, m, sb)
	if score != 0 || passed {
		t.Errorf("a failed hard check must zero the attempt, got score=%v passed=%v", score, passed)
	}
}
