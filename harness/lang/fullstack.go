package lang

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"harness/harness/events"
)

// backendPortFor returns the dev port the evaluator boots each backend stack on, so
// the frontend proxy targets the right one (Node 3001, Python 8000, Go 8080).
func backendPortFor(framework string) string {
	switch framework {
	case "django", "fastapi", "flask":
		return "8000"
	case "go", "gin", "fiber":
		return "8080"
	default: // express, nestjs, fastify, node
		return "3001"
	}
}

// viteProxyConfig is the frontend vite.config for a fullstack app: React plugin, a
// fixed dev-server port (so many apps run at once), plus a /api → backend dev proxy.
// Written verbatim so the frontend always reaches the API same-origin without the
// model configuring anything.
func viteProxyConfig(backendPort, vitePort string) string {
	return `import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The app talks to the backend through this proxy: fetch('/api/...') is forwarded
// to the API on http://localhost:` + backendPort + `. Do NOT hardcode the backend URL in the app.
export default defineConfig({
  plugins: [react()],
  server: {
    port: ` + vitePort + `,
    strictPort: true,
    proxy: {
      '/api': { target: 'http://localhost:` + backendPort + `', changeOrigin: true },
    },
  },
})
`
}

// envValue reads KEY=value from a .env text blob; returns "" if absent.
func envValue(env, key string) string {
	for _, line := range strings.Split(env, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimSpace(strings.TrimPrefix(line, key+"="))
		}
	}
	return ""
}

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

	// Drop the provisioned .env into backend/ too. The backend process runs from
	// backend/, so its dotenv.config() must find the real ports and DB credentials
	// here; without it the model, seeing no .env in its working dir, invents one with
	// wrong defaults (PORT=3001, localhost:5432) that shadows the root .env and points
	// the app at a database that does not exist.
	if base.Env != "" {
		if err := os.WriteFile(filepath.Join(beDir, ".env"), []byte(base.Env), 0o644); err != nil {
			emit(events.Text("\n[warn: could not write backend/.env: " + err.Error() + "]\n"))
		}
	}

	if err := os.MkdirAll(feDir, 0o755); err != nil {
		return Result{}, err
	}
	emit(events.Text("\n[scaffolding frontend/ — " + plan.Frontend + "]\n"))
	feRes, err := feH.Scaffold(ctx, Req{Framework: plan.Frontend, WorkDir: feDir, Prompt: base.Prompt, Env: base.Env, Emit: emit})
	if err != nil {
		return Result{}, err
	}

	// Wire the frontend to the backend deterministically: overwrite the generated
	// vite.config with one that proxies /api → the API on :3001. The app then
	// fetches '/api/...' same-origin — no CORS, no hardcoded backend URL, and no
	// chance for the model to write a broken proxy (the single most common break).
	if plan.Frontend == "react" {
		bePort := envValue(base.Env, "PORT")
		if bePort == "" {
			bePort = backendPortFor(plan.Backend)
		}
		vitePort := envValue(base.Env, "VITE_PORT")
		if vitePort == "" {
			vitePort = "5173"
		}
		if err := os.WriteFile(filepath.Join(feDir, "vite.config.ts"), []byte(viteProxyConfig(bePort, vitePort)), 0o644); err != nil {
			emit(events.Text("\n[warn: could not write vite proxy config: " + err.Error() + "]\n"))
		} else {
			emit(events.Text("\n[wired frontend :" + vitePort + " → backend /api proxy (:" + bePort + ")]\n"))
		}
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
