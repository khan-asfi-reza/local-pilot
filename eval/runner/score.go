// Package runner is the eval loop: run each PRD through the headless agent in a
// fresh sandbox, score it against a machine-checkable manifest, retry below
// threshold, and report a pass rate.
package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Grounding struct {
	Action          string   `json:"action"`
	ExplicitTargets []string `json:"explicit_targets"`
	ForbidNewFiles  bool     `json:"forbid_new_files"`
}

type Check struct {
	ID             string   `json:"id"`
	Type           string   `json:"type"`
	Describe       string   `json:"describe"`
	Weight         float64  `json:"weight"`
	Hard           bool     `json:"hard"`
	Path           string   `json:"path"`
	Run            string   `json:"run"`
	ExpectExit     *int     `json:"expect_exit"`
	StdoutContains string   `json:"stdout_contains"`
	Allow          []string `json:"allow"`
	Criterion      string   `json:"criterion"`
	Rubric         string   `json:"rubric"`
	Files          []string `json:"files"`
}

type Manifest struct {
	Name          string    `json:"name"`
	Difficulty    int       `json:"difficulty"`
	Seed          string    `json:"seed"`
	MaxSteps      int       `json:"max_steps"`
	PassThreshold float64   `json:"pass_threshold"`
	RetryCap      int       `json:"retry_cap"`
	Grounding     Grounding `json:"grounding"`
	Checks        []Check   `json:"checks"`
}

func (m Manifest) threshold() float64 {
	if m.PassThreshold > 0 {
		return m.PassThreshold
	}
	return 0.80
}

func (m Manifest) retries() int {
	if m.RetryCap > 0 {
		return m.RetryCap
	}
	return 2
}

func weightOf(c Check) float64 {
	if c.Weight > 0 {
		return c.Weight
	}
	return 1
}

type CheckResult struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Passed   bool    `json:"passed"`
	Weight   float64 `json:"weight"`
	Hard     bool    `json:"hard"`
	Observed string  `json:"observed"`
}

// scoreAttempt runs the objective checks; a failed hard check forces score 0.
// judge checks are NOT run here — they are deferred to Claude (see recordJudge).
func scoreAttempt(ctx context.Context, m Manifest, sandbox string) ([]CheckResult, float64, bool) {
	mutated := mutatedSet(sandbox)
	var results []CheckResult
	var got, total float64
	hardFail := false
	for _, c := range m.Checks {
		if c.Type == "judge" {
			continue
		}
		r := execCheck(ctx, sandbox, c, mutated)
		results = append(results, r)
		total += r.Weight
		if r.Passed {
			got += r.Weight
		} else if c.Hard {
			hardFail = true
		}
	}
	score := 0.0
	if total > 0 {
		score = got / total
	}
	if hardFail {
		score = 0
	}
	return results, score, score >= m.threshold()
}

func execCheck(ctx context.Context, sandbox string, c Check, mutated map[string]bool) CheckResult {
	res := func(pass bool, observed string) CheckResult {
		return CheckResult{ID: c.ID, Type: c.Type, Passed: pass, Weight: weightOf(c), Hard: c.Hard, Observed: observed}
	}
	switch c.Type {
	case "file_exists":
		return res(fileExists(sandbox, c.Path), c.Describe)
	case "file_absent":
		return res(!fileExists(sandbox, c.Path), c.Describe)
	case "file_unchanged":
		return res(!mutated[normSlash(c.Path)], c.Describe)
	case "mutated_only":
		if extra := notAllowed(mutated, c.Allow); len(extra) > 0 {
			return res(false, "unexpected files changed: "+strings.Join(extra, ", "))
		}
		return res(true, c.Describe)
	case "cmd", "pytest":
		run := c.Run
		if c.Type == "pytest" {
			p := c.Path
			if p == "" {
				p = "."
			}
			run = "python3 -m pytest -q " + p
		}
		out, code := runInSandbox(ctx, sandbox, run)
		want := 0
		if c.ExpectExit != nil {
			want = *c.ExpectExit
		}
		ok := code == want && (c.StdoutContains == "" || strings.Contains(out, c.StdoutContains))
		return res(ok, trimOut(out))
	default:
		return res(false, "unknown check type: "+c.Type)
	}
}

func fileExists(sandbox, rel string) bool {
	_, err := os.Stat(filepath.Join(sandbox, rel))
	return err == nil
}

func normSlash(p string) string {
	return filepath.ToSlash(filepath.Clean(strings.TrimSpace(p)))
}

var ignoredSeg = map[string]bool{
	".pilot": true, ".git": true, ".harness": true, ".venv": true, "venv": true,
	"node_modules": true, "__pycache__": true, ".pytest_cache": true,
}

// mutatedSet returns project files the agent created or changed (git status),
// excluding tool/agent state.
func mutatedSet(sandbox string) map[string]bool {
	out := gitOut(sandbox, "status", "--porcelain")
	set := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if arrow := strings.Index(path, " -> "); arrow >= 0 {
			path = path[arrow+4:]
		}
		path = normSlash(strings.Trim(path, "\""))
		if path == "" || path == "." {
			continue
		}
		if seg := strings.SplitN(path, "/", 2)[0]; ignoredSeg[seg] {
			continue
		}
		set[path] = true
	}
	return set
}

func notAllowed(mutated map[string]bool, allow []string) []string {
	allowed := map[string]bool{}
	base := map[string]bool{}
	for _, a := range allow {
		na := normSlash(a)
		allowed[na] = true
		if !strings.Contains(na, "/") {
			base[na] = true
		}
	}
	var extra []string
	for p := range mutated {
		if allowed[p] || base[filepath.Base(p)] {
			continue
		}
		extra = append(extra, p)
	}
	return extra
}

func gitInitBaseline(sandbox string) {
	runGit(sandbox, "init", "-q")
	runGit(sandbox, "config", "user.email", "eval@local")
	runGit(sandbox, "config", "user.name", "eval")
	runGit(sandbox, "add", "-A")
	runGit(sandbox, "commit", "-q", "-m", "seed", "--allow-empty")
}

func runGit(sandbox string, args ...string) {
	_ = exec.Command("git", append([]string{"-C", sandbox}, args...)...).Run()
}

func gitOut(sandbox string, args ...string) string {
	out, _ := exec.Command("git", append([]string{"-C", sandbox}, args...)...).Output()
	return string(out)
}

func runInSandbox(ctx context.Context, sandbox, command string) (string, int) {
	cctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "sh", "-c", command)
	cmd.Dir = sandbox
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 127
		}
	}
	return string(out), code
}

func trimOut(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}
