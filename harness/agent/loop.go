package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"harness/harness/events"
	"harness/harness/model"
	"harness/harness/tools"
)

// action is one structured-JSON step from the planner: a reasoning sentence, the
// tool to call (or "final" to end), and its arguments.
type action struct {
	Reasoning string         `json:"reasoning"`
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
}

// Run executes the ReAct loop for one request, streaming events through emit and
// returning the conversation for the caller to persist. The active model's
// tool_mode picks native tool calls (default) or the grammar-JSON fallback.
func (a *Agent) Run(ctx context.Context, req Request, emit func(events.Event), confirm tools.ConfirmFunc) []model.Message {
	agentsMD := discoverAgentsMD(req.WorkDir)
	repoMap := buildRepoMap(req.WorkDir)
	includeMutating := req.Mode != tools.ModePlan

	env := tools.Env{
		Ctx:        ctx,
		WorkDir:    req.WorkDir,
		Skills:     a.skills.bodies,
		SkillNames: a.skills.names,
		Seen:       map[string]bool{},
		Procs:      tools.NewProcSet(),
	}
	defer env.Procs.StopAll()
	conv := append([]model.Message(nil), req.Messages...)

	if a.router.ToolMode() == model.ToolModeNative {
		defs := a.reg.Defs(req.Allowed, includeMutating)
		system := buildSystem(a.prompt, model.ToolModeNative, agentsMD, a.skills.catalog, repoMap, "", req.Mode)
		return a.runNative(ctx, req, emit, confirm, system, defs, env, conv)
	}

	toolDocs, names := a.reg.Describe(req.Allowed, includeMutating)
	system := buildSystem(a.prompt, model.ToolModeJSON, agentsMD, a.skills.catalog, repoMap, toolDocs, req.Mode)
	schema := actionSchema(names)
	return a.runJSON(ctx, req, emit, confirm, system, schema, env, conv)
}

// runNative drives the loop with native tool calls: each turn's tool_calls are
// dispatched and fed back as role:"tool" messages; a turn with none is the final.
func (a *Agent) runNative(ctx context.Context, req Request, emit func(events.Event), confirm tools.ConfirmFunc, system string, defs []model.ToolDef, env tools.Env, conv []model.Message) []model.Message {
	seen := map[string]int{}
	totalTokens := 0
	debugInjected := false
	serveInjected := false
	depsInjected := false
	finishNudged := false

	for step := 0; step < a.maxSteps; step++ {
		msg, tokens, err := a.chatStep(ctx, system, conv, defs)
		if err != nil {
			emit(events.Error(err.Error()))
			return conv
		}
		totalTokens += tokens
		emit(events.Usage(totalTokens))

		// No tool calls: the model is giving its final answer.
		if len(msg.ToolCalls) == 0 {
			text := strings.TrimSpace(msg.Content)
			// Nudge once before accepting the finish: force a completeness check so
			// the model doesn't stop with files uncreated or checks unvalidated.
			if !finishNudged {
				finishNudged = true
				conv = append(conv, model.Message{Role: "assistant", Content: text})
				conv = append(conv, model.Message{Role: "user", Content: "Before finishing, verify completeness: list_dir to confirm EVERY file the task asked for actually exists, and confirm each check (test/build/curl) truly returned the expected result, not just exited. If any requested file is missing or any result is wrong, keep working with tools. Only if everything is genuinely done and verified, reply with your final summary."})
				continue
			}
			emit(events.Text(text))
			conv = append(conv, model.Message{Role: "assistant", Content: text})
			emit(events.Done())
			return conv
		}

		conv = append(conv, msg)
		if reasoning := strings.TrimSpace(msg.Content); reasoning != "" {
			emit(events.Text(reasoning + "\n"))
		}

		anyFailure := false
		for _, tc := range msg.ToolCalls {
			emit(events.ToolCall(tc.Function.Name, summarizeCall(tc)))
			result, diff := a.reg.Dispatch(tc, req.Allowed, req.Mode, env, confirm)
			emit(events.ToolResult(tc.Function.Name, shortResult(result), result, diff))

			key := tc.Function.Name + "|" + tc.Function.Arguments + "|" + shortResult(result)
			if seen[key]++; seen[key] >= 3 {
				emit(events.Error("stopping: the assistant keeps repeating actions with no new result. Rephrase the task or break it into smaller steps."))
				return conv
			}

			conv = append(conv, model.Message{Role: "tool", ToolCallID: tc.ID, Name: tc.Function.Name, Content: result})
			if isFailure(result) {
				anyFailure = true
			}

			if !serveInjected && tc.Function.Name == "shell_run" {
				var argm map[string]any
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &argm)
				if cmd, _ := argm["command"].(string); tools.ServerStartHint(cmd) != "" {
					if body := a.skills.bodies["serving"]; body != "" {
						conv = append(conv, model.Message{Role: "user", Content: "To run and verify a server, follow this procedure:\n" + body})
						serveInjected = true
					}
				}
			}

			// Inject the dependencies skill once when a dep install fails on the env.
			if !depsInjected && tools.DepsInstallFailure(result) {
				if body := a.skills.bodies["dependencies"]; body != "" {
					conv = append(conv, model.Message{Role: "user", Content: "Installing dependencies failed for an environment reason. Follow this procedure:\n" + body})
					depsInjected = true
				}
			}
		}

		// The first time something fails, inject the debug procedure, since a small
		// model will not reliably load it on its own.
		if !debugInjected && anyFailure {
			if body := a.skills.bodies["debug"]; body != "" {
				conv = append(conv, model.Message{Role: "user", Content: "The last step failed. Follow this debugging procedure:\n" + body})
				debugInjected = true
			}
		}
	}

	emit(events.Error(fmt.Sprintf("reached the step limit of %d without finishing", a.maxSteps)))
	return conv
}

// runJSON drives the grammar-constrained JSON-ReAct fallback (tool_mode "json").
func (a *Agent) runJSON(ctx context.Context, req Request, emit func(events.Event), confirm tools.ConfirmFunc, system string, schema json.RawMessage, env tools.Env, conv []model.Message) []model.Message {
	seen := map[string]int{}
	totalTokens := 0
	debugInjected := false

	for step := 0; step < a.maxSteps; step++ {
		raw, tokens, err := a.planStep(ctx, system, conv, schema)
		if err != nil {
			emit(events.Error(err.Error()))
			return conv
		}
		totalTokens += tokens
		emit(events.Usage(totalTokens))

		act, perr := parseAction(raw)
		if perr != nil {
			// The output was valid JSON but not a usable action; ask for a retry.
			conv = append(conv, model.Message{Role: "assistant", Content: raw})
			conv = append(conv, model.Message{Role: "user", Content: "That was not a valid action. Reply with one JSON object having reasoning, tool, and arguments."})
			continue
		}

		if act.Tool == "final" {
			text := stringArg(act.Arguments, "text")
			if text == "" {
				text = act.Reasoning
			}
			emit(events.Text(text))
			conv = append(conv, model.Message{Role: "assistant", Content: text})
			emit(events.Done())
			return conv
		}

		if act.Reasoning != "" {
			emit(events.Text(act.Reasoning + "\n"))
		}

		argsJSON, _ := json.Marshal(act.Arguments)
		tc := model.ToolCall{
			ID:       fmt.Sprintf("call_%d", step),
			Type:     "function",
			Function: model.FunctionCall{Name: act.Tool, Arguments: string(argsJSON)},
		}
		emit(events.ToolCall(tc.Function.Name, summarizeCall(tc)))
		result, diff := a.reg.Dispatch(tc, req.Allowed, req.Mode, env, confirm)
		emit(events.ToolResult(tc.Function.Name, shortResult(result), result, diff))

		// Stop if the same action keeps producing the same result, even when the
		// model alternates between a couple of them.
		key := tc.Function.Name + "|" + tc.Function.Arguments + "|" + shortResult(result)
		if seen[key]++; seen[key] >= 3 {
			emit(events.Error("stopping: the assistant keeps repeating actions with no new result. Rephrase the task or break it into smaller steps."))
			return conv
		}

		// Record the step and feed the observation back for the next turn.
		conv = append(conv, model.Message{Role: "assistant", Content: raw})
		conv = append(conv, model.Message{Role: "user", Content: fmt.Sprintf("Result of %s:\n%s", tc.Function.Name, result)})

		// The first time something fails, inject the debug procedure, since a
		// small model will not reliably load it on its own.
		if !debugInjected && isFailure(result) {
			if body := a.skills.bodies["debug"]; body != "" {
				conv = append(conv, model.Message{Role: "user", Content: "The last step failed. Follow this debugging procedure:\n" + body})
				debugInjected = true
			}
		}
	}

	emit(events.Error(fmt.Sprintf("reached the step limit of %d without finishing", a.maxSteps)))
	return conv
}

// planStep asks the planner for one action, compacting the conversation to fit
// the context budget first. If the backend still reports a context overflow, the
// budget is shrunk and the call retried, so a token limit never surfaces as an
// error to the user.
func (a *Agent) planStep(ctx context.Context, system string, conv []model.Message, schema []byte) (string, int, error) {
	budget := a.contextTokens
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		msgs := compact(system, conv, budget)
		raw, tokens, err := a.router.Constrained(ctx, msgs, schema)
		if err == nil {
			return raw, tokens, nil
		}
		lastErr = err
		if !isContextError(err) || budget <= 1500 {
			return "", 0, err
		}
		budget = budget * 2 / 3
	}
	return "", 0, lastErr
}

// chatStep asks the model for one native tool-calling turn, compacting to fit the
// context budget and shrinking it on overflow. Mirrors planStep.
func (a *Agent) chatStep(ctx context.Context, system string, conv []model.Message, defs []model.ToolDef) (model.Message, int, error) {
	budget := a.contextTokens
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		msgs := compact(system, conv, budget)
		msg, tokens, err := a.router.Chat(ctx, msgs, defs)
		if err == nil {
			return msg, tokens, nil
		}
		lastErr = err
		if !isContextError(err) || budget <= 1500 {
			return model.Message{}, 0, err
		}
		budget = budget * 2 / 3
	}
	return model.Message{}, 0, lastErr
}

// isContextError reports whether a backend error looks like the prompt exceeded
// the model's context window, which the harness handles by compacting rather
// than surfacing.
func isContextError(err error) bool {
	s := strings.ToLower(err.Error())
	for _, needle := range []string{"context", "n_ctx", "exceed", "too long", "too large", "kv cache", "tokens"} {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

// actionSchema builds the JSON schema the planner output is constrained to. The
// tool enum is the allowed tool names plus "final", so the model can only name a
// real tool or end the turn.
func actionSchema(names []string) json.RawMessage {
	toolEnum := append(append([]string{}, names...), "final")
	enum, _ := json.Marshal(toolEnum)
	s := fmt.Sprintf(`{"type":"object","properties":{"reasoning":{"type":"string"},"tool":{"type":"string","enum":%s},"arguments":{"type":"object"}},"required":["reasoning","tool","arguments"]}`, enum)
	return json.RawMessage(s)
}

// parseAction decodes the planner's JSON reply into an action.
func parseAction(raw string) (action, error) {
	var act action
	if err := json.Unmarshal([]byte(raw), &act); err != nil {
		return action{}, err
	}
	if act.Tool == "" {
		return action{}, fmt.Errorf("no tool named")
	}
	if act.Arguments == nil {
		act.Arguments = map[string]any{}
	}
	return act, nil
}

// isFailure reports whether a tool result signals an error or a nonzero exit,
// which is the cue to inject the debug procedure.
func isFailure(result string) bool {
	var m map[string]any
	if json.Unmarshal([]byte(result), &m) != nil {
		return false
	}
	if _, ok := m["error"]; ok {
		return true
	}
	if ec, ok := m["exit_code"].(float64); ok && int(ec) != 0 {
		return true
	}
	return false
}

func stringArg(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

// summarizeCall turns a tool call into a short human note for the tool_call
// event, picking the most telling argument per tool.
func summarizeCall(tc model.ToolCall) string {
	var args map[string]any
	_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
	get := func(k string) string {
		if v, ok := args[k].(string); ok {
			return v
		}
		return ""
	}
	switch tc.Function.Name {
	case "search":
		return get("query")
	case "list_dir":
		return orDot(get("path"))
	case "read_file":
		return get("path")
	case "write_file":
		return get("path")
	case "edit_file":
		return get("path")
	case "shell_run":
		return get("command")
	case "code_run":
		return get("language") + " snippet"
	case "web_search":
		return get("query")
	case "load_skill":
		return get("name")
	default:
		return truncate(tc.Function.Arguments, 80)
	}
}

// shortResult produces a one-line summary of a tool result for the tool_result
// event, so the terminal can show progress without dumping the full payload.
func shortResult(result string) string {
	var m map[string]any
	if json.Unmarshal([]byte(result), &m) != nil {
		return truncate(result, 120)
	}
	if e, ok := m["error"].(string); ok {
		return "error: " + e
	}
	if code, ok := m["exit_code"].(float64); ok {
		return fmt.Sprintf("exit %d", int(code))
	}
	if matches, ok := m["matches"].([]any); ok {
		return fmt.Sprintf("%d matches", len(matches))
	}
	if applied, ok := m["applied"].(float64); ok {
		return fmt.Sprintf("%d edit(s) applied", int(applied))
	}
	if b, ok := m["bytes_written"].(float64); ok {
		return fmt.Sprintf("%d bytes written", int(b))
	}
	if _, ok := m["code"]; ok {
		return "code drafted"
	}
	if _, ok := m["body"]; ok {
		return "skill loaded"
	}
	return "ok"
}

func orDot(s string) string {
	if s == "" {
		return "."
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
