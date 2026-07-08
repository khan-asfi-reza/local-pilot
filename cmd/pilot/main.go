// Command pilot is the local-pilot entry point: it sets up ollama and the local
// model, and launches the coding agent. Config and skills live in a global data
// directory (see the appdir package) so pilot runs from anywhere.
package main

import (
	"slices"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"harness/harness/appdir"
	"harness/harness/model"
	"harness/terminal"
)

const defaultNumCtx = 32768

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Print(helpText())
		return
	}
	var err error
	switch args[0] {
	case "start":
		err = start()
	case "stop":
		err = stop()
	case "models":
		err = modelsCmd(args[1:])
	case "code":
		cfgPath, e := appdir.Ensure()
		if e != nil {
			fatal(e)
		}
		terminal.Code(withConfig(args[1:], cfgPath))
		return
	case "run":
		cfgPath, e := appdir.Ensure()
		if e != nil {
			fatal(e)
		}
		terminal.Run(withConfig(args[1:], cfgPath))
		return
	case "help", "-h", "--help":
		fmt.Print(helpText())
		return
	default:
		fmt.Print(helpText())
		os.Exit(2)
	}
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, red("error: ")+err.Error())
	os.Exit(1)
}

// start sets up ollama and a model, then reports readiness.
func start() error {
	cfgPath, err := appdir.Ensure()
	if err != nil {
		return err
	}
	cfg, err := model.LoadConfig(cfgPath)
	if err != nil {
		return err
	}
	header()
	if err := ensureOllamaInstalled(); err != nil {
		return err
	}
	if err := ensureOllama("http://localhost:11434"); err != nil {
		return err
	}

	base := chooseModelBase(cfg)
	name, err := buildAndRegister(cfgPath, cfg, base)
	if err != nil {
		return fmt.Errorf("could not install %q. Is that an ollama model? See https://ollama.com/library\n  %v", base, err)
	}
	cfg.Default = name
	if err := cfg.Save(cfgPath); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println(green("✓ ") + bold("pilot ready") + dim(" — model "+name))
	fmt.Println(dim("  data dir: ") + appdir.Dir())
	fmt.Println(dim("  next: ") + cyan("pilot code") + dim(" --dir <project>"))
	return nil
}

// toolModeFor picks the tool-calling strategy by model size: small models (≤3B)
// are unreliable at native tool calls, so they use the grammar-constrained JSON
// path (enforced via ollama's response_format); larger models use native calls.
func toolModeFor(base string) string {
	b := strings.ToLower(base)
	for _, s := range []string{"0.5b", "1b", "1.5b", "2b", "3b"} {
		if strings.Contains(b, ":"+s) || strings.Contains(b, "-"+s) {
			return model.ToolModeJSON
		}
	}
	return model.ToolModeNative
}

// buildAndRegister pulls base, creates the <base>-tools variant with the tool-call
// template and num_ctx, and records it in models.json. Idempotent.
func buildAndRegister(cfgPath string, cfg *model.Config, base string) (string, error) {
	name := base + "-tools"
	entry := model.ModelEntry{Name: name, Base: base, NumCtx: defaultNumCtx, ToolMode: toolModeFor(base), Port: 11434}
	if !modelInstalled(name) {
		if err := installModel(entry); err != nil {
			return "", err
		}
	} else {
		fmt.Println(green("✓ ") + name + dim(" already installed"))
	}
	cfg.AddModel(entry)
	if err := cfg.Save(cfgPath); err != nil {
		return "", err
	}
	return name, nil
}

// chooseModelBase shows the preset menu and returns the base model the user picks.
// Enter (or a non-interactive run) selects the first (default) preset.
func chooseModelBase(cfg *model.Config) string {
	presets := cfg.Suggested
	if len(presets) == 0 {
		presets = []string{"qwen2.5-coder:7b"}
	}
	if !stdinIsTTY() {
		return presets[0]
	}
	fmt.Println()
	fmt.Println(bold("Choose a model:"))
	for i, p := range presets {
		tag := ""
		if i == 0 {
			tag = dim("  (default)")
		}
		fmt.Printf("  %s %s%s\n", cyan(strconv.Itoa(i+1)+"."), p, tag)
	}
	custom := len(presets) + 1
	fmt.Printf("  %s %s\n", cyan(strconv.Itoa(custom)+"."), "Enter an ollama model name")

	choice := strings.TrimSpace(promptLine(fmt.Sprintf("Select [1-%d] (Enter for default): ", custom)))
	if choice == "" {
		return presets[0]
	}
	n, err := strconv.Atoi(choice)
	if err != nil {
		return presets[0]
	}
	if n >= 1 && n <= len(presets) {
		return presets[n-1]
	}
	if n == custom {
		name := strings.TrimSpace(promptLine("Enter ollama model name (e.g. qwen2.5-coder:7b): "))
		if name != "" {
			return name
		}
	}
	return presets[0]
}

// ensureOllamaInstalled installs ollama if it is missing, with the user's consent.
func ensureOllamaInstalled() error {
	if _, err := exec.LookPath("ollama"); err == nil {
		fmt.Println(green("✓ ") + "ollama installed")
		return nil
	}
	fmt.Println(yellow("• ") + "ollama is not installed.")
	if !confirm("Install ollama now?") {
		return fmt.Errorf("ollama is required. Install it from https://ollama.com/download and re-run %s", cyan("pilot start"))
	}
	switch osName() {
	case "windows":
		return stream("powershell", "-NoProfile", "-Command", "irm https://ollama.com/install.ps1 | iex")
	case "linux":
		return stream("sh", "-c", "curl -fsSL https://ollama.com/install.sh | sh")
	case "darwin":
		if _, err := exec.LookPath("brew"); err == nil {
			return stream("brew", "install", "ollama")
		}
		return fmt.Errorf("Homebrew not found. Install ollama from https://ollama.com/download, then re-run %s", cyan("pilot start"))
	default:
		return fmt.Errorf("automatic install is not supported here. Get ollama from https://ollama.com/download, then re-run %s", cyan("pilot start"))
	}
}

// modelsCmd handles `pilot models add|list|set-default`.
func modelsCmd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: pilot models <add <model> [--host URL] | list | set-default [name]>")
	}
	cfgPath, err := appdir.Ensure()
	if err != nil {
		return err
	}
	cfg, err := model.LoadConfig(cfgPath)
	if err != nil {
		return err
	}
	switch args[0] {
	case "add":
		base, host := parseAddArgs(args[1:])
		if base == "" {
			return fmt.Errorf("usage: pilot models add <ollama-model> [--host URL]")
		}
		if host != "" {
			return addRemoteModel(cfgPath, cfg, base, host)
		}
		if err := ensureOllama("http://localhost:11434"); err != nil {
			return err
		}
		name, err := buildAndRegister(cfgPath, cfg, base)
		if err != nil {
			return fmt.Errorf("could not add %q: %w", base, err)
		}
		fmt.Println(green("✓ ") + fmt.Sprintf("added %s. Use it with %s", bold(name), cyan("/model "+name)))
		return nil
	case "list":
		return modelsList(cfg)
	case "set-default":
		name := ""
		if len(args) >= 2 {
			name = args[1]
		}
		return setDefaultModel(cfgPath, cfg, name)
	default:
		return fmt.Errorf("unknown: pilot models %s (try add, list, or set-default)", args[0])
	}
}

// parseAddArgs pulls the model name and optional --host URL out of add's args.
func parseAddArgs(rest []string) (base, host string) {
	for i := 0; i < len(rest); i++ {
		switch {
		case rest[i] == "--host" && i+1 < len(rest):
			host = rest[i+1]
			i++
		case strings.HasPrefix(rest[i], "--host="):
			host = strings.TrimPrefix(rest[i], "--host=")
		case !strings.HasPrefix(rest[i], "-") && base == "":
			base = rest[i]
		}
	}
	return base, host
}

// addRemoteModel registers a model that lives on another ollama server on the
// network. It verifies the model exists there; it does not pull or create.
func addRemoteModel(cfgPath string, cfg *model.Config, name, host string) error {
	url := model.NormalizeHost(host)
	inst, err := model.NewClient().InstalledModels(url)
	if err != nil {
		return fmt.Errorf("cannot reach ollama at %s: %w", url, err)
	}
	if !contains(inst, name) {
		return fmt.Errorf("%q is not installed on %s. Models there: %s", name, url, strings.Join(inst, ", "))
	}
	cfg.AddModel(model.ModelEntry{Name: name, Host: url, ToolMode: toolModeFor(name)})
	if err := cfg.Save(cfgPath); err != nil {
		return err
	}
	fmt.Println(green("✓ ") + fmt.Sprintf("added %s from %s. Use it with %s", bold(name), url, cyan("/model "+name)))
	return nil
}


func installedModels() []string {
	names, _ := model.NewClient().InstalledModels("http://localhost:11434")
	return names
}

func registered(cfg *model.Config, name string) bool {
	return slices.Contains(cfg.Names(), name)
}

// modelsList shows every configured model with its server and readiness,
// marking the default.
func modelsList(cfg *model.Config) error {
	names := cfg.Names()
	if len(names) == 0 {
		fmt.Println(dim("no models configured. Run ") + cyan("pilot start"))
		return nil
	}
	client := model.NewClient()
	cache := map[string]map[string]bool{}
	fmt.Println(bold("Configured models:"))
	for _, n := range names {
		url, _ := cfg.URLFor(n)
		set, ok := cache[url]
		if !ok {
			set = map[string]bool{}
			inst, _ := client.InstalledModels(url)
			for _, m := range inst {
				set[m] = true
			}
			cache[url] = set
		}
		mark := "  "
		if n == cfg.Default {
			mark = green("➤ ")
		}
		status := red("offline")
		if len(set) > 0 {
			if set[n] {
				status = green("ready")
			} else {
				status = yellow("not found")
			}
		}
		fmt.Printf("%s%s  %s  %s\n", mark, pad(n, 26), status, dim(url))
	}
	return nil
}

// setDefaultModel makes a configured model the default, asking the user to pick
// one when no name is given. Remote models (registered by host) are eligible.
func setDefaultModel(cfgPath string, cfg *model.Config, name string) error {
	if name == "" {
		choices := cfg.Names()
		if len(choices) == 0 {
			return fmt.Errorf("no models configured. Run %s first", cyan("pilot start"))
		}
		name = selectModel(choices, cfg.Default)
		if name == "" {
			return fmt.Errorf("no model selected")
		}
	} else if !registered(cfg, name) && !contains(installedModels(), name) {
		return fmt.Errorf("%q is not configured or installed locally. Add it with %s", name, cyan("pilot models add "+name))
	}
	if !registered(cfg, name) {
		cfg.AddModel(model.ModelEntry{Name: name, ToolMode: toolModeFor(name), Port: 11434, NumCtx: defaultNumCtx})
	}
	cfg.Default = name
	if err := cfg.Save(cfgPath); err != nil {
		return err
	}
	fmt.Println(green("✓ ") + "default model is now " + bold(name))
	return nil
}

// selectModel shows a numbered picker of installed models.
func selectModel(models []string, current string) string {
	if !stdinIsTTY() {
		return ""
	}
	fmt.Println(bold("Select the default model:"))
	for i, m := range models {
		tag := ""
		if m == current {
			tag = dim("  (current)")
		}
		fmt.Printf("  %s %s%s\n", cyan(strconv.Itoa(i+1)+"."), m, tag)
	}
	choice := strings.TrimSpace(promptLine(fmt.Sprintf("Select [1-%d]: ", len(models))))
	if n, err := strconv.Atoi(choice); err == nil && n >= 1 && n <= len(models) {
		return models[n-1]
	}
	return ""
}

func contains(list []string, s string) bool {
	return slices.Contains(list, s)
}

// stop stops the running ollama server.
func stop() error {
	url := "http://localhost:11434"
	if !ollamaUp(url) {
		fmt.Println(dim("ollama is not running"))
		return nil
	}
	var cmd *exec.Cmd
	if osName() == "windows" {
		cmd = exec.Command("taskkill", "/F", "/IM", "ollama.exe")
	} else {
		cmd = exec.Command("pkill", "-x", "ollama")
	}
	_ = cmd.Run() // non-zero when nothing matched; readiness check below is the truth
	for i := 0; i < 10; i++ {
		if !ollamaUp(url) {
			fmt.Println(green("✓ ") + "ollama stopped")
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("could not stop ollama; it may be running as a service. Stop it manually")
}

// ensureOllama makes sure the ollama server answers, starting it if it does not.
func ensureOllama(url string) error {
	if ollamaUp(url) {
		fmt.Println(green("✓ ") + "ollama running")
		return nil
	}
	fmt.Println(dim("… starting ollama serve"))
	cmd := exec.Command("ollama", "serve")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ollama: %w", err)
	}
	for range 40 {
		if ollamaUp(url) {
			fmt.Println(green("✓ ") + "ollama running")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("ollama did not come up on %s", url)
}

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

func modelInstalled(name string) bool {
	return exec.Command("ollama", "show", name).Run() == nil
}

// installModel pulls the base model and creates the customized local model.
func installModel(entry model.ModelEntry) error {
	base := entry.Base
	if base == "" {
		base = entry.Name
	}
	fmt.Println(dim("… pulling ") + base + dim(" (one-time download)"))
	if err := stream("ollama", "pull", base); err != nil {
		return fmt.Errorf("pull %s: %w", base, err)
	}
	fmt.Printf("%screating %s (tool-call template + num_ctx %d)\n", dim("… "), entry.Name, entry.NumCtx)
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
	return stream("ollama", "create", entry.Name, "-f", tmp.Name())
}

// buildModelfile derives the local Modelfile from the base and bakes in num_ctx.
// For qwen2.5-coder ONLY, it swaps the <tool_call> tags to [tool_call], which
// that family emits reliably; other models emit <tool_call> natively, so their
// template is left untouched (swapping it would break their tool-call parsing).
func buildModelfile(base string, numCtx int) (string, error) {
	out, err := exec.Command("ollama", "show", base, "--modelfile").Output()
	if err != nil {
		return "", fmt.Errorf("read base modelfile: %w", err)
	}
	mf := string(out)
	if strings.Contains(strings.ToLower(base), "qwen2.5-coder") {
		mf = strings.ReplaceAll(mf, "</tool_call>", "[/tool_call]")
		mf = strings.ReplaceAll(mf, "<tool_call>", "[tool_call]")
	}
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

// withConfig prepends --config unless the args already set one.
func withConfig(args []string, cfg string) []string {
	for _, a := range args {
		if a == "--config" || strings.HasPrefix(a, "--config=") {
			return args
		}
	}
	return append([]string{"--config", cfg}, args...)
}
