package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"harness/harness/events"
)

const maxOutputBytes = 20_000

// runResult is the structured shape every executing tool returns, so a small
// model reasons over a clean, separated failure instead of one blob.
type runResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Command  string `json:"command"`
}

// shellRunTool runs a shell command in the working directory. It can run
// anything, so it is always mutating.
func shellRunTool() *Tool {
	return &Tool{
		Name:        "shell_run",
		Description: "Run a shell command in the working directory and return its output. Use it to run tests, builds, checks, installs, and tools yourself. It WAITS for the command to finish, so do NOT use it to start a long-running server (uvicorn, npm start/dev, a dev server): that blocks until the timeout. Use the `serve` tool for a server instead. Keep all paths inside the working directory: a command that reads or writes outside it needs the user's approval. Treat as able to change things. Terminal only.",
		Params:      json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"The command line to run."},"timeout_seconds":{"type":"integer","description":"Kill the command after this many seconds.","default":60}},"required":["command"]}`),
		Mutating:    true,
		EscapesSandbox: func(env Env, args Args) bool {
			return shellEscapesWorkDir(args.Str("command"), env.WorkDir)
		},
		Preview: func(env Env, args Args) (string, *events.Diff, error) {
			return "run: " + args.Str("command"), nil, nil
		},
		Run: func(env Env, args Args) (any, *events.Diff, error) {
			command := args.Str("command")
			if command == "" {
				return nil, nil, fmt.Errorf("command is required")
			}
			// A server command would block shell_run; redirect to the serve tool.
			if hint := ServerStartHint(command); hint != "" {
				return map[string]any{"error": "This command " + hint + ", so shell_run will not run it (it would block until timeout). Use the `serve` tool instead: call serve with this command and its port, then verify with a shell_run curl. Load the 'serving' skill for how to run and check a server in any language."}, nil, nil
			}
			// Bare pip fails on managed Python; force a virtualenv.
			if hint := NonVenvPipHint(command); hint != "" {
				return map[string]any{"error": "This command " + hint + ". First run `python3 -m venv .venv`, then install with `.venv/bin/pip install -r requirements.txt`, and afterwards use `.venv/bin/python` and `.venv/bin/uvicorn` (not bare pip/python3). Do not retry bare pip."}, nil, nil
			}
			timeout := time.Duration(args.Int("timeout_seconds", 60)) * time.Second
			ctx, cancel := context.WithTimeout(env.Ctx, timeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, "sh", "-c", command)
			cmd.Dir = env.WorkDir
			return runCommand(cmd, command), nil, nil
		},
	}
}

// serveTool starts a server as a background process and waits for its port,
// so the agent can verify a running server without blocking on shell_run.
func serveTool() *Tool {
	return &Tool{
		Name:        "serve",
		Description: "Start a long-running server (uvicorn, npm start/dev, or anything that does not exit on its own) as a BACKGROUND process and wait until it is listening on `port`. It returns once the server is up (or the wait times out), so you can then verify it with shell_run, e.g. `curl -s http://localhost:PORT/path`. The server runs in a separate process and is stopped automatically when the task ends. Use serve — NOT shell_run — for anything that stays running.",
		Params:      json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"The server command, e.g. 'uvicorn main:app --port 8000'."},"port":{"type":"integer","description":"The TCP port the server listens on, used to detect readiness."},"wait_seconds":{"type":"integer","description":"How long to wait for the port to open.","default":20}},"required":["command","port"]}`),
		Mutating:    true,
		Preview: func(env Env, args Args) (string, *events.Diff, error) {
			return "start server: " + args.Str("command"), nil, nil
		},
		Run: func(env Env, args Args) (any, *events.Diff, error) {
			command := args.Str("command")
			if command == "" {
				return nil, nil, fmt.Errorf("command is required")
			}
			port := args.Int("port", 0)
			wait := time.Duration(args.Int("wait_seconds", 20)) * time.Second

			cmd := exec.Command("sh", "-c", command)
			cmd.Dir = env.WorkDir
			// Own process group so the whole server tree can be killed at turn end.
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			var log bytes.Buffer
			cmd.Stdout = &log
			cmd.Stderr = &log
			if err := cmd.Start(); err != nil {
				return nil, nil, fmt.Errorf("start server: %w", err)
			}
			env.Procs.add(&bgProc{label: command, cmd: cmd, log: &log})

			ready := waitForPort(port, wait)
			note := fmt.Sprintf("server did not open port %d in time; read logs for the error", port)
			if ready {
				note = fmt.Sprintf("server is listening on port %d; verify it with a shell_run curl, then finish", port)
			}
			return map[string]any{
				"started": true,
				"ready":   ready,
				"pid":     cmd.Process.Pid,
				"port":    port,
				"logs":    truncateMiddle(log.String(), 4000),
				"note":    note,
			}, nil, nil
		},
	}
}

// codeRunTool runs a short snippet in an isolated temporary directory. It has no
// access to the project files, since it runs outside the working directory.
// Network isolation is not enforced by the OS here yet; this is the seam where a
// stronger sandbox (a container or jail) would slot in without changing callers.
func codeRunTool() *Tool {
	return &Tool{
		Name:        "code_run",
		Description: "Run a short code snippet in an isolated sandbox and return its output. No access to the project files or the host. Use to test logic or compute something, not to modify the project.",
		Params:      json.RawMessage(`{"type":"object","properties":{"language":{"type":"string","enum":["python","javascript"],"description":"Language of the snippet."},"code":{"type":"string","description":"The code to run."},"stdin":{"type":"string","description":"Optional standard input to feed the program."}},"required":["language","code"]}`),
		Mutating:    true,
		WebSafe:     true,
		Preview: func(env Env, args Args) (string, *events.Diff, error) {
			return fmt.Sprintf("run %s snippet in sandbox", args.Str("language")), nil, nil
		},
		Run: func(env Env, args Args) (any, *events.Diff, error) {
			lang := args.Str("language")
			code := args.Str("code")
			if code == "" {
				return nil, nil, fmt.Errorf("code is required")
			}
			dir, err := os.MkdirTemp("", "harness-sandbox-")
			if err != nil {
				return nil, nil, err
			}
			defer os.RemoveAll(dir)

			var file, bin string
			switch lang {
			case "python":
				file, bin = "snippet.py", "python3"
			case "javascript":
				file, bin = "snippet.js", "node"
			default:
				return nil, nil, fmt.Errorf("unsupported language %q", lang)
			}
			path := filepath.Join(dir, file)
			if err := os.WriteFile(path, []byte(code), 0o644); err != nil {
				return nil, nil, err
			}
			ctx, cancel := context.WithTimeout(env.Ctx, 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bin, path)
			cmd.Dir = dir
			// Start from a bare environment so the snippet cannot read host secrets.
			cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/local/bin", "HOME=" + dir}
			if in := args.Str("stdin"); in != "" {
				cmd.Stdin = strings.NewReader(in)
			}
			return runCommand(cmd, bin+" "+file), nil, nil
		},
	}
}

// runCommand executes a prepared command and captures its output into the
// structured result shape.
func runCommand(cmd *exec.Cmd, label string) runResult {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := runResult{
		Stdout:  truncateMiddle(stdout.String(), maxOutputBytes),
		Stderr:  truncateMiddle(stderr.String(), maxOutputBytes),
		Command: label,
	}
	if err == nil {
		res.ExitCode = 0
		return res
	}
	if ee, ok := err.(*exec.ExitError); ok {
		res.ExitCode = ee.ExitCode()
	} else {
		res.ExitCode = -1
		if res.Stderr == "" {
			res.Stderr = err.Error()
		}
	}
	return res
}
