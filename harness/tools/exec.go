package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"harness/harness/events"
)

const maxOutputBytes = 20_000

// npmPkgRe validates an npm package spec (optional scope, optional @version), so
// the narrow npm_install tool can never smuggle shell arguments.
var npmPkgRe = regexp.MustCompile(`^@?[a-z0-9][a-z0-9._-]*(/[a-z0-9][a-z0-9._-]*)?(@[a-zA-Z0-9._~^><=.\-+*x]+)?$`)

// npmInstallTool installs a single npm package into the working directory. It is
// a narrow alternative to a full shell: the package name is validated and the
// command runs via argv (no shell), so it cannot execute arbitrary commands.
func npmInstallTool() *Tool {
	return &Tool{
		Name:        "npm_install",
		Description: "Install ONE npm package into the current project. Give just the package name, optionally with a version (e.g. \"recharts\" or \"zod@3\"). Runs `npm install <name>`. Use only when the app needs a library that is not already available.",
		Params:      json.RawMessage(`{"type":"object","properties":{"package":{"type":"string","description":"npm package name, optionally name@version. Exactly one package."}},"required":["package"]}`),
		Mutating:    true,
		Preview: func(env Env, args Args) (string, *events.Diff, error) {
			return "npm install " + args.Str("package"), nil, nil
		},
		Run: func(env Env, args Args) (any, *events.Diff, error) {
			pkg := strings.TrimSpace(args.Str("package"))
			if pkg == "" {
				return nil, nil, fmt.Errorf("package is required")
			}
			if !npmPkgRe.MatchString(pkg) {
				return nil, nil, fmt.Errorf("invalid package name %q", pkg)
			}
			ctx, cancel := context.WithTimeout(env.Ctx, 180*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "npm", "install", pkg) // argv, no shell
			cmd.Dir = env.WorkDir
			cmd.Env = projectEnv()
			var out, errb bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errb
			err := cmd.Run()
			code := 0
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else if err != nil {
				code = -1
			}
			clip := func(s string) string {
				if len(s) > maxOutputBytes {
					return s[:maxOutputBytes] + "\n… [truncated]"
				}
				return s
			}
			return runResult{ExitCode: code, Stdout: clip(out.String()), Stderr: clip(errb.String()), Command: "npm install " + pkg}, nil, nil
		},
	}
}

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
// projectEnv returns the process environment with framework/database config vars
// stripped, so a command building or running a NEW project in the working
// directory is not hijacked by ANOTHER project's settings inherited from the
// user's shell (e.g. DJANGO_SETTINGS_MODULE or DATABASE_URL pointing elsewhere,
// which silently redirect the fresh project and cause baffling import errors).
func projectEnv(extra ...string) []string {
	drop := map[string]bool{
		"DJANGO_SETTINGS_MODULE": true, "DJANGO_CONFIGURATION": true,
		"FLASK_APP": true, "FLASK_ENV": true, "FLASK_DEBUG": true,
		"RAILS_ENV": true, "RACK_ENV": true, "ASPNETCORE_ENVIRONMENT": true,
		"NODE_ENV": true,
	}
	var out []string
	for _, kv := range os.Environ() {
		k := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			k = kv[:i]
		}
		if drop[k] || strings.Contains(k, "DATABASE_URL") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, extra...)
}

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
			cmd := newShellCmd(ctx, command)
			cmd.Dir = env.WorkDir
			cmd.Env = projectEnv()
			// If the command backgrounds a long-running server (e.g. `node dist/index.js &`),
			// the child inherits the stdout pipe and Wait() would block forever even after
			// the timeout kills the shell. WaitDelay forces Wait() to return shortly after
			// cancellation, closing the pipes; newShellCmd kills the whole process group.
			cmd.WaitDelay = 5 * time.Second
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

			cmd := newShellCmd(context.Background(), command)
			cmd.Dir = env.WorkDir
			cmd.Env = projectEnv()
			// Own process group so the whole server tree can be killed at turn end.
			setProcGroup(cmd)
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

// codeRunTool runs a short snippet. On the web path (env.Sandboxed) it runs in
// an isolated temp dir with a bare environment and no project access. In the
// terminal it runs in the working directory with the inherited environment, so
// the snippet can import and exercise the project's own files.
// Network isolation is not enforced by the OS here yet; this is the seam where a
// stronger sandbox (a container or jail) would slot in without changing callers.
func codeRunTool() *Tool {
	return &Tool{
		Name:        "code_run",
		Description: "Run a short code snippet (python or javascript) and return its output. In the terminal it runs in the project's working directory, so it can import and use the project's files; in web chats it runs in an isolated sandbox with no project access.",
		Params:      json.RawMessage(`{"type":"object","properties":{"language":{"type":"string","enum":["python","javascript"],"description":"Language of the snippet."},"code":{"type":"string","description":"The code to run."},"stdin":{"type":"string","description":"Optional standard input to feed the program."}},"required":["language","code"]}`),
		Mutating:    true,
		WebSafe:     true,
		Preview: func(env Env, args Args) (string, *events.Diff, error) {
			where := "sandbox"
			if !env.Sandboxed && env.WorkDir != "" {
				where = "working directory"
			}
			return fmt.Sprintf("run %s snippet in %s", args.Str("language"), where), nil, nil
		},
		Run: func(env Env, args Args) (any, *events.Diff, error) {
			lang := args.Str("language")
			code := args.Str("code")
			if code == "" {
				return nil, nil, fmt.Errorf("code is required")
			}

			var file, bin string
			switch lang {
			case "python":
				file, bin = "snippet.py", "python3"
			case "javascript":
				file, bin = "snippet.js", "node"
			default:
				return nil, nil, fmt.Errorf("unsupported language %q", lang)
			}

			sandboxed := env.Sandboxed || env.WorkDir == ""
			// Choose where the snippet lives and runs. Sandboxed: an isolated temp
			// dir. Otherwise: a uniquely named file inside the project so its cwd
			// and import path are the project itself.
			var runDir, path string
			if sandboxed {
				dir, err := os.MkdirTemp("", "harness-sandbox-")
				if err != nil {
					return nil, nil, err
				}
				defer os.RemoveAll(dir)
				runDir, path = dir, filepath.Join(dir, file)
			} else {
				runDir = env.WorkDir
				ext := filepath.Ext(file)
				f, err := os.CreateTemp(env.WorkDir, ".harness_snippet_*"+ext)
				if err != nil {
					return nil, nil, err
				}
				path = f.Name()
				f.Close()
				defer os.Remove(path)
			}
			if err := os.WriteFile(path, []byte(code), 0o644); err != nil {
				return nil, nil, err
			}

			ctx, cancel := context.WithTimeout(env.Ctx, 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bin, path)
			cmd.Dir = runDir
			if sandboxed {
				// Bare environment so the snippet cannot read host secrets.
				cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/local/bin", "HOME=" + runDir}
			} else {
				// Inherit the environment (minus other projects' framework config)
				// and make this project importable.
				cmd.Env = projectEnv("PYTHONPATH="+runDir, "NODE_PATH="+runDir)
			}
			if in := args.Str("stdin"); in != "" {
				cmd.Stdin = strings.NewReader(in)
			}
			return runCommand(cmd, bin+" "+filepath.Base(path)), nil, nil
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
