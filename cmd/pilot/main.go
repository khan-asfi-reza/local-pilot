// Command pilot is the local-pilot entry point. `pilot start` ensures ollama is
// running and the default model is installed; `pilot add <base>` pulls a base
// model, applies the tool-call template, and registers it.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"harness/harness/model"
	"harness/terminal"
)

const defaultNumCtx = 32768

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		terminal.Code(nil) // no subcommand opens the coding agent
		return
	}
	var err error
	switch args[0] {
	case "start":
		err = start()
	case "add":
		if len(args) < 2 {
			usage()
		}
		err = add(args[1])
	case "code":
		terminal.Code(args[1:])
		return
	case "run":
		terminal.Run(args[1:])
		return
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "pilot: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("usage: pilot start | pilot add <base-model> | pilot code [--dir X] | pilot run --dir X --task \"...\"")
	os.Exit(2)
}

// add pulls a base model, builds the tool-call variant (<base>-tools) with the
// template swap + num_ctx, creates it, and registers it in models.json.
func add(base string) error {
	cfgPath := findConfig()
	cfg, err := model.LoadConfig(cfgPath)
	if err != nil {
		return err
	}
	if err := ensureOllama("http://localhost:11434"); err != nil {
		return err
	}
	name := base + "-tools"
	entry := model.ModelEntry{Name: name, Base: base, NumCtx: defaultNumCtx, ToolMode: model.ToolModeNative, Port: 11434}
	if err := installModel(entry); err != nil {
		return err
	}
	cfg.AddModel(entry)
	if err := cfg.Save(cfgPath); err != nil {
		return fmt.Errorf("register model in %s: %w", cfgPath, err)
	}
	fmt.Printf("\n✓ added %q (from %s) and registered it. Switch to it with /model %s\n", name, base, name)
	return nil
}

// start runs the full bootstrap.
func start() error {
	cfgPath := findConfig()
	cfg, err := model.LoadConfig(cfgPath)
	if err != nil {
		return err
	}
	entry, ok := cfg.DefaultEntry()
	if !ok {
		return fmt.Errorf("no default model in %s", cfgPath)
	}
	port := entry.Port
	if port == 0 {
		port = 11434
	}
	url := fmt.Sprintf("http://localhost:%d", port)

	if err := ensureOllama(url); err != nil {
		return err
	}
	if modelInstalled(entry.Name) {
		fmt.Printf("✓ model %s already installed\n", entry.Name)
	} else if err := installModel(entry); err != nil {
		return err
	}
	if !modelInstalled(entry.Name) {
		return fmt.Errorf("model %s did not install", entry.Name)
	}
	fmt.Printf("\n✓ pilot ready — model %q serving at %s\n", entry.Name, url)
	fmt.Println("  run a task:  ./pilot run --dir <project> --task \"...\"   (or use bin/harness)")
	return nil
}

// ensureOllama makes sure the ollama server answers, starting it if it does not.
func ensureOllama(url string) error {
	if ollamaUp(url) {
		fmt.Println("✓ ollama already running")
		return nil
	}
	if _, err := exec.LookPath("ollama"); err != nil {
		return fmt.Errorf("ollama is not installed (get it from https://ollama.com)")
	}
	fmt.Println("… starting ollama serve")
	cmd := exec.Command("ollama", "serve")
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ollama: %w", err)
	}
	for i := 0; i < 40; i++ {
		if ollamaUp(url) {
			fmt.Println("✓ ollama running")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("ollama did not come up on %s", url)
}

// ollamaUp probes the version endpoint.
func ollamaUp(url string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(url, "/")+"/api/version", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// modelInstalled reports whether ollama already has the named model.
func modelInstalled(name string) bool {
	return exec.Command("ollama", "show", name).Run() == nil
}

// installModel pulls the base model and creates the customized local model.
func installModel(entry model.ModelEntry) error {
	base := entry.Base
	if base == "" {
		base = entry.Name
	}
	fmt.Printf("… pulling base model %s (one-time download)\n", base)
	if err := stream("ollama", "pull", base); err != nil {
		return fmt.Errorf("pull %s: %w", base, err)
	}
	fmt.Printf("… building local model %s (tool-call template + num_ctx %d)\n", entry.Name, entry.NumCtx)
	mf, err := buildModelfile(base, entry.NumCtx)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "pilot-*.modelfile")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(mf); err != nil {
		return err
	}
	tmp.Close()
	if err := stream("ollama", "create", entry.Name, "-f", tmp.Name()); err != nil {
		return fmt.Errorf("create %s: %w", entry.Name, err)
	}
	return nil
}

// buildModelfile derives the local model's Modelfile from the base: it swaps the
// <tool_call> tags to [tool_call] (which the coder model actually emits, so
// ollama can parse native tool_calls) and bakes in num_ctx.
func buildModelfile(base string, numCtx int) (string, error) {
	out, err := exec.Command("ollama", "show", base, "--modelfile").Output()
	if err != nil {
		return "", fmt.Errorf("read base modelfile: %w", err)
	}
	mf := string(out)
	mf = strings.ReplaceAll(mf, "</tool_call>", "[/tool_call]")
	mf = strings.ReplaceAll(mf, "<tool_call>", "[tool_call]")
	if numCtx > 0 && !strings.Contains(mf, "num_ctx") {
		mf += fmt.Sprintf("\nPARAMETER num_ctx %d\n", numCtx)
	}
	return mf, nil
}

// stream runs a command with its output attached to the terminal.
func stream(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// findConfig walks up from the cwd for models/models.json.
func findConfig() string {
	cur, _ := os.Getwd()
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
