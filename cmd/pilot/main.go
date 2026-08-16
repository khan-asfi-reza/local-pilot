// Command pilot is the local-pilot entry point: it sets up ollama and the local
// model, and launches the coding agent. Config and skills live in a global data
// directory (see the appdir package) so pilot runs from anywhere.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"harness/eval/runner"
	"harness/harness/appdir"
	"harness/harness/model"
	"harness/terminal"
)

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
	case "web":
		err = web()
	case "models":
		err = modelsCmd(args[1:])
	case "skill", "skills":
		err = skillCmd(args[1:])
	case "context":
		err = contextCmd(args[1:])
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
	case "eval":
		cfgPath, e := appdir.Ensure()
		if e != nil {
			fatal(e)
		}
		runner.Run(withConfig(args[1:], cfgPath))
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
	header()
	_, name, err := ensureStack()
	if err != nil {
		return err
	}
	readyBanner(name)
	return nil
}

// ensureStack brings ollama up and guarantees a default model is installed,
// returning the config path and the model name. Shared by `start` and `web`.
func ensureStack() (cfgPath, name string, err error) {
	cfgPath, err = appdir.Ensure()
	if err != nil {
		return
	}
	cfg, err := model.LoadConfig(cfgPath)
	if err != nil {
		return
	}
	// Only start/manage a LOCAL ollama when the default model actually lives on
	// localhost. A remote default (another box on the LAN) must NOT trigger a local
	// server — starting one wastes resources and can hijack requests.
	if d := cfg.Default; d != "" {
		if url, ok := cfg.URLFor(d); ok && url != "" && !isLocalOllama(url) {
			fmt.Println(green("✓ ") + "default model is remote (" + url + ") — not starting a local ollama")
			name = d
			return
		}
	}
	if err = ensureOllamaInstalled(); err != nil {
		return
	}
	if err = ensureOllama("http://localhost:11434", desiredContextLength(cfg)); err != nil {
		return
	}

	name = cfg.Default
	if name == "" || !defaultUsable(cfg, name) {
		// first run, or the saved default is no longer usable: pick + build one.
		base := chooseModelBase(cfg)
		n, e := buildAndRegister(cfgPath, cfg, base)
		if e != nil {
			err = fmt.Errorf("could not install %q. Is that an ollama model? See https://ollama.com/library\n  %v", base, e)
			return
		}
		name = n
		cfg.Default = name
		if err = cfg.Save(cfgPath); err != nil {
			return
		}
	} else {
		fmt.Println(green("✓ ") + name + dim(" already set up"))
	}
	return
}

// readyBanner prints the post-start summary: the default model, how to change it,
// and the LAN address other machines on the network can point at.
func readyBanner(name string) {
	fmt.Println()
	fmt.Println(green("✓ ") + bold("pilot ready"))
	fmt.Println(dim("  data dir:       ") + appdir.Dir())
	fmt.Println(bold("  Default Model:  ") + cyan(name))
	fmt.Println(dim("  change default: ") + cyan("pilot models set-default <name>"))
	if ip := localIP(); ip != "" {
		fmt.Println(dim("  LAN access:     ") + cyan("http://"+ip+":11434"))
		fmt.Println(dim("  add from another machine: ") + cyan("pilot models add <name> --host "+ip+":11434"))
	}
	fmt.Println(dim("  next:           ") + cyan("pilot code") + dim(" --dir <project>"))
}

// localIP returns this machine's primary LAN IPv4 address, or "" if none.
func localIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return ""
}

const (
	harnessPort  = 9000
	backendPort  = 8182
	frontendPort = 5173
)

// web starts the full browser stack — harness-server, the FastAPI backend, and
// the Vite frontend — opens the chat UI, and blocks until interrupted.
func web() error {
	header()
	cfgPath, name, err := ensureStack()
	if err != nil {
		return err
	}

	root, err := repoRoot()
	if err != nil {
		return err
	}
	backendDir := filepath.Join(root, "backend")
	frontendDir := filepath.Join(root, "frontend")
	if !dirExists(backendDir) || !dirExists(frontendDir) {
		return fmt.Errorf("run %s from the local-pilot project root (needs ./backend and ./frontend)", cyan("pilot web"))
	}

	// Free the stack ports so re-running `pilot web` restarts cleanly instead of
	// colliding with a previous run (ollama on 11434 is left alone).
	for _, p := range []int{harnessPort, backendPort, frontendPort} {
		freePort(p)
	}

	frontendURL := fmt.Sprintf("http://localhost:%d", frontendPort)
	backendURL := fmt.Sprintf("http://localhost:%d", backendPort)

	var procs []*exec.Cmd
	stopAll := func() {
		for _, c := range procs {
			killProcess(c)
		}
	}
	defer stopAll()

	// harness-server bridges the web backend to the model. Prefer the prebuilt
	// binary; fall back to `go run` when it is not built yet.
	var hs *exec.Cmd
	if bin := filepath.Join(root, "bin", "harness-server"); fileExists(bin) {
		hs = command(root, bin, "-port", strconv.Itoa(harnessPort), "-config", cfgPath)
	} else {
		hs = command(root, "go", "run", "./harness/server", "-port", strconv.Itoa(harnessPort), "-config", cfgPath)
	}
	// Bind 0.0.0.0 so other machines on the LAN can reach the backend and UI.
	be := command(backendDir, pythonBin(backendDir), "-m", "uvicorn", "main:app", "--host", "0.0.0.0", "--port", strconv.Itoa(backendPort))
	// The backend resolves its SQLite DB from the global data dir (see
	// backend/core/database.py), so profile/thread state lives in one place no
	// matter where pilot runs — don't pin it to a cwd-relative file.
	be.Env = append(os.Environ(),
		fmt.Sprintf("HARNESS_URL=http://localhost:%d/run", harnessPort),
	)
	fe := command(frontendDir, npmBin(), "run", "dev", "--", "--host", "--port", strconv.Itoa(frontendPort))

	// The Telegram bridge is optional: launch it when present. It idles until a
	// bot token is set in Settings, so starting it unconditionally is safe; a
	// missing Python dep just makes it exit without affecting the rest.
	telegramDir := filepath.Join(root, "telegram")
	var tg *exec.Cmd
	if fileExists(filepath.Join(telegramDir, "bot.py")) {
		tg = command(telegramDir, pythonBin(telegramDir), "bot.py")
		tg.Env = append(os.Environ(), fmt.Sprintf("BACKEND_URL=http://localhost:%d", backendPort))
	}

	fmt.Println(dim("… starting services"))
	svcs := []*exec.Cmd{hs, be, fe}
	if tg != nil {
		svcs = append(svcs, tg)
	}
	for _, c := range svcs {
		setProcGroup(c)
		if e := c.Start(); e != nil {
			return fmt.Errorf("start %s: %w", filepath.Base(c.Path), e)
		}
		procs = append(procs, c)
	}

	waitPort(harnessPort)
	waitPort(backendPort)
	if !waitPort(frontendPort) {
		fmt.Println(yellow("• ") + "frontend still starting; give it a moment")
	}

	fmt.Println()
	fmt.Println(green("✓ ") + bold("Local Pilot web is running"))
	fmt.Println(bold("  Open:   ") + cyan(frontendURL))
	fmt.Println(dim("  api:    ") + backendURL)
	fmt.Println(dim("  model:  ") + name)
	if tg != nil {
		fmt.Println(dim("  telegram: ") + "bridge running — set a bot token in Settings → Telegram")
	}
	if ip := localIP(); ip != "" {
		fmt.Println(dim("  on your LAN: ") + cyan(fmt.Sprintf("http://%s:%d", ip, frontendPort)))
	}
	fmt.Println(dim("  press Ctrl-C to stop"))
	openBrowser(frontendURL)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	fmt.Println("\n" + dim("… stopping services"))
	return nil
}

// command builds a child process rooted at dir with its output attached.
func command(dir, name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	return c
}

// repoRoot walks up from the working directory to the folder holding go.mod.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if fileExists(filepath.Join(dir, "go.mod")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return os.Getwd()
		}
		dir = parent
	}
}

// pythonBin prefers a backend virtualenv interpreter, else the system python.
func pythonBin(backendDir string) string {
	for _, rel := range [][]string{
		{".venv", "bin", "python"}, {"venv", "bin", "python"},
		{".venv", "Scripts", "python.exe"}, {"venv", "Scripts", "python.exe"},
	} {
		if p := filepath.Join(append([]string{backendDir}, rel...)...); fileExists(p) {
			return p
		}
	}
	if p, err := exec.LookPath("python3"); err == nil {
		return p
	}
	return "python"
}

// npmBin returns the npm executable name for this OS.
func npmBin() string {
	if osName() == "windows" {
		return "npm.cmd"
	}
	return "npm"
}

// freePort kills whatever is listening on a local TCP port, so a restart can
// rebind it. Best effort and OS-specific.
func freePort(port int) {
	if osName() == "windows" {
		out, err := exec.Command("netstat", "-ano", "-p", "tcp").Output()
		if err != nil {
			return
		}
		needle := fmt.Sprintf(":%d", port)
		seen := map[string]bool{}
		for _, line := range strings.Split(string(out), "\n") {
			if !strings.Contains(line, needle) || !strings.Contains(line, "LISTENING") {
				continue
			}
			f := strings.Fields(line)
			if len(f) == 0 {
				continue
			}
			pid := f[len(f)-1]
			if pid != "0" && !seen[pid] {
				seen[pid] = true
				_ = exec.Command("taskkill", "/F", "/PID", pid).Run()
			}
		}
		return
	}
	out, err := exec.Command("lsof", "-ti", fmt.Sprintf("tcp:%d", port)).Output()
	if err != nil {
		return
	}
	for _, pid := range strings.Fields(string(out)) {
		_ = exec.Command("kill", pid).Run()
	}
}

// waitPort polls a local TCP port until it accepts connections (up to ~30s).
func waitPort(port int) bool {
	addr := fmt.Sprintf("localhost:%d", port)
	for range 60 {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// openBrowser opens url in the default browser (best effort).
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch osName() {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// toolModeFor picks the tool-calling strategy by model size: small models (≤3B)
// are unreliable at native tool calls, so they use the grammar-constrained JSON
// path (enforced via ollama's response_format); larger models — including the
// qwen3 family, which handle native calls fine — use native tool calls.
func toolModeFor(base string) string {
	b := strings.ToLower(base)
	for _, s := range []string{"0.5b", "1b", "1.5b", "2b", "3b"} {
		if strings.Contains(b, ":"+s) || strings.Contains(b, "-"+s) {
			return model.ToolModeJSON
		}
	}
	return model.ToolModeNative
}

// needsToolTemplate reports whether a base model needs the tool-call template
// edit. Only qwen2.5-coder does: it emits [tool_call] reliably but not the
// <tool_call> tags the OpenAI-compatible path expects. Every other family emits
// native tool calls unedited, so it is used directly with no derived model.
func needsToolTemplate(base string) bool {
	return strings.Contains(strings.ToLower(base), "qwen2.5-coder")
}

// buildAndRegister installs base and records it in models.json, returning the
// model name to use. Models that need the tool-call template edit get a derived
// <base>-tools variant (and the base copy is removed); all others are registered
// and used directly. Context size is not baked — it is set globally via
// OLLAMA_CONTEXT_LENGTH. Idempotent.
func buildAndRegister(cfgPath string, cfg *model.Config, base string) (string, error) {
	name := base
	var entry model.ModelEntry
	if needsToolTemplate(base) {
		name = base + "-tools"
		entry = model.ModelEntry{Name: name, Base: base, ToolMode: toolModeFor(base), Port: 11434}
		if !modelInstalled(name) {
			if err := installModel(entry); err != nil {
				return "", err
			}
		} else {
			fmt.Println(green("✓ ") + name + dim(" already installed"))
		}
	} else {
		entry = model.ModelEntry{Name: base, ToolMode: toolModeFor(base), Port: 11434}
		if !modelInstalled(base) {
			fmt.Println(dim("… pulling ") + base + dim(" (one-time download)"))
			if err := stream("ollama", "pull", base); err != nil {
				return "", fmt.Errorf("pull %s: %w", base, err)
			}
		} else {
			fmt.Println(green("✓ ") + base + dim(" already installed"))
		}
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
	fmt.Println(dim("  or type any ollama model name, e.g. ") + "qwen2.5-coder:14b")

	choice := strings.TrimSpace(promptLine(fmt.Sprintf("Select [1-%d], type a model name, or Enter for default: ", len(presets))))
	if choice == "" {
		return presets[0]
	}
	// A bare number picks a preset; anything else is taken as a model name and
	// looked up directly in ollama (no need to pick a "custom" option first).
	if n, err := strconv.Atoi(choice); err == nil {
		if n >= 1 && n <= len(presets) {
			return presets[n-1]
		}
		return presets[0] // number out of range → default
	}
	return choice
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
		return fmt.Errorf("usage: pilot models <add <model> [--host URL] [--name LABEL] | remove <name> | list | set-default [name]>")
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
		base, host, name := parseAddArgs(args[1:])
		if base == "" {
			return fmt.Errorf("usage: pilot models add <ollama-model> [--host URL] [--name LABEL]")
		}
		if host != "" {
			return addRemoteModel(cfgPath, cfg, base, host, name)
		}
		if err := ensureOllama("http://localhost:11434", desiredContextLength(cfg)); err != nil {
			return err
		}
		name, err := buildAndRegister(cfgPath, cfg, base)
		if err != nil {
			return fmt.Errorf("could not add %q: %w", base, err)
		}
		fmt.Println(green("✓ ") + fmt.Sprintf("added %s. Use it with %s", bold(name), cyan("/model "+name)))
		return nil
	case "remove", "rm":
		name := ""
		if len(args) >= 2 {
			name = args[1]
		}
		return removeModel(cfgPath, cfg, name)
	case "list":
		return modelsList(cfg)
	case "set-default":
		name := ""
		if len(args) >= 2 {
			name = args[1]
		}
		return setDefaultModel(cfgPath, cfg, name)
	case "set-default-planner":
		name := ""
		if len(args) >= 2 {
			name = args[1]
		}
		return setDefaultPlanner(cfgPath, cfg, name)
	default:
		return fmt.Errorf("unknown: pilot models %s (try add, remove, list, set-default, or set-default-planner)", args[0])
	}
}

// removeModel drops a model from the registry and, for a local model, deletes it
// from ollama to free disk (a remote model lives on another server). It reassigns
// the default/active if they pointed at the removed model, so the next start is
// clean.
func removeModel(cfgPath string, cfg *model.Config, name string) error {
	if name == "" {
		choices := cfg.Names()
		if len(choices) <= 1 {
			return fmt.Errorf("usage: pilot models remove <name>")
		}
		name = selectModel(choices, cfg.Default)
		if name == "" {
			return fmt.Errorf("no model selected")
		}
	}
	entry, ok := cfg.EntryFor(name)
	if !ok {
		return fmt.Errorf("%q is not a configured model. See %s", name, cyan("pilot models list"))
	}
	if err := cfg.Remove(name); err != nil {
		return err
	}
	if err := cfg.Save(cfgPath); err != nil {
		return err
	}
	fmt.Println(green("✓ ") + "removed " + bold(name) + " from the registry")
	if entry.Host == "" {
		fmt.Println(dim("… deleting ollama model ") + entry.Name)
		_ = stream("ollama", "rm", entry.Name)
		if base := entry.Base; base != "" && base != entry.Name {
			_ = stream("ollama", "rm", base)
		}
	}
	return nil
}

// contextCmd shows or sets the ollama context window. With no argument it prints
// the machine profile and the size in effect; with a number (or "auto") it saves
// the choice and restarts ollama so the new window takes effect immediately.
func contextCmd(args []string) error {
	cfgPath, err := appdir.Ensure()
	if err != nil {
		return err
	}
	cfg, err := model.LoadConfig(cfgPath)
	if err != nil {
		return err
	}
	hw := detectHardware()
	auto := pickContextLength(hw)

	if len(args) == 0 {
		fmt.Println(bold("Ollama context length"))
		fmt.Printf("%s%d GiB RAM", dim("  hardware:  "), hw.RAMGiB)
		if hw.VRAMGiB > 0 {
			fmt.Printf(", %d GiB VRAM", hw.VRAMGiB)
		}
		if hw.AppleSilicon {
			fmt.Print(", Apple Silicon")
		}
		fmt.Println()
		fmt.Printf("%s%d tokens\n", dim("  auto-size: "), auto)
		want := desiredContextLength(cfg)
		if cfg.ContextLength > 0 {
			fmt.Printf("%s%d tokens (override)\n", dim("  in use:    "), cfg.ContextLength)
		} else {
			fmt.Printf("%s%d tokens (auto)\n", dim("  in use:    "), auto)
		}
		// Show the window the RUNNING server actually loaded models at — this is what
		// bites a build, and it can be far below the configured value if ollama was
		// started elsewhere (e.g. the Windows app) at its 4096 default.
		if now, ok := ollamaLoadedContext("http://localhost:11434"); ok {
			if now < want {
				fmt.Printf("%s%d tokens — SMALLER than configured; run %s to restart ollama and apply\n",
					yellow("  server now:"), now, cyan("pilot context "+strconv.Itoa(want)))
			} else {
				fmt.Printf("%s%d tokens\n", dim("  server now:"), now)
			}
		}
		fmt.Println(dim("  set with:  ") + cyan("pilot context <tokens>") + dim(" or ") + cyan("pilot context auto"))
		return nil
	}

	var n int
	if strings.EqualFold(args[0], "auto") {
		n = 0
	} else {
		v, e := strconv.Atoi(args[0])
		if e != nil || v < ctxMin {
			return fmt.Errorf("context length must be a number >= %d, or \"auto\"", ctxMin)
		}
		n = v
	}
	cfg.ContextLength = n
	if err := cfg.Save(cfgPath); err != nil {
		return err
	}

	effective := desiredContextLength(cfg)
	url := "http://localhost:11434"
	if ollamaUp(url) {
		fmt.Println(dim("… restarting ollama"))
		killOllama()
		for range 10 {
			if !ollamaUp(url) {
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
	}
	if err := ensureOllama(url, effective); err != nil {
		return err
	}
	label := fmt.Sprintf("%d tokens", effective)
	if n == 0 {
		label += " (auto)"
	}
	fmt.Println(green("✓ ") + "context length is now " + bold(label))
	return nil
}

// parseAddArgs pulls the model name and optional --host URL out of add's args.
func parseAddArgs(rest []string) (base, host, name string) {
	for i := 0; i < len(rest); i++ {
		switch {
		case rest[i] == "--host" && i+1 < len(rest):
			host = rest[i+1]
			i++
		case strings.HasPrefix(rest[i], "--host="):
			host = strings.TrimPrefix(rest[i], "--host=")
		case rest[i] == "--name" && i+1 < len(rest):
			name = rest[i+1]
			i++
		case strings.HasPrefix(rest[i], "--name="):
			name = strings.TrimPrefix(rest[i], "--name=")
		case !strings.HasPrefix(rest[i], "-") && base == "":
			base = rest[i]
		}
	}
	return base, host, name
}

// addRemoteModel registers a model that lives on another ollama server on the
// network. It verifies the model exists there; it does not pull or create. The
// registry label is disambiguated from any same-tag local model (or an explicit
// --name), and the real ollama tag is stored in Model so both can be used.
func addRemoteModel(cfgPath string, cfg *model.Config, tag, host, name string) error {
	url := model.NormalizeHost(host)
	inst, err := model.NewClient().InstalledModels(url)
	if err != nil {
		return fmt.Errorf("cannot reach ollama at %s: %w", url, err)
	}
	if !contains(inst, tag) {
		return fmt.Errorf("%q is not installed on %s. Models there: %s", tag, url, strings.Join(inst, ", "))
	}
	regName := strings.TrimSpace(name)
	if regName == "" {
		regName = cfg.DeriveName(tag, host)
	}
	if registered(cfg, regName) {
		return fmt.Errorf("the name %q is already in use; pass a different --name", regName)
	}
	entry := model.ModelEntry{Name: regName, Host: url, ToolMode: toolModeFor(tag)}
	if regName != tag {
		entry.Model = tag
	}
	cfg.AddModel(entry)
	if err := cfg.Save(cfgPath); err != nil {
		return err
	}
	fmt.Println(green("✓ ") + fmt.Sprintf("added %s (%s on %s). Use it with %s", bold(regName), tag, url, cyan("/model "+regName)))
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
func setDefaultPlanner(cfgPath string, cfg *model.Config, name string) error {
	if name != "" && !registered(cfg, name) && !contains(installedModels(), name) {
		return fmt.Errorf("%q is not configured or installed locally. Add it with %s", name, cyan("pilot models add "+name))
	}
	if name != "" && !registered(cfg, name) {
		cfg.AddModel(model.ModelEntry{Name: name, ToolMode: toolModeFor(name), Port: 11434})
	}
	if err := cfg.SetPlanner(name); err != nil {
		return err
	}
	if err := cfg.Save(cfgPath); err != nil {
		return err
	}
	if name == "" {
		fmt.Println(green("✓ ") + "planner model cleared (falls back to the default model)")
	} else {
		fmt.Println(green("✓ ") + "planner model is now " + bold(name))
	}
	return nil
}

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
		cfg.AddModel(model.ModelEntry{Name: name, ToolMode: toolModeFor(name), Port: 11434})
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

// ensuredContext is the OLLAMA_CONTEXT_LENGTH pilot has already guaranteed the
// running server was started with, this process. It lets repeated ensureOllama
// calls in one launch skip a redundant restart once the window is known-good.
var ensuredContext int

// ensureOllama makes sure the ollama server answers, starting it if it does not.
// It ALWAYS enforces the context window: an already-running server whose window
// is below ctxLen (or cannot be confirmed) is restarted with OLLAMA_CONTEXT_LENGTH,
// so `pilot` never silently serves at ollama's small default (e.g. 4096). It also
// keeps ollama bound to all interfaces so LAN machines can reach it.
func ensureOllama(url string, ctxLen int) error {
	persistOllamaLANHost()
	if ctxLen > 0 {
		persistOllamaContext(ctxLen)
	}
	if ollamaUp(url) {
		// Decide if the window is already big enough. A loaded model is authoritative
		// (its context_length is the real window). When nothing is loaded /api/ps says
		// nothing, so fall back to the marker of what pilot last launched ollama with —
		// otherwise every launch would restart an already-correctly-sized idle server
		// (it just restarted, no model loaded yet → can't confirm → restart again…).
		ctxOK := ctxLen <= 0 || ensuredContext >= ctxLen
		if !ctxOK {
			if now, known := ollamaLoadedContext(url); known {
				ctxOK = now >= ctxLen
			} else {
				ctxOK = readLaunchedCtx() >= ctxLen
			}
		}
		if ctxOK && ollamaReachableOnLAN() {
			if ctxLen > 0 {
				ensuredContext = ctxLen
			}
			fmt.Println(green("✓ ") + "ollama running")
			return nil
		}
		if !ctxOK {
			fmt.Printf("%sollama context window is below %d tokens — restarting to resize\n", yellow("• "), ctxLen)
		} else {
			// Up, but bound to localhost only: rebind it so the LAN can reach it.
			fmt.Println(yellow("• ") + "ollama bound to localhost only — rebinding for LAN access")
		}
		killOllama()
		for range 10 {
			if !ollamaUp(url) {
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
		// Could not stop it (likely a managed service/desktop app): use it as-is
		// rather than fail, but say how to apply the window manually.
		if ollamaUp(url) {
			fmt.Printf("%scould not restart ollama (running as a service?); using it as-is. "+
				"To apply the window, set OLLAMA_CONTEXT_LENGTH=%d on that server and restart it.\n", yellow("• "), ctxLen)
			return nil
		}
	}
	fmt.Println(dim("… starting ollama serve"))
	cmd := exec.Command("ollama", "serve")
	env := os.Environ()
	// Bind all interfaces so other machines on the LAN can use this model host.
	if os.Getenv("OLLAMA_HOST") == "" {
		env = append(env, "OLLAMA_HOST=0.0.0.0:11434")
	}
	// Size the context window to this machine; our value wins so sizing is
	// predictable regardless of any inherited env.
	if ctxLen > 0 {
		env = append(env, fmt.Sprintf("OLLAMA_CONTEXT_LENGTH=%d", ctxLen))
	}
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ollama: %w", err)
	}
	for range 40 {
		if ollamaUp(url) {
			if ctxLen > 0 {
				ensuredContext = ctxLen
				writeLaunchedCtx(ctxLen) // remember the window across pilot invocations
			}
			fmt.Println(green("✓ ") + "ollama running")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("ollama did not come up on %s", url)
}

// ollamaCtxMarkerPath is the file recording the OLLAMA_CONTEXT_LENGTH pilot last
// launched ollama with, so a later pilot run trusts an already-sized idle server
// (which /api/ps cannot confirm) instead of restarting it on every launch.
func ollamaCtxMarkerPath() string {
	cfgPath, err := appdir.Ensure()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(cfgPath), ".ollama-ctx")
}

func readLaunchedCtx() int {
	p := ollamaCtxMarkerPath()
	if p == "" {
		return 0
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

func writeLaunchedCtx(n int) {
	if p := ollamaCtxMarkerPath(); p != "" {
		_ = os.WriteFile(p, []byte(strconv.Itoa(n)), 0o644)
	}
}

// ollamaLoadedContext returns the smallest context window across currently-loaded
// models (via /api/ps), and whether anything is loaded to report. Ollama loads a
// model at the server's OLLAMA_CONTEXT_LENGTH default, so this reveals the window
// actually in force — which can be far below what pilot configured if the server
// was started elsewhere (e.g. the Windows Ollama app) at its 4096 default.
func ollamaLoadedContext(url string) (int, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(url, "/")+"/api/ps", nil)
	if err != nil {
		return 0, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	var body struct {
		Models []struct {
			ContextLength int `json:"context_length"`
		} `json:"models"`
	}
	if json.NewDecoder(resp.Body).Decode(&body) != nil || len(body.Models) == 0 {
		return 0, false
	}
	min := body.Models[0].ContextLength
	for _, m := range body.Models {
		if m.ContextLength < min {
			min = m.ContextLength
		}
	}
	return min, true
}

// ollamaContextAtLeast reports whether a currently-loaded model confirms the
// server's context window is at least want tokens. If nothing is loaded or the
// probe fails it cannot confirm and returns false, so the caller restarts the
// server to guarantee the window rather than trust a small default.
func ollamaContextAtLeast(url string, want int) bool {
	n, ok := ollamaLoadedContext(url)
	return ok && n >= want
}

// ollamaReachableOnLAN reports whether ollama answers on this machine's LAN IP
// (not just localhost). True when there is no LAN address to expose.
func ollamaReachableOnLAN() bool {
	ip := localIP()
	if ip == "" {
		return true
	}
	return ollamaUp("http://" + ip + ":11434")
}

// persistOllamaLANHost sets OLLAMA_HOST=0.0.0.0:11434 persistently so future
// ollama launches (including the desktop app) bind to the LAN. Best effort.
func persistOllamaLANHost() {
	if os.Getenv("OLLAMA_HOST") != "" {
		return
	}
	switch osName() {
	case "darwin":
		_ = exec.Command("launchctl", "setenv", "OLLAMA_HOST", "0.0.0.0:11434").Run()
	case "windows":
		_ = exec.Command("setx", "OLLAMA_HOST", "0.0.0.0:11434").Run()
	}
}

// persistOllamaContext records OLLAMA_CONTEXT_LENGTH persistently so future
// ollama launches (including the desktop app) use the same window. Best effort.
func persistOllamaContext(n int) {
	v := strconv.Itoa(n)
	switch osName() {
	case "darwin":
		_ = exec.Command("launchctl", "setenv", "OLLAMA_CONTEXT_LENGTH", v).Run()
	case "windows":
		_ = exec.Command("setx", "OLLAMA_CONTEXT_LENGTH", v).Run()
	}
}

// desiredContextLength is the OLLAMA_CONTEXT_LENGTH to run with: the user's saved
// override when set, otherwise the size that fits this machine's hardware.
func desiredContextLength(cfg *model.Config) int {
	if cfg != nil && cfg.ContextLength > 0 {
		return cfg.ContextLength
	}
	return pickContextLength(detectHardware())
}

// killOllama stops any running ollama server (so it can be rebound).
func killOllama() {
	if osName() == "windows" {
		_ = exec.Command("taskkill", "/F", "/IM", "ollama.exe").Run()
		return
	}
	_ = exec.Command("pkill", "-x", "ollama").Run()
}

// isLocalOllama reports whether an ollama URL points at this machine (so a remote
// default model does not trigger a local server).
func isLocalOllama(url string) bool {
	u := strings.ToLower(url)
	return strings.Contains(u, "localhost") || strings.Contains(u, "127.0.0.1") ||
		strings.Contains(u, "0.0.0.0") || strings.Contains(u, "::1")
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

// defaultUsable reports whether the configured default needs no setup at startup:
// a REMOTE model lives on another server (never a local install, so never prompt
// or pull for it), and a LOCAL model is checked by its ollama TAG — not its
// registry label, which may differ (e.g. "qwen3.5:9b (192.168.10.99)").
func defaultUsable(cfg *model.Config, name string) bool {
	entry, ok := cfg.EntryFor(name)
	if !ok {
		return false
	}
	if entry.Host != "" {
		return true // remote: configured deliberately; don't force a local model
	}
	tag := entry.Model
	if tag == "" {
		tag = entry.Name
	}
	return modelInstalled(tag)
}

// installModel pulls the base model, creates the edited <base>-tools variant,
// then removes the base copy. Only called for models that need the tool-call
// template edit (see needsToolTemplate).
func installModel(entry model.ModelEntry) error {
	base := entry.Base
	if base == "" {
		base = entry.Name
	}
	fmt.Println(dim("… pulling ") + base + dim(" (one-time download)"))
	if err := stream("ollama", "pull", base); err != nil {
		return fmt.Errorf("pull %s: %w", base, err)
	}
	fmt.Printf("%screating %s (tool-call template)\n", dim("… "), entry.Name)
	mf, err := buildModelfile(base)
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
		return err
	}
	// The derived model is self-contained (its own weight blob), so drop the base
	// copy to reclaim disk — only the edited model is ever used.
	if base != entry.Name {
		fmt.Println(dim("… removing base copy ") + base)
		_ = stream("ollama", "rm", base)
	}
	return nil
}

// buildModelfile derives the local Modelfile from the base, swapping the
// <tool_call> tags to [tool_call], which qwen2.5-coder emits reliably. Context
// size is set globally via OLLAMA_CONTEXT_LENGTH, so nothing is baked here.
func buildModelfile(base string) (string, error) {
	out, err := exec.Command("ollama", "show", base, "--modelfile").Output()
	if err != nil {
		return "", fmt.Errorf("read base modelfile: %w", err)
	}
	mf := string(out)
	mf = strings.ReplaceAll(mf, "</tool_call>", "[/tool_call]")
	mf = strings.ReplaceAll(mf, "<tool_call>", "[tool_call]")
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
