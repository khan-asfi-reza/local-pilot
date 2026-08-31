package orchestrator

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"harness/harness/ports"
)

// The last gate before an app is called finished: load it in a real browser and
// look at the page.
//
// A build check only proves the code compiles. The defects users actually report -
// a white screen, a page with nothing on it - are runtime faults that compile
// perfectly: a provider used outside its context, a null deref during the first
// render, a bad import at module scope. React catches the throw, unmounts the tree,
// and leaves an empty mount node, so `npm run build` passes on an app that shows
// nothing at all. Only rendering it catches that.

var browserCandidates = map[string][]string{
	"darwin": {
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
	},
	"linux":   {"google-chrome", "chromium", "chromium-browser", "microsoft-edge"},
	"windows": {`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`},
}

// browserBinary finds a Chrome-family browser to render with, or "" when the
// machine has none (the render gate is then skipped, never failed).
func browserBinary() string {
	if p := os.Getenv("PILOT_BROWSER"); p != "" && fileExists(p) {
		return p
	}
	for _, c := range browserCandidates[runtime.GOOS] {
		if filepath.IsAbs(c) {
			if fileExists(c) {
				return c
			}
			continue
		}
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}

var (
	emptyMountRe  = regexp.MustCompile(`(?s)<div[^>]*\bid="(?:root|app|__next)"[^>]*>\s*</div>`)
	tagRe         = regexp.MustCompile(`(?s)<(script|style)\b.*?</(?:script|style)>|<[^>]*>`)
	consoleErrRe  = regexp.MustCompile(`(?i):CONSOLE\(\d+\)\]\s*"([^"]{0,300})`)
	objectRouteRe = regexp.MustCompile(`\bpath:\s*['"]([^'"]+)['"]`)
)

// MinRenderedText is the least visible text a working page must produce. A page
// that renders a spinner and nothing else is still a broken page.
const MinRenderedText = 30

// staticSPAServer serves a built frontend the way a host would, falling back to
// index.html so client-side routes resolve. Returns the base URL and a stop func.
func staticSPAServer(dist string) (string, func(), error) {
	port := ports.Free(5400)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return "", nil, err
	}
	files := http.FileServer(http.Dir(dist))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		clean := filepath.Join(dist, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		if r.URL.Path != "/" && fileExists(clean) {
			files.ServeHTTP(w, r)
			return
		}
		if filepath.Ext(r.URL.Path) != "" && r.URL.Path != "/" {
			http.NotFound(w, r) // a real missing asset must 404, not return HTML
			return
		}
		http.ServeFile(w, r, filepath.Join(dist, "index.html"))
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	return fmt.Sprintf("http://127.0.0.1:%d", port), func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}, nil
}

// renderPage returns the DOM after scripts have run, plus any console errors.
func renderPage(ctx context.Context, browser, url string) (string, []string, error) {
	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, browser,
		"--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run",
		"--disable-dev-shm-usage", "--enable-logging=stderr", "--v=0",
		"--virtual-time-budget=8000", "--dump-dom", url)
	var out, errOut strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()

	var consoleErrors []string
	seen := map[string]bool{}
	for _, line := range strings.Split(errOut.String(), "\n") {
		if !strings.Contains(line, ":CONSOLE(") {
			continue
		}
		m := consoleErrRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		msg := strings.TrimSpace(m[1])
		if !strings.Contains(msg, "Uncaught") && !strings.Contains(strings.ToLower(msg), "error") {
			continue
		}
		if !seen[msg] {
			seen[msg] = true
			consoleErrors = append(consoleErrors, msg)
		}
	}
	return out.String(), consoleErrors, err
}

// visibleText is roughly what a reader sees: markup and script bodies removed.
func visibleText(dom string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(tagRe.ReplaceAllString(dom, " ")), " "))
}

// appRoutes lists the client routes to check, always including "/". Both router
// styles are read: JSX <Route path> and the createBrowserRouter object form.
func appRoutes(dir string) []string {
	src := concatFrontendSource(dir)
	out := []string{"/"}
	seen := map[string]bool{"/": true}
	for _, re := range []*regexp.Regexp{routePathRe, objectRouteRe} {
		for _, m := range re.FindAllStringSubmatch(src, -1) {
			p := m[1]
			if p == "" || p == "*" || strings.Contains(p, ":") || seen[p] {
				continue
			}
			if !strings.HasPrefix(p, "/") {
				p = "/" + p
			}
			if seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
			if len(out) >= 4 {
				return out
			}
		}
	}
	return out
}

// renderIssues reports what is actually wrong with the running page: an empty
// mount node, a page with no content, or an uncaught error during render.
// Returns nil when the app renders, or when this machine has no browser to check
// with (a missing browser must never fail a build).
func renderIssues(ctx context.Context, app nodeApp) []string {
	browser := browserBinary()
	if browser == "" {
		return nil
	}
	dist := filepath.Join(app.dir, "dist")
	if !fileExists(filepath.Join(dist, "index.html")) {
		return nil // not a built static SPA (or the build never succeeded)
	}
	base, stop, err := staticSPAServer(dist)
	if err != nil {
		return nil
	}
	defer stop()

	var issues []string
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			issues = append(issues, s)
		}
	}
	for _, route := range appRoutes(app.dir) {
		dom, consoleErrors, err := renderPage(ctx, browser, base+route)
		if err != nil && dom == "" {
			continue // the browser itself failed; not the app's fault
		}
		blank := emptyMountRe.MatchString(dom) || len(visibleText(dom)) < MinRenderedText
		for _, msg := range consoleErrors {
			add("loading " + route + " in a browser logs an uncaught error: " + msg +
				" — fix the code that throws; the build passing does not mean the page renders")
		}
		if blank {
			detail := "the mount node is empty"
			if !emptyMountRe.MatchString(dom) {
				detail = "the page renders almost no content"
			}
			add("opening " + route + " shows a blank page (" + detail +
				") — the app compiles but throws while rendering, so React unmounts everything")
		}
	}
	return issues
}
