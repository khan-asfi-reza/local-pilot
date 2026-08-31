// Package ports knows which TCP ports Local Pilot itself runs on, so a generated
// app never binds one and no command frees one. Binding or killing one of these
// takes down the UI the user is watching from inside their own build.
package ports

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The stack pilot starts: Vite UI, backend API, harness, ollama.
var defaults = []int{5173, 8182, 9000, 11434}

// EnvKey carries the live list from the pilot launcher to every child process.
const EnvKey = "PILOT_RESERVED_PORTS"

// Reserved returns the ports Local Pilot occupies, from the launcher's env when
// it set one, else the built-in stack defaults.
func Reserved() []int {
	raw := strings.TrimSpace(os.Getenv(EnvKey))
	if raw == "" {
		return append([]int(nil), defaults...)
	}
	var out []int
	for _, f := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' }) {
		if n, err := strconv.Atoi(strings.TrimSpace(f)); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return append([]int(nil), defaults...)
	}
	return out
}

// IsReserved reports whether p belongs to Local Pilot's own stack.
func IsReserved(p int) bool {
	for _, r := range Reserved() {
		if p == r {
			return true
		}
	}
	return false
}

// Describe names what runs on a reserved port, for an error the model can act on.
func Describe(p int) string {
	switch p {
	case 5173:
		return "the Local Pilot web UI"
	case 8182:
		return "the Local Pilot backend API"
	case 9000:
		return "the harness server"
	case 11434:
		return "ollama"
	}
	return "part of Local Pilot"
}

// InUse reports whether something is already listening on p.
func InUse(p int) bool {
	if p <= 0 {
		return false
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", p), 400*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Free returns the first bindable port at or after start that Local Pilot does
// not already own.
func Free(start int) int {
	if start < 1024 {
		start = 1024
	}
	for p := start; p < start+500 && p < 65535; p++ {
		if IsReserved(p) || InUse(p) {
			continue
		}
		if l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p)); err == nil {
			_ = l.Close()
			return p
		}
	}
	return start
}

var killVerbs = []string{"kill", "pkill", "killall", "fuser", "kill-port", "taskkill", "stop-process"}

// Broad process-name kills that always take Local Pilot's own node/python stack
// down with the app's, however the command is written.
var blanketKillTargets = []string{"node", "vite", "npm", "uvicorn", "python", "python3", "esbuild"}

var portRe = regexp.MustCompile(`\b(\d{2,5})\b`)

// KillHint returns a reason to refuse a command that would kill Local Pilot's own
// processes, or "" when the command is safe to run.
//
// A model whose dev server hits "port in use" reaches for `kill $(lsof -t -i:5173)`,
// which shuts down the very UI the user is working in. There is no legitimate
// reason for a build to free one of Pilot's ports, so this is refused outright.
func KillHint(command string) string {
	lower := strings.ToLower(command)
	verb := ""
	for _, v := range killVerbs {
		if containsWord(lower, v) {
			verb = v
			break
		}
	}
	if verb == "" {
		return ""
	}
	for _, m := range portRe.FindAllStringSubmatch(command, -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && IsReserved(n) {
			return fmt.Sprintf("would kill whatever holds port %d, which is %s - not your app", n, Describe(n))
		}
	}
	if verb == "pkill" || verb == "killall" {
		for _, t := range blanketKillTargets {
			if containsWord(lower, t) {
				return "would kill every '" + t + "' process on this machine, including the ones running Local Pilot itself"
			}
		}
	}
	return ""
}

// containsWord reports whether s contains tok bounded by non-word characters, so
// "kill" matches in "kill -9" and "$(kill" but not in "killer" or "skill".
func containsWord(s, tok string) bool {
	for i := 0; i+len(tok) <= len(s); i++ {
		if s[i:i+len(tok)] != tok {
			continue
		}
		if i > 0 && isWordByte(s[i-1]) {
			continue
		}
		if j := i + len(tok); j < len(s) && isWordByte(s[j]) {
			continue
		}
		return true
	}
	return false
}

func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-'
}
