package terminal

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"harness/harness/agent"
	"harness/harness/events"
	"harness/harness/model"
	"harness/harness/projects"
	"harness/harness/tools"
)

// Run is the headless one-shot entry (`pilot run`): it runs one task to
// completion in auto mode, streams events, and exits non-zero on error.
func Run(argv []string) {
	fs := flag.NewFlagSet("pilot run", flag.ExitOnError)
	dir := fs.String("dir", ".", "project working directory the assistant operates in")
	task := fs.String("task", "", "the task/PRD text to run")
	taskFile := fs.String("task-file", "", "read the task/PRD from this file instead of --task")
	configPath := fs.String("config", "", "path to the model registry (default: the local-pilot config)")
	skillsDir := fs.String("skills", "", "skills directory (default: alongside the config)")
	mode := fs.String("mode", "auto", "tool mode: auto, ask, or plan (headless runs auto)")
	format := fs.String("format", "ndjson", "event output format: ndjson or human")
	maxSteps := fs.Int("max-steps", 0, "override the per-request step cap (0 = default)")
	grounding := fs.String("grounding", "", "path to a JSON grounding contract {action, explicit_targets} pinning the named target(s)")
	modelName := fs.String("model", "", "build model to use (default: the active/default model)")
	plannerName := fs.String("planner", "", "planner model for decomposition (default: config default_planner, else the build model)")
	_ = fs.Parse(argv)

	prompt := *task
	if *taskFile != "" {
		if tools.IsDocument(*taskFile) {
			text, err := tools.ExtractDocument(*taskFile)
			if err != nil {
				fatal("extract task document: %v", err)
			}
			prompt = text
		} else {
			raw, err := os.ReadFile(*taskFile)
			if err != nil {
				fatal("read task file: %v", err)
			}
			prompt = string(raw)
		}
	}
	if prompt == "" {
		fatal("nothing to do: pass --task \"...\" or --task-file <path>")
	}

	workDir, err := filepath.Abs(*dir)
	if err != nil {
		fatal("resolve working directory: %v", err)
	}
	if !isDir(workDir) {
		fatal("working directory does not exist: %s", workDir)
	}
	_, _ = projects.Upsert(workDir, "terminal")
	model.SetLogDir(filepath.Join(workDir, ".pilot", "logs"))

	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = findConfig(workDir)
	}
	cfg, err := model.LoadConfig(cfgPath)
	if err != nil {
		fatal("%v", err)
	}
	if *plannerName != "" {
		cfg.DefaultPlanner = *plannerName
	}

	skills := *skillsDir
	if skills == "" {
		repoRoot := filepath.Dir(filepath.Dir(cfgPath))
		if cand := filepath.Join(repoRoot, "skills"); isDir(cand) {
			skills = cand
		}
	}

	ag, err := agent.New(cfg, skills)
	if err != nil {
		fatal("%v", err)
	}
	ag.SetMaxSteps(*maxSteps)
	if *modelName != "" {
		if err := ag.SetModel(*modelName); err != nil {
			fatal("%v", err)
		}
	} else {
		pickRunningModel(ag)
	}
	if !ag.Reachable(ag.ActiveModel()) {
		fatal("no model backend is running for %q; start one first (e.g. ollama serve)", ag.ActiveModel())
	}

	enc := json.NewEncoder(os.Stdout)
	exitCode := 0
	emit := func(ev events.Event) {
		if *format == "human" {
			printHuman(ev)
		} else {
			_ = enc.Encode(ev)
		}
		if ev.Type == "error" {
			exitCode = 1
		}
	}

	req := agent.Request{
		Messages: []model.Message{{Role: "user", Content: prompt}},
		Allowed:  ag.ToolNames(),
		Mode:     *mode,
		WorkDir:  workDir,
	}
	// A grounding contract (from the eval harness) pins the named target(s) so the
	// loop verifies they were actually changed instead of running intake itself.
	if *grounding != "" {
		if raw, err := os.ReadFile(*grounding); err == nil {
			var g agent.Grounding
			if json.Unmarshal(raw, &g) == nil {
				req.Grounding = &g
			}
		}
	}
	// Headless auto mode never pauses for confirmation, so confirm is nil.
	ag.Run(context.Background(), req, emit, nil)
	ag.Wait() // let any background memory update finish before exiting
	os.Exit(exitCode)
}

// printHuman renders one event as a readable line to stderr.
func printHuman(ev events.Event) {
	switch ev.Type {
	case "text":
		fmt.Fprint(os.Stderr, ev.Content)
	case "reasoning":
		fmt.Fprintf(os.Stderr, "\033[2m%s\033[0m", ev.Content)
	case "tool_call":
		fmt.Fprintf(os.Stderr, "\n→ %s: %s\n", ev.Tool, ev.Info)
	case "tool_result":
		fmt.Fprintf(os.Stderr, "  %s: %s\n", ev.Tool, ev.Info)
	case "usage":
		fmt.Fprintf(os.Stderr, "  [%d tokens]\n", ev.Tokens)
	case "error":
		fmt.Fprintf(os.Stderr, "\n✗ error: %s\n", ev.Message)
	case "done":
		fmt.Fprintln(os.Stderr, "\n✓ done")
	}
}
