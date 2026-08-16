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
				fmt.Sprintf("DATABASE_URL=postgresql://pilot:pilot@127.0.0.1:%d/app", p),
				"POSTGRES_HOST=127.0.0.1", fmt.Sprintf("POSTGRES_PORT=%d", p),
				"POSTGRES_USER=pilot", "POSTGRES_PASSWORD=pilot", "POSTGRES_DB=app",
			}
		},
	},
	"mysql": {
		Image: "mysql:9", Port: 3306,
		RunArgs: []string{"-e", "MYSQL_ROOT_PASSWORD=pilot", "-e", "MYSQL_DATABASE=app"},
		EnvFor: func(p int) []string {
			return []string{
				fmt.Sprintf("DATABASE_URL=mysql://root:pilot@127.0.0.1:%d/app", p),
				"MYSQL_HOST=127.0.0.1", fmt.Sprintf("MYSQL_PORT=%d", p),
				"MYSQL_USER=root", "MYSQL_PASSWORD=pilot", "MYSQL_DATABASE=app",
			}
		},
	},
	"redis": {
		Image: "redis:8-alpine", Port: 6379,
		EnvFor: func(p int) []string {
			return []string{fmt.Sprintf("REDIS_URL=redis://127.0.0.1:%d", p), "REDIS_HOST=127.0.0.1", fmt.Sprintf("REDIS_PORT=%d", p)}
		},
	},
	"mongodb": {
		Image: "mongo:8", Port: 27017,
		EnvFor: func(p int) []string {
			return []string{fmt.Sprintf("MONGODB_URI=mongodb://127.0.0.1:%d/app", p), "MONGODB_HOST=127.0.0.1", fmt.Sprintf("MONGODB_PORT=%d", p)}
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

// provision wires a project to backing services WITHOUT blasting docker: it reuses
// ONE shared container per service kind (pilot-shared-<kind>, started once and
// shared by every project) and gives this project its own namespace inside it — a
// dedicated postgres/mysql DATABASE and a redis logical-DB index — so many projects
// coexist on a single infra instance. It also assigns this project unique host ports
// (PORT for the API, VITE_PORT for the frontend) so several apps run at once without
// clashing. Writes .env and returns the env text. No-ops when docker is unavailable.
func (o *Orchestrator) provision(ctx context.Context, prompt, workDir string, emit func(events.Event)) string {
	// Idempotent: never clobber an existing .env or re-provision.
	if _, err := os.Stat(filepath.Join(workDir, ".env")); err == nil {
		return ""
	}
	slug := projectSlug(workDir)
	var lines []string

	if dockerAvailable() {
		kinds, err := Provision(ctx, o.planner, prompt)
		if err != nil || len(kinds) == 0 {
			if err != nil {
				emit(events.Text("\n[service planner failed (" + err.Error() + "); inferring services from the spec]\n"))
			}
			kinds = inferServices(prompt)
		}
		for _, kind := range kinds {
			svc, ok := catalog[kind]
			if !ok {
				continue
			}
			port, err := sharedInfra(kind, svc)
			if err != nil {
				emit(events.Text("\n[could not start shared " + kind + " (" + err.Error() + ")]\n"))
				continue
			}
			switch kind {
			case "postgres":
				if err := ensurePGDatabase(port, slug); err != nil {
					emit(events.Text("\n[warn: could not create database " + slug + ": " + err.Error() + "]\n"))
				}
				lines = append(lines,
					fmt.Sprintf("DATABASE_URL=postgresql://pilot:pilot@127.0.0.1:%d/%s", port, slug),
					"POSTGRES_HOST=127.0.0.1", fmt.Sprintf("POSTGRES_PORT=%d", port),
					"POSTGRES_USER=pilot", "POSTGRES_PASSWORD=pilot", "POSTGRES_DB="+slug)
				emit(events.Text(fmt.Sprintf("\n[postgres: shared pilot-shared-postgres:%d, database '%s']\n", port, slug)))
			case "mysql":
				if err := ensureMySQLDatabase(port, slug); err != nil {
					emit(events.Text("\n[warn: could not create mysql database " + slug + ": " + err.Error() + "]\n"))
				}
				lines = append(lines,
					fmt.Sprintf("DATABASE_URL=mysql://root:pilot@127.0.0.1:%d/%s", port, slug),
					"MYSQL_HOST=127.0.0.1", fmt.Sprintf("MYSQL_PORT=%d", port),
					"MYSQL_USER=root", "MYSQL_PASSWORD=pilot", "MYSQL_DATABASE="+slug)
				emit(events.Text(fmt.Sprintf("\n[mysql: shared pilot-shared-mysql:%d, database '%s']\n", port, slug)))
			case "redis":
				idx := redisIndex(slug)
				lines = append(lines,
					fmt.Sprintf("REDIS_URL=redis://127.0.0.1:%d/%d", port, idx),
					"REDIS_HOST=127.0.0.1", fmt.Sprintf("REDIS_PORT=%d", port),
					fmt.Sprintf("REDIS_DB=%d", idx), "REDIS_NAMESPACE="+slug)
				emit(events.Text(fmt.Sprintf("\n[redis: shared pilot-shared-redis:%d, logical db %d, namespace '%s']\n", port, idx, slug)))
			case "mongodb":
				lines = append(lines,
					fmt.Sprintf("MONGODB_URI=mongodb://127.0.0.1:%d/%s", port, slug),
					"MONGODB_HOST=127.0.0.1", fmt.Sprintf("MONGODB_PORT=%d", port), "MONGODB_DB="+slug)
				emit(events.Text(fmt.Sprintf("\n[mongodb: shared pilot-shared-mongodb:%d, database '%s']\n", port, slug)))
			}
		}
	} else {
		emit(events.Text("\n[docker not available — services use in-app fallbacks (e.g. sqlite); no .env services]\n"))
	}

	// Assign unique app ports so multiple generated apps run at once. Spread the
	// starting point by project so two projects rarely target the same port.
	apiPort := freePort(8000 + int(hashMod(slug, 60))*10)
	vitePort := freePort(5173 + int(hashMod(slug, 60))*2)
	lines = append(lines, fmt.Sprintf("PORT=%d", apiPort), fmt.Sprintf("VITE_PORT=%d", vitePort))
	emit(events.Text(fmt.Sprintf("\n[ports: API :%d, frontend :%d]\n", apiPort, vitePort)))

	env := strings.Join(lines, "\n") + "\n"
	_ = os.WriteFile(filepath.Join(workDir, ".env"), []byte(env), 0o644)
	return env
}

// projectSlug derives a DB/namespace-safe name from the project dir (lowercase,
// alnum + underscore, leading letter) so it is a valid postgres database name.
func projectSlug(workDir string) string {
	base := strings.ToLower(filepath.Base(workDir))
	var b strings.Builder
	for _, r := range base {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" || (s[0] >= '0' && s[0] <= '9') {
		s = "app_" + s
	}
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

// hashMod returns a stable non-negative hash of s modulo n.
func hashMod(s string, n int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return int(h.Sum32() % uint32(n))
}

// redisIndex maps a project to one of redis' 16 logical DBs.
func redisIndex(slug string) int { return hashMod(slug, 16) }

// sharedInfra ensures ONE shared container for a service kind (pilot-shared-<kind>),
// started once and reused by every project, and returns its published host port.
func sharedInfra(kind string, s svcConf) (int, error) {
	name := "pilot-shared-" + kind
	if port := runningHostPort(name, s.Port); port > 0 {
		return port, nil
	}
	// Not running: remove any stopped remnant and start it on a free host port.
	return startContainer(name, s)
}

// runningHostPort returns the host port a running container publishes for the given
// internal port, or 0 if the container is not running.
func runningHostPort(name string, internal int) int {
	out, err := exec.Command("docker", "ps", "--filter", "name=^"+name+"$",
		"--format", "{{.Ports}}").CombinedOutput()
	if err != nil {
		return 0
	}
	// e.g. "0.0.0.0:5433->5432/tcp, [::]:5433->5432/tcp"
	for _, part := range strings.Split(string(out), ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, fmt.Sprintf("->%d/", internal)) {
			continue
		}
		if i := strings.LastIndex(part[:strings.Index(part, "->")], ":"); i >= 0 {
			var p int
			if _, e := fmt.Sscanf(part[i+1:strings.Index(part, "->")], "%d", &p); e == nil {
				return p
			}
		}
	}
	return 0
}

// ensurePGDatabase creates the per-project database in the shared postgres if it
// does not already exist (createdb errors harmlessly when it does).
func ensurePGDatabase(port int, slug string) error {
	// Wait for postgres to accept connections, then create the db (idempotent).
	_ = exec.Command("docker", "exec", "pilot-shared-postgres", "sh", "-c",
		"for i in $(seq 1 30); do pg_isready -U pilot && break; sleep 1; done").Run()
	out, err := exec.Command("docker", "exec", "pilot-shared-postgres",
		"createdb", "-U", "pilot", slug).CombinedOutput()
	if err != nil && strings.Contains(strings.ToLower(string(out)), "already exists") {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// ensureMySQLDatabase creates the per-project database in the shared mysql.
func ensureMySQLDatabase(port int, slug string) error {
	_ = exec.Command("docker", "exec", "pilot-shared-mysql", "sh", "-c",
		"for i in $(seq 1 30); do mysqladmin ping -uroot -ppilot --silent && break; sleep 1; done").Run()
	out, err := exec.Command("docker", "exec", "pilot-shared-mysql",
		"mysql", "-uroot", "-ppilot", "-e", "CREATE DATABASE IF NOT EXISTS `"+slug+"`").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
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
		// --restart unless-stopped so shared infra survives a docker daemon restart
		// (otherwise every project pointing at it breaks when the daemon bounces).
		args := []string{"run", "-d", "--restart", "unless-stopped", "--name", name, "-p", fmt.Sprintf("%d:%d", port, s.Port)}
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
