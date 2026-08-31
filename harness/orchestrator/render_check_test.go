package orchestrator

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVisibleTextIgnoresMarkupAndScripts(t *testing.T) {
	dom := `<html><head><style>.a{color:red}</style></head>` +
		`<body><div id="root"><h1>Our Games</h1></div><script>var x = "not visible"</script></body></html>`
	got := visibleText(dom)
	if !strings.Contains(got, "Our Games") {
		t.Errorf("visible heading missing from %q", got)
	}
	if strings.Contains(got, "not visible") {
		t.Errorf("script body leaked into visible text: %q", got)
	}
}

func TestEmptyMountIsDetected(t *testing.T) {
	blank := []string{
		`<body><div id="root"></div><script src="/x.js"></script></body>`,
		`<body><div id="app">   </div></body>`,
		`<body><div id="root" class="h-full"></div></body>`,
	}
	for _, dom := range blank {
		if !emptyMountRe.MatchString(dom) {
			t.Errorf("empty mount not detected in %q", dom)
		}
	}
	if emptyMountRe.MatchString(`<body><div id="root"><nav>Home</nav></div></body>`) {
		t.Error("a populated mount node must not be reported as empty")
	}
}

func TestAppRoutesReadsBothRouterStyles(t *testing.T) {
	jsx := writeApp(t, map[string]string{
		"src/App.tsx": `<Routes><Route path="/" element={<Home />} /><Route path="/about" element={<About />} /></Routes>`,
	})
	if got := appRoutes(jsx); len(got) != 2 || got[0] != "/" || got[1] != "/about" {
		t.Errorf("JSX routes = %v, want [/ /about]", got)
	}

	obj := writeApp(t, map[string]string{
		"src/routes.tsx": `createBrowserRouter([{ path: '/', element: <Layout /> , children: [
			{ path: 'about', element: <AboutPage /> },
			{ path: 'games', element: <GamesPage /> },
			{ path: 'game/:id', element: <Detail /> },
		]}])`,
	})
	got := appRoutes(obj)
	if len(got) != 3 || got[0] != "/" || got[1] != "/about" || got[2] != "/games" {
		t.Errorf("object routes = %v, want [/ /about /games]", got)
	}
	for _, p := range got {
		if strings.Contains(p, ":") {
			t.Errorf("a parameterised route cannot be visited blind: %v", got)
		}
	}
}

func TestStaticSPAServerFallsBackForRoutesButNotAssets(t *testing.T) {
	dist := writeApp(t, map[string]string{
		"index.html":     `<!doctype html><div id="root"></div>`,
		"assets/app.js":  `console.log(1)`,
	})
	base, stop, err := staticSPAServer(dist)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	body, code := get(t, base+"/games")
	if code != 200 || !strings.Contains(body, `id="root"`) {
		t.Errorf("a client route should serve index.html, got %d %q", code, body)
	}
	if _, code := get(t, base+"/assets/app.js"); code != 200 {
		t.Errorf("a real asset should be served, got %d", code)
	}
	// A missing image must 404, not silently return HTML — that is what hides a
	// broken image behind a "200 OK" during a check.
	if _, code := get(t, base+"/assets/games/missing.png"); code != 404 {
		t.Errorf("a missing asset should 404, got %d", code)
	}
}

func get(t *testing.T, url string) (string, int) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body), resp.StatusCode
}

// End to end through a real browser: an app that throws while rendering must be
// reported, and one that renders must not be. Uses a handwritten dist so the test
// needs no npm install.
func TestRenderIssuesCatchesAThrowingApp(t *testing.T) {
	if browserBinary() == "" {
		t.Skip("no Chrome-family browser on this machine")
	}

	broken := writeApp(t, map[string]string{
		"dist/index.html": `<!doctype html><html><body><div id="root"></div>
<script>document.addEventListener('DOMContentLoaded', function () { null.boom; });</script>
</body></html>`,
	})
	issues := renderIssues(context.Background(), nodeApp{dir: broken, label: "app", kind: "frontend"})
	if len(issues) == 0 {
		t.Fatal("an app that throws and renders nothing must be reported")
	}
	if !strings.Contains(issues[0], "blank page") {
		t.Errorf("issue should name the symptom, got %q", issues[0])
	}

	working := writeApp(t, map[string]string{
		"dist/index.html": `<!doctype html><html><body><div id="root"></div>
<script>document.getElementById('root').innerHTML =
  '<nav>Home About Games Store</nav><h1>PixelForge Studios</h1><p>Crafting games since 2015.</p>';</script>
</body></html>`,
	})
	if issues := renderIssues(context.Background(), nodeApp{dir: working, label: "app", kind: "frontend"}); len(issues) != 0 {
		t.Errorf("a page that renders content must pass, got %v", issues)
	}
}

func TestRenderIssuesSkipsWhenThereIsNoBuild(t *testing.T) {
	dir := writeApp(t, map[string]string{"src/App.tsx": "export default () => <div />"})
	if issues := renderIssues(context.Background(), nodeApp{dir: dir, label: "app", kind: "frontend"}); issues != nil {
		t.Errorf("with no dist/ there is nothing to render-check, got %v", issues)
	}
}

func TestBrowserBinaryHonoursTheEnvOverride(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "browser")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PILOT_BROWSER", fake)
	if got := browserBinary(); got != fake {
		t.Errorf("browserBinary() = %q, want %q", got, fake)
	}
}
