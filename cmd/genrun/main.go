// Command genrun drives one full-access generation from a PRD file into a target
// directory, exactly as the Code IDE /run path would — triage decides to
// orchestrate on a large spec, so this exercises the whole pipeline (scaffold →
// decompose → feature tasks → per-task eval → final boot-and-run evaluator). It is
// the from-scratch counterpart to evalfix (which only re-evaluates an existing app).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"harness/harness/agent"
	"harness/harness/appdir"
	"harness/harness/events"
	"harness/harness/model"
	"harness/harness/tools"
)

func main() {
	configPath := flag.String("config", "", "model registry path (default: local-pilot config)")
	flag.Parse()
	if flag.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: genrun [-config path] <work-dir> <prd-file>")
		os.Exit(2)
	}
	workDir, prdFile := flag.Arg(0), flag.Arg(1)
	prd, err := os.ReadFile(prdFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genrun: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "genrun: %v\n", err)
		os.Exit(1)
	}

	cfgPath := *configPath
	if cfgPath == "" {
		if p, e := appdir.Ensure(); e == nil {
			cfgPath = p
		} else {
			fmt.Fprintf(os.Stderr, "genrun: %v\n", e)
			os.Exit(1)
		}
	}
	cfg, err := model.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genrun: %v\n", err)
		os.Exit(1)
	}
	ag, err := agent.New(cfg, filepath.Join(appdir.Dir(), "skills"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "genrun: %v\n", err)
		os.Exit(1)
	}
	model.SetLogDir(filepath.Join(workDir, ".pilot", "logs"))

	emit := func(e events.Event) {
		switch e.Type {
		case "text", "error":
			fmt.Print(e.Content + e.Message)
		case "tool_call":
			fmt.Printf("  · %s %s\n", e.Tool, e.Info)
		}
	}
	confirm := func(string, string, *events.Diff) (tools.Decision, string) {
		return tools.ApproveAlways, ""
	}

	// full-access shape: all tools, no sandbox, auto mode, not chat.
	req := agent.Request{
		Messages: []model.Message{{Role: "user", Content: string(prd)}},
		WorkDir:  workDir,
		Mode:     tools.ModeAuto,
	}
	ag.Run(context.Background(), req, emit, confirm)
	ag.Wait()
	fmt.Println("\n[genrun done]")
}
