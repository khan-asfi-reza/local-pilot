package lang

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// scaffoldEnv returns the process environment with framework/database config
// stripped, so a generator building a FRESH project is never hijacked by ANOTHER
// project's settings inherited from the user's shell (e.g. DJANGO_SETTINGS_MODULE
// or DATABASE_URL pointing elsewhere). Mirrors tools.projectEnv, which is
// unexported in that package. CI=1 makes interactive generators non-interactive.
func scaffoldEnv() []string {
	drop := map[string]bool{
		"DJANGO_SETTINGS_MODULE": true, "DJANGO_CONFIGURATION": true,
		"FLASK_APP": true, "FLASK_ENV": true, "FLASK_DEBUG": true,
		"RAILS_ENV": true, "RACK_ENV": true, "ASPNETCORE_ENVIRONMENT": true,
		"NODE_ENV": true,
	}
	var out []string
	for _, kv := range os.Environ() {
		k := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			k = kv[:i]
		}
		if drop[k] || strings.Contains(k, "DATABASE_URL") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "CI=1", "PIP_DISABLE_PIP_VERSION_CHECK=1")
}

// runCmd runs one argv command (NO shell) in dir with a sanitized environment
// and no stdin, so a generator can never block on a prompt or run smuggled shell
// syntax. It returns a trimmed combined-output error on non-zero exit.
func runCmd(ctx context.Context, dir, bin string, args []string) error {
	// A bin with a path separator (e.g. ".venv/bin/pip") is a project-relative
	// tool: resolve it against dir, since exec would otherwise resolve a relative
	// path against the process CWD, not cmd.Dir. A bare name is looked up on PATH.
	resolved := bin
	if strings.ContainsAny(bin, `/\`) {
		if !filepath.IsAbs(bin) {
			resolved = filepath.Join(dir, bin)
		}
		if _, err := os.Stat(resolved); err != nil {
			return fmt.Errorf("%s not found", bin)
		}
	} else if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("%s not found on PATH", bin)
	}
	cmd := exec.CommandContext(ctx, resolved, args...) // argv, no shell
	cmd.Dir = dir
	cmd.Env = scaffoldEnv()
	cmd.Stdin = nil
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(out.String())
		if len(msg) > 2000 {
			msg = msg[len(msg)-2000:]
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s %s: %s", bin, strings.Join(args, " "), msg)
	}
	return nil
}

// haveAll reports whether every binary is on PATH.
func haveAll(bins ...string) bool {
	for _, b := range bins {
		if _, err := exec.LookPath(b); err != nil {
			return false
		}
	}
	return true
}
