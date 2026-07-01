// Command harness-server is the thin server wrapper around the harness core. It
// is the second door into the same brain the terminal uses: the web backend
// calls it over HTTP and reads the streamed events. It only ever offers the safe
// tool set and runs in auto mode, since the web path relies on its sandboxed
// tools rather than the terminal's permission modes.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"

	"harness/harness/agent"
	"harness/harness/events"
	"harness/harness/model"
)

// safeTools is the only set the web path may use: a sandboxed runner and web
// search, never file, shell, or code-intelligence tools.
var safeTools = []string{"code_run", "web_search"}

type runRequest struct {
	Messages         []model.Message `json:"messages"`
	AllowedTools     []string        `json:"allowed_tools"`
	WorkingDirectory string          `json:"working_directory"`
}

func main() {
	port := flag.Int("port", 9000, "port to listen on")
	configPath := flag.String("config", "models/models.json", "path to the model registry")
	flag.Parse()

	cfg, err := model.LoadConfig(*configPath)
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
		// The web path never pauses for confirmation, so confirm is nil.
		ag.Run(context.Background(), agentReq, emit, nil)
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
