package orchestrator

import (
	"context"
	"strings"

	"harness/harness/events"
	"harness/harness/lang"
)

// scaffold deterministically initializes a known framework by running its REAL
// generator (or laying templates) and installing its libraries via the language
// handlers — never calling the model. It mirrors provision() (provision.go:97):
// idempotent, event-emitting, and it returns the compact state text on success
// or "" so the caller falls back to the LLM initialize() (state.go:151). The
// language handler cleans up any partial output before returning an error, so a
// "" return never leaves a half-scaffolded directory.
func (o *Orchestrator) scaffold(ctx context.Context, prompt, workDir, env string, emit func(events.Event)) string {
	// Idempotent: a manifest/entrypoint already on disk means we (or a prior run)
	// already scaffolded — same guard the caller uses (state.go:112).
	if detectInitialized(workDir) {
		return loadStateText(workDir)
	}
	h, framework, score := lang.Detect(prompt, workDir)
	if h == nil || score < 0 {
		emit(events.Text("\n[no deterministic template matched this stack; using the model to scaffold]\n"))
		return "" // no deterministic recipe → LLM initialize()
	}
	emit(events.Text("\n[scaffolding " + framework + " deterministically with its real generator]\n"))
	res, err := h.Scaffold(ctx, lang.Req{
		Framework: framework, WorkDir: workDir, Prompt: prompt, Env: env, Emit: emit,
	})
	if err != nil {
		emit(events.Text("\n[scaffold " + framework + " failed (" + err.Error() + "); using LLM scaffold]\n"))
		return ""
	}
	st := State{Initialized: true, Stack: res.Stack, Project: res.Project, App: res.App,
		Settings: res.Settings, Entry: res.Entry, Layout: res.Layout}
	saveState(workDir, st)
	emit(events.Text("\n[scaffolded " + res.Stack + " — " + strings.Join(res.Layout, ", ") + "]\n"))
	return st.Render()
}
