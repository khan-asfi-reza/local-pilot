package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"harness/harness/events"
	"harness/harness/tools"
)

type Orchestrator struct {
	planner  Planner
	exec     Executor
	fc       FileChecker
	pol      Policy
	evalMu   sync.Mutex   // serializes the evaluator (npm/tsc/boot) across parallel sub-tasks
	contract *APIContract // the API contract both sides build against (set in Execute)
}

func New(p Planner, e Executor, fc FileChecker, pol Policy) *Orchestrator {
	if fc == nil {
		fc = OSFiles{}
	}
	return &Orchestrator{planner: p, exec: e, fc: fc, pol: pol}
}

// EvaluateDir runs the final boot-and-run evaluator (install → migrate → boot →
// probe → repair, plus frontend build/repair) against an already-built project.
// It is the same pass Execute runs at the end, exposed so a broken project can be
// re-evaluated and repaired without a full regeneration.
func (o *Orchestrator) EvaluateDir(ctx context.Context, workDir string, emit func(events.Event), confirm tools.ConfirmFunc) {
	if emit == nil {
		emit = func(events.Event) {}
	}
	o.smokeAndRepair(ctx, workDir, serialize(emit), confirm)
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
	stateText += authScaffoldNote(spec.WorkDir)

	// Contract-first: design the whole REST API up front and make it the single
	// source of truth. The backend implements it exactly; the frontend calls it only
	// through a generated typed client (written into frontend/src/lib/api.ts), so the
	// two sides cannot drift on paths or shapes. Also written as openapi.yaml.
	cAPI, cErr := GenerateContract(ctx, o.planner, prompt)
	if cErr != nil || len(cAPI.Endpoints) == 0 {
		emit(events.Text("\n[contract design skipped (" + errText(cErr) + ") — building without a generated client]\n"))
	}
	if c := cAPI; cErr == nil && len(c.Endpoints) > 0 {
		o.contract = &c
		_ = os.WriteFile(filepath.Join(spec.WorkDir, "openapi.yaml"), []byte(c.openAPIYAML()), 0o644)
		if raw, e := json.MarshalIndent(c, "", "  "); e == nil {
			_ = os.WriteFile(filepath.Join(spec.WorkDir, "apispec.json"), raw, 0o644)
		}
		writeTSClient(spec.WorkDir, c)
		emit(events.Text(fmt.Sprintf("\n[designed API contract: %d endpoints → openapi.yaml + typed client]\n", len(c.Endpoints))))
		stateText += "\n\nAPI CONTRACT — the SINGLE SOURCE OF TRUTH (also in openapi.yaml). The backend MUST implement " +
			"these endpoints EXACTLY (same method, path, request body, and response fields). The typed client " +
			"frontend/src/lib/api.ts is ALREADY GENERATED from this contract — do NOT create a task to write or generate " +
			"the API client; frontend tasks simply IMPORT its functions and NEVER hardcode a " +
			"fetch('/api/...') path or invent an endpoint. Each endpoint below lists the EXACT symbol api.ts exports " +
			"under 'api.ts:' — import those names VERBATIM (function + its Result type). NEVER invent a client name or " +
			"a Result type (no getV1UsersByUsernameResult-style guesses); if a name is not listed here it does not " +
			"exist in api.ts. Endpoints:\n" + c.renderForPrompt()
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

	// Init phase: install every package the plan says its tasks need, up front, so
	// no sub-task hits a missing import mid-build.
	o.installPlanned(ctx, plan, spec.WorkDir, emit)

	mem := NewMemory()
	results := Schedule(ctx, dag, mem, o.pol, o.childExec(dag, sections, stateText, spec, emit, confirm), emit)
	o.smokeAndRepair(ctx, spec.WorkDir, emit, confirm)
	return o.finish(*contract, results, spec, emit)
}

func (o *Orchestrator) childExec(dag *DAG, sections []Section, stateText string, spec ChildSpec, emit func(events.Event), confirm tools.ConfirmFunc) ExecFunc {
	fileMap := planFileMap(dag) // the whole project's file layout, shared with every child
	return func(ctx context.Context, t SubTask, mem *Memory) Result {
		existing := o.existingTargets(spec.WorkDir, t)
		prompt := buildChildPrompt(t, sections, stateText, fileMap, mem.Digest(dag.TransitiveDeps(t.ID)), existing)
		return o.runAndVerify(ctx, t, prompt, spec, emit, confirm)
	}
}

// planFileMap renders every sub-task's target files as a shared project map, so a
// blind child knows the exact paths and names the other tasks produce and wires to
// them instead of inventing parallel files.
func planFileMap(d *DAG) string {
	var tasks []SubTask
	for _, id := range d.TopoIDs() {
		tasks = append(tasks, d.Task(id))
	}
	return projectMap(tasks)
}

// projectMap renders the shared cross-task map every child sees: for each task, the
// files it owns and the public interface it exposes. Independent children cannot
// read each other's code, so this is how a consumer learns the EXACT paths and
// contract strings a producer will publish — the single source of truth that keeps
// them wired together. Domain-agnostic: exposes may be routes, signatures, CLI
// flags, tables, message shapes — whatever couples the pieces.
func projectMap(tasks []SubTask) string {
	var b strings.Builder
	for _, t := range tasks {
		if len(t.TargetFiles) == 0 && len(t.Exposes) == 0 {
			continue
		}
		b.WriteString(t.ID + " — " + t.Title + "\n")
		if len(t.TargetFiles) > 0 {
			b.WriteString("    files: " + strings.Join(t.TargetFiles, ", ") + "\n")
		}
		if len(t.Exposes) > 0 {
			b.WriteString("    exposes: " + strings.Join(t.Exposes, " | ") + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// authScaffoldNote tells decomposition that auth is already built (the scaffold
// dropped in a JWT+password module), so it plans features that USE it instead of a
// redundant auth/users task that would clobber the working module.
func authScaffoldNote(workDir string) string {
	for _, p := range []string{"backend/app/auth.py", "app/auth.py", "backend/auth.py", "auth.py", "backend/src/auth.ts", "src/auth.ts"} {
		if fileExists(filepath.Join(workDir, p)) {
			node := strings.HasSuffix(p, ".ts")
			migDir := "backend/migrations/"
			note := "\n\nAUTHENTICATION IS ALREADY IMPLEMENTED in " + p + " (a users table, register/login/me endpoints, " +
				"and a token guard). Do NOT create a task for authentication, a users table, JWT, or password hashing — they " +
				"exist. Feature tasks must IMPORT and use the provided auth (guard protected routes with it) and reference the " +
				"existing users table for ownership (user_id)."
			if node {
				note += " DATABASE SCHEMA: put every migration as a numbered .sql file in " + migDir + " (e.g. 001_items.sql, " +
					"002_orders.sql) — that is the ONLY directory the migration runner scans; SQL placed anywhere else (e.g. " +
					"src/db/migrations) is NEVER applied and the tables will not exist. Each migration file must CREATE its own " +
					"tables; every column and table your service code queries must exist in a migration here."
			}
			return note
		}
	}
	return ""
}

// planFileMap2 is the same map from a raw plan (the sequential fallback has no DAG).
func planFileMap2(p Plan) string {
	return projectMap(p.Tasks)
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
	// Layer 1 of the evaluator: right after this sub-task's child produced files,
	// install any package it imported and repair a broken edit / bad syntax before
	// the task is judged. Layer 2 (boot + run + repair) is the final smokeAndRepair.
	o.evaluateTask(ctx, t, spec, emit, confirm)
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
	fileMap := planFileMap2(plan)
	for _, t := range plan.Tasks {
		existing := o.existingTargets(spec.WorkDir, t)
		prompt := buildChildPrompt(t, sections, stateText, fileMap, mem.Digest(t.Deps), existing)
		emit(events.Text("\n▸ " + t.ID + ": " + t.Title + "\n"))
		r := o.runAndVerify(ctx, t, prompt, spec, emit, confirm)
		results[t.ID] = r
		if r.Status == StatusDone {
			mem.Add(Note{TaskID: t.ID, Title: t.Title, Files: r.Written, Summary: r.Summary})
		}
	}
	o.smokeAndRepair(ctx, spec.WorkDir, emit, confirm)
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

func buildChildPrompt(t SubTask, sections []Section, stateText, fileMap, digest, existing string) string {
	var b strings.Builder
	b.WriteString(t.Description)
	if stateText != "" {
		b.WriteString("\n\nProject state (canonical names & layout — match these EXACTLY):\n" + stateText + "\n")
	}
	if fileMap != "" {
		b.WriteString("\n\nPROJECT MAP — every sub-task's files and the public interface it exposes. Other sub-tasks " +
			"build what you don't own; you cannot see their code, so this is your ONLY source of truth for how the " +
			"pieces connect. Rules: (1) import from these EXACT file paths and reuse these module/export names — never " +
			"invent a parallel or differently-named version of something listed; (2) when you CONSUME another task's " +
			"interface (call its route, function, table, command, event), use its `exposes` string VERBATIM; (3) when " +
			"YOU build something another task exposes, implement it to match that exact string. Do not deviate from a " +
			"listed contract:\n" +
			fileMap + "\n")
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
		"or credentials. Only create or modify YOUR target files — never delete or overwrite others. " +
		"ROUTES ARE AUTO-MOUNTED (Node/Express): the entrypoint scans backend/src/routes/ and mounts each file at " +
		"/api/<filename>. To add an endpoint group, create backend/src/routes/<resource>.ts that `export default`s an " +
		"Express Router using RELATIVE paths (router.get('/'), router.get('/:id'), router.post('/')) — it goes live " +
		"automatically at /api/<resource>. Do NOT wire it anywhere and do NOT edit the entrypoint. " +
		"DO NOT TOUCH the scaffold-owned files (they are already correct — rewriting them is the #1 cause of a broken " +
		"app): NEVER create, overwrite, or edit backend/src/index.ts, backend/src/db.ts, backend/src/auth.ts, " +
		"backend/src/env.ts, backend/src/migrate.ts, or frontend/vite.config.ts. Use the DB via `import { pool } from " +
		"'../db'` and `pool.query(sql, params)` (node-postgres — params is an ARRAY, e.g. pool.query('SELECT * FROM t " +
		"WHERE id=$1', [id])); use auth via `import { requireAuth } from '../auth'`. The frontend reaches the API by calling " +
		"fetch('/api/...') same-origin; never hardcode a backend host/port in the app. For an app with login, send the " +
		"stored JWT on EVERY request (Authorization: Bearer) — an authed endpoint without the header returns 401. Login " +
		"and registration pages must be SEPARATE simple forms (login = email + password only; do not reuse a signup " +
		"form with a required confirm-password for login) and must actually store the token and load the session. " +
		"IMPORTS & EXPORTS (mismatches crash the browser with 'does not provide an export named X' — follow EXACTLY): " +
		"(1) Use a NAMED export for every component, page, hook, and helper — `export function Foo() {}` or " +
		"`export const foo = ...` — and import it the SAME way: `import { Foo } from './Foo'`. Do NOT use `export " +
		"default` or default imports anywhere. (2) A TYPE or interface — including any `...Result` type from the API " +
		"client — MUST be imported type-only: `import type { FooResult } from '@/lib/api'`. A plain value import of a " +
		"type (`import { FooResult }`) compiles but breaks Vite at runtime. (3) Import a symbol ONLY if the module " +
		"actually exports it under that exact name — never guess or invent an export. " +
		"Your ONLY tools are read_file, write_file, edit_file, list_dir, and search — there is NO shell in this task. " +
		"Do NOT try to run, install, migrate, test, or verify anything (no shell_run, no serve, no commands): those " +
		"tools do not exist here and every attempt just errors and wastes a step. You do NOT need to check your work — " +
		"after these files are written the project is installed, booted, and tested for you automatically, and any " +
		"errors are fixed in a later repair step. So once your target files are written, STOP immediately and reply " +
		"with a one-line summary of what you wrote; do not attempt to confirm it runs.")
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
