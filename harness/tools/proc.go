package tools

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type bgProc struct {
	label string
	cmd   *exec.Cmd
	log   *bytes.Buffer
}

// ProcSet tracks background servers started during one request so they can all
// be stopped when the turn ends. Safe for concurrent use and when nil.
type ProcSet struct {
	mu    sync.Mutex
	procs []*bgProc
}

func NewProcSet() *ProcSet { return &ProcSet{} }

func (p *ProcSet) add(bp *bgProc) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.procs = append(p.procs, bp)
	p.mu.Unlock()
}

// StopAll kills every tracked process group. Safe on a nil set and to call twice.
func (p *ProcSet) StopAll() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, bp := range p.procs {
		killProcess(bp.cmd)
	}
	p.procs = nil
}

// waitForPort dials localhost:port until it connects or the timeout passes.
func waitForPort(port int, timeout time.Duration) bool {
	if port <= 0 {
		return false
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

// waitForServer waits until the server is reachable, preferring the port parsed
// from its startup banner over the caller's expected port (which itself should be
// the provisioned .env port, not a guess). Returns the resolved port and whether it
// opened before the timeout.
func waitForServer(logBuf *bytes.Buffer, expected int, timeout time.Duration) (int, bool) {
	resolved := expected
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if logBuf != nil {
			if bp := parseBoundPort(logBuf.String()); bp > 0 {
				resolved = bp
			}
		}
		if resolved > 0 {
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", resolved), 500*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				return resolved, true
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return resolved, false
}

var boundPortRes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)https?://[^\s/]*:(\d{2,5})`),            // Local: http://localhost:5173 | Uvicorn running on http://127.0.0.1:8000
	regexp.MustCompile(`(?i)listening on\s+\S*?:(\d{2,5})`),         // listening on 0.0.0.0:8360
	regexp.MustCompile(`(?i)listening on port\s+(\d{2,5})`),         // listening on port 8360
	regexp.MustCompile(`(?i)running on port\s+(\d{2,5})`),           // server running on port 8360
}

// parseBoundPort scans server startup output for the port it actually bound.
func parseBoundPort(logText string) int {
	for _, re := range boundPortRes {
		if m := re.FindStringSubmatch(logText); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

// readEnvPort reads KEY=<port> from workDir/.env (or ../.env for a monorepo backend)
// and returns it, or 0 if absent/unparsable. This is the port the app actually binds.
func readEnvPort(workDir, key string) int {
	for _, rel := range []string{".env", filepath.Join("..", ".env")} {
		data, err := os.ReadFile(filepath.Join(workDir, rel))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, key+"=") {
				v := strings.TrimSpace(strings.TrimPrefix(line, key+"="))
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					return n
				}
			}
		}
	}
	return 0
}

// isFrontendServeCmd reports whether a serve command starts a Vite/frontend dev
// server, so the VITE_PORT from .env applies rather than PORT.
func isFrontendServeCmd(command string) bool {
	c := strings.ToLower(command)
	for _, sig := range []string{"vite", "npm run dev", "yarn dev", "pnpm dev", "next dev", "ng serve"} {
		if strings.Contains(c, sig) {
			return true
		}
	}
	return false
}

var serverSignatures = []string{
	"uvicorn", "gunicorn", "hypercorn", "daphne", "waitress-serve",
	"flask run", "fastapi run", "runserver", "http.server",
	"npm start", "npm run dev", "npm run start", "yarn dev", "yarn start", "pnpm dev",
	"next dev", "ng serve", "webpack serve", "webpack-dev-server", "nodemon",
	"http-server", "live-server",
	"php -s", "artisan serve", "symfony serve", "symfony server",
	"rails server", "rails s ", "rackup", "puma", "unicorn",
	"spring-boot:run", "phx.server",
}

// ServerStartHint returns a short reason if the command starts a long-running
// server (which must use the serve tool, not shell_run), or "" otherwise.
func ServerStartHint(command string) string {
	c := strings.ToLower(command)
	for _, sig := range serverSignatures {
		if strings.Contains(c, sig) {
			return "starts a long-running server (matched '" + strings.TrimSpace(sig) + "')"
		}
	}
	if strings.Contains(c, "--reload") || strings.Contains(c, "--watch") {
		return "runs in reload/watch mode, which does not exit"
	}
	return ""
}

// NonVenvPipHint returns a redirect message if the command installs Python
// packages with bare pip (no virtualenv), which fails on managed systems, or ""
// otherwise.
func NonVenvPipHint(command string) string {
	c := strings.ToLower(command)
	installsPip := (strings.Contains(c, "pip install") || strings.Contains(c, "pip3 install") || strings.Contains(c, "-m pip install"))
	if !installsPip {
		return ""
	}
	if strings.Contains(c, ".venv/") || strings.Contains(c, "venv/bin") || strings.Contains(c, "/venv/") {
		return "" // already using a virtualenv
	}
	return "installs Python packages with bare pip, which fails on this system"
}

// DepsInstallFailure reports whether a result looks like a dependency install
// that failed for an environment reason a virtualenv would fix.
func DepsInstallFailure(result string) bool {
	r := strings.ToLower(result)
	for _, sig := range []string{
		"externally-managed", "pip: command not found",
		"no module named pip", "no such file or directory: 'pip'",
	} {
		if strings.Contains(r, sig) {
			return true
		}
	}
	return false
}

// shellEscapesWorkDir reports whether a shell command would touch the filesystem
// outside workDir, so dispatch can require approval. It errs toward flagging.
func shellEscapesWorkDir(command, workDir string) bool {
	if strings.TrimSpace(command) == "" {
		return false
	}
	absWork, err := filepath.Abs(workDir)
	if err != nil {
		return false
	}
	for _, tok := range strings.Fields(command) {
		tok = strings.Trim(tok, "\"'`")
		if tok == "" || strings.HasPrefix(tok, "-") || strings.Contains(tok, "://") {
			continue
		}
		var p string
		switch {
		case strings.HasPrefix(tok, "~"):
			return true
		case strings.HasPrefix(tok, "/"):
			p = filepath.Clean(tok)
		case strings.Contains(tok, "/") || tok == ".." || strings.HasPrefix(tok, ".."):
			p = filepath.Clean(filepath.Join(absWork, tok))
		default:
			continue
		}
		if !withinDir(absWork, p) {
			return true
		}
	}
	return false
}

func withinDir(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false // climbs out of base
	}
	return !filepath.IsAbs(rel)
}
