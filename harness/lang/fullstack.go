package lang

import (
	"context"
	"os"
	"path/filepath"

	"harness/harness/events"
)

// ScaffoldFullstack scaffolds a multi-service app: the backend into backend/ and
// the frontend into frontend/, each with its own real generator. It returns one
// combined Result describing the monorepo layout, so downstream feature tasks
// build into the right subdirectory. A single-service (monolith) app does NOT go
// through here — DetectStack keeps it at the root (src/).
func ScaffoldFullstack(ctx context.Context, base Req, plan StackPlan) (Result, error) {
	emit := base.Emit
	if emit == nil {
		emit = func(events.Event) {}
	}
	emit(events.Text("\n[full-stack app detected — scaffolding backend/ and frontend/ separately]\n"))

	beDir := filepath.Join(base.WorkDir, "backend")
	feDir := filepath.Join(base.WorkDir, "frontend")

	beH := HandlerForFramework(plan.Backend)
	feH := HandlerForFramework(plan.Frontend)
	if beH == nil || feH == nil {
		return Result{}, os.ErrInvalid
	}

	if err := os.MkdirAll(beDir, 0o755); err != nil {
		return Result{}, err
	}
	emit(events.Text("\n[scaffolding backend/ — " + plan.Backend + "]\n"))
	beRes, err := beH.Scaffold(ctx, Req{Framework: plan.Backend, WorkDir: beDir, Prompt: base.Prompt, Env: base.Env, Emit: emit})
	if err != nil {
		return Result{}, err
	}

	if err := os.MkdirAll(feDir, 0o755); err != nil {
		return Result{}, err
	}
	emit(events.Text("\n[scaffolding frontend/ — " + plan.Frontend + "]\n"))
	feRes, err := feH.Scaffold(ctx, Req{Framework: plan.Frontend, WorkDir: feDir, Prompt: base.Prompt, Env: base.Env, Emit: emit})
	if err != nil {
		return Result{}, err
	}

	var layout []string
	for _, f := range beRes.Layout {
		layout = append(layout, "backend/"+f)
	}
	for _, f := range feRes.Layout {
		layout = append(layout, "frontend/"+f)
	}
	return Result{
		Lang:      beRes.Lang + "+" + feRes.Lang,
		Framework: "fullstack",
		Stack:     "monorepo — backend/ (" + beRes.Stack + "), frontend/ (" + feRes.Stack + ")",
		Project:   "backend + frontend",
		Entry:     "backend: cd backend && " + beRes.Entry + "  |  frontend: cd frontend && " + feRes.Entry,
		Layout:    layout,
	}, nil
}
