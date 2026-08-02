// Package orchestrator turns a large request into a plan of small, independent
// sub-tasks and runs them top-down, so a small model never holds a whole PRD at
// once. It imports only leaf packages (never agent), so the agent drives it
// without a cycle.
package orchestrator

import (
	"context"
	"encoding/json"

	"harness/harness/events"
	"harness/harness/model"
	"harness/harness/tools"
)

type Contract struct {
	Action             string   `json:"action"`
	FileCount          int      `json:"file_count"`
	ExplicitTargets    []string `json:"explicit_targets"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

type Section struct {
	Index int
	Title string
	Body  string
}

type SubTask struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Deps        []string `json:"deps"`
	TargetFiles []string `json:"target_files"`
	Acceptance  []string `json:"acceptance"`
	SectionIdx  int      `json:"section_idx"`
}

type Plan struct {
	Tasks []SubTask `json:"tasks"`
}

type Status string

const (
	StatusPending Status = "pending"
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
)

type Result struct {
	Task     SubTask
	Status   Status
	Attempts int
	Summary  string
	Written  []string
	Missing  []string
}

type Summary struct {
	Contract Contract
	Results  map[string]Result
	Failed   []string
	Skipped  []string
}

type Note struct {
	TaskID  string
	Title   string
	Files   []string
	Summary string
}

type ChildSpec struct {
	WorkDir      string
	Mode         string
	Allowed      []string
	Sandbox      bool
	InjectSkills []string
}

type Policy struct {
	MaxParallel int
	MaxRetries  int
	AbortOnFail bool
}

// Planner runs one stateless, grammar-constrained JSON call on the active model.
type Planner interface {
	PlanJSON(ctx context.Context, system, user string, schema json.RawMessage) (string, error)
}

// Executor runs one sub-task as an isolated child agent run.
type Executor interface {
	RunChild(ctx context.Context, prompt string, spec ChildSpec, emit func(events.Event), confirm tools.ConfirmFunc) []model.Message
}

// FileChecker reports whether a target file exists under the work dir.
type FileChecker interface {
	Exists(workDir, rel string) bool
}
