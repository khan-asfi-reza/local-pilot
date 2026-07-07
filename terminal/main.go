// Package terminal is the interactive coding agent (the TUI) and the headless
// one-shot run command, driven by the pilot launcher.
package terminal

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"harness/harness/agent"
	"harness/harness/model"
)

// Code launches the interactive terminal UI. args are the flags after `code`.
func Code(args []string) {
	fs := flag.NewFlagSet("pilot code", flag.ExitOnError)
	dir := fs.String("dir", ".", "project working directory the assistant operates in")
	configPath := fs.String("config", "", "path to the model registry (default: the local-pilot config)")
	skillsDir := fs.String("skills", "", "skills directory (default: alongside the config)")
	_ = fs.Parse(args)

	workDir, err := filepath.Abs(*dir)
	if err != nil {
		fatal("resolve working directory: %v", err)
	}
	if !isDir(workDir) {
		fatal("working directory does not exist: %s", workDir)
	}

	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = findConfig(workDir)
	}
	cfg, err := model.LoadConfig(cfgPath)
	if err != nil {
		fatal("%v", err)
	}

	session := loadSession(workDir)
	// Restore the model the user chose in a previous session, if still valid.
	if session.Model != "" {
		_ = cfg.SetActive(session.Model)
	}

	skills := *skillsDir
	if skills == "" {
		// models/models.json lives in the models dir; skills sits beside it.
		repoRoot := filepath.Dir(filepath.Dir(cfgPath))
		if cand := filepath.Join(repoRoot, "skills"); isDir(cand) {
			skills = cand
		}
	}

	ag, err := agent.New(cfg, skills)
	if err != nil {
		fatal("%v", err)
	}

	// Prefer a model whose backend is actually running: the current selection if
	// up, otherwise the first running model, so the session starts on something
	// usable.
	pickRunningModel(ag)

	if err := newUI(ag, session, workDir).run(); err != nil {
		fatal("%v", err)
	}
}

// pickRunningModel switches the active model to one that is up, if the current
// choice is not running but another is.
func pickRunningModel(ag *agent.Agent) {
	models := ag.Models()
	for _, m := range models {
		if m.Active && m.Running {
			return // current choice is up, keep it
		}
	}
	for _, m := range models {
		if m.Running {
			_ = ag.SetModel(m.Name)
			return
		}
	}
	// none running; keep the default and let the UI warn
}

// findConfig walks up from the working directory looking for models/models.json.
func findConfig(workDir string) string {
	cur := workDir
	for {
		cand := filepath.Join(cur, "models", "models.json")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return filepath.Join("models", "models.json")
		}
		cur = parent
	}
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "pilot: "+format+"\n", a...)
	os.Exit(1)
}
