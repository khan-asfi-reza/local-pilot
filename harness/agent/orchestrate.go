package agent

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"strconv"
	"strings"

	"harness/harness/events"
	"harness/harness/model"
	"harness/harness/orchestrator"
	"harness/harness/tools"
)

type agentPlanner struct{ a *Agent }

// PlanJSON runs planning as a NATIVE TOOL CALL, not grammar-constrained json.
// ollama's json_schema decoding carries a ~100s fixed overhead per call on this
// hardware; native tool-calling uses the model's trained tool format (the same
// fast path as the main loop) and returns clean structured args. Falls back to
// lenient text extraction if the model replies without calling the tool.
func (p agentPlanner) PlanJSON(ctx context.Context, system, user string, schema json.RawMessage) (string, error) {
	defs := []model.ToolDef{{
		Type: "function",
		Function: model.ToolFunc{
			Name:        "submit",
			Description: "Submit the structured result.",
			Parameters:  schema,
		},
	}}
	sys := system + "\n\nCall the submit function with the result."
	msgs := compact(sys, []model.Message{{Role: "user", Content: user}}, p.a.contextTokens)
	// Planning runs on the dedicated planner model (a stronger model decomposes);
	// the children build on the fast default model.
	msg, _, err := p.a.router.ChatWith(ctx, p.a.router.PlannerName(), msgs, defs, nil)
	if err != nil {
		return "", err
	}
	for _, tc := range msg.ToolCalls {
		if strings.TrimSpace(tc.Function.Arguments) != "" {
			return normalizeJSON(tc.Function.Arguments), nil
		}
	}
	return normalizeJSON(extractJSON(msg.Content)), nil
}

// normalizeJSON un-stringifies nested JSON values, fixing small models that
// double-encode array/object tool-call args (e.g. {"tasks":"[...]"} -> {"tasks":[...]}).
func normalizeJSON(raw string) string {
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return raw
	}
	out, err := json.Marshal(unstring(v))
	if err != nil {
		return raw
	}
	return string(out)
}

func unstring(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			t[k] = unstring(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = unstring(val)
		}
		return t
	case string:
		s := strings.TrimSpace(t)
		if len(s) > 1 && (s[0] == '[' || s[0] == '{') {
			var inner any
			if json.Unmarshal([]byte(s), &inner) == nil {
				return unstring(inner)
			}
		}
		return t
	default:
		return v
	}
}

// extractJSON pulls the first JSON object out of a model reply, tolerating code
// fences and surrounding prose.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		if nl := strings.IndexByte(s, '\n'); nl >= 0 && !strings.Contains(s[:nl], "{") {
			s = s[nl+1:]
		}
		if j := strings.LastIndex(s, "```"); j >= 0 {
			s = s[:j]
		}
	}
	a := strings.IndexByte(s, '{')
	b := strings.LastIndexByte(s, '}')
	if a >= 0 && b > a {
		return s[a : b+1]
	}
	return strings.TrimSpace(s)
}

type agentExecutor struct{ a *Agent }

func (e agentExecutor) RunChild(ctx context.Context, prompt string, s orchestrator.ChildSpec, emit func(events.Event), confirm tools.ConfirmFunc) []model.Message {
	req := Request{
		Messages:     []model.Message{{Role: "user", Content: prompt}},
		Allowed:      s.Allowed,
		Mode:         s.Mode,
		WorkDir:      s.WorkDir,
		Sandbox:      s.Sandbox,
		InjectSkills: s.InjectSkills,
		noTriage:     true,
	}
	// Gate the child on its own target files so it cannot stop until every one is
	// written (reuses the grounding finish-gate). This is what forces a small model
	// to actually build all its files instead of replying with text after one.
	if len(s.Targets) > 0 {
		req.Grounding = &Grounding{Action: "create", ExplicitTargets: s.Targets}
	}
	return e.a.Run(ctx, req, emit, confirm)
}

func (a *Agent) intake(ctx context.Context, prompt string) *orchestrator.Contract {
	c, err := orchestrator.Intake(ctx, agentPlanner{a}, prompt)
	if err != nil {
		return nil
	}
	return c
}

// runOrchestrated fans a large request out into isolated dependency-ordered child
// runs, then does one memory update from the union of what they produced.
func (a *Agent) runOrchestrated(ctx context.Context, req Request, emit func(events.Event), confirm tools.ConfirmFunc, contract *orchestrator.Contract) []model.Message {
	pol := orchestrator.Policy{MaxParallel: boundParallelism(), MaxRetries: 2}
	o := orchestrator.New(agentPlanner{a}, agentExecutor{a}, orchestrator.OSFiles{}, pol)
	childMode := req.Mode
	if childMode == "" {
		childMode = tools.ModeAuto
	}
	// Build children get a write-focused tool set. read/write/edit are the point;
	// list_dir/search are read-only orientation. We do NOT hard-block list_dir/search:
	// a native model calls list_dir out of habit, and blocking it returns a derailing
	// error that burns steps and ends with "produced no files". The live file tree is
	// now pushed into every step (see runNative), so the child rarely needs them — but
	// when it does, the call succeeds harmlessly. shell/serve stay out: per-sub-task
	// server runs are wrong; verification is structural and top-level.
	allowed := []string{"read_file", "write_file", "edit_file", "list_dir", "search"}
	if len(req.Allowed) > 0 && len(req.Allowed) < len(allowed) {
		allowed = req.Allowed
	}
	spec := orchestrator.ChildSpec{
		WorkDir:      req.WorkDir,
		Allowed:      allowed,
		Sandbox:      req.Sandbox,
		InjectSkills: req.InjectSkills,
		Mode:         childMode,
	}
	sum := o.Execute(ctx, lastUserText(req.Messages), contract, spec, emit, confirm)

	changed := map[string]bool{}
	for _, r := range sum.Results {
		for _, f := range r.Written {
			changed[f] = true
		}
	}
	a.scheduleMemoryUpdate(req, changed)
	return append(req.Messages, model.Message{Role: "assistant", Content: sum.Text()})
}

// boundParallelism caps concurrent children to min(NumCPU-2, OLLAMA_NUM_PARALLEL, 4), floor 1.
func boundParallelism() int {
	n := runtime.NumCPU() - 2
	if v := os.Getenv("OLLAMA_NUM_PARALLEL"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 && p < n {
			n = p
		}
	}
	if n > 4 {
		n = 4
	}
	if n < 1 {
		n = 1
	}
	return n
}

// isBigDocument reports whether the prompt is a long or multi-section spec.
func isBigDocument(prompt string) bool {
	if len(prompt) >= 1200 {
		return true
	}
	headings := 0
	for _, ln := range strings.Split(prompt, "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "#") {
			headings++
		} else if len(t) >= 2 && t[0] >= '1' && t[0] <= '9' && (t[1] == '.' || t[1] == ')') {
			headings++
		}
	}
	return headings >= 4
}

func lastUserText(msgs []model.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Content
		}
	}
	if len(msgs) > 0 {
		return msgs[len(msgs)-1].Content
	}
	return ""
}
