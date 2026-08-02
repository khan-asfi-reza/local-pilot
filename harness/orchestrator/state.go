package orchestrator

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"harness/harness/events"
	"harness/harness/tools"
)

var snapshotSkip = map[string]bool{
	".git": true, ".venv": true, "venv": true, "node_modules": true,
	"__pycache__": true, ".pilot": true, ".harness": true, "dist": true, "build": true,
}

// projectSnapshot is a compact, capped listing of the files that already exist,
// so decomposition of a later PRD plans incremental work instead of clobbering.
func projectSnapshot(workDir string) string {
	var files []string
	_ = filepath.WalkDir(workDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != workDir && (snapshotSkip[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return fs.SkipDir
			}
			return nil
		}
		if len(files) >= 80 {
			return fs.SkipAll
		}
		if rel, e := filepath.Rel(workDir, path); e == nil {
			files = append(files, rel)
		}
		return nil
	})
	sort.Strings(files)
	return strings.Join(files, "\n")
}

// State is the compact, token-cheap project state injected into planning and
// every child: whether the project is scaffolded, its stack, canonical names,
// and layout — so independently-built files wire together.
type State struct {
	Initialized bool
	Stack       string
	Project     string
	App         string
	Settings    string
	Entry       string
	Layout      []string
}

// Render is the compact markdown form (key: value lines, not JSON).
func (s State) Render() string {
	var b strings.Builder
	b.WriteString("initialized: ")
	if s.Initialized {
		b.WriteString("true\n")
	} else {
		b.WriteString("false\n")
	}
	if s.Stack != "" {
		b.WriteString("stack: " + s.Stack + "\n")
	}
	var names []string
	if s.Project != "" {
		names = append(names, "project="+s.Project)
	}
	if s.App != "" {
		names = append(names, "app="+s.App)
	}
	if s.Settings != "" {
		names = append(names, "settings="+s.Settings)
	}
	if len(names) > 0 {
		b.WriteString("names: " + strings.Join(names, ", ") + "\n")
	}
	if s.Entry != "" {
		b.WriteString("run: " + s.Entry + "\n")
	}
	if len(s.Layout) > 0 {
		b.WriteString("layout: " + strings.Join(s.Layout, ", ") + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func statePath(workDir string) string { return filepath.Join(workDir, ".pilot", "state.md") }

func saveState(workDir string, s State) {
	p := statePath(workDir)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(s.Render()+"\n"), 0o644)
}

func loadStateText(workDir string) string {
	raw, err := os.ReadFile(statePath(workDir))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// detectInitialized is the cheap reconciliation: a manifest/entrypoint (or our
// own state file) means the project is already scaffolded.
func detectInitialized(workDir string) bool {
	for _, m := range []string{
		"manage.py", "package.json", "go.mod", "pyproject.toml",
		"Cargo.toml", "pom.xml", "Gemfile", ".pilot/state.md",
	} {
		if _, err := os.Stat(filepath.Join(workDir, m)); err == nil {
			return true
		}
	}
	return false
}

type initPlan struct {
	Stack    string         `json:"stack"`
	Project  string         `json:"project"`
	App      string         `json:"app"`
	Settings string         `json:"settings"`
	Entry    string         `json:"entry"`
	Scaffold []scaffoldFile `json:"scaffold"`
}

type scaffoldFile struct {
	Path    string `json:"path"`
	Purpose string `json:"purpose"`
}

const initSchema = `{"type":"object","properties":{` +
	`"stack":{"type":"string"},` +
	`"project":{"type":"string"},` +
	`"app":{"type":"string"},` +
	`"settings":{"type":"string"},` +
	`"entry":{"type":"string"},` +
	`"scaffold":{"type":"array","items":{"type":"object","properties":{"path":{"type":"string"},"purpose":{"type":"string"}},"required":["path","purpose"]}}` +
	`},"required":["stack","project","app","settings","entry","scaffold"]}`

// initialize runs the up-front project-init phase: one planner call for the
// canonical names + minimal scaffold, then one focused child to write the
// runnable skeleton. Feature sub-tasks then build on a consistent foundation.
// Returns the compact state text (empty on failure).
func (o *Orchestrator) initialize(ctx context.Context, prompt string, c *Contract, spec ChildSpec, emit func(events.Event), confirm tools.ConfirmFunc) string {
	sys := "Plan the MINIMAL skeleton to initialize this project with a FLAT, conventional layout. Choose " +
		"concrete canonical names (project/package, app, config/settings module). List ONLY the scaffold files " +
		"(entrypoint, config/settings, routing, package inits) with exact root-relative paths — the smallest set " +
		"that makes the project runnable and ready for feature code. Do NOT include feature files (models, " +
		"business logic, UI screens). Output ONLY the JSON."
	user := prompt
	if len(c.AcceptanceCriteria) > 0 {
		user += "\n\nAcceptance:\n" + strings.Join(c.AcceptanceCriteria, "; ")
	}
	raw, err := o.planner.PlanJSON(ctx, sys, clip(user, 8000), json.RawMessage(initSchema))
	if err != nil {
		return ""
	}
	var ip initPlan
	if json.Unmarshal([]byte(raw), &ip) != nil {
		return ""
	}
	emit(events.Text("\n[initializing project: " + ip.Stack + "]\n"))

	var b strings.Builder
	b.WriteString("Initialize the project skeleton. Stack: " + ip.Stack + ".")
	b.WriteString("\nCanonical names — use these EXACT names everywhere: project=" + ip.Project +
		", app=" + ip.App + ", settings=" + ip.Settings + ".")
	b.WriteString("\nCreate EXACTLY these files at these exact root-relative paths, wired together and runnable, " +
		"using write_file directly (NEVER run generators like django-admin startproject/startapp, create-react-app " +
		"or vite create — they nest folders):\n")
	var paths []string
	for _, f := range ip.Scaffold {
		b.WriteString("- " + f.Path + " — " + f.Purpose + "\n")
		paths = append(paths, f.Path)
	}
	b.WriteString("Use a FLAT layout (files at the given paths from the project root). Wire config/settings to " +
		"include the app and its framework so feature code can be added next.")

	o.exec.RunChild(ctx, b.String(), spec, tagEmit(emit, "init"), confirm)

	st := State{Initialized: true, Stack: ip.Stack, Project: ip.Project, App: ip.App,
		Settings: ip.Settings, Entry: ip.Entry, Layout: paths}
	saveState(spec.WorkDir, st)
	return st.Render()
}
