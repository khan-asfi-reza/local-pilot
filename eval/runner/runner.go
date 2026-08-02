package runner

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"harness/harness/model"
)

type Attempt struct {
	N       int           `json:"attempt"`
	Sandbox string        `json:"sandbox"`
	Results []CheckResult `json:"results"`
	Score   float64       `json:"score"`
	Passed  bool          `json:"passed"`
	Output  string        `json:"-"`
}

type PRDReport struct {
	Name         string        `json:"name"`
	Difficulty   int           `json:"difficulty"`
	Attempts     []Attempt     `json:"attempts"`
	BestScore    float64       `json:"best_score"`
	Passed       bool          `json:"passed"`
	AttemptsUsed int           `json:"attempts_used"`
	Duration     time.Duration `json:"duration_ms"`
}

type Report struct {
	PRDs         []PRDReport   `json:"prds"`
	Passed       int           `json:"passed"`
	Total        int           `json:"total"`
	MeanScore    float64       `json:"mean_score"`
	MeanAttempts float64       `json:"mean_attempts"`
	Duration     time.Duration `json:"duration_ms"`
}

type Driver struct {
	pilotBin         string
	cfgPath          string
	evalDir          string
	sandboxRoot      string
	maxStepsOverride int
	keep             bool
	memLog           string
	judgeQueue       string
	bg               sync.WaitGroup
}

// Run is the `pilot eval` entry: run the PRD ladder small→large and report a pass rate.
func Run(argv []string) {
	fs := flag.NewFlagSet("pilot eval", flag.ExitOnError)
	configPath := fs.String("config", "", "path to the model registry")
	evalDir := fs.String("eval-dir", "", "directory holding prds/ checks/ fixtures/ (default: ./eval)")
	sandboxRoot := fs.String("sandbox", "", "root for attempt sandboxes (default: tests/sandbox/eval)")
	only := fs.String("only", "", "run only PRDs whose name contains this substring")
	keep := fs.Bool("keep", false, "keep sandboxes after the run for debugging")
	maxSteps := fs.Int("max-steps", 0, "override per-attempt step cap (0 = each manifest's value)")
	_ = fs.Parse(argv)

	if *configPath == "" {
		fatalf("no --config given (run via `pilot eval`)")
	}
	dir := *evalDir
	if dir == "" {
		dir = findEvalDir()
	}
	if !isDir(filepath.Join(dir, "checks")) {
		fatalf("eval data not found at %q (expected a checks/ subdir); pass --eval-dir", dir)
	}
	sbRoot := *sandboxRoot
	if sbRoot == "" {
		sbRoot = filepath.Join("tests", "sandbox", "eval")
	}
	self, err := os.Executable()
	if err != nil {
		fatalf("locate pilot binary: %v", err)
	}
	cfg, err := model.LoadConfig(*configPath)
	if err != nil {
		fatalf("%v", err)
	}
	if _, url, err := cfg.Active(); err != nil || !model.NewRouter(cfg, model.NewClient()).Reachable(url) {
		fmt.Fprintln(os.Stderr, yellow("warning: the active model backend is not reachable. Start ollama and the model first."))
	}

	manifests := loadManifests(filepath.Join(dir, "checks"), *only)
	if len(manifests) == 0 {
		fatalf("no manifests found in %q", filepath.Join(dir, "checks"))
	}

	d := &Driver{
		pilotBin: self, cfgPath: *configPath, evalDir: dir, sandboxRoot: sbRoot,
		maxStepsOverride: *maxSteps, keep: *keep,
		memLog:     filepath.Join(dir, "reports", "memory-log.jsonl"),
		judgeQueue: filepath.Join(dir, "reports", "judge-queue.jsonl"),
	}
	rep := d.loop(context.Background(), manifests)
	d.bg.Wait()
	printReport(rep)
	writeReport(dir, rep)
}

func (d *Driver) loop(ctx context.Context, manifests []Manifest) Report {
	rep := Report{Total: len(manifests)}
	start := time.Now()
	fmt.Printf("%-28s %-4s %-6s %-5s %s\n", "PRD", "Diff", "Score", "Pass", "Attempts")
	fmt.Println(strings.Repeat("-", 58))

	for _, m := range manifests {
		prd := d.readPRD(m.Name)
		pr := PRDReport{Name: m.Name, Difficulty: m.Difficulty}
		pstart := time.Now()
		feedback := ""
		var lastSandbox string
		for n := 0; n <= m.retries(); n++ {
			at := d.runAttempt(ctx, m, prd, feedback, n)
			at.Results, at.Score, at.Passed = scoreAttempt(ctx, m, at.Sandbox)
			lastSandbox = at.Sandbox
			d.scheduleMemoryLog(m, at)
			pr.Attempts = append(pr.Attempts, at)
			if at.Score > pr.BestScore {
				pr.BestScore = at.Score
			}
			if at.Passed {
				pr.Passed = true
				pr.AttemptsUsed = n + 1
				break
			}
			feedback = buildFeedback(at.Results)
			if !d.keep && n < m.retries() {
				_ = os.RemoveAll(at.Sandbox)
			}
		}
		if !pr.Passed {
			pr.AttemptsUsed = m.retries() + 1
		}
		d.recordJudge(m, lastSandbox)
		pr.Duration = time.Since(pstart)
		rep.PRDs = append(rep.PRDs, pr)
		printPRDLine(pr)
	}
	rep.Duration = time.Since(start)
	aggregate(&rep)
	return rep
}

// runAttempt sets up a fresh git-baselined sandbox and runs the agent as an isolated process.
func (d *Driver) runAttempt(ctx context.Context, m Manifest, prd, feedback string, n int) Attempt {
	sb := filepath.Join(d.sandboxRoot, m.Name, fmt.Sprintf("attempt-%d", n))
	_ = os.RemoveAll(sb)
	_ = os.MkdirAll(sb, 0o755)
	if m.Seed != "" {
		_ = copyTree(filepath.Join(d.evalDir, m.Seed), sb)
	}
	gitInitBaseline(sb)

	scratch := sb + "-scratch"
	_ = os.RemoveAll(scratch)
	_ = os.MkdirAll(scratch, 0o755)
	defer os.RemoveAll(scratch)

	taskFile := filepath.Join(scratch, "TASK.md")
	_ = os.WriteFile(taskFile, []byte(prd+feedback), 0o644)
	gPath := filepath.Join(scratch, "grounding.json")
	gj, _ := json.Marshal(m.Grounding)
	_ = os.WriteFile(gPath, gj, 0o644)

	steps := m.MaxSteps
	if d.maxStepsOverride > 0 {
		steps = d.maxStepsOverride
	}
	if steps == 0 {
		steps = 25
	}
	args := []string{"run", "--config", d.cfgPath, "--dir", sb, "--task-file", taskFile,
		"--grounding", gPath, "--mode", "auto", "--format", "ndjson", "--max-steps", strconv.Itoa(steps)}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	out, _ := exec.CommandContext(cctx, d.pilotBin, args...).CombinedOutput()
	traceDir := filepath.Join(d.evalDir, "reports", "traces")
	_ = os.MkdirAll(traceDir, 0o755)
	_ = os.WriteFile(filepath.Join(traceDir, fmt.Sprintf("%s-attempt-%d.ndjson", m.Name, n)), out, 0o644)
	return Attempt{N: n, Sandbox: sb, Output: tailStr(string(out), 2000)}
}

func (d *Driver) scheduleMemoryLog(m Manifest, at Attempt) {
	entry := map[string]any{
		"ts": time.Now().UTC().Format(time.RFC3339), "prd": m.Name, "attempt": at.N,
		"score": at.Score, "passed": at.Passed, "failed": failedIDs(at.Results),
	}
	d.bg.Add(1)
	go func() {
		defer d.bg.Done()
		_ = os.MkdirAll(filepath.Dir(d.memLog), 0o755)
		f, err := os.OpenFile(d.memLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		line, _ := json.Marshal(entry)
		_, _ = f.Write(append(line, '\n'))
	}()
}

// recordJudge writes each judge check's artifact + rubric to the queue for Claude
// to grade, since a 9B is an unreliable aesthetic judge.
func (d *Driver) recordJudge(m Manifest, sandbox string) {
	if sandbox == "" {
		return
	}
	for _, c := range m.Checks {
		if c.Type != "judge" {
			continue
		}
		files := map[string]string{}
		for _, f := range c.Files {
			if raw, err := os.ReadFile(filepath.Join(sandbox, f)); err == nil {
				body := string(raw)
				if len(body) > 24000 {
					body = body[:24000]
				}
				files[f] = body
			}
		}
		entry := map[string]any{
			"prd": m.Name, "check": c.ID, "criterion": c.Criterion,
			"rubric": c.Rubric, "sandbox": sandbox, "files": files,
		}
		_ = os.MkdirAll(filepath.Dir(d.judgeQueue), 0o755)
		if f, err := os.OpenFile(d.judgeQueue, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			line, _ := json.Marshal(entry)
			_, _ = f.Write(append(line, '\n'))
			f.Close()
		}
	}
}

func (d *Driver) readPRD(name string) string {
	raw, err := os.ReadFile(filepath.Join(d.evalDir, "prds", name+".md"))
	if err != nil {
		return "(PRD text missing for " + name + ")"
	}
	return string(raw)
}

func buildFeedback(results []CheckResult) string {
	var b strings.Builder
	b.WriteString("\n\n---\nYOUR PREVIOUS ATTEMPT FAILED THESE ACCEPTANCE CHECKS. Fix ONLY these; do not start over or touch unrelated files:\n")
	for _, r := range results {
		if !r.Passed {
			fmt.Fprintf(&b, "- [%s] %s\n", r.ID, r.Observed)
		}
	}
	return b.String()
}

func aggregate(r *Report) {
	var sumScore, sumAtt float64
	for _, p := range r.PRDs {
		if p.Passed {
			r.Passed++
		}
		sumScore += p.BestScore
		sumAtt += float64(p.AttemptsUsed)
	}
	if r.Total > 0 {
		r.MeanScore = sumScore / float64(r.Total)
		r.MeanAttempts = sumAtt / float64(r.Total)
	}
}

func loadManifests(checksDir, only string) []Manifest {
	entries, err := os.ReadDir(checksDir)
	if err != nil {
		return nil
	}
	var out []Manifest
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".check.json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(checksDir, e.Name()))
		if err != nil {
			continue
		}
		var m Manifest
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		if !matchesOnly(m.Name, only) {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Difficulty != out[j].Difficulty {
			return out[i].Difficulty < out[j].Difficulty
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func printPRDLine(pr PRDReport) {
	mark := "✗"
	if pr.Passed {
		mark = "✓"
	}
	fmt.Printf("%-28s %-4d %-6.2f %-5s %d\n", pr.Name, pr.Difficulty, pr.BestScore, mark, pr.AttemptsUsed)
}

func printReport(r Report) {
	fmt.Println(strings.Repeat("-", 58))
	fmt.Printf("AGGREGATE  passed %d/%d   mean %.2f   mean-attempts %.1f   %s\n",
		r.Passed, r.Total, r.MeanScore, r.MeanAttempts, r.Duration.Round(time.Second))
}

func writeReport(evalDir string, r Report) {
	dir := filepath.Join(evalDir, "reports")
	_ = os.MkdirAll(dir, 0o755)
	name := "report-" + time.Now().UTC().Format("20060102-150405") + ".json"
	raw, _ := json.MarshalIndent(r, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err == nil {
		fmt.Printf("report written to %s\n", filepath.Join(dir, name))
	}
}

// matchesOnly reports whether name contains any of the comma-separated terms in
// only (empty only matches everything).
func matchesOnly(name, only string) bool {
	only = strings.TrimSpace(only)
	if only == "" {
		return true
	}
	for _, term := range strings.Split(only, ",") {
		if term = strings.TrimSpace(term); term != "" && strings.Contains(name, term) {
			return true
		}
	}
	return false
}

func failedIDs(results []CheckResult) []string {
	var out []string
	for _, r := range results {
		if !r.Passed {
			out = append(out, r.ID)
		}
	}
	return out
}

func findEvalDir() string {
	for _, c := range []string{"eval", filepath.Join("..", "eval")} {
		if isDir(filepath.Join(c, "checks")) {
			return c
		}
	}
	return "eval"
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func tailStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

func copyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyFile(src, dst)
	}
	return filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if fi.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func fatalf(format string, a ...any) {
	fmt.Fprintln(os.Stderr, "error: "+fmt.Sprintf(format, a...))
	os.Exit(1)
}

func yellow(s string) string { return "\033[33m" + s + "\033[0m" }
