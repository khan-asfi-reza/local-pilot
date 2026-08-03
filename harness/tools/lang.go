package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"harness/harness/events"
	"harness/harness/lang"
)

// installDepsTool installs project libraries with the right package manager for
// the project's language, detected from its manifest files. It is a deterministic
// alternative to the model guessing pip/npm/go get via shell_run.
func installDepsTool() *Tool {
	return &Tool{
		Name: "install_deps",
		Description: "Install project libraries using the correct package manager for the project's language, " +
			"detected automatically (pip into .venv for Python, npm for Node, go get for Go, cargo add for Rust, " +
			"composer for PHP, bundle for Ruby). Give the package names. Prefer this over shell_run for adding " +
			"dependencies. Example: {\"packages\":[\"djangorestframework\",\"celery\"]}.",
		Params:   json.RawMessage(`{"type":"object","properties":{"packages":{"type":"array","items":{"type":"string"},"description":"Library names to install (optionally name@version)."},"language":{"type":"string","description":"Optional: force a language (python, javascript, go, rust, java, ruby, php, c, cpp). Default: auto-detect from the project's manifest files."}},"required":["packages"]}`),
		Mutating: true,
		Preview: func(env Env, args Args) (string, *events.Diff, error) {
			return "install deps: " + strings.Join(strList(args["packages"]), ", "), nil, nil
		},
		Run: func(env Env, args Args) (any, *events.Diff, error) {
			pkgs := strList(args["packages"])
			if len(pkgs) == 0 {
				return nil, nil, fmt.Errorf("packages is required")
			}
			var h lang.Handler
			if l := args.Str("language"); l != "" {
				if h = lang.HandlerFor(l); h == nil {
					return nil, nil, fmt.Errorf("unknown language %q", l)
				}
			} else {
				h = lang.DetectLanguage(env.WorkDir)
			}
			if h == nil {
				return map[string]any{"error": "Could not detect the project language from its files. Use shell_run with the right package manager instead, or pass \"language\"."}, nil, nil
			}
			ctx, cancel := context.WithTimeout(env.Ctx, 300*time.Second)
			defer cancel()
			if err := h.Install(ctx, env.WorkDir, pkgs); err != nil {
				return map[string]any{"installed": false, "language": h.Lang(), "error": err.Error()}, nil, nil
			}
			return map[string]any{"installed": true, "language": h.Lang(), "packages": pkgs}, nil, nil
		},
	}
}

// strList decodes a JSON array argument into a slice of non-empty strings.
func strList(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, e := range arr {
		if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}
