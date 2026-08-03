package orchestrator

import (
	"context"
	"path/filepath"
	"sort"
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
	// Provision backing services up front on ANY build lacking a .env — the LLM (or
	// a spec scan) picks the infra, docker starts it, .env is written — BEFORE the
	// slow scaffold/build work, so the scaffold and every feature task read DB/cache
	// config from .env and infra never sits gated behind init. Idempotent: it
	// no-ops when a .env already exists, so it is safe on a re-run too.
	env := o.provision(ctx, prompt, spec.WorkDir, emit)

	var stateText string
	if !detectInitialized(spec.WorkDir) {
		// Try the deterministic language handlers first (run the real generator +
		// install libraries, no tokens). Fall back to the LLM scaffold when no
		// handler matches, its toolchain is missing, or the generator fails.
		stateText = o.scaffold(ctx, prompt, spec.WorkDir, env, emit)
		if stateText == "" {
			stateText = o.initialize(ctx, prompt, contract, spec, emit, confirm)
		}
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
	if env != "" {
		stateText += "\n\nBACKING SERVICES (provisioned via docker — read ALL db/cache config from the .env file, never hardcode ports/hosts):\n" + env
	}

	plan, err := Decompose(ctx, o.planner, contract, Outline(sections), stateText)
	if err != nil || len(plan.Tasks) == 0 {
		// NEVER collapse to a single whole-PRD agent. When the planner call fails
		// (or a client blip cancels it), build a deterministic plan straight from
		// the spec sections so the work still fans out into parallel sub-tasks.
		if err != nil {
			emit(events.Text("\n[decompose planner failed (" + err.Error() + "); building from spec sections]\n"))
		} else {
			emit(events.Text("\n[planner returned no tasks; building from spec sections]\n"))
		}
		plan = sectionPlan(sections, contract)
	}
	emit(events.Text("\n[decomposed into " + itoa(len(plan.Tasks)) + " sub-tasks]\n"))

	dag, err := NewDAG(plan)
	if err != nil {
		// A bad model-produced DAG (cycle/dupe) must not collapse to one task
		// either: rebuild deterministically from sections (no deps → valid DAG).
		emit(events.Text("\n[dependency problem (" + err.Error() + "); rebuilding from spec sections]\n"))
		plan = sectionPlan(sections, contract)
		if dag, err = NewDAG(plan); err != nil {
			return o.runSequential(ctx, plan, sections, *contract, stateText, spec, emit, confirm)
		}
	}

	mem := NewMemory()
	results := Schedule(ctx, dag, mem, o.pol, o.childExec(dag, sections, stateText, spec, emit, confirm), emit)
	return o.finish(*contract, results, spec, emit)
}

func (o *Orchestrator) childExec(dag *DAG, sections []Section, stateText string, spec ChildSpec, emit func(events.Event), confirm tools.ConfirmFunc) ExecFunc {
	return func(ctx context.Context, t SubTask, mem *Memory) Result {
		existing := o.existingTargets(spec.WorkDir, t)
		prompt := buildChildPrompt(t, sections, stateText, mem.Digest(dag.TransitiveDeps(t.ID)), existing)
		return o.runAndVerify(ctx, t, prompt, spec, emit, confirm)
	}
}

// runAndVerify runs one child and decides pass/fail. A task with declared target
// files is verified structurally (those exact paths must exist); a target-less
// task — the deterministic section plan — is judged by whatever files the child
// actually wrote, captured live from its tool_result diffs. Either way a task
// that produced SOME files is accepted (partial) so the build keeps flowing.
func (o *Orchestrator) runAndVerify(ctx context.Context, t SubTask, prompt string, spec ChildSpec, emit func(events.Event), confirm tools.ConfirmFunc) Result {
	wrote := map[string]bool{}
	tagged := tagEmit(emit, t.ID)
	emitChild := func(ev events.Event) {
		if ev.Type == "tool_result" && ev.Diff != nil && ev.Diff.Path != "" {
			wrote[ev.Diff.Path] = true
		}
		tagged(ev)
	}
	o.exec.RunChild(ctx, prompt, childSpecFor(spec, t), emitChild, confirm)
	written, missing := verifyStructural(o.fc, spec.WorkDir, t)
	if len(t.TargetFiles) == 0 {
		written, missing = sortedKeys(wrote), nil
	}
	if len(written) == 0 {
		summary := "produced no files"
		if len(missing) > 0 {
			summary = "produced no files (targets: " + strings.Join(missing, ", ") + ")"
		}
		return Result{Task: t, Status: StatusFailed, Written: written, Missing: missing, Summary: summary}
	}
	return Result{Task: t, Status: StatusDone, Written: written, Missing: missing, Summary: childSummary(t, written)}
}

func (o *Orchestrator) runSequential(ctx context.Context, plan Plan, sections []Section, contract Contract, stateText string, spec ChildSpec, emit func(events.Event), confirm tools.ConfirmFunc) Summary {
	mem := NewMemory()
	results := map[string]Result{}
	for _, t := range plan.Tasks {
		existing := o.existingTargets(spec.WorkDir, t)
		prompt := buildChildPrompt(t, sections, stateText, mem.Digest(t.Deps), existing)
		emit(events.Text("\n▸ " + t.ID + ": " + t.Title + "\n"))
		r := o.runAndVerify(ctx, t, prompt, spec, emit, confirm)
		results[t.ID] = r
		if r.Status == StatusDone {
			mem.Add(Note{TaskID: t.ID, Title: t.Title, Files: r.Written, Summary: r.Summary})
		}
	}
	return o.finish(contract, results, spec, emit)
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

// childSpecFor derives a per-task child spec: pins its target files (completion
// gate) and injects the frontend-ui skill for UI tasks.
func childSpecFor(base ChildSpec, t SubTask) ChildSpec {
	cs := base
	cs.Targets = t.TargetFiles
	if isFrontendTask(t) {
		cs.InjectSkills = append(append([]string(nil), base.InjectSkills...), "frontend-ui")
	}
	return cs
}

func isFrontendTask(t SubTask) bool {
	for _, f := range t.TargetFiles {
		switch strings.ToLower(filepath.Ext(f)) {
		case ".tsx", ".jsx", ".vue", ".svelte", ".astro", ".html", ".css", ".scss":
			return true
		}
	}
	low := strings.ToLower(t.Title + " " + t.Description)
	for _, kw := range []string{"frontend", " ui", "component", "page", "screen", "dashboard", "layout"} {
		if strings.Contains(low, kw) {
			return true
		}
	}
	return false
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
	b.WriteString("\nHow to work: this is a focused build task on a fresh, mostly-empty project — there is nothing to " +
		"explore. Do NOT waste steps listing directories, searching, or reading files that don't exist. Go straight " +
		"to writing: create each target file directly and completely with write_file at the EXACT path given. " +
		"Do NOT run project generators or scaffolders — they nest folders and break the layout. Match every module " +
		"name, import path, and symbol to the components listed above so your files wire together. " +
		"In any manifest, list dependencies by NAME ONLY with no version number (package.json: use the \"latest\" " +
		"tag) so the package manager installs the current stable; add a version only if the spec asks. " +
		"Read all config (DB host/port, secrets) from the .env file / environment variables — never hardcode ports " +
		"or credentials. Only create or modify YOUR target files — never delete or overwrite others.")
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

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func tagEmit(emit func(events.Event), id string) func(events.Event) {
	return func(ev events.Event) {
		// A child's own completion/error MUST NOT reach the top-level stream. A web
		// client treats the first "done" (or "error") as the whole run ending and
		// disconnects, which cancels the harness request context and kills every
		// remaining sub-task ("✗ … canceled"). The orchestrator emits the single real
		// "done" in finish(); per-task failures are reported there and as ✓/✗ text.
		if ev.Type == "done" || ev.Type == "error" {
			return
		}
		if ev.Type == "tool_call" || ev.Type == "tool_result" {
			ev.Info = "[" + id + "] " + ev.Info
		}
		emit(ev)
	}
}
