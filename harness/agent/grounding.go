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

// maxGroundingStall is how many consecutive finish attempts may write no NEW
// target before the loop gives up. As long as each round produces a fresh file,
// the gate keeps going, so a many-file task completes one write at a time.
const maxGroundingStall = 4

// groundingMissMsg nudges toward the next single missing file. A degrading small
// model complies with one named write + "tool call only" far better than "write
// all of them", which it tends to answer with prose and no tool call at all.
func groundingMissMsg(missing []string) string {
	next := missing[0]
	tail := ""
	if len(missing) > 1 {
		tail = fmt.Sprintf(" (%d files still needed overall: %s)", len(missing), strings.Join(missing, ", "))
	}
	return fmt.Sprintf("Not finished — required files are still missing%s. Create them ONE AT A TIME. Your very "+
		"next message must be a single write_file tool call for exactly this file: %s, with its COMPLETE content "+
		"in the arguments. It does not exist yet, so do NOT read_file or list_dir it; write_file creates parent "+
		"folders. Reply with the tool call ALONE — no prose, no explanation, no code pasted in the message body.", tail, next)
}

func forceWriteMsg(missing []string) string {
	j := strings.Join(missing, ", ")
	return fmt.Sprintf("STOP exploring — you have used tools but written nothing. Your target files do not exist "+
		"yet; that is expected on a fresh task, so list_dir and read_file on them only error. Your NEXT action MUST "+
		"be write_file, creating one of these files with its COMPLETE content: %s. write_file makes any missing "+
		"parent folders — do not list or read first, just write each file.", j)
}

const falseDoneMsg = "You replied as if the task is finished, but you have NOT created or edited any file " +
	"this run. A create/edit/fix task requires an actual write_file or edit_file. Perform the change now on " +
	"the named target, or say explicitly what is blocking you — do not claim you did something you did not do."
