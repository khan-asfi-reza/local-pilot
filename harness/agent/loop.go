package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"harness/harness/events"
	"harness/harness/model"
	"harness/harness/tools"
)

var digitsRe = regexp.MustCompile(`\d+`)

// canServe reports whether this run may start a long-lived server: only when the
// `serve` tool is available (empty allowed = top-level run with all tools). An
// orchestrated child's allow-list omits serve, so it must fix code, not run servers.
func canServe(allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, t := range allowed {
		if t == "serve" {
			return true
		}
	}
	return false
}

// normalizeCall builds a repeat key that ignores volatile numbers, so retries that
// differ only by a port or a timeout collapse to the same key. For shell_run it
// normalizes the command string; for other tools it normalizes the raw arguments.
func normalizeCall(name, args string) string {
	cmd := args
	var m map[string]any
	if json.Unmarshal([]byte(args), &m) == nil {
		if c, ok := m["command"].(string); ok && c != "" {
			cmd = c
		}
	}
	cmd = digitsRe.ReplaceAllString(cmd, "N")
	cmd = strings.Join(strings.Fields(cmd), " ")
	return name + "|" + cmd
}

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
	a.log.turn(a.router.Active(), string(req.Mode), req.WorkDir, req.Messages)
	if a.log != nil {
		userEmit := emit
		emit = func(ev events.Event) {
			a.log.event(ev)
			userEmit(ev)
		}
	}
	// Decide orchestration analytically. A big document wins outright and skips the
	// slow intake call (size already decides). Otherwise intake once for the
	// grounding contract + the file-count fallback. Skipped for chat/plan/children.
	if !req.noTriage && !req.Chat && req.Mode != tools.ModePlan {
		promptText := lastUserText(req.Messages)
		if isBigDocument(promptText) {
			return a.runOrchestrated(ctx, req, emit, confirm, nil)
		}
		c := a.intake(ctx, promptText)
		if c != nil && req.Grounding == nil {
			req.Grounding = &Grounding{Action: c.Action, ExplicitTargets: c.ExplicitTargets}
		}
		if c != nil && c.FileCount >= 4 {
			return a.runOrchestrated(ctx, req, emit, confirm, c)
		}
	}

	agentsMD := discoverAgentsMD(req.WorkDir)
	// Write an AGENTS.md (and seed memory) before the first task if the project has none.
	if a.shouldBootstrap(req, agentsMD) {
		if md := a.bootstrapProject(ctx, req); md != "" {
			agentsMD = md
		}
	}
	projMemory := discoverMemory(req.WorkDir)
	repoMap := a.repoDigest(req)
	includeMutating := req.Mode != tools.ModePlan

	env := tools.Env{
		Ctx:        ctx,
		WorkDir:    req.WorkDir,
		Skills:     a.skills.bodies,
		SkillNames: a.skills.names,
		Seen:       map[string]bool{},
		Procs:      tools.NewProcSet(),
		Sandboxed:  req.Sandbox,
	}
	defer env.Procs.StopAll()
	conv := append([]model.Message(nil), req.Messages...)

	// Silently pick up guidance for this task: any skills the caller forces
	// (e.g. the App Builder forces "app-builder"), then the detected
	// language/framework skills. All injected into the system prompt, never shown
	// as a loaded skill.
	// Chat (no-project) mode skips skill detection and all the coding scaffolding.
	var guidance string
	if !req.Chat {
		var guides []string
		for _, name := range req.InjectSkills {
			if body := a.skills.bodies[name]; body != "" {
				guides = append(guides, body)
			}
		}
		if d := detectInternalSkills(req.WorkDir, req.Messages, a.skills); d != "" {
			guides = append(guides, d)
		}
		guidance = strings.Join(guides, "\n\n")
	}

	changed := map[string]bool{} // files mutated this run, for the memory update

	if a.router.ToolMode() == model.ToolModeNative {
		defs := a.reg.Defs(req.Allowed, includeMutating)
		system := buildSystem(a.prompt, model.ToolModeNative, agentsMD, projMemory, a.skills.catalog, repoMap, "", req.Mode, guidance)
		if req.Chat {
			system = buildChatSystem(a.prompt, model.ToolModeNative, "")
		}
		conv = a.runNative(ctx, req, emit, confirm, system, defs, env, conv, changed)
		a.scheduleMemoryUpdate(req, changed)
		return conv
	}

	toolDocs, names := a.reg.Describe(req.Allowed, includeMutating)
	system := buildSystem(a.prompt, model.ToolModeJSON, agentsMD, projMemory, a.skills.catalog, repoMap, toolDocs, req.Mode, guidance)
	if req.Chat {
		system = buildChatSystem(a.prompt, model.ToolModeJSON, toolDocs)
	}
	schema := actionSchema(names)
	conv = a.runJSON(ctx, req, emit, confirm, system, schema, env, conv, changed)
	a.scheduleMemoryUpdate(req, changed)
	return conv
}

// repeatArgCap is the real progress cap: the same tool called with the SAME
// arguments this many times, with no file mutation in between, is a stuck loop
// (regardless of whether the result text varies). Raw step count is only a distant
// backstop (a.maxSteps); this is what normally stops a run.
const repeatArgCap = 6

// runNative drives the loop with native tool calls: each turn's tool_calls are
// dispatched and fed back as role:"tool" messages; a turn with none is the final.
func (a *Agent) runNative(ctx context.Context, req Request, emit func(events.Event), confirm tools.ConfirmFunc, system string, defs []model.ToolDef, env tools.Env, conv []model.Message, changed map[string]bool) []model.Message {
	seen := map[string]int{}
	seenNorm := map[string]int{}
	seenArgs := map[string]int{}
	repeatNudged := map[string]bool{}
	totalTokens := 0
	serverAttempts := 0
	debugInjected := false
	serveInjected := false
	depsInjected := false
	finishNudged := false
	usedTools := false

	// Grounding: named targets, which were mutated, whether anything changed, nudge guards.
	targets := req.Grounding.Targets()
	mutatedTargets := map[string]bool{}
	mutatedAny := false
	groundingStall := 0 // consecutive finish attempts that wrote no NEW target file
	groundedAt := 0     // count of targets written at the last finish attempt
	driftNudged := false
	falseDoneNudged := false
	noWriteTurns := 0 // turns that used tools but wrote nothing (create task explore-loop)
	writeForced := false

	// onDelta streams the model's tokens live: the answer as text, the thinking as
	// reasoning. Content is therefore already shown by the time a turn returns, so
	// the branches below do not re-emit it.
	onDelta := func(kind, text string) {
		switch kind {
		case "content":
			emit(events.Text(text))
		case "reasoning":
			emit(events.Reasoning(text))
		}
	}

	for step := 0; step < a.maxSteps; step++ {
		// Refresh the working-directory tree each step so a native run always sees the
		// current layout — including files a sibling sub-task just wrote — without
		// spending a list_dir call to re-orient. Mirrors runJSON; chat has no workdir.
		liveSystem := system
		if !req.Chat && env.WorkDir != "" {
			liveSystem = system + "\n\nCURRENT FILES in the working directory (refreshed every step):\n" + currentTree(env.WorkDir)
		}
		msg, tokens, err := a.chatStep(ctx, liveSystem, conv, defs, onDelta)
		if err != nil {
			emit(events.Error(err.Error()))
			return conv
		}
		totalTokens += tokens
		emit(events.Usage(totalTokens))

		// No tool calls: the model is giving its final answer.
		if len(msg.ToolCalls) == 0 {
			text := strings.TrimSpace(msg.Content)
			// A thinking-only turn (all reasoning, no answer) still has something to
			// show; fall back to the reasoning so the turn is never blank.
			if text == "" {
				text = strings.TrimSpace(msg.Reasoning)
			}
			// Grounding gate: a create task is not done until every named target exists.
			// Keep nudging AS LONG AS the model writes a new target each round (progress),
			// naming ONE file at a time — a degrading small model complies far better with
			// a single named write than "write all of them". Give up only after several
			// rounds with no new file.
			if !req.Chat && req.Grounding.RequiresMutation() {
				if missing := missingTargets(targets, mutatedTargets); len(missing) > 0 {
					if len(mutatedTargets) > groundedAt {
						groundingStall = 0
					} else {
						groundingStall++
					}
					groundedAt = len(mutatedTargets)
					if groundingStall <= maxGroundingStall {
						conv = append(conv, model.Message{Role: "assistant", Content: text})
						conv = append(conv, model.Message{Role: "user", Content: groundingMissMsg(missing)})
						continue
					}
					emit(events.Error("grounding failure: finished without creating the named target(s): " + strings.Join(missing, ", ")))
					return conv
				}
			}
			// False-completion guard: a coding task that changed no file at all.
			if !req.Chat && req.Grounding.IsCoding() && !mutatedAny {
				if !falseDoneNudged {
					falseDoneNudged = true
					conv = append(conv, model.Message{Role: "assistant", Content: text})
					conv = append(conv, model.Message{Role: "user", Content: falseDoneMsg})
					continue
				}
				emit(events.Error("false completion: finished a create/edit/fix task without any file mutation"))
				return conv
			}
			// Nudge once before accepting the finish, but only if the turn actually
			// did tool work — a plain answer or chat reply finishes immediately. In
			// chat mode there is no project to verify, so never nudge.
			if !req.Chat && !finishNudged && usedTools {
				finishNudged = true
				conv = append(conv, model.Message{Role: "assistant", Content: text})
				conv = append(conv, model.Message{Role: "user", Content: "Before finishing, verify completeness: list_dir to confirm EVERY file the task asked for actually exists, and confirm each check (test/build/curl) truly returned the expected result, not just exited. If any requested file is missing or any result is wrong, keep working with tools. Only if everything is genuinely done and verified, reply with your final summary."})
				continue
			}
			// The answer streamed live via onDelta; only append it and close out.
			conv = append(conv, model.Message{Role: "assistant", Content: finalText(text)})
			emit(events.Done())
			return conv
		}

		conv = append(conv, msg)
		usedTools = true

		anyFailure := false
		for _, tc := range msg.ToolCalls {
			emit(events.ToolCall(tc.Function.Name, summarizeCall(tc), tc.Function.Arguments))
			result, diff := a.reg.Dispatch(tc, req.Allowed, req.Mode, env, confirm)
			emit(events.ToolResult(tc.Function.Name, shortResult(result), result, diff))

			// Record mutations; trip the drift alarm on a non-target before any named target.
			if diff != nil && diff.Path != "" {
				mutatedAny = true
				changed[diff.Path] = true
				if t, ok := targetHit(targets, diff.Path); ok {
					mutatedTargets[t] = true
				} else if len(targets) > 0 && len(mutatedTargets) == 0 && !driftNudged {
					driftNudged = true
					conv = append(conv, model.Message{Role: "user", Content: driftMsg(diff.Path, targets)})
				}
				// A landed edit is real progress: reset the repeat counters so an
				// edit→re-verify→edit loop (e.g. fixing several routes, re-running the
				// same check each time) is not mistaken for a stuck loop. Pure read/run
				// loops with no edits still trip the breakers below.
				seen = map[string]int{}
				seenNorm = map[string]int{}
				seenArgs = map[string]int{}
			}

			conv = append(conv, model.Message{Role: "tool", ToolCallID: tc.ID, Name: tc.Function.Name, Content: result})
			if isFailure(result) {
				anyFailure = true
			}

			// Graceful repeat handling: an identical call with an identical result
			// gives no new information (e.g. re-reading a file already read to
			// "verify"). Nudge the model to finish rather than hard-erroring — the
			// work is usually already done. Only force a clean finish if it keeps
			// looping past the nudge, and never surface it as an error.
			key := tc.Function.Name + "|" + tc.Function.Arguments + "|" + shortResult(result)
			seen[key]++
			if seen[key] == 2 && !repeatNudged[key] {
				repeatNudged[key] = true
				conv = append(conv, model.Message{Role: "user", Content: "You just repeated the same action and got the same result — that gives no new information. If the change is already made and looks correct, STOP and reply with a one- or two-sentence summary now. Otherwise take a genuinely different step; do not read or run the same thing again."})
			} else if seen[key] >= 4 {
				summary := strings.TrimSpace(msg.Content)
				if summary == "" {
					summary = stuckSummary(env.WorkDir, result)
				}
				emit(events.Text(summary))
				conv = append(conv, model.Message{Role: "assistant", Content: summary})
				emit(events.Done())
				return conv
			}

			// Args-only repeat cap: the same tool + same arguments, this many times with
			// no landed edit in between, is stuck even when the result text differs (a
			// flaky check, timestamped output). This is the primary limiter the user asked
			// for — not a raw step count. Resets above on any file mutation.
			argKey := tc.Function.Name + "|" + tc.Function.Arguments
			seenArgs[argKey]++
			if seenArgs[argKey] >= repeatArgCap {
				summary := strings.TrimSpace(msg.Content)
				if summary == "" {
					summary = stuckSummary(env.WorkDir, result)
				}
				emit(events.Text(summary))
				conv = append(conv, model.Message{Role: "assistant", Content: summary})
				emit(events.Done())
				return conv
			}

			// Near-duplicate breaker: a small model often retries the SAME command with
			// only a number changed (port 8043→8044, sleep 42→43) after a refusal, so the
			// exact-key breaker above never fires. Collapse volatile numbers and catch the
			// pattern: numbers are never the fix — the code is.
			nkey := normalizeCall(tc.Function.Name, tc.Function.Arguments)
			seenNorm[nkey]++
			if seenNorm[nkey] == 3 && !repeatNudged[nkey] {
				repeatNudged[nkey] = true
				conv = append(conv, model.Message{Role: "user", Content: "You have retried near-identical commands that differ only by a number (a port, a timeout). Changing the number will NOT fix this and you do not need to start a server — that is handled for you. STOP running commands: open the file the error names and EDIT the code to fix the actual error. If it is already fixed, give your final answer now."})
			} else if seenNorm[nkey] >= 5 {
				summary := strings.TrimSpace(msg.Content)
				if summary == "" {
					summary = stuckSummary(env.WorkDir, result)
				}
				emit(events.Text(summary))
				conv = append(conv, model.Message{Role: "assistant", Content: summary})
				emit(events.Done())
				return conv
			}

			if tc.Function.Name == "shell_run" {
				var argm map[string]any
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &argm)
				if cmd, _ := argm["command"].(string); tools.ServerStartHint(cmd) != "" {
					serverAttempts++
					if canServe(req.Allowed) {
						if !serveInjected {
							if body := a.skills.bodies["serving"]; body != "" {
								conv = append(conv, model.Message{Role: "user", Content: "To run and verify a server, follow this procedure:\n" + body})
								serveInjected = true
							}
						}
					} else if serverAttempts >= 2 && !serveInjected {
						// A child without the serve tool must NOT try to run a server: it
						// blocks or fails, and the evaluator already boots and re-tests after
						// each edit. Redirect it to editing — this is the flail a boot-repair
						// child falls into (it varies the command each time, so the
						// near-duplicate breaker above does not catch it).
						serveInjected = true
						conv = append(conv, model.Message{Role: "user", Content: "Stop trying to start the server — you do NOT need to run it, and it will not help. The app is booted and re-tested for you automatically after you edit. Open the file the error names, fix it with edit_file/write_file, then give your final answer."})
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

		// Explore-loop breaker for create tasks: a child whose targets don't exist yet
		// keeps calling list_dir/read_file on those not-yet-created paths (they error)
		// and never writes. After two tool turns with no mutation, force it to write.
		if len(targets) > 0 && !mutatedAny {
			noWriteTurns++
			if noWriteTurns >= 2 && !writeForced {
				writeForced = true
				conv = append(conv, model.Message{Role: "user", Content: forceWriteMsg(missingTargets(targets, mutatedTargets))})
			}
		}
	}

	emit(events.Error(fmt.Sprintf("reached the step limit of %d without finishing", a.maxSteps)))
	return conv
}

// runJSON drives the grammar-constrained JSON-ReAct fallback (tool_mode "json").
func (a *Agent) runJSON(ctx context.Context, req Request, emit func(events.Event), confirm tools.ConfirmFunc, system string, schema json.RawMessage, env tools.Env, conv []model.Message, changed map[string]bool) []model.Message {
	seen := map[string]int{}
	seenNorm := map[string]int{}
	normNudged := map[string]bool{}
	totalTokens := 0
	debugInjected := false
	writesSinceRun := 0 // file writes since the last time anything was executed
	runNudged := false
	editFails := map[string]int{} // failed edit_file attempts per path
	editNudged := map[string]bool{}

	// Grounding (mirrors runNative).
	targets := req.Grounding.Targets()
	mutatedTargets := map[string]bool{}
	mutatedAny := false
	groundingStall := 0
	groundedAt := 0
	driftNudged := false
	falseDoneNudged := false

	for step := 0; step < a.maxSteps; step++ {
		// Refresh the working-directory tree each step so the model always sees the
		// current layout (including files it just created) without spending a
		// list_dir call to re-orient.
		liveSystem := system + "\n\nCURRENT FILES in the working directory (refreshed every step):\n" + currentTree(env.WorkDir)
		raw, tokens, err := a.planStep(ctx, liveSystem, conv, schema)
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
			text = finalText(text)

			// Grounding gate: progress-aware, one file at a time (mirrors runNative).
			if !req.Chat && req.Grounding.RequiresMutation() {
				if missing := missingTargets(targets, mutatedTargets); len(missing) > 0 {
					if len(mutatedTargets) > groundedAt {
						groundingStall = 0
					} else {
						groundingStall++
					}
					groundedAt = len(mutatedTargets)
					if groundingStall <= maxGroundingStall {
						conv = append(conv, model.Message{Role: "assistant", Content: raw})
						conv = append(conv, model.Message{Role: "user", Content: groundingMissMsg(missing)})
						continue
					}
					emit(events.Error("grounding failure: finished without creating the named target(s): " + strings.Join(missing, ", ")))
					return conv
				}
			}
			if !req.Chat && req.Grounding.IsCoding() && !mutatedAny {
				if !falseDoneNudged {
					falseDoneNudged = true
					conv = append(conv, model.Message{Role: "assistant", Content: raw})
					conv = append(conv, model.Message{Role: "user", Content: falseDoneMsg})
					continue
				}
				emit(events.Error("false completion: finished a create/edit/fix task without any file mutation"))
				return conv
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
		emit(events.ToolCall(tc.Function.Name, summarizeCall(tc), tc.Function.Arguments))
		result, diff := a.reg.Dispatch(tc, req.Allowed, req.Mode, env, confirm)
		emit(events.ToolResult(tc.Function.Name, shortResult(result), result, diff))

		// Record the step and feed the observation back for the next turn.
		conv = append(conv, model.Message{Role: "assistant", Content: raw})
		conv = append(conv, model.Message{Role: "user", Content: fmt.Sprintf("Result of %s:\n%s", tc.Function.Name, result)})

		// Record mutations; trip the drift alarm on a non-target before any named target.
		if diff != nil && diff.Path != "" {
			mutatedAny = true
			changed[diff.Path] = true
			if t, ok := targetHit(targets, diff.Path); ok {
				mutatedTargets[t] = true
			} else if len(targets) > 0 && len(mutatedTargets) == 0 && !driftNudged {
				driftNudged = true
				conv = append(conv, model.Message{Role: "user", Content: driftMsg(diff.Path, targets)})
			}
			// A landed edit is progress — reset the repeat counters so edit→verify→edit
			// is not mistaken for a stuck loop (see runNative).
			seen = map[string]int{}
			seenNorm = map[string]int{}
		}

		// Detect a repeated action with the same result. Rather than hard-stop on
		// the first repeat, nudge the model to change course; only give up if it
		// keeps repeating, and then finish with a progress summary, not a bare error.
		key := tc.Function.Name + "|" + tc.Function.Arguments + "|" + shortResult(result)
		seen[key]++
		if seen[key] == 2 {
			conv = append(conv, model.Message{Role: "user", Content: "You just repeated the SAME action and got the SAME result. Do not run it again. Read that result carefully and take a genuinely different next step — fix the actual file the error names, or use a different tool. If the task is already complete, give your final answer instead."})
		} else if seen[key] >= 3 {
			summary := stuckSummary(env.WorkDir, result)
			emit(events.Text(summary))
			conv = append(conv, model.Message{Role: "assistant", Content: summary})
			emit(events.Done())
			return conv
		}

		// Near-duplicate breaker: catch retries that differ only by a number (a port,
		// a timeout) — the exact-key check above never fires on those.
		nkey := normalizeCall(tc.Function.Name, tc.Function.Arguments)
		seenNorm[nkey]++
		if seenNorm[nkey] == 3 && !normNudged[nkey] {
			normNudged[nkey] = true
			conv = append(conv, model.Message{Role: "user", Content: "You have retried near-identical commands that differ only by a number (a port, a timeout). Changing the number will NOT fix this and you do not need to start a server — that is handled for you. STOP running commands: open the file the error names and EDIT the code to fix the actual error. If it is already fixed, give your final answer now."})
		} else if seenNorm[nkey] >= 5 {
			summary := stuckSummary(env.WorkDir, result)
			emit(events.Text(summary))
			conv = append(conv, model.Message{Role: "assistant", Content: summary})
			emit(events.Done())
			return conv
		}

		// Break write-churn: a small model tends to keep rewriting files it has
		// never executed, chasing imagined bugs. After several writes with no
		// execution, force it to RUN the project for real error output.
		switch tc.Function.Name {
		case "shell_run", "code_run", "serve":
			writesSinceRun = 0
			runNudged = false
		case "write_file", "edit_file":
			writesSinceRun++
		}
		if writesSinceRun >= 5 && !runNudged {
			conv = append(conv, model.Message{Role: "user", Content: "You have written several files without running anything. STOP writing more files. Install dependencies if needed, then RUN the project (its tests, or start the server and curl it) to get REAL error output, and fix based on that actual output. Running gives ground truth that re-reading your own code cannot — do not rewrite files you have not executed."})
			runNudged = true
		}

		// edit_file keeps failing when the model's old_text is not actually in the
		// file. After two failures on a file, force a full rewrite with write_file.
		if tc.Function.Name == "edit_file" && strings.Contains(result, "old_text not found") {
			p, _ := act.Arguments["path"].(string)
			editFails[p]++
			if editFails[p] >= 2 && !editNudged[p] {
				conv = append(conv, model.Message{Role: "user", Content: "edit_file has failed repeatedly on " + p + " because your old_text is not present in the file. Do NOT use edit_file on this file again. Instead call write_file with path " + p + " and the ENTIRE corrected file content (every line), which replaces the file completely."})
				editNudged[p] = true
			}
		}

		// The first time something fails, inject the debug procedure, since a
		// small model will not reliably load it on its own.
		if !debugInjected && isFailure(result) {
			if body := a.skills.bodies["debug"]; body != "" {
				conv = append(conv, model.Message{Role: "user", Content: "The last step failed. Follow this debugging procedure:\n" + body})
				debugInjected = true
			}
		}
	}

	summary := stuckSummary(env.WorkDir, fmt.Sprintf("reached the step limit of %d", a.maxSteps))
	emit(events.Text(summary))
	emit(events.Done())
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
		a.log.request(msgs, nil)
		raw, tokens, err := a.router.Constrained(ctx, msgs, schema)
		if err == nil {
			a.log.response(raw, nil, tokens)
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
// context budget and shrinking it on overflow. Mirrors planStep. onDelta streams
// tokens as they arrive; an overflow surfaces as a non-200 before any token, so a
// retry never double-emits.
func (a *Agent) chatStep(ctx context.Context, system string, conv []model.Message, defs []model.ToolDef, onDelta func(kind, text string)) (model.Message, int, error) {
	budget := a.contextTokens
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		msgs := compact(system, conv, budget)
		a.log.request(msgs, defs)
		msg, tokens, err := a.router.Chat(ctx, msgs, defs, onDelta)
		if err == nil {
			a.log.response(msg.Content, msg.ToolCalls, tokens)
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

// finalText guards against a blank final answer (some models, e.g. thinking
// models in the wrong mode, return empty content) so the UI never shows an
// empty reply.
func finalText(s string) string {
	if strings.TrimSpace(s) == "" {
		return "I couldn't produce a response for that. Try rephrasing, or switch models with the picker."
	}
	return s
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
