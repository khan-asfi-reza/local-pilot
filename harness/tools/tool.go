// Package tools defines the harness tool set, how tools are dispatched, and the
// safety and mode gates that decide whether a tool may run.
package tools

import (
	"context"
	"encoding/json"

	"harness/harness/events"
)

// Env is the ambient context a tool runs in. It carries the working directory,
// the skill bodies for load_skill, and the set of files seen (read or written)
// this turn so the harness can force read-before-modify.
type Env struct {
	Ctx        context.Context
	WorkDir    string
	Skills     map[string]string
	SkillNames []string
	Seen       map[string]bool
	Procs      *ProcSet
}

// Args is a decoded tool-call argument object with typed getters.
type Args map[string]any

// Str returns a string argument, or empty if missing or not a string.
func (a Args) Str(k string) string {
	if v, ok := a[k].(string); ok {
		return v
	}
	return ""
}

// StrOr returns a string argument, or a default if missing.
func (a Args) StrOr(k, def string) string {
	if v, ok := a[k].(string); ok && v != "" {
		return v
	}
	return def
}

// Int returns an integer argument, or a default. JSON numbers decode to float64,
// so this handles that.
func (a Args) Int(k string, def int) int {
	switch v := a[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return def
}

// Tool is one capability the model can call. Every tool has the same outside
// shape so dispatch can treat them all equally.
type Tool struct {
	Name        string
	Description string
	Params      json.RawMessage
	// Mutating marks a tool that changes the filesystem or runs a command, which
	// the mode gate treats specially.
	Mutating bool
	// MutatingWhen, when set, decides per call whether this invocation mutates,
	// for tools that only sometimes change things (write_code writes only when
	// given a path). It overrides Mutating for the gate.
	MutatingWhen func(args Args) bool
	// WebSafe marks a tool the backend web path is allowed to pass.
	WebSafe bool
	// EscapesSandbox reports whether this call operates outside the working dir.
	EscapesSandbox func(env Env, args Args) bool
	// Preview describes a mutating action before it runs, for the ask-mode
	// confirmation. It must not change anything. Edits also return the diff.
	Preview func(env Env, args Args) (summary string, diff *events.Diff, err error)
	// Run executes the tool and returns a JSON-serializable result, an optional
	// diff to render, and an error.
	Run func(env Env, args Args) (any, *events.Diff, error)
}

// Decision is the user's answer to an ask-mode confirmation.
type Decision int

const (
	// Decline means do not run the action.
	Decline Decision = iota
	// Approve means run it this once.
	Approve
	// ApproveAlways means run it and stop asking for this kind of action.
	ApproveAlways
)

// ConfirmFunc asks the client to approve a mutating action. It returns the
// decision and, when the user chose to redirect the agent instead of a plain
// reject, a feedback string to hand back to the model.
type ConfirmFunc func(tool, summary string, diff *events.Diff) (Decision, string)
