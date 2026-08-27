package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

// TestClassifyApp checks that frontend vs backend is decided by package.json
// content, so a Next.js monolith is not misread as an Express backend and a
// routerless client app is routed to the frontend build path.
func TestClassifyApp(t *testing.T) {
	cases := []struct {
		name     string
		files    map[string]string
		wantKind string
		wantFW   string
	}{
		{"vite-react-game", map[string]string{
			"package.json": `{"dependencies":{"three":"latest","react":"latest"},"devDependencies":{"vite":"latest","@vitejs/plugin-react":"latest"}}`,
			"index.html":   "<div id=root>",
		}, "frontend", "react"},
		{"nextjs-monolith", map[string]string{
			"package.json": `{"dependencies":{"next":"latest","react":"latest"}}`,
		}, "frontend", "next"},
		{"express-backend", map[string]string{
			"package.json": `{"dependencies":{"express":"latest","pg":"latest"}}`,
		}, "backend", "express"},
		{"fastify-backend", map[string]string{
			"package.json": `{"dependencies":{"fastify":"latest"}}`,
		}, "backend", "fastify"},
		{"vue-frontend", map[string]string{
			"package.json": `{"dependencies":{"vue":"latest"},"devDependencies":{"vite":"latest"}}`,
		}, "frontend", "vue"},
		{"sveltekit-frontend", map[string]string{
			"package.json": `{"devDependencies":{"@sveltejs/kit":"latest","svelte":"latest","vite":"latest"}}`,
		}, "frontend", "svelte"},
		{"fastapi-backend", map[string]string{
			"requirements.txt": "fastapi\nuvicorn[standard]\n",
		}, "backend", "fastapi"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, body := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			app, ok := classifyApp(dir, "app")
			if !ok {
				t.Fatalf("classifyApp returned ok=false")
			}
			if app.kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", app.kind, tc.wantKind)
			}
			if app.framework != tc.wantFW {
				t.Errorf("framework = %q, want %q", app.framework, tc.wantFW)
			}
		})
	}
}

// TestFrontendStaticIssuesGating confirms the route/page-defect scan never fires on
// a routerless app (a canvas game) and still catches a stub route in a router app.
func TestFrontendStaticIssuesGating(t *testing.T) {
	// Routerless canvas game: has three + a navigate-like string, no react-router.
	game := t.TempDir()
	writeFiles(t, game, map[string]string{
		"package.json":     `{"dependencies":{"three":"latest","react":"latest"}}`,
		"src/App.tsx":      "export function App(){ window.location.href='/win'; return <canvas/> }",
	})
	if issues := frontendStaticIssues(game); len(issues) != 0 {
		t.Errorf("routerless app: got issues %v, want none", issues)
	}

	// Router app with a genuine stub route (element is a bare <Link>).
	router := t.TempDir()
	writeFiles(t, router, map[string]string{
		"package.json": `{"dependencies":{"react-router-dom":"latest","react":"latest"}}`,
		"src/App.tsx": `import { Routes, Route, Link } from 'react-router-dom'
export function App(){ return <Routes>
  <Route path="/" element={<Home/>} />
  <Route path="/about" element={<Link to="/">home</Link>} />
</Routes> }`,
	})
	issues := frontendStaticIssues(router)
	if len(issues) == 0 {
		t.Errorf("router app with a stub route: got no issues, want the /about stub flagged")
	}
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
