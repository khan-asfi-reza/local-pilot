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
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"harness/harness/agent"
	"harness/harness/appdir"
	"harness/harness/events"
	"harness/harness/model"
	"harness/harness/tools"
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
	FullAccess       bool            `json:"full_access"`
	Mode             string          `json:"mode"`          // "ask" pauses on mutating ops; default "auto"
	InjectSkills     []string        `json:"inject_skills"` // skills to inject silently regardless of detection (e.g. App Builder forces "app-builder")
}

// confirmReply is a client's answer to an ask-mode confirmation.
type confirmReply struct {
	decision tools.Decision
	feedback string
}

// Pending ask-mode confirmations. A run in ask mode emits a "confirm" event with
// an id and blocks on the channel registered here; the /confirm endpoint (a
// separate connection) delivers the client's decision by id. HTTP streaming is
// one-way, so the round trip uses a second request rather than the run's stream.
var (
	confirmMu   sync.Mutex
	confirmWait = map[string]chan confirmReply{}
	confirmSeq  uint64
)

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
	// Scan the seeded skills dir so the web path (App Builder, Code IDE) can use
	// load_skill. The terminal seeds this on startup via appdir.Ensure().
	ag, err := agent.New(cfg, filepath.Join(appdir.Dir(), "skills"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness-server: %v\n", err)
		os.Exit(1)
	}

	// Reload the registry when models.json changes on disk (e.g. `pilot models
	// add`), so new models appear without restarting the server.
	mjson := filepath.Join(cfg.Dir(), "models.json")
	var cfgMu sync.Mutex
	var lastMod time.Time
	if info, e := os.Stat(mjson); e == nil {
		lastMod = info.ModTime()
	}
	reloadIfChanged := func() {
		cfgMu.Lock()
		defer cfgMu.Unlock()
		info, e := os.Stat(mjson)
		if e != nil || !info.ModTime().After(lastMod) {
			return
		}
		runMu.Lock()
		e = ag.Reload(cfgPath)
		runMu.Unlock()
		if e == nil {
			lastMod = info.ModTime()
		}
	}

	mux := http.NewServeMux()

	// GET /models lists the configured models with readiness and the default, so
	// a client can offer a model picker.
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		reloadIfChanged()
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
		reloadIfChanged() // pick up newly added models before selecting one
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

		// full_access widens the harness to the full tool set (file, shell, and
		// code-intelligence tools) with no sandbox, working directly in the given
		// directory — a privilege escalation. The trust decision is made by the
		// server from the connection, never from the request body alone: honor it
		// only for callers on this machine (loopback) that are not a browser
		// cross-site request. This shuts out LAN clients (the server also listens
		// on the LAN for the safe chat path) and CSRF / DNS-rebinding that would
		// otherwise reach a loopback listener. An empty Allowed set means "every
		// tool" (see Registry.Defs); the default path is unchanged (safe tools +
		// sandbox).
		if req.FullAccess && (!isLoopback(r) || browserCrossSite(r)) {
			http.Error(w, "full_access is restricted to local IDE clients", http.StatusForbidden)
			return
		}
		var allowed []string
		sandbox := true
		if req.FullAccess {
			// Full access: no sandbox, work in the real directory. Honor an explicit
			// tool allowlist when given — the App Builder restricts itself to file
			// tools so it never starts servers or runs shells — while an empty list
			// means the full tool set (the Code IDE).
			allowed = req.AllowedTools
			sandbox = false
		} else {
			allowed = intersect(req.AllowedTools, safeTools)
		}
		enc := json.NewEncoder(w)
		emit := func(ev events.Event) {
			_ = enc.Encode(ev)
			flusher.Flush()
		}
		// Ask mode pauses on mutating actions for approval. Only the full_access
		// (Code IDE) path may request it; the safe chat path stays auto. When off,
		// confirm is nil and the run never pauses.
		mode := "auto"
		var confirm tools.ConfirmFunc
		if req.FullAccess && req.Mode == tools.ModeAsk {
			mode = tools.ModeAsk
			confirm = func(tool, summary string, diff *events.Diff) (tools.Decision, string) {
				id := fmt.Sprintf("c%d", atomic.AddUint64(&confirmSeq, 1))
				ch := make(chan confirmReply, 1)
				confirmMu.Lock()
				confirmWait[id] = ch
				confirmMu.Unlock()
				defer func() {
					confirmMu.Lock()
					delete(confirmWait, id)
					confirmMu.Unlock()
				}()
				// Ask the client and block until it answers on /confirm or the
				// connection drops (treated as decline so the action is skipped).
				emit(events.Confirm(id, tool, summary, diff))
				select {
				case <-r.Context().Done():
					return tools.Decline, ""
				case reply := <-ch:
					return reply.decision, reply.feedback
				}
			}
		}
		agentReq := agent.Request{
			Messages: req.Messages,
			Allowed:  allowed,
			Mode:     mode,
			WorkDir:  req.WorkingDirectory,
			Sandbox:  sandbox,
			// The non-full-access path is the conversational chat path (web chat,
			// Telegram no-project): it has only the sandboxed code_run + web_search
			// tools, so it answers inline and must not act like a project agent.
			Chat:         !req.FullAccess,
			InjectSkills: req.InjectSkills,
		}
		// Switch to the request's model (falling back to the default) and run.
		// Serialized so per-request model switching does not race. The request
		// context cancels the turn when the client disconnects (pause).
		runMu.Lock()
		defer runMu.Unlock()
		ag.UseSessionModel(req.Model)
		ag.Run(r.Context(), agentReq, emit, confirm)
	})

	// POST /confirm delivers an ask-mode decision to a blocked run, matched by the
	// id from its "confirm" event. Restricted to local IDE clients, like the
	// escalation path it answers for.
	mux.HandleFunc("/confirm", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if !isLoopback(r) || browserCrossSite(r) {
			http.Error(w, "restricted to local IDE clients", http.StatusForbidden)
			return
		}
		var body struct {
			ID       string `json:"id"`
			Decision string `json:"decision"`
			Feedback string `json:"feedback"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		confirmMu.Lock()
		ch := confirmWait[body.ID]
		confirmMu.Unlock()
		if ch == nil {
			http.Error(w, "no pending confirmation for that id", http.StatusNotFound)
			return
		}
		decision := tools.Decline
		switch body.Decision {
		case "approve":
			decision = tools.Approve
		case "approve_always":
			decision = tools.ApproveAlways
		}
		ch <- confirmReply{decision: decision, feedback: body.Feedback}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})

	addr := fmt.Sprintf(":%d", *port)
	fmt.Printf("harness-server listening on %s (safe tools only)\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "harness-server: %v\n", err)
		os.Exit(1)
	}
}

// isLoopback reports whether the request originates from this machine. Only
// loopback callers may request full_access, so a host elsewhere on the LAN
// cannot escalate past the safe tool set.
func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// browserCrossSite reports whether the request carries a browser cross-site
// marker. The trusted local IDE backend calls with a plain HTTP client and sets
// neither header; a browser making a cross-origin fetch sets Origin (and modern
// browsers Sec-Fetch-Site). Rejecting these on the escalation path defeats CSRF
// and DNS-rebinding that reach a loopback listener from the victim's browser.
func browserCrossSite(r *http.Request) bool {
	if s := r.Header.Get("Sec-Fetch-Site"); s != "" && s != "same-origin" && s != "none" {
		return true
	}
	if r.Header.Get("Origin") != "" {
		return true
	}
	return false
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
