package agent

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"harness/harness/model"
	"harness/harness/tools"
)

type keyFile struct {
	Path    string `json:"path"`
	Purpose string `json:"purpose"`
}

type memory struct {
	Summary     string    `json:"summary"`
	Stack       []string  `json:"stack"`
	KeyFiles    []keyFile `json:"key_files"`
	Done        []string  `json:"done"`
	Conventions []string  `json:"conventions"`
	TODO        []string  `json:"todo"`
}

const (
	maxMemoryBytes   = 4000
	maxSummaryBytes  = 320
	maxStack         = 12
	maxKeyFiles      = 20
	maxDone          = 15
	maxConventions   = 12
	maxTODO          = 10
	maxAgentsMDBytes = 2600
	memMergeInputCap = 8000
)

const memorySchemaBody = `{"type":"object","properties":{` +
	`"summary":{"type":"string"},` +
	`"stack":{"type":"array","items":{"type":"string"}},` +
	`"key_files":{"type":"array","items":{"type":"object","properties":{"path":{"type":"string"},"purpose":{"type":"string"}},"required":["path","purpose"]}},` +
	`"done":{"type":"array","items":{"type":"string"}},` +
	`"conventions":{"type":"array","items":{"type":"string"}},` +
	`"todo":{"type":"array","items":{"type":"string"}}` +
	`},"required":["summary","stack","key_files","done","conventions","todo"]}`

const memorySchema = memorySchemaBody

const bootstrapSchema = `{"type":"object","properties":{` +
	`"agents_md":{"type":"string"},` +
	`"memory":` + memorySchemaBody +
	`},"required":["agents_md","memory"]}`

var memLocks sync.Map

func memLock(path string) *sync.Mutex {
	m, _ := memLocks.LoadOrStore(path, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// memoryPath is <gitRoot>/.pilot/memory.md; .pilot is dot-prefixed so the repo
// walkers skip it and the memory never pollutes itself.
func memoryPath(workDir string) string {
	abs, err := filepath.Abs(workDir)
	if err != nil {
		abs = workDir
	}
	root := findGitRoot(abs)
	if root == "" {
		root = abs
	}
	return filepath.Join(root, ".pilot", "memory.md")
}

func discoverMemory(workDir string) string {
	raw, err := os.ReadFile(memoryPath(workDir))
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(raw))
	if len(s) > maxMemoryBytes {
		s = s[:maxMemoryBytes]
	}
	return s
}

// scheduleMemoryUpdate merges what this run changed into the worktree memory on a
// detached goroutine; drained by Agent.Wait for headless callers.
func (a *Agent) scheduleMemoryUpdate(req Request, changed map[string]bool) {
	if req.noTriage || req.Chat || req.Mode == tools.ModePlan || len(changed) == 0 {
		return
	}
	workDir := req.WorkDir
	task := lastUserText(req.Messages)
	paths := sortedKeys(changed)
	a.bg.Add(1)
	go func() {
		defer a.bg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		a.updateMemory(ctx, workDir, paths, task)
	}()
}

func (a *Agent) updateMemory(ctx context.Context, workDir string, changedPaths []string, task string) {
	path := memoryPath(workDir)
	lock := memLock(path)
	lock.Lock()
	defer lock.Unlock()

	existing := "(none yet)"
	if raw, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(raw)); s != "" {
			existing = s
		}
	}

	var b strings.Builder
	b.WriteString("EXISTING MEMORY:\n" + existing)
	b.WriteString("\n\nTASK JUST COMPLETED:\n" + truncate(task, 800))
	b.WriteString("\n\nFILES CHANGED THIS RUN:\n" + changedContext(workDir, changedPaths))
	b.WriteString("\n\nCURRENT FILE TREE:\n" + currentTree(workDir))

	sys := "You maintain a compact project memory a coding agent reads before every task. Merge the new " +
		"facts into the existing memory. Keep it SHORT. Fold finished TODOs into Done, drop duplicates and " +
		"anything no longer true. summary is 1-2 sentences. Record only durable facts (what exists, key files " +
		"and their purpose, decisions), never step-by-step narration. Output ONLY the JSON."
	msgs := []model.Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: b.String()},
	}
	raw, _, err := a.router.Constrained(ctx, msgs, json.RawMessage(memorySchema))
	if err != nil {
		return
	}
	var m memory
	if json.Unmarshal([]byte(raw), &m) != nil {
		return
	}
	enforceBudget(&m, workDir)
	_ = writeMemory(path, renderMarkdown(m))
}

func changedContext(workDir string, paths []string) string {
	var b strings.Builder
	for i, p := range paths {
		if i >= 15 {
			b.WriteString("... (more files changed)\n")
			break
		}
		b.WriteString(p + ":\n")
		if raw, err := os.ReadFile(filepath.Join(workDir, p)); err == nil {
			lines := strings.Split(string(raw), "\n")
			if len(lines) > 15 {
				lines = lines[:15]
			}
			b.WriteString(strings.Join(lines, "\n") + "\n")
		}
		if b.Len() > memMergeInputCap {
			b.WriteString("... (truncated)\n")
			break
		}
	}
	return b.String()
}

// enforceBudget clamps every section; Done/TODO keep newest, others keep order,
// and vanished key files are pruned.
func enforceBudget(m *memory, workDir string) {
	if len(m.Summary) > maxSummaryBytes {
		m.Summary = m.Summary[:maxSummaryBytes]
	}
	m.Stack = capStrings(m.Stack, maxStack)

	var kept []keyFile
	for _, kf := range m.KeyFiles {
		if strings.TrimSpace(kf.Path) == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(workDir, kf.Path)); err == nil {
			kept = append(kept, kf)
		}
	}
	m.KeyFiles = capKeyFiles(kept, maxKeyFiles)
	m.Done = tailStrings(m.Done, maxDone)
	m.Conventions = capStrings(m.Conventions, maxConventions)
	m.TODO = tailStrings(m.TODO, maxTODO)
}

func renderMarkdown(m memory) string {
	var b strings.Builder
	b.WriteString("## Summary\n" + strings.TrimSpace(m.Summary))
	b.WriteString("\n\n## Stack\n")
	for _, s := range m.Stack {
		b.WriteString("- " + s + "\n")
	}
	b.WriteString("\n## Key files\n")
	for _, kf := range m.KeyFiles {
		b.WriteString("- " + kf.Path + " — " + kf.Purpose + "\n")
	}
	b.WriteString("\n## Done\n")
	for _, s := range m.Done {
		b.WriteString("- " + s + "\n")
	}
	b.WriteString("\n## Conventions\n")
	for _, s := range m.Conventions {
		b.WriteString("- " + s + "\n")
	}
	b.WriteString("\n## TODO\n")
	for _, s := range m.TODO {
		b.WriteString("- " + s + "\n")
	}
	out := b.String()
	if len(out) > maxMemoryBytes {
		out = out[:maxMemoryBytes]
	}
	return out
}

func writeMemory(path, md string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(md+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (a *Agent) shouldBootstrap(req Request, agentsMD string) bool {
	return !req.Chat && req.Mode != tools.ModePlan && agentsMD == "" && meaningfulRepo(req.WorkDir)
}

// meaningfulRepo reports whether the dir has a manifest or 3+ source files worth
// analyzing (a blank project is skipped and memory seeds incrementally).
func meaningfulRepo(workDir string) bool {
	manifests := map[string]bool{
		"go.mod": true, "package.json": true, "requirements.txt": true,
		"pyproject.toml": true, "Cargo.toml": true, "pom.xml": true,
		"build.gradle": true, "Gemfile": true,
	}
	srcCount := 0
	ok := false
	_ = filepath.WalkDir(workDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || ok {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != workDir && (ignoredDirs[name] || strings.HasPrefix(name, ".")) {
				return fs.SkipDir
			}
			return nil
		}
		if manifests[d.Name()] {
			ok = true
			return fs.SkipAll
		}
		if _, isSrc := symbolPatterns[strings.ToLower(filepath.Ext(path))]; isSrc {
			srcCount++
			if srcCount >= 3 {
				ok = true
				return fs.SkipAll
			}
		}
		return nil
	})
	return ok
}

// bootstrapProject writes a concise AGENTS.md and seed memory from one analysis
// call; blocks the first task, never fails it.
func (a *Agent) bootstrapProject(ctx context.Context, req Request) string {
	root := findGitRoot(req.WorkDir)
	if root == "" {
		abs, err := filepath.Abs(req.WorkDir)
		if err != nil {
			return ""
		}
		root = abs
	}
	path := memoryPath(req.WorkDir)
	lock := memLock(path)
	lock.Lock()
	defer lock.Unlock()

	if discoverAgentsMD(req.WorkDir) != "" {
		return ""
	}

	var user strings.Builder
	user.WriteString("PROJECT MAP:\n" + buildRepoMap(req.WorkDir))
	user.WriteString("\n\nFILE TREE:\n" + currentTree(req.WorkDir))
	user.WriteString("\n\nMANIFESTS / README:\n" + manifestsAndReadme(req.WorkDir))

	sys := "Analyze this codebase and produce (1) a concise AGENTS.md for a coding agent — stack, structure, " +
		"how to build/run/test, key conventions, under ~2500 bytes — and (2) an initial project memory of " +
		"what exists. Be accurate; do not invent. Output ONLY the JSON."
	msgs := []model.Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: user.String()},
	}
	raw, _, err := a.router.Constrained(ctx, msgs, json.RawMessage(bootstrapSchema))
	if err != nil {
		return ""
	}
	var out struct {
		AgentsMD string `json:"agents_md"`
		Memory   memory `json:"memory"`
	}
	if json.Unmarshal([]byte(raw), &out) != nil {
		return ""
	}
	md := strings.TrimSpace(out.AgentsMD)
	if md == "" {
		return ""
	}
	if len(md) > maxAgentsMDBytes {
		md = md[:maxAgentsMDBytes]
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(md+"\n"), 0o644); err != nil {
		return ""
	}
	enforceBudget(&out.Memory, req.WorkDir)
	_ = writeMemory(path, renderMarkdown(out.Memory))
	return md
}

func manifestsAndReadme(workDir string) string {
	var b strings.Builder
	for _, name := range []string{"go.mod", "package.json", "requirements.txt", "pyproject.toml", "Cargo.toml"} {
		if raw, err := os.ReadFile(filepath.Join(workDir, name)); err == nil {
			b.WriteString(name + ":\n" + truncate(string(raw), 600) + "\n\n")
		}
	}
	for _, name := range []string{"README.md", "README", "readme.md"} {
		if raw, err := os.ReadFile(filepath.Join(workDir, name)); err == nil {
			b.WriteString(name + " (head):\n" + truncate(string(raw), 800) + "\n")
			break
		}
	}
	return b.String()
}

func capStrings(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func tailStrings(s []string, n int) []string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

func capKeyFiles(s []keyFile, n int) []keyFile {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
