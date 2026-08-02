package agent

import "testing"

func TestTargetHitExactAndBasename(t *testing.T) {
	targets := []string{"area.c", "backend/main.py"}

	cases := []struct {
		path string
		want string
		hit  bool
	}{
		{"area.c", "area.c", true},                   // exact single-segment
		{"./area.c", "area.c", true},                 // normalized
		{"src/area.c", "area.c", true},               // basename match for single-segment target
		{"backend/main.py", "backend/main.py", true}, // exact nested
		{"main.py", "", false},                       // nested target does NOT match by basename
		{"palindrome_checker.py", "", false},         // the decoy never matches
	}
	for _, c := range cases {
		got, ok := targetHit(targets, c.path)
		if ok != c.hit || got != c.want {
			t.Errorf("targetHit(%q) = (%q,%v), want (%q,%v)", c.path, got, ok, c.want, c.hit)
		}
	}
}

func TestMissingTargets(t *testing.T) {
	targets := []string{"area.c", "util.go"}
	mutated := map[string]bool{"area.c": true}
	missing := missingTargets(targets, mutated)
	if len(missing) != 1 || missing[0] != "util.go" {
		t.Fatalf("missingTargets = %v, want [util.go]", missing)
	}
	if got := missingTargets(targets, map[string]bool{"area.c": true, "util.go": true}); len(got) != 0 {
		t.Fatalf("missingTargets (all mutated) = %v, want []", got)
	}
}

func TestGroundingPredicates(t *testing.T) {
	var nilG *Grounding
	if nilG.RequiresMutation() || nilG.IsCoding() || nilG.Targets() != nil {
		t.Fatal("nil grounding must be inert")
	}
	edit := &Grounding{Action: "edit", ExplicitTargets: []string{"area.c"}}
	if !edit.IsCoding() || !edit.RequiresMutation() {
		t.Fatal("edit with a target must require mutation")
	}
	noTarget := &Grounding{Action: "create", ExplicitTargets: nil}
	if !noTarget.IsCoding() || noTarget.RequiresMutation() {
		t.Fatal("coding action without a named target must not require a specific-target mutation")
	}
	question := &Grounding{Action: "question"}
	if question.IsCoding() || question.RequiresMutation() {
		t.Fatal("question must not be a coding action")
	}
}
