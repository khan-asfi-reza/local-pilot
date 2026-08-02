package orchestrator

import (
	"context"
	"strings"
	"sync"

	"harness/harness/events"
	"harness/harness/tools"
)

type Orchestrator struct {
	planner Planner
	exec    Executor
	fc      FileChecker
	pol     Policy
}

func New(p Planner, e Executor, fc FileChecker, pol Policy) *Orchestrator {
	if fc == nil {
		fc = OSFiles{}
	}
	return &Orchestrator{planner: p, exec: e, fc: fc, pol: pol}
}

// Execute runs the top-down pipeline (intake → enrich → chunk → decompose →
// schedule → verify) and returns a summary of what was built.
func (o *Orchestrator) Execute(ctx context.Context, prompt string, contract *Contract, spec ChildSpec, emit func(events.Event), confirm tools.ConfirmFunc) Summary {
	emit = serialize(emit)

	// A big-document caller passes nil to skip the slow intake call; decomposition
	// derives its own targets from the PRD sections.
	if contract == nil {
		contract = &Contract{Action: "create"}
	}

	// Reached only for big documents (the agent decides deterministically by
	// document size), so always decompose — never trust the model's size
	// self-assessment.
	sections := SplitSections(prompt)

	// Up-front init phase: scaffold once so feature sub-tasks run in parallel and
	// wire against consistent canonical names (removes the mega foundational task).
	var stateText string
	if !detectInitialized(spec.WorkDir) {
		stateText = o.initialize(ctx, prompt, contract, spec, emit, confirm)
	} else {
		// Existing project: feed decomposition the current state + file list so a
		// follow-up PRD extends it incrementally instead of clobbering it.
		stateText = loadStateText(spec.WorkDir)
		if snap := projectSnapshot(spec.WorkDir); snap != "" {
			if stateText != "" {
				stateText += "\n"
			}
			stateText += "existing files:\n" + snap
		}
	}

	plan, err := Decompose(ctx, o.planner, contract, Outline(sections), stateText)
	if err != nil || len(plan.Tasks) == 0 {
		emit(events.Text("\n[could not decompose; building as one task]\n"))
		o.runSingle(ctx, prompt, spec, emit, confirm)
		return Summary{Contract: *contract}
	}
	emit(events.Text("\n[decomposed into " + itoa(len(plan.Tasks)) + " sub-tasks]\n"))

	dag, err := NewDAG(plan)
	if err != nil {
		emit(events.Text("\n[dependency problem (" + err.Error() + "); running sequentially]\n"))
		return o.runSequential(ctx, plan, sections, *contract, stateText, spec, emit, confirm)
	}

	mem := NewMemory()
	results := Schedule(ctx, dag, mem, o.pol, o.childExec(dag, sections, stateText, spec, emit, confirm), emit)
	return o.finish(*contract, results, spec, emit)
}

func (o *Orchestrator) childExec(dag *DAG, sections []Section, stateText string, spec ChildSpec, emit func(events.Event), confirm tools.ConfirmFunc) ExecFunc {
	return func(ctx context.Context, t SubTask, mem *Memory) Result {
		existing := o.existingTargets(spec.WorkDir, t)
		prompt := buildChildPrompt(t, sections, stateText, mem.Digest(dag.TransitiveDeps(t.ID)), existing)
		o.exec.RunChild(ctx, prompt, spec, tagEmit(emit, t.ID), confirm)
		written, missing := verifyStructural(o.fc, spec.WorkDir, t)
		if len(missing) > 0 {
			return Result{Task: t, Status: StatusFailed, Written: written, Missing: missing,
				Summary: "missing target file(s): " + strings.Join(missing, ", ")}
		}
		return Result{Task: t, Status: StatusDone, Written: written, Summary: childSummary(t, written)}
	}
}

func (o *Orchestrator) runSequential(ctx context.Context, plan Plan, sections []Section, contract Contract, stateText string, spec ChildSpec, emit func(events.Event), confirm tools.ConfirmFunc) Summary {
	mem := NewMemory()
	results := map[string]Result{}
	for _, t := range plan.Tasks {
		existing := o.existingTargets(spec.WorkDir, t)
		prompt := buildChildPrompt(t, sections, stateText, mem.Digest(t.Deps), existing)
		emit(events.Text("\n▸ " + t.ID + ": " + t.Title + "\n"))
		o.exec.RunChild(ctx, prompt, spec, tagEmit(emit, t.ID), confirm)
		written, missing := verifyStructural(o.fc, spec.WorkDir, t)
		if len(missing) > 0 {
			results[t.ID] = Result{Task: t, Status: StatusFailed, Written: written, Missing: missing,
				Summary: "missing: " + strings.Join(missing, ", ")}
			continue
		}
		results[t.ID] = Result{Task: t, Status: StatusDone, Written: written, Summary: childSummary(t, written)}
		mem.Add(Note{TaskID: t.ID, Title: t.Title, Files: written, Summary: childSummary(t, written)})
	}
	return o.finish(contract, results, spec, emit)
}

func (o *Orchestrator) runSingle(ctx context.Context, prompt string, spec ChildSpec, emit func(events.Event), confirm tools.ConfirmFunc) {
	o.exec.RunChild(ctx, prompt, spec, emit, confirm)
}

// finish assembles the summary and sweeps for named targets that were never produced.
func (o *Orchestrator) finish(contract Contract, results map[string]Result, spec ChildSpec, emit func(events.Event)) Summary {
	sum := Summary{Contract: contract, Results: results}
	for id, r := range results {
		switch r.Status {
		case StatusFailed:
			sum.Failed = append(sum.Failed, id)
		case StatusSkipped:
			sum.Skipped = append(sum.Skipped, id)
		}
	}
	var missingTop []string
	for _, tgt := range contract.ExplicitTargets {
		if strings.TrimSpace(tgt) != "" && !o.fc.Exists(spec.WorkDir, tgt) {
			missingTop = append(missingTop, tgt)
		}
	}
	if len(missingTop) > 0 {
		emit(events.Error("grounding: named target(s) not produced: " + strings.Join(missingTop, ", ")))
	}
	emit(events.Text("\n" + sum.Text() + "\n"))
	emit(events.Done())
	return sum
}

func (o *Orchestrator) existingTargets(workDir string, t SubTask) string {
	var have []string
	for _, f := range t.TargetFiles {
		if o.fc.Exists(workDir, strings.TrimSpace(f)) {
			have = append(have, f)
		}
	}
	return strings.Join(have, ", ")
}

func (s Summary) Text() string {
	total := len(s.Results)
	done := total - len(s.Failed) - len(s.Skipped)
	var b strings.Builder
	b.WriteString("Build finished: " + itoa(done) + "/" + itoa(total) + " sub-tasks completed.")
	if len(s.Failed) > 0 {
		b.WriteString(" Failed: " + strings.Join(s.Failed, ", ") + ".")
	}
	if len(s.Skipped) > 0 {
		b.WriteString(" Skipped: " + strings.Join(s.Skipped, ", ") + ".")
	}
	return b.String()
}

func buildChildPrompt(t SubTask, sections []Section, stateText, digest, existing string) string {
	var b strings.Builder
	b.WriteString(t.Description)
	if stateText != "" {
		b.WriteString("\n\nProject state (canonical names & layout — match these EXACTLY):\n" + stateText + "\n")
	}
	if len(t.TargetFiles) > 0 {
		b.WriteString("\n\nFiles you must create or modify (produce EXACTLY these, nothing else):\n")
		for _, f := range t.TargetFiles {
			b.WriteString("- " + f + "\n")
		}
	}
	if len(t.Acceptance) > 0 {
		b.WriteString("\nAcceptance criteria (all must hold):\n")
		for _, a := range t.Acceptance {
			b.WriteString("- " + a + "\n")
		}
	}
	if body := SectionBody(sections, t.SectionIdx); body != "" {
		b.WriteString("\nRelevant spec section:\n" + body + "\n")
	}
	if digest != "" {
		b.WriteString("\nAlready-built components you can rely on (do NOT rebuild them):\n" + digest + "\n")
	}
	if existing != "" {
		b.WriteString("\nThese target files already exist from a previous attempt — create only the MISSING ones:\n" + existing + "\n")
	}
	b.WriteString("\nHow to work: write each target file directly and completely with write_file at the EXACT path given. " +
		"Do NOT run project generators or scaffolders (django-admin startproject/startapp, npx create-react-app, " +
		"npm create vite, etc.) — they create nested folders that break the layout. Match every module name, import " +
		"path, and symbol to the components listed above so your files wire together with them. " +
		"Only create or modify YOUR target files — never delete, rename, or overwrite other existing files, and " +
		"never remove code outside this task.")
	return b.String()
}

func childSummary(t SubTask, written []string) string {
	if len(written) > 0 {
		return t.Title + " — wrote " + strings.Join(written, ", ")
	}
	return t.Title + " done"
}

func serialize(emit func(events.Event)) func(events.Event) {
	var mu sync.Mutex
	return func(ev events.Event) {
		mu.Lock()
		emit(ev)
		mu.Unlock()
	}
}

func tagEmit(emit func(events.Event), id string) func(events.Event) {
	return func(ev events.Event) {
		if ev.Type == "tool_call" || ev.Type == "tool_result" {
			ev.Info = "[" + id + "] " + ev.Info
		}
		emit(ev)
	}
}
