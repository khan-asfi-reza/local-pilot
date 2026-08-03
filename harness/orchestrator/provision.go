package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"harness/harness/events"
)

// svcConf is a known backing service the harness can dockerize.
type svcConf struct {
	Image   string
	Port    int
	RunArgs []string
	EnvFor  func(port int) []string
}

// catalog maps a service kind to its docker config + the .env it contributes.
// Images are pinned to current major versions.
var catalog = map[string]svcConf{
	"postgres": {
		Image: "postgres:17-alpine", Port: 5432,
		RunArgs: []string{"-e", "POSTGRES_USER=pilot", "-e", "POSTGRES_PASSWORD=pilot", "-e", "POSTGRES_DB=app"},
		EnvFor: func(p int) []string {
			return []string{
				fmt.Sprintf("DATABASE_URL=postgresql://pilot:pilot@localhost:%d/app", p),
				"POSTGRES_HOST=localhost", fmt.Sprintf("POSTGRES_PORT=%d", p),
				"POSTGRES_USER=pilot", "POSTGRES_PASSWORD=pilot", "POSTGRES_DB=app",
			}
		},
	},
	"mysql": {
		Image: "mysql:9", Port: 3306,
		RunArgs: []string{"-e", "MYSQL_ROOT_PASSWORD=pilot", "-e", "MYSQL_DATABASE=app"},
		EnvFor: func(p int) []string {
			return []string{
				fmt.Sprintf("DATABASE_URL=mysql://root:pilot@localhost:%d/app", p),
				"MYSQL_HOST=localhost", fmt.Sprintf("MYSQL_PORT=%d", p),
				"MYSQL_USER=root", "MYSQL_PASSWORD=pilot", "MYSQL_DATABASE=app",
			}
		},
	},
	"redis": {
		Image: "redis:8-alpine", Port: 6379,
		EnvFor: func(p int) []string {
			return []string{fmt.Sprintf("REDIS_URL=redis://localhost:%d", p), "REDIS_HOST=localhost", fmt.Sprintf("REDIS_PORT=%d", p)}
		},
	},
	"mongodb": {
		Image: "mongo:8", Port: 27017,
		EnvFor: func(p int) []string {
			return []string{fmt.Sprintf("MONGODB_URI=mongodb://localhost:%d/app", p), "MONGODB_HOST=localhost", fmt.Sprintf("MONGODB_PORT=%d", p)}
		},
	},
}

const provisionSchema = `{"type":"object","properties":{"services":{"type":"array","items":{"type":"string","enum":["postgres","mysql","redis","mongodb"]}}},"required":["services"]}`

// Provision asks the model which backing services the project needs (from the
// known catalog only).
func Provision(ctx context.Context, p Planner, prompt string) ([]string, error) {
	sys := "List the backing services this project needs to RUN, choosing only from: postgres, mysql, redis, " +
		"mongodb. Return exactly the ones the spec requires (a Postgres app needs postgres; a caching/session/" +
		"real-time app may also need redis). Return [] if none apply. Output ONLY the JSON."
	raw, err := p.PlanJSON(ctx, sys, clip(prompt, 8000), json.RawMessage(provisionSchema))
	if err != nil {
		return nil, err
	}
	var out struct {
		Services []string `json:"services"`
	}
	if json.Unmarshal([]byte(raw), &out) != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var svcs []string
	for _, s := range out.Services {
		s = strings.ToLower(strings.TrimSpace(s))
		if _, ok := catalog[s]; ok && !seen[s] {
			seen[s] = true
			svcs = append(svcs, s)
		}
	}
	return svcs, nil
}

// provision dockerizes the services the model picked, choosing a free host port
// per service (default, else the next open one), writes .env, and returns the
// env text to inject into the build. No-ops when docker is unavailable.
func (o *Orchestrator) provision(ctx context.Context, prompt, workDir string, emit func(events.Event)) string {
	// Idempotent: never clobber an existing .env or double-start containers.
	if _, err := os.Stat(filepath.Join(workDir, ".env")); err == nil {
		return ""
	}
	if !dockerAvailable() {
		emit(events.Text("\n[docker not available — skipping service provisioning; .env not written]\n"))
		return ""
	}
	// Ask the model which services the spec needs; if that call fails (e.g. a
	// client blip cancels it) or comes back empty, fall back to scanning the spec
	// text so provisioning never silently no-ops on a spec that names its infra.
	kinds, err := Provision(ctx, o.planner, prompt)
	if err != nil || len(kinds) == 0 {
		if err != nil {
			emit(events.Text("\n[service planner failed (" + err.Error() + "); inferring services from the spec]\n"))
		}
		kinds = inferServices(prompt)
	}
	if len(kinds) == 0 {
		return ""
	}
	var lines []string
	for _, kind := range kinds {
		svc := catalog[kind]
		name := fmt.Sprintf("pilot-%s-%s", shortHash(workDir), kind)
		port, err := startContainer(name, svc)
		if err != nil {
			emit(events.Text("\n[could not start " + kind + " (" + err.Error() + ")]\n"))
			continue
		}
		emit(events.Text(fmt.Sprintf("\n[provisioned %s → localhost:%d (docker: %s)]\n", kind, port, name)))
		lines = append(lines, svc.EnvFor(port)...)
	}
	if len(lines) == 0 {
		return ""
	}
	env := strings.Join(lines, "\n") + "\n"
	_ = os.WriteFile(filepath.Join(workDir, ".env"), []byte(env), 0o644)
	return env
}

func dockerAvailable() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	return exec.Command("docker", "info").Run() == nil
}

// portFree reports whether p is bindable on ALL interfaces and both IP families.
// A 127.0.0.1-only check misses a container already publishing on 0.0.0.0/[::]
// (common on Docker Desktop, where the conflict is the IPv6 bind), so binding
// ":p" — which covers 0.0.0.0 and [::] — is what actually detects a taken port.
func portFree(p int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// freePort returns the first bindable port at or after start.
func freePort(start int) int {
	for p := start; p < start+500; p++ {
		if portFree(p) {
			return p
		}
	}
	return start
}

// startContainer starts the service on a host port that is actually free: it
// tries the service default, then the next free ports, and — because the host
// port check can still race docker's own allocator — retries on the next port
// whenever docker itself reports the port already allocated. Returns the bound port.
func startContainer(name string, s svcConf) (int, error) {
	_ = exec.Command("docker", "rm", "-f", name).Run()
	port := s.Port
	var lastErr error
	for i := 0; i < 50; i++ {
		port = freePort(port)
		args := []string{"run", "-d", "--name", name, "-p", fmt.Sprintf("%d:%d", port, s.Port)}
		args = append(args, s.RunArgs...)
		args = append(args, s.Image)
		out, err := exec.Command("docker", args...).CombinedOutput()
		if err == nil {
			return port, nil
		}
		lastErr = fmt.Errorf("%s", strings.TrimSpace(string(out)))
		if portAllocated(string(out)) {
			// docker created the container then failed to publish the port; drop it
			// so the name is free, then try the next port.
			_ = exec.Command("docker", "rm", "-f", name).Run()
			port++
			continue
		}
		return 0, lastErr
	}
	return 0, lastErr
}

// portAllocated reports whether a docker error is a host-port conflict, which we
// recover from by moving to the next port (vs. a real failure we surface).
func portAllocated(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "port is already allocated") ||
		strings.Contains(m, "address already in use") ||
		strings.Contains(m, "bind for")
}

func shortHash(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("%08x", h.Sum32())
}
