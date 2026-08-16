// Command evalfix runs the harness boot-and-run evaluator against an existing
// project directory and repairs whatever stops it from working. It is the tight
// loop for hardening the evaluator: point it at a generated app, watch it install,
// migrate, boot, probe, and repair each stack — without a full regeneration.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"harness/harness/agent"
	"harness/harness/appdir"
	"harness/harness/events"
	"harness/harness/model"
	"harness/harness/tools"
)

func main() {
	configPath := flag.String("config", "", "model registry path (default: local-pilot config)")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: evalfix [-config path] <project-dir>")
		os.Exit(2)
	}
	workDir := flag.Arg(0)

	cfgPath := *configPath
	if cfgPath == "" {
		p, err := appdir.Ensure()
		if err != nil {
			fmt.Fprintf(os.Stderr, "evalfix: %v\n", err)
			os.Exit(1)
		}
		cfgPath = p
	}
	cfg, err := model.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "evalfix: %v\n", err)
		os.Exit(1)
	}
	ag, err := agent.New(cfg, appdir.Dir()+"/skills")
	if err != nil {
		fmt.Fprintf(os.Stderr, "evalfix: %v\n", err)
		os.Exit(1)
	}

	emit := func(e events.Event) {
		switch e.Type {
		case "text", "error":
			fmt.Print(e.Content + e.Message)
		case "tool_call":
			fmt.Printf("  · %s %s\n", e.Tool, e.Info)
		}
	}
	// Full-access: the evaluator's repair children need to read/write/run freely.
	confirm := func(string, string, *events.Diff) (tools.Decision, string) {
		return tools.ApproveAlways, ""
	}

	ag.EvaluateProject(context.Background(), workDir, emit, confirm)
	ag.Wait()
	fmt.Println("\n[evalfix done]")
}
