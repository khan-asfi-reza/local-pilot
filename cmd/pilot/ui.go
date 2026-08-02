package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"
	"unicode/utf8"
)

var (
	stdin    = bufio.NewReader(os.Stdin)
	useColor = colorEnabled()
)

func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func paint(code, s string) string {
	if !useColor {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func bold(s string) string   { return paint("1", s) }
func dim(s string) string    { return paint("2", s) }
func red(s string) string    { return paint("31", s) }
func green(s string) string  { return paint("32", s) }
func yellow(s string) string { return paint("33", s) }
func cyan(s string) string   { return paint("36", s) }

func osName() string { return runtime.GOOS }

// stdinIsTTY reports whether input is an interactive terminal.
func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// promptLine prints a label and reads one line.
func promptLine(label string) string {
	fmt.Print(cyan("› ") + label)
	line, _ := stdin.ReadString('\n')
	return line
}

// confirm asks a yes/no question, defaulting to yes; non-interactive runs proceed.
func confirm(q string) bool {
	if !stdinIsTTY() {
		return true
	}
	ans := strings.ToLower(strings.TrimSpace(promptLine(q + " [Y/n]: ")))
	return ans == "" || ans == "y" || ans == "yes"
}

// header prints the start banner.
func header() {
	fmt.Println()
	fmt.Println(bold(cyan("local-pilot")) + dim("  ·  local offline coding agent"))
	fmt.Println(dim(strings.Repeat("─", 44)))
}

// helpText is the colored help shown for `pilot`, `pilot help`, and bad usage.
func helpText() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(bold(cyan("local-pilot")) + dim("  ·  local, offline-first coding agent") + "\n\n")
	b.WriteString(bold("USAGE") + "\n")
	b.WriteString("  " + cyan("pilot") + " <command> [options]\n\n")
	b.WriteString(bold("COMMANDS") + "\n")
	rows := [][2]string{
		{"start", "set up ollama and pick a model, then you're ready"},
		{"web", "run the browser chat stack and open it in your browser"},
		{"stop", "stop the ollama server"},
		{"models add <model> [--host URL]", "add a model (local, or from another ollama server)"},
		{"models list", "show configured models, their server, and the default"},
		{"models set-default", "pick the default from your configured models"},
		{"skill add <source>", "install a local skill (owner/repo[/path], git URL, or path)"},
		{"skill list", "list installed local skills"},
		{"context [tokens|auto]", "show or set the ollama context window (restarts ollama)"},
		{"code [--dir DIR]", "open the interactive coding agent"},
		{"run --dir DIR --task \"…\"", "run one task headless (--task-file, --max-steps, --format)"},
		{"help", "show this help"},
	}
	for _, r := range rows {
		b.WriteString("  " + cyan(pad(r[0], 26)) + dim(r[1]) + "\n")
	}
	b.WriteString("\n" + bold("EXAMPLES") + "\n")
	b.WriteString("  " + dim("# first-time setup") + "\n")
	b.WriteString("  " + cyan("pilot start") + "\n")
	b.WriteString("  " + dim("# chat in your browser") + "\n")
	b.WriteString("  " + cyan("pilot web") + "\n")
	b.WriteString("  " + dim("# work in a project") + "\n")
	b.WriteString("  " + cyan("pilot code --dir ~/my-app") + "\n")
	b.WriteString("  " + dim("# one-shot task") + "\n")
	b.WriteString("  " + cyan("pilot run --dir ~/my-app --task \"add a health endpoint\"") + "\n\n")
	b.WriteString(dim("data dir: ") + dataDirHint() + "\n")
	return b.String()
}

func pad(s string, n int) string {
	w := utf8.RuneCountInString(s)
	if w >= n {
		return s + " "
	}
	return s + strings.Repeat(" ", n-w)
}

func dataDirHint() string {
	switch runtime.GOOS {
	case "windows":
		return "%LOCALAPPDATA%\\localpilot"
	case "darwin":
		return "~/.localpilot"
	default:
		return "~/.local/share/localpilot"
	}
}
