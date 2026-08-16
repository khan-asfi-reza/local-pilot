package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"harness/harness/events"
	"harness/harness/tools"
)

// The evaluator. Two layers, both of which install missing packages and repair
// errors rather than just report them:
//
//   evaluateTask — runs after EVERY sub-task. It installs any package the new code
//     imports, then syntax-checks the sub-app the task wrote into and repairs a
//     broken edit / bad syntax on the spot. Cheap, catches breakage at the source.
//
//   smokeAndRepair — runs once at the end and is robust: it does not just compile,
//     it RUNS the app. It applies migrations (repairing bad SQL), BOOTS the backend
//     and repairs whatever crashes it (missing module, undefined route handler,
//     bad query), and builds the frontend, repairing real breakers. Bounded loops.
//
// All evaluation is serialized on evalMu so parallel sub-tasks never run npm / a
// server on the same dir at once.

const (
	maxEvalRounds = 5
	// bootRepair has two phases — fix startup crashes, THEN fix mis-mounted routes —
	// so a complex app can legitimately need several of each. Give it more headroom
	// than the single-purpose build/migrate loops; it is monotonic, so extra rounds
	// only ever improve the result.
	maxBootRounds = 8
	backendPort   = "3001"
)

// evaluateTask is the per-sub-task evaluator: install imports, then syntax-check
// and repair the sub-app the task targeted. Best-effort; never blocks completion.
func (o *Orchestrator) evaluateTask(ctx context.Context, t SubTask, spec ChildSpec, emit func(events.Event), confirm tools.ConfirmFunc) {
	app := appForTask(spec.WorkDir, t)
	if app == nil {
		return
	}
	o.evalMu.Lock()
	defer o.evalMu.Unlock()

	if note := installDepsFor(ctx, *app); note != "" {
		emit(events.Text("  [eval " + app.label + " · " + app.stack + "] " + note + "\n"))
	}
	// Syntax check is node-only (tsc); Python/Go breakage is caught by the final boot.
	if app.stack != "node" {
		return
	}
	for round := 0; round < 2; round++ {
		errText := syntaxErrors(ctx, app)
		if errText == "" {
			return
		}
		if round == 1 {
			return // leave it for the final pass rather than loop forever mid-build
		}
		emit(events.Text("  [eval " + app.label + "] " + t.ID + " left a syntax error — repairing\n"))
		o.repair(ctx, *app, "a syntax error (likely a broken or half-applied edit)", errText, emit, confirm)
		installDepsFor(ctx, *app)
	}
}

// installPlanned installs, up front (before any sub-task runs), every package the
// plan declared each task needs — grouped into the sub-app (backend/frontend/root)
// its target files live in. So a task's imports are already present when it builds,
// and the reactive scan-install only has to catch anything the plan missed.
func (o *Orchestrator) installPlanned(ctx context.Context, plan Plan, workDir string, emit func(events.Event)) {
	byDir := map[string]map[string]bool{}
	label := map[string]string{}
	for _, t := range plan.Tasks {
		if len(t.Packages) == 0 {
			continue
		}
		app := appForTask(workDir, t)
		if app == nil || app.stack != "node" {
			continue // npm-based up-front install; Python/Go use requirements.txt/go.mod via installDepsFor
		}
		if byDir[app.dir] == nil {
			byDir[app.dir] = map[string]bool{}
			label[app.dir] = app.label
		}
		for _, p := range t.Packages {
			p = strings.TrimSpace(p)
			if p != "" && !isRelative(p) && !isBuiltin(packageRoot(p)) {
				byDir[app.dir][p] = true
			}
		}
	}
	for dir, set := range byDir {
		var pkgs []string
		for p := range set {
			if !fileExists(filepath.Join(dir, "node_modules", packageRoot(p))) {
				pkgs = append(pkgs, p)
			}
		}
		if len(pkgs) == 0 {
			continue
		}
		sort.Strings(pkgs)
		emit(events.Text("\n[init: installing " + label[dir] + " packages: " + strings.Join(pkgs, ", ") + "]\n"))
		for _, p := range pkgs {
			_, _ = runTool(ctx, dir, 300*time.Second, "npm", "install", p)
		}
	}
}

// smokeAndRepair is the final, robust "make it actually run" pass.
func (o *Orchestrator) smokeAndRepair(ctx context.Context, workDir string, emit func(events.Event), confirm tools.ConfirmFunc) {
	apps := detectNodeApps(workDir)
	if len(apps) == 0 {
		return
	}
	o.evalMu.Lock()
	defer o.evalMu.Unlock()
	emit(events.Text("\n[evaluate: making the app actually run]\n"))
	for _, app := range apps {
		if note := installDepsFor(ctx, app); note != "" {
			emit(events.Text("  [" + app.label + " · " + app.stack + "] " + note + "\n"))
		}
		if app.kind == "backend" {
			o.migrateWithRepair(ctx, app, emit, confirm)
			o.seedWithRepair(ctx, app, emit, confirm)
			o.bootRepair(ctx, app, emit, confirm)
		} else {
			o.frontendStaticRepair(ctx, app, emit, confirm)
			o.buildRepair(ctx, app, emit, confirm)
		}
	}
}

// frontendStaticRepair catches product-level frontend defects a build/boot check
// misses: a stub route (a page that is just a <Link> or bare text, e.g. a homepage
// that only says "Find a Doctor"), and dead navigation (a to=/navigate() target no
// <Route> matches, e.g. linking /doctors/:id when the route is /doctor/:id). It
// hands the concrete list to a repair child. Static — no browser needed.
func (o *Orchestrator) frontendStaticRepair(ctx context.Context, app nodeApp, emit func(events.Event), confirm tools.ConfirmFunc) {
	for round := 0; round < 3; round++ {
		issues := frontendStaticIssues(app.dir)
		if len(issues) == 0 {
			return
		}
		if round == 2 {
			emit(events.Text("  [" + app.label + "] some UI wiring issues remain: " + issues[0] + "\n"))
			return
		}
		emit(events.Text("  [" + app.label + "] UI wiring issues — repairing (round " + itoa(round+1) + ")\n"))
		what := "the frontend has real product defects that a build check misses:\n- " + strings.Join(issues, "\n- ") +
			"\nFix each: a stub route must render a real page component with content and working links; a navigation " +
			"target must match a defined <Route path>. Use the EXACT route paths the <Route> elements declare. Keep every " +
			"page functional — no placeholder pages."
		o.repair(ctx, app, what, "", emit, confirm)
	}
}

var (
	routePathRe = regexp.MustCompile(`<Route\s[^>]*\bpath=["']([^"']+)["']`)
	routeElemRe = regexp.MustCompile(`<Route\s[^>]*\bpath=["']([^"']+)["'][^>]*\belement=\{([^}]*(?:\{[^}]*\}[^}]*)*)\}`)
	linkToRe    = regexp.MustCompile(`(?:\bto=|navigate\(|history\.push\(|window\.location(?:\.href)?\s*=\s*)["'` + "`" + `](/[A-Za-z0-9_\-/:${}.]*)["'` + "`" + `]`)
	compTagRe   = regexp.MustCompile(`<([A-Z][A-Za-z0-9]+)`)
)

// nonPageTags are capitalized JSX tags that are NOT a page component, so a route
// element containing only these (plus text) is a stub, not a real page.
var nonPageTags = map[string]bool{
	"Link": true, "Navigate": true, "Outlet": true, "Fragment": true,
	"Route": true, "Routes": true, "Redirect": true, "NavLink": true,
}

// hasPageComponent reports whether a JSX snippet renders a real page component
// (a capitalized tag that is not a router primitive).
func hasPageComponent(elem string) bool {
	for _, m := range compTagRe.FindAllStringSubmatch(elem, -1) {
		if !nonPageTags[m[1]] {
			return true
		}
	}
	return false
}

// frontendStaticIssues scans a frontend for stub routes and dead navigation links.
func frontendStaticIssues(dir string) []string {
	src := concatFrontendSource(dir)
	if src == "" {
		return nil
	}
	var issues []string
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			issues = append(issues, s)
		}
	}

	// Collect declared route paths.
	var routes []string
	for _, m := range routePathRe.FindAllStringSubmatch(src, -1) {
		routes = append(routes, m[1])
	}
	// Stub route: element renders no real PAGE component — router primitives
	// (Link/Navigate/Outlet/Fragment) don't count, so a route whose element is just
	// <Link>…</Link> or bare text is a stub.
	for _, m := range routeElemRe.FindAllStringSubmatch(src, -1) {
		path, elem := m[1], m[2]
		if path == "*" {
			continue // 404 catch-all is allowed to be inline
		}
		if !hasPageComponent(elem) {
			add("route \"" + path + "\" renders no real page component (it is a stub such as a bare <Link> or text) — build a real page for it")
		}
	}
	// Dead links: a navigation target that matches no declared route.
	if len(routes) > 0 {
		for _, m := range linkToRe.FindAllStringSubmatch(src, -1) {
			target := m[1]
			if strings.HasPrefix(target, "/api") || target == "/" || target == "#" {
				continue
			}
			if !routeMatches(target, routes) {
				add("navigation to \"" + target + "\" matches no <Route path> (declared routes: " + strings.Join(routes, ", ") + ")")
			}
		}
	}
	return issues
}

// routeMatches reports whether a link target matches any declared route pattern
// (static segments must be equal; :param / {param} / * segments match anything).
func routeMatches(target string, routes []string) bool {
	ts := splitSegs(target)
	for _, r := range routes {
		rs := splitSegs(r)
		if len(rs) == 1 && (rs[0] == "*" || rs[0] == "") {
			return true
		}
		if len(ts) != len(rs) {
			continue
		}
		ok := true
		for i := range rs {
			seg := rs[i]
			if strings.HasPrefix(seg, ":") || strings.HasPrefix(seg, "{") || seg == "*" {
				continue // param — matches anything
			}
			if seg != ts[i] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func splitSegs(p string) []string {
	var out []string
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// concatFrontendSource joins the frontend's .tsx/.jsx/.vue source (routing + links
// live there), skipping vendored dirs. Capped so a huge app stays bounded.
func concatFrontendSource(dir string) string {
	var b strings.Builder
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if n := d.Name(); n == "node_modules" || n == ".git" || n == "dist" || n == "build" {
				return fs.SkipDir
			}
			return nil
		}
		switch filepath.Ext(p) {
		case ".tsx", ".jsx", ".vue", ".svelte", ".ts", ".js":
			if b.Len() > 400_000 {
				return fs.SkipAll
			}
			if by, e := os.ReadFile(p); e == nil {
				b.Write(by)
				b.WriteByte('\n')
			}
		}
		return nil
	})
	return b.String()
}

// installDepsFor installs an app's dependencies per stack: Node scans imports and
// npm-installs the missing; Python creates a venv and pip-installs requirements; Go
// runs `go mod tidy`. Returns a short note of what it did (or "").
func installDepsFor(ctx context.Context, app nodeApp) string {
	switch app.stack {
	case "python":
		venv := filepath.Join(app.dir, ".venv")
		if !fileExists(venv) {
			if _, err := runTool(ctx, app.dir, 120*time.Second, "python3", "-m", "venv", ".venv"); err != nil {
				return "venv create failed"
			}
		}
		pip := filepath.Join(venv, "bin", "pip")
		if fileExists(filepath.Join(app.dir, "requirements.txt")) {
			if _, err := runTool(ctx, app.dir, 400*time.Second, pip, "install", "-q", "-r", "requirements.txt"); err == nil {
				return "installed Python requirements"
			}
			return "pip install had errors"
		}
		if fileExists(filepath.Join(app.dir, "pyproject.toml")) {
			_, _ = runTool(ctx, app.dir, 400*time.Second, pip, "install", "-q", ".")
			return "installed from pyproject"
		}
		return ""
	case "go":
		if _, err := runTool(ctx, app.dir, 300*time.Second, "go", "mod", "tidy"); err == nil {
			return "go mod tidy"
		}
		return "go mod tidy had errors"
	default: // node
		if pkgs := installMissingDeps(ctx, app.dir); len(pkgs) > 0 {
			return "installed: " + strings.Join(pkgs, ", ")
		}
		return ""
	}
}

// bootRepair boots the backend and repairs whatever stops it from WORKING: a
// startup crash (missing module auto-installed; undefined handler / bad query
// repaired), AND — once it is listening — API endpoints that 404/500. A server
// that boots but whose routes are mis-mounted looks "healthy" to a boot check, so
// this actually probes every /api mount and repairs the routing until they answer.
func (o *Orchestrator) bootRepair(ctx context.Context, app nodeApp, emit func(events.Event), confirm tools.ConfirmFunc) {
	// Monotonic repair: keep a snapshot of the last state that at least BOOTED, so a
	// routing repair that breaks startup can be rolled back. The evaluator must never
	// leave the app worse than a state it already reached.
	var bootingSnap string
	defer func() {
		if bootingSnap != "" {
			_ = os.RemoveAll(bootingSnap)
		}
	}()

	for round := 0; round < maxBootRounds; round++ {
		booted, ok, errText := bootAndProbe(ctx, app)
		if ok {
			emit(events.Text("  [" + app.label + "] boots and every API endpoint responds ✓\n"))
			return
		}
		// A missing module is deterministic — install it and retry without a round.
		if !booted && app.stack == "node" {
			if mods := missingModules(errText); len(mods) > 0 {
				args := append([]string{"install"}, mods...)
				if _, err := runTool(ctx, app.dir, 300*time.Second, "npm", args...); err == nil {
					emit(events.Text("  [" + app.label + "] installed missing module(s): " + strings.Join(mods, ", ") + "\n"))
					round--
					continue
				}
			}
		}
		// Refresh the monotonic floor whenever the app boots, so it captures the route
		// fixes made so far. This is the version we fall back to only if we run out of
		// rounds still broken — never mid-loop.
		if booted {
			if bootingSnap != "" {
				_ = os.RemoveAll(bootingSnap)
			}
			bootingSnap = snapshotSrc(app.dir)
		}
		if round == maxBootRounds-1 {
			if !booted && bootingSnap != "" {
				restoreSrc(bootingSnap, app.dir)
				emit(events.Text("  [" + app.label + "] kept the last booting version (boots; some endpoints may still 404)\n"))
			} else if !booted {
				emit(events.Text("  [" + app.label + "] still not booting after repair; leaving as-is\n"))
			} else {
				emit(events.Text("  [" + app.label + "] endpoints not fully repaired; kept the booting version\n"))
			}
			return
		}
		var what string
		if booted {
			what = "the server boots but some API endpoints return 404 or 500 — a router/route is mis-mounted or its path is wrong. Mount every router/route exactly once at the correct /api/<name> path and align the paths. IMPORTANT: keep the server booting — do not remove working imports or routes, only fix the broken ones"
			emit(events.Text("  [" + app.label + "] boots but API endpoints fail — repairing routes (round " + itoa(round+1) + ")\n"))
		} else {
			// Not booting. If we previously reached a booting state, the LAST repair
			// introduced this crash — do NOT throw the work away; fix the specific error
			// (the added route/file is likely good, it just has a bug) and keep going.
			// The monotonic floor above still guarantees we never ship worse than booting.
			what = "the server crashes on startup — fix the SPECIFIC error in the traceback below. It was likely introduced by the last edit; keep the routes and files that were just added, only correct the error that stops startup (a bad import, a name typo, a syntax error)"
			emit(events.Text("  [" + app.label + "] does not boot — repairing (round " + itoa(round+1) + ")\n"))
		}
		o.repair(ctx, app, what, errText, emit, confirm)
		installDepsFor(ctx, app)
	}
}

// snapshotSrc copies an app's editable source (the whole dir minus vendored/build
// output) into a temp dir so a regressing repair can be rolled back. Stack-agnostic.
func snapshotSrc(dir string) string {
	tmp, err := os.MkdirTemp("", "eval-snap-")
	if err != nil {
		return ""
	}
	if copyTree(dir, tmp) != nil {
		return ""
	}
	return tmp
}

// restoreSrc copies a snapshot's tracked source back over the app dir. It only
// overwrites/adds files present in the snapshot (never touches node_modules/.venv).
func restoreSrc(snap, dir string) {
	_ = copyTree(snap, dir)
}

// copyTree recursively copies src to dst, skipping vendored dirs.
func copyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		b, e := os.ReadFile(src)
		if e != nil {
			return e
		}
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		return os.WriteFile(dst, b, info.Mode())
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			if n := d.Name(); n == "node_modules" || n == ".git" || n == "dist" || n == "build" || n == ".venv" || n == "__pycache__" || n == "vendor" {
				return fs.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return nil
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
}

// bootAndProbe starts the backend, waits until it is listening, then GETs every
// /api mount base. Returns booted (did it start), ok (started AND all probes pass),
// and an error/report to repair from. The server process tree is always killed.
func bootAndProbe(ctx context.Context, app nodeApp) (booted bool, ok bool, errText string) {
	pf := runProfile(app)
	cctx, cancel := context.WithTimeout(ctx, 55*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, pf.cmd[0], pf.cmd[1:]...)
	cmd.Dir = app.dir
	// Layer: process env < the app's .env (DATABASE_URL/REDIS_URL) < the boot profile
	// (PORT etc.), so the boot hits the SAME database the seed populated.
	cmd.Env = append(os.Environ(), appEnvLines(app.dir)...)
	cmd.Env = append(cmd.Env, pf.env...)
	setProcGroup(cmd)
	var buf safeBuffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		return false, false, err.Error()
	}
	defer killGroup(cmd)

	deadline := time.Now().Add(38 * time.Second)
	for time.Now().Before(deadline) {
		s := buf.String()
		if pf.readyRe.MatchString(s) {
			break
		}
		if pf.errRe.MatchString(s) {
			return false, false, s
		}
		time.Sleep(400 * time.Millisecond)
	}
	if !pf.readyRe.MatchString(buf.String()) {
		return false, false, buf.String()
	}

	time.Sleep(1500 * time.Millisecond) // let route registration settle
	before := buf.String()
	// Conformance-first: if a contract exists, test every declared endpoint against
	// it (route+method exists, list endpoints return arrays); else fall back to the
	// literal-scan + consumer probe.
	var fails []string
	if c := loadContract(app.dir); c != nil {
		fails = probeContract(*c, pf.port)
	} else {
		fails = probeAPI(app, pf.port)
	}
	if len(fails) == 0 {
		return true, true, ""
	}
	// Include whatever the server logged WHILE probing — a 500 handler prints its
	// traceback to stderr, which is the exact error the repair child needs (the bare
	// status code is not enough to fix a crashing endpoint).
	serverLog := strings.TrimPrefix(buf.String(), before)
	report := "The server boots and listens, but these API requests fail:\n" +
		strings.Join(fails, "\n")
	if tail := strings.TrimSpace(serverLog); tail != "" {
		report += "\n\nServer log during those requests (the real error is here):\n" + lastLines(tail, 40)
	}
	report += "\n\nRoute/mount lines found in the backend source:\n" + mountLines(app.dir)
	return true, false, report
}

// lastLines returns the last n lines of s.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// bootProfile is how to start a backend of a given stack and detect readiness.
type bootProfile struct {
	cmd     []string
	env     []string
	port    string
	readyRe *regexp.Regexp
	errRe   *regexp.Regexp
}

var pyErrRe = regexp.MustCompile(`(?i)(Traceback|ModuleNotFoundError|ImportError|SyntaxError|NameError|AttributeError|IndentationError|Error loading ASGI app|django\.|OperationalError|ProgrammingError|address already in use)`)
var goErrRe = regexp.MustCompile(`(?i)(panic:|cannot find|undefined:|syntax error|build failed|no required module|\.go:\d+:|imported and not used)`)

// runProfile returns the start command + readiness signals for an app's stack. The
// boot port is the project's assigned PORT (from .env) so eval and the concurrent
// run agree and multiple apps never clash; it falls back to the stack default.
func runProfile(app nodeApp) bootProfile {
	switch app.stack {
	case "python":
		py := filepath.Join(app.dir, ".venv", "bin", "python")
		bin := filepath.Join(app.dir, ".venv", "bin")
		port := appPort(app.dir, "8000")
		switch app.framework {
		case "django":
			// PYTHONUNBUFFERED so the "Starting development server" line is flushed
			// immediately — Python block-buffers stdout when it is not a TTY, which
			// otherwise hides the readiness signal and looks like "does not boot".
			return bootProfile{[]string{py, "-u", "manage.py", "runserver", "0.0.0.0:" + port, "--noreload"}, []string{"PORT=" + port, "PYTHONUNBUFFERED=1"}, port,
				regexp.MustCompile(`(?i)(Starting development server|Watching for file changes|Quit the server)`), pyErrRe}
		case "fastapi":
			return bootProfile{[]string{filepath.Join(bin, "uvicorn"), fastapiModule(app.dir), "--port", port}, []string{"PORT=" + port, "PYTHONUNBUFFERED=1"}, port,
				regexp.MustCompile(`(?i)(Uvicorn running|Application startup complete)`), pyErrRe}
		default: // flask
			return bootProfile{[]string{py, "-u", pyEntry(app.dir)}, []string{"PORT=" + port, "FLASK_RUN_PORT=" + port, "PYTHONUNBUFFERED=1"}, port,
				regexp.MustCompile(`(?i)(Running on|Debugger is active|Serving Flask)`), pyErrRe}
		}
	case "go":
		port := appPort(app.dir, "8080")
		return bootProfile{[]string{"go", "run", "."}, []string{"PORT=" + port}, port,
			regexp.MustCompile(`(?i)(listening|server started|server running|running on|Fiber v|:\d{4})`), goErrRe}
	default: // node
		port := appPort(app.dir, backendPort)
		return bootProfile{[]string{"npm", "run", "dev"}, []string{"PORT=" + port}, port, bootReadyRe, bootErrorRe}
	}
}

// appPort reads the project's assigned PORT from the app's .env (or the monorepo
// root .env one level up), falling back to the stack default.
func appPort(dir, def string) string {
	for _, p := range []string{filepath.Join(dir, ".env"), filepath.Join(filepath.Dir(dir), ".env")} {
		if b, err := os.ReadFile(p); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "PORT=") {
					if v := strings.TrimSpace(strings.TrimPrefix(line, "PORT=")); v != "" {
						return v
					}
				}
			}
		}
	}
	return def
}

// fastapiModule finds the FastAPI app module ("pkg.main:app") by locating a file
// that constructs FastAPI(); defaults to "main:app".
func fastapiModule(dir string) string {
	for _, cand := range []string{"main.py", "app.py", "app/main.py", "src/main.py", "api/main.py"} {
		if fileContains(filepath.Join(dir, cand), "FastAPI(") {
			mod := strings.TrimSuffix(cand, ".py")
			return strings.ReplaceAll(mod, "/", ".") + ":app"
		}
	}
	return "main:app"
}

// pyEntry finds a Flask entrypoint file; defaults to "app.py".
func pyEntry(dir string) string {
	for _, cand := range []string{"app.py", "main.py", "wsgi.py", "run.py"} {
		if fileExists(filepath.Join(dir, cand)) {
			return cand
		}
	}
	return "app.py"
}

// probeAPI GETs each /api path found in the backend source and flags any that 404 or
// 5xx (or refuse the connection). 4xx like 401/403/400 mean the route EXISTS
// (auth/validation), so those pass. Stack-agnostic: scans source for "/api/..." literals.
func probeAPI(app nodeApp, port string) []string {
	// Probe two sources: what the backend DECLARES (its own routes) and what its
	// CONSUMERS EXPECT (the /api paths sibling apps like the frontend actually call).
	// The second catches contract drift — a consumer calling an endpoint the producer
	// never mounted — which a backend-only probe cannot see.
	origin := map[string]string{}
	for _, p := range apiPaths(app.dir) {
		origin[p] = "declared by the backend"
	}
	for _, p := range consumerExpectedPaths(app.dir) {
		if _, ok := origin[p]; !ok {
			origin[p] = "called by the frontend but not served by the backend"
		}
	}
	if len(origin) == 0 {
		// Nothing parseable to probe (e.g. a router-only app). Booting is the bar —
		// do NOT fail on a synthetic /health the app may not define.
		return nil
	}
	// A path that is only a mount PREFIX of other routes (e.g. "/api/auth" behind
	// "/api/auth/login") is not itself a GET endpoint, so a 404 on it is expected, not
	// a bug. Collect prefixes so we don't flag them.
	prefixes := map[string]bool{}
	for a := range origin {
		for b := range origin {
			if a != b && strings.HasPrefix(b, a+"/") {
				prefixes[a] = true
			}
		}
	}
	var fails []string
	client := &http.Client{Timeout: 4 * time.Second}
	for base, why := range origin {
		resp, err := client.Get("http://localhost:" + port + base)
		if err != nil {
			fails = append(fails, "GET "+base+" -> connection failed ("+why+")")
			continue
		}
		code := resp.StatusCode
		_ = resp.Body.Close()
		if code == 404 && prefixes[base] {
			continue // a mount prefix, not a leaf endpoint — 404 here is fine
		}
		if code == 404 || code >= 500 {
			fails = append(fails, fmt.Sprintf("GET %s -> %d (%s)", base, code, why))
		}
	}
	sort.Strings(fails)
	return fails
}

// consumerExpectedPaths returns the distinct /api collection bases that SIBLING apps
// (e.g. the frontend next to a backend/) call, so the probe can verify the backend
// actually serves what its consumers expect. Dynamic trailing segments (an id, a
// {param}, a ${var}) are truncated to the collection base so probing an absent row
// is not mistaken for a missing route.
func consumerExpectedPaths(backendDir string) []string {
	parent := filepath.Dir(backendDir)
	self := filepath.Base(backendDir)
	seen := map[string]bool{}
	var out []string
	entries, _ := os.ReadDir(parent)
	for _, e := range entries {
		if !e.IsDir() || e.Name() == self || e.Name() == "node_modules" || e.Name() == ".git" || e.Name() == ".venv" {
			continue
		}
		_ = filepath.WalkDir(filepath.Join(parent, e.Name()), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if n := d.Name(); n == "node_modules" || n == ".git" || n == "dist" || n == "build" || n == ".venv" {
					return fs.SkipDir
				}
				return nil
			}
			switch filepath.Ext(p) {
			case ".ts", ".tsx", ".js", ".jsx", ".vue", ".svelte":
			default:
				return nil
			}
			b, e := os.ReadFile(p)
			if e != nil {
				return nil
			}
			for _, m := range apiPathRe.FindAllStringSubmatch(string(b), -1) {
				base := consumerBase(m[1])
				if base == "" || base == "/api" || seen[base] {
					continue
				}
				seen[base] = true
				out = append(out, base)
				if len(out) >= 20 {
					return fs.SkipAll
				}
			}
			return nil
		})
	}
	return out
}

// consumerBase truncates a called path at its first dynamic segment (a number, a
// {param}/:param/${var}) so /api/doctors/1 and /api/doctors/${id} both become the
// collection base /api/doctors — the thing whose existence we can safely probe.
func consumerBase(path string) string {
	segs := strings.Split(path, "/")
	var keep []string
	for _, s := range segs {
		if s == "" {
			continue
		}
		if s == "" || strings.ContainsAny(s, "{:$") || isAllDigits(s) {
			break
		}
		keep = append(keep, s)
	}
	if len(keep) == 0 {
		return ""
	}
	return "/" + strings.Join(keep, "/")
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

var apiPathRe = regexp.MustCompile("[\"'`](/api/[A-Za-z0-9_\\-/{}:]*)[\"'`]")
var paramRe = regexp.MustCompile(`/(:|\{)[^/]+`)

// Django/DRF routes are not "/api/..." literals: a ViewSet is registered by name
// and mounted under an include() prefix. Discover both so the probe hits real URLs.
var drfRegisterRe = regexp.MustCompile(`(?:router|routes)\.register\(\s*r?["']([A-Za-z0-9_\-/]+)["']`)
var pyApiIncludeRe = regexp.MustCompile(`path\(\s*["']([A-Za-z0-9_\-/]*api[A-Za-z0-9_\-/]*?)/?["']\s*,\s*include`)
var pyApiDirectRe = regexp.MustCompile(`(?:path|re_path)\(\s*r?["'](api/[A-Za-z0-9_\-/]+?)/?["']`)

// apiPaths scans the backend source for API routes and returns distinct collection
// bases to probe: "/api/..." string literals (Express/Fastify/Go), plus Django/DRF
// routes reconstructed from router.register() names and the api/ include prefix.
func apiPaths(dir string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(base string) {
		base = paramRe.ReplaceAllString(base, "")
		base = "/" + strings.Trim(base, "/")
		if base == "" || base == "/" || base == "/api" || seen[base] {
			return
		}
		seen[base] = true
		out = append(out, base)
	}
	var drfNames []string
	prefix := ""
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if n := d.Name(); n == "node_modules" || n == ".venv" || n == ".git" || n == "dist" || n == "__pycache__" {
				return fs.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(p)
		switch ext {
		case ".ts", ".tsx", ".js", ".py", ".go":
		default:
			return nil
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return nil
		}
		src := string(b)
		for _, m := range apiPathRe.FindAllStringSubmatch(src, -1) {
			add(m[1])
		}
		if ext == ".py" {
			for _, m := range drfRegisterRe.FindAllStringSubmatch(src, -1) {
				drfNames = append(drfNames, m[1])
			}
			for _, m := range pyApiDirectRe.FindAllStringSubmatch(src, -1) {
				add("/" + m[1] + "/")
			}
			if prefix == "" {
				if m := pyApiIncludeRe.FindStringSubmatch(src); m != nil {
					prefix = strings.Trim(m[1], "/")
				}
			}
		}
		return nil
	})
	// Reconstruct DRF collection URLs: <prefix>/<name>/ (default prefix "api"). DRF
	// list routes need a trailing slash; APPEND_SLASH redirects if we omit it.
	if prefix == "" && len(drfNames) > 0 {
		prefix = "api"
	}
	for _, n := range drfNames {
		add("/" + prefix + "/" + strings.Trim(n, "/") + "/")
	}
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}

// mountLines returns route/mount lines from the backend source so the repair child
// sees the current (possibly duplicated / incomplete) wiring, across frameworks.
func mountLines(dir string) string {
	var lines []string
	seen := map[string]bool{}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if n := d.Name(); n == "node_modules" || n == ".venv" || n == ".git" || n == "dist" || n == "__pycache__" {
				return fs.SkipDir
			}
			return nil
		}
		switch filepath.Ext(p) {
		case ".ts", ".js", ".py", ".go":
		default:
			return nil
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return nil
		}
		for _, ln := range strings.Split(string(b), "\n") {
			t := strings.TrimSpace(ln)
			if strings.Contains(t, "/api") && (strings.Contains(t, "use(") || strings.Contains(t, "include_router") ||
				strings.Contains(t, "add_url_rule") || strings.Contains(t, "register") || strings.Contains(t, "Group(") ||
				strings.Contains(t, "router.") || strings.Contains(t, "app.")) {
				if !seen[t] && len(lines) < 30 {
					seen[t] = true
					lines = append(lines, t)
				}
			}
		}
		return nil
	})
	return strings.Join(lines, "\n")
}

// buildRepair builds a frontend and repairs the real breakers (duplicate export,
// missing import, bad syntax, a broken vite config) until the build passes.
func (o *Orchestrator) buildRepair(ctx context.Context, app nodeApp, emit func(events.Event), confirm tools.ConfirmFunc) {
	for round := 0; round < maxEvalRounds; round++ {
		out, err := runTool(ctx, app.dir, 240*time.Second, "npx", "vite", "build")
		if err == nil {
			emit(events.Text("  [" + app.label + "] builds ✓\n"))
			return
		}
		if round == maxEvalRounds-1 {
			emit(events.Text("  [" + app.label + "] still failing to build; leaving as-is\n"))
			return
		}
		emit(events.Text("  [" + app.label + "] build failed — repairing (round " + itoa(round+1) + ")\n"))
		o.repair(ctx, app, "the frontend does not build", tailBoot(out), emit, confirm)
		installMissingDeps(ctx, app.dir)
	}
}

// migrateWithRepair runs `npm run migrate` and repairs a failing .sql file until
// every migration applies (or the rounds run out). Bad generated DDL — a non-
// IMMUTABLE function in an index predicate, a table-name typo, an invalid view —
// is fixed here instead of leaving the app with no tables.
func (o *Orchestrator) migrateWithRepair(ctx context.Context, app nodeApp, emit func(events.Event), confirm tools.ConfirmFunc) {
	migrateCmd := migrateCommand(app)
	if len(migrateCmd) == 0 {
		return
	}
	for round := 0; round < maxEvalRounds; round++ {
		out, err := runTool(ctx, app.dir, 150*time.Second, migrateCmd[0], migrateCmd[1:]...)
		if err == nil {
			emit(events.Text("  [" + app.label + "] migrations applied ✓\n"))
			return
		}
		if round == maxEvalRounds-1 {
			emit(events.Text("  [" + app.label + "] migrations still failing; leaving as-is\n"))
			return
		}
		emit(events.Text("  [" + app.label + "] migration failed — repairing (round " + itoa(round+1) + ")\n"))
		o.repair(ctx, app, "a database migration fails to apply — fix the migration (bad SQL such as a NOW()/non-IMMUTABLE function in an index predicate, a wrong table/column name, an invalid view; or a Django model / Alembic revision error). Keep the schema the app's code expects", tailBoot(out), emit, confirm)
	}
}

// seedWithRepair runs the project's seed script (populating demo data) and repairs
// its errors from the traceback, so the UI shows REAL data instead of empty lists —
// a data-less app that "boots" is still a broken product. No-op if there is no seed.
func (o *Orchestrator) seedWithRepair(ctx context.Context, app nodeApp, emit func(events.Event), confirm tools.ConfirmFunc) {
	seedCmd := seedCommand(app)
	if len(seedCmd) == 0 {
		return
	}
	env := appEnvLines(app.dir)
	for round := 0; round < maxEvalRounds; round++ {
		out, err := runToolEnv(ctx, app.dir, 150*time.Second, env, seedCmd[0], seedCmd[1:]...)
		if err == nil && !seedLooksEmpty(out) {
			emit(events.Text("  [" + app.label + "] seed data loaded ✓\n"))
			return
		}
		if round == maxEvalRounds-1 {
			emit(events.Text("  [" + app.label + "] seed still not populating; leaving as-is\n"))
			return
		}
		emit(events.Text("  [" + app.label + "] seed failed — repairing (round " + itoa(round+1) + ")\n"))
		what := "the database seed script does not populate data. Run it and fix the error. Common causes: using a " +
			"session generator (`db = get_db()`) instead of a real Session (`db = SessionLocal()`); forgetting `db.commit()` " +
			"so nothing persists; not importing every model class before mappers configure (a relationship like " +
			"`relationship(\"User\")` fails to locate the class); or bad value construction. After fixing, the seed must " +
			"INSERT and COMMIT rows so the API returns them. Also fix the seed if it reports inserting 0 rows"
		o.repair(ctx, app, what, tailBoot(out), emit, confirm)
	}
}

// seedLooksEmpty flags a seed run that "succeeded" but inserted nothing (a common
// bug: adds without a flush/commit), so the repair pass still fixes it.
func seedLooksEmpty(out string) bool {
	return regexp.MustCompile(`(?i)inserted\s+0\b|0\s+(rows|records|doctors|users|items)\s+(inserted|created)`).MatchString(out)
}

// seedCommand returns how to run an app's data seed, or nil if it defines none.
func seedCommand(app nodeApp) []string {
	switch app.stack {
	case "python":
		py := filepath.Join(app.dir, ".venv", "bin", "python")
		for _, m := range []string{"app.seed", "app.seeds", "app.seed_data", "seed", "seeds", "seed_data"} {
			if fileExists(filepath.Join(app.dir, strings.ReplaceAll(m, ".", "/")+".py")) {
				return []string{py, "-m", m}
			}
		}
		for _, f := range []string{"scripts/seed.py", "app/db/seed.py", "core/seed.py"} {
			if fileExists(filepath.Join(app.dir, f)) {
				return []string{py, f}
			}
		}
		return nil
	case "go":
		return nil
	default: // node
		if fileContains(filepath.Join(app.dir, "package.json"), "\"seed\"") {
			return []string{"npm", "run", "seed"}
		}
		for _, f := range []string{"src/seed.ts", "seed.ts", "prisma/seed.ts", "scripts/seed.ts", "src/db/seed.ts"} {
			if fileExists(filepath.Join(app.dir, f)) {
				return []string{"npx", "ts-node", "--transpile-only", f}
			}
		}
		return nil
	}
}

// migrateCommand returns how to apply migrations for an app's stack, or nil if the
// app defines no migration step.
func migrateCommand(app nodeApp) []string {
	switch app.stack {
	case "python":
		py := filepath.Join(app.dir, ".venv", "bin", "python")
		if app.framework == "django" && fileExists(filepath.Join(app.dir, "manage.py")) {
			return []string{py, "manage.py", "migrate", "--noinput"}
		}
		if fileExists(filepath.Join(app.dir, "alembic.ini")) {
			return []string{filepath.Join(app.dir, ".venv", "bin", "alembic"), "upgrade", "head"}
		}
		if fileExists(filepath.Join(app.dir, "migrate.py")) {
			return []string{py, "migrate.py"}
		}
		return nil
	case "go":
		return nil // Go apps typically migrate on boot or embed SQL; leave to boot
	default: // node
		if fileContains(filepath.Join(app.dir, "package.json"), "\"migrate\"") {
			return []string{"npm", "run", "migrate"}
		}
		return nil
	}
}

// repair spawns a focused child agent to fix one concrete error.
func (o *Orchestrator) repair(ctx context.Context, app nodeApp, what, errText string, emit func(events.Event), confirm tools.ConfirmFunc) {
	prompt := "The " + app.label + " has a problem: " + what + ". Below is the exact error. Fix it:\n" +
		"1. Open the file at the location the error names AND read it.\n" +
		"2. If a symbol is undefined (e.g. `X.foo is not a function`, `Route.post() ... got undefined`, `Cannot find " +
		"name`), open the module it is imported FROM and add or rename the export so the names match on both sides. " +
		"A route handler or middleware that is `undefined` means the controller/middleware file does not export it — " +
		"add that export.\n" +
		"3. For a bad SQL/migration error, fix the SQL in the named backend/migrations/*.sql file.\n" +
		"4. For a syntax error, rewrite the whole broken file correctly with write_file.\n" +
		"Make the smallest change that resolves THIS error. Do NOT start servers or run commands; just fix the code.\n\nERROR:\n" + tailBoot(errText)
	spec := ChildSpec{
		WorkDir: app.dir,
		Mode:    tools.ModeAuto,
		// shell_run lets the repair child actually TEST its fix (re-run the migration,
		// tsc, a curl) and iterate — without it a 9B flails trying blocked commands.
		// Server-start commands are still redirected by shell_run's own guard.
		Allowed: []string{"read_file", "write_file", "edit_file", "list_dir", "search", "shell_run"},
	}
	quiet := func(ev events.Event) {
		if ev.Type == "done" || ev.Type == "error" {
			return
		}
		emit(ev)
	}
	o.exec.RunChild(ctx, prompt, spec, quiet, confirm)
}

type nodeApp struct {
	dir       string
	label     string
	kind      string // "backend" | "frontend"
	stack     string // "node" | "python" | "go"
	framework string // express|nestjs|fastify|django|fastapi|flask|gin|fiber|react|vue|...
}

// appForTask maps a sub-task to the sub-app its target files live in.
func appForTask(workDir string, t SubTask) *nodeApp {
	sub := ""
	for _, f := range t.TargetFiles {
		f = filepath.ToSlash(f)
		if strings.HasPrefix(f, "backend/") {
			sub = "backend"
			break
		}
		if strings.HasPrefix(f, "frontend/") {
			sub = "frontend"
			break
		}
	}
	for _, a := range detectNodeApps(workDir) {
		if (sub == "" && a.label == "app") || a.label == sub {
			return &a
		}
	}
	return nil
}

// detectNodeApps finds every sub-app (root, backend/, frontend/) and classifies its
// stack (node/python/go), kind (backend/frontend), and framework. Name kept for
// call-site stability; it now covers Python and Go, not just Node.
func detectNodeApps(workDir string) []nodeApp {
	var apps []nodeApp
	for _, c := range []struct{ rel, label string }{{".", "app"}, {"backend", "backend"}, {"frontend", "frontend"}} {
		dir := filepath.Join(workDir, c.rel)
		if a, ok := classifyApp(dir, c.label); ok {
			apps = append(apps, a)
		}
	}
	return apps
}

// classifyApp inspects a directory and returns its stack/kind/framework, or ok=false
// if it holds no recognizable app.
func classifyApp(dir, label string) (nodeApp, bool) {
	has := func(f string) bool { return fileExists(filepath.Join(dir, f)) }
	// Node (has package.json). Frontend if a bundler/entry is present.
	if has("package.json") {
		if has("vite.config.ts") || has("vite.config.js") || has("index.html") {
			fw := "react"
			if fileContains(filepath.Join(dir, "package.json"), "vue") {
				fw = "vue"
			}
			return nodeApp{dir: dir, label: label, kind: "frontend", stack: "node", framework: fw}, true
		}
		pkg := filepath.Join(dir, "package.json")
		fw := "express"
		switch {
		case fileContains(pkg, "@nestjs/core"):
			fw = "nestjs"
		case fileContains(pkg, "fastify"):
			fw = "fastify"
		}
		return nodeApp{dir: dir, label: label, kind: "backend", stack: "node", framework: fw}, true
	}
	// Python.
	if has("manage.py") {
		return nodeApp{dir: dir, label: label, kind: "backend", stack: "python", framework: "django"}, true
	}
	if has("requirements.txt") || has("pyproject.toml") {
		fw := "flask"
		req := filepath.Join(dir, "requirements.txt")
		if fileContains(req, "fastapi") || fileContains(filepath.Join(dir, "pyproject.toml"), "fastapi") {
			fw = "fastapi"
		} else if fileContains(req, "flask") || fileContains(filepath.Join(dir, "pyproject.toml"), "flask") {
			fw = "flask"
		}
		return nodeApp{dir: dir, label: label, kind: "backend", stack: "python", framework: fw}, true
	}
	// Go.
	if has("go.mod") {
		fw := "go"
		gm := filepath.Join(dir, "go.mod")
		switch {
		case fileContains(gm, "gin-gonic/gin"):
			fw = "gin"
		case fileContains(gm, "gofiber/fiber"):
			fw = "fiber"
		}
		return nodeApp{dir: dir, label: label, kind: "backend", stack: "go", framework: fw}, true
	}
	return nodeApp{}, false
}

// fileContains reports whether a file exists and contains substr.
func fileContains(path, substr string) bool {
	b, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(b), substr)
}

var bootReadyRe = regexp.MustCompile(`(?i)(listening|running on|server started|ready on|API listening|app listening)`)
var bootErrorRe = regexp.MustCompile(`(?i)(Error:|Cannot find module|SyntaxError|TSError|throw new|Cannot read propert|is not a function|is not defined|ELIFECYCLE|UnhandledPromiseRejection|requires a callback function)`)

// missingModules extracts package names from "Cannot find module 'X'" lines,
// keeping only bare package names (a relative path is a code bug, not a dep).
func missingModules(out string) []string {
	var mods []string
	seen := map[string]bool{}
	for _, m := range moduleMissRe.FindAllStringSubmatch(out, -1) {
		spec := m[1]
		if isRelative(spec) {
			continue
		}
		pkg := packageRoot(spec)
		if pkg == "" || isBuiltin(pkg) || seen[pkg] {
			continue
		}
		seen[pkg] = true
		mods = append(mods, pkg)
	}
	return mods
}

var moduleMissRe = regexp.MustCompile(`Cannot find module '([^']+)'`)

// syntaxErrors returns TS syntax errors (TS1xxx) in the app, which flag a broken
// or half-applied edit. Module-resolution errors are ignored here — mid-build a
// file may reference a sibling not yet written; the final boot pass catches those.
func syntaxErrors(ctx context.Context, app *nodeApp) string {
	if !fileExists(filepath.Join(app.dir, "tsconfig.json")) {
		return ""
	}
	out, _ := runTool(ctx, app.dir, 120*time.Second, "npx", "tsc", "--noEmit", "--skipLibCheck")
	lines := tsSyntaxRe.FindAllString(out, -1)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

var tsSyntaxRe = regexp.MustCompile(`(?m)^.*error TS1\d{3}:.*$`)

// installMissingDeps installs every package the app imports but does not have,
// one at a time so one bad name doesn't block the rest. Returns those installed.
func installMissingDeps(ctx context.Context, dir string) []string {
	var installed []string
	for _, pkg := range scanMissingPackages(dir) {
		if _, err := runTool(ctx, dir, 300*time.Second, "npm", "install", pkg); err == nil {
			installed = append(installed, pkg)
		}
	}
	return installed
}

var importRe = regexp.MustCompile(`(?m)(?:\bfrom\s+|\brequire\s*\(\s*|\bimport\s+)['"]([^'"]+)['"]`)

func scanMissingPackages(dir string) []string {
	seen := map[string]bool{}
	var missing []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == "node_modules" || name == "dist" || name == "build" || strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts":
		default:
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return nil
		}
		for _, m := range importRe.FindAllStringSubmatch(string(b), -1) {
			spec := m[1]
			if isRelative(spec) {
				continue
			}
			pkg := packageRoot(spec)
			if pkg == "" || seen[pkg] || isBuiltin(pkg) {
				continue
			}
			seen[pkg] = true
			if !fileExists(filepath.Join(dir, "node_modules", pkg)) {
				missing = append(missing, pkg)
			}
		}
		return nil
	})
	return missing
}

func isRelative(spec string) bool {
	return strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") || strings.HasPrefix(spec, "@/") || strings.HasPrefix(spec, "~")
}

// packageRoot reduces an import specifier to its installable package name:
// "@scope/name/sub" -> "@scope/name", "name/sub" -> "name", "node:fs" -> "".
func packageRoot(spec string) string {
	if strings.HasPrefix(spec, "node:") {
		return ""
	}
	parts := strings.Split(spec, "/")
	if strings.HasPrefix(spec, "@") {
		if len(parts) >= 2 {
			return parts[0] + "/" + parts[1]
		}
		return ""
	}
	return parts[0]
}

var builtins = map[string]bool{
	"assert": true, "buffer": true, "child_process": true, "cluster": true, "console": true,
	"crypto": true, "dgram": true, "dns": true, "domain": true, "events": true, "fs": true,
	"http": true, "http2": true, "https": true, "module": true, "net": true, "os": true,
	"path": true, "perf_hooks": true, "process": true, "punycode": true, "querystring": true,
	"readline": true, "repl": true, "stream": true, "string_decoder": true, "timers": true,
	"tls": true, "trace_events": true, "tty": true, "url": true, "util": true, "v8": true,
	"vm": true, "worker_threads": true, "zlib": true,
}

func isBuiltin(pkg string) bool { return builtins[pkg] }

func runTool(ctx context.Context, dir string, timeout time.Duration, bin string, args ...string) (string, error) {
	return runToolEnv(ctx, dir, timeout, nil, bin, args...)
}

// runToolEnv runs a command with extra environment (KEY=VALUE) layered over the
// process env — used to give seed/boot the project's .env (DATABASE_URL, etc.).
func runToolEnv(ctx context.Context, dir string, timeout time.Duration, env []string, bin string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// appEnvLines reads the project's .env (the app dir and the monorepo root one level
// up) into KEY=VALUE lines, so a subprocess (seed, boot) sees DATABASE_URL/REDIS_URL/
// etc. and hits the SAME database — otherwise the seed and the boot-probe can diverge
// (one on sqlite, one on postgres) and the probe never sees the seeded rows.
func appEnvLines(dir string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range []string{filepath.Join(dir, ".env"), filepath.Join(filepath.Dir(dir), ".env")} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			eq := strings.Index(line, "=")
			if line == "" || strings.HasPrefix(line, "#") || eq <= 0 {
				continue
			}
			key := line[:eq]
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, line)
		}
	}
	return out
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func tailBoot(s string) string {
	const n = 4000
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

// safeBuffer is an io.Writer safe for the child process's stdout/stderr goroutine
// to write while the poll loop reads.
type safeBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
