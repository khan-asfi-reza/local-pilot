// Command harness-server is the thin server wrapper around the harness core. It
// is the second door into the same brain the terminal uses: the web backend
// calls it over HTTP and reads the streamed events. It only ever offers the safe
// tool set and runs in auto mode, since the web path relies on its sandboxed
// tools rather than the terminal's permission modes.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sync"

	"harness/harness/agent"
	"harness/harness/appdir"
	"harness/harness/events"
	"harness/harness/model"
)

// safeTools is the only set the web path may use: a sandboxed runner and web
// search, never file, shell, or code-intelligence tools.
var safeTools = []string{"code_run", "web_search"}

// runMu serializes runs: the agent switches its active model per request, which
// must not race across concurrent runs.
var runMu sync.Mutex

type runRequest struct {
	Messages         []model.Message `json:"messages"`
	AllowedTools     []string        `json:"allowed_tools"`
	WorkingDirectory string          `json:"working_directory"`
	Model            string          `json:"model"`
}

func main() {
	port := flag.Int("port", 9000, "port to listen on")
	configPath := flag.String("config", "", "path to the model registry (default: the local-pilot config)")
	flag.Parse()

	cfgPath := *configPath
	if cfgPath == "" {
		p, err := appdir.Ensure()
		if err != nil {
			fmt.Fprintf(os.Stderr, "harness-server: %v\n", err)
			os.Exit(1)
		}
		cfgPath = p
	}
	cfg, err := model.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness-server: %v\n", err)
		os.Exit(1)
	}
	ag, err := agent.New(cfg, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness-server: %v\n", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()

	// GET /models lists the configured models with readiness and the default, so
	// a client can offer a model picker.
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		type modelJSON struct {
			Name   string `json:"name"`
			Ready  bool   `json:"ready"`
			URL    string `json:"url"`
			Active bool   `json:"active"`
		}
		var list []modelJSON
		for _, m := range ag.Models() {
			list = append(list, modelJSON{Name: m.Name, Ready: m.Running, URL: m.URL, Active: m.Active})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"models": list, "default": ag.DefaultModel()})
	})

	mux.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req runRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")

		allowed := intersect(req.AllowedTools, safeTools)
		enc := json.NewEncoder(w)
		emit := func(ev events.Event) {
			_ = enc.Encode(ev)
			flusher.Flush()
		}
		agentReq := agent.Request{
			Messages: req.Messages,
			Allowed:  allowed,
			Mode:     "auto",
			WorkDir:  req.WorkingDirectory,
		}
		// Switch to the request's model (falling back to the default) and run.
		// Serialized so per-request model switching does not race.
		runMu.Lock()
		defer runMu.Unlock()
		ag.UseSessionModel(req.Model)
		// The web path never pauses for confirmation, so confirm is nil. The
		// request context cancels the turn when the client disconnects (pause).
		ag.Run(r.Context(), agentReq, emit, nil)
	})

	addr := fmt.Sprintf(":%d", *port)
	fmt.Printf("harness-server listening on %s (safe tools only)\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "harness-server: %v\n", err)
		os.Exit(1)
	}
}

// intersect returns the requested tools limited to the safe set. An empty
// request defaults to the full safe set.
func intersect(requested, safe []string) []string {
	if len(requested) == 0 {
		return safe
	}
	allow := map[string]bool{}
	for _, s := range safe {
		allow[s] = true
	}
	var out []string
	for _, r := range requested {
		if allow[r] {
			out = append(out, r)
		}
	}
	return out
}
