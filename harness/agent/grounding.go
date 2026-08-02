package agent

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Grounding pins the exact target(s) a create/edit/fix request named, so the
// loop can verify they were actually changed and refuse a false "done".
type Grounding struct {
	Action          string   `json:"action"`
	ExplicitTargets []string `json:"explicit_targets"`
}

func (g *Grounding) Targets() []string {
	if g == nil {
		return nil
	}
	return g.ExplicitTargets
}

func (g *Grounding) IsCoding() bool {
	if g == nil {
		return false
	}
	switch g.Action {
	case "create", "edit", "fix":
		return true
	}
	return false
}

func (g *Grounding) RequiresMutation() bool {
	return g.IsCoding() && len(g.Targets()) > 0
}

func normTarget(p string) string {
	return filepath.ToSlash(filepath.Clean(strings.TrimSpace(p)))
}

// targetHit reports whether a mutated path matches a named target (exact, or by
// basename for a single-segment target).
func targetHit(targets []string, path string) (string, bool) {
	np := normTarget(path)
	base := filepath.Base(np)
	for _, t := range targets {
		nt := normTarget(t)
		if nt == np {
			return nt, true
		}
		if !strings.Contains(nt, "/") && nt == base {
			return nt, true
		}
	}
	return "", false
}

func missingTargets(targets []string, mutated map[string]bool) []string {
	var missing []string
	for _, t := range targets {
		if !mutated[normTarget(t)] {
			missing = append(missing, t)
		}
	}
	return missing
}

func driftMsg(path string, targets []string) string {
	return fmt.Sprintf("STOP — you just modified %q, which is NOT the file this task named (%s). "+
		"You have drifted onto a different task. Go back to the named target now: read it and make "+
		"the change there. Do not edit any other file.", path, strings.Join(targets, ", "))
}

func groundingMissMsg(missing []string) string {
	j := strings.Join(missing, ", ")
	return fmt.Sprintf("You have NOT modified the named target file(s): %s. A create/edit/fix task is not "+
		"complete until those exact files are changed in place. Do it now: read %s, apply the change with "+
		"write_file/edit_file, and do not create or touch any other file. Do not claim success until %s is "+
		"actually changed.", j, j, j)
}

const falseDoneMsg = "You replied as if the task is finished, but you have NOT created or edited any file " +
	"this run. A create/edit/fix task requires an actual write_file or edit_file. Perform the change now on " +
	"the named target, or say explicitly what is blocking you — do not claim you did something you did not do."
