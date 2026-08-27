package systemtest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"harness/harness/agent"
	"harness/harness/events"
	"harness/harness/model"
	"harness/harness/tools"
)

// stubModel is a scripted OpenAI-compatible backend.
//
// Planning (intake, decomposition) reaches it as an ordinary chat turn offering a
// single "submit" tool, so those requests are answered from plan; memory updates
// arrive as grammar-constrained calls carrying a response_format. Everything else
// is a normal agent turn, answered from turns in order with the last reply
// repeated afterwards.
type stubModel struct {
	mu        sync.Mutex
	turns     []string // raw SSE data payloads, one per agent turn
	plan      func(body string) string
	chatCalls int
	planCalls int
}

func (s *stubModel) start(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			fmt.Fprint(w, `{"version":"stub"}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		bodyText := string(raw)

		s.mu.Lock()
		defer s.mu.Unlock()

		// A grammar-constrained call (the memory update): reply with an empty object.
		if strings.Contains(bodyText, `"response_format"`) {
			payload, _ := json.Marshal(map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": "{}"}}},
				"usage":   map[string]any{"total_tokens": 10},
			})
			w.Write(payload)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")

		// A planning call: the only tool offered is "submit".
		if strings.Contains(bodyText, `"name":"submit"`) {
			s.planCalls++
			out := "{}"
			if s.plan != nil {
				out = s.plan(bodyText)
			}
			fmt.Fprintf(w, "data: %s\n\n", toolTurnRaw("submit", out))
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}

		idx := s.chatCalls
		s.chatCalls++
		if idx >= len(s.turns) {
			idx = len(s.turns) - 1
		}
		fmt.Fprintf(w, "data: %s\n\n", s.turns[idx])
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// newAgent builds a real Agent wired to a stub backend.
func newAgent(t *testing.T, url string) *agent.Agent {
	t.Helper()
	host := strings.TrimPrefix(url, "http://")
	cfg, _ := writeConfig(t, `{"context_tokens":8000,"default":"stub","models":[{"name":"stub","model":"stub","host":"`+host+`"}]}`)
	a, err := agent.New(cfg, "")
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	a.SetMaxSteps(20)
	return a
}

func textTurn(s string) string {
	payload, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"delta": map[string]any{"content": s}}},
	})
	return string(payload)
}

func toolTurn(name string, args map[string]any) string {
	raw, _ := json.Marshal(args)
	return toolTurnRaw(name, string(raw))
}

func toolTurnRaw(name, raw string) string {
	payload, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"delta": map[string]any{
			"tool_calls": []any{map[string]any{
				"index": 0, "id": "call_1", "type": "function",
				"function": map[string]any{"name": name, "arguments": raw},
			}},
		}}},
	})
	return string(payload)
}

// collect gathers the events a run emits.
type collected struct {
	mu     sync.Mutex
	events []events.Event
}

func (c *collected) emit(ev events.Event) {
	c.mu.Lock()
	c.events = append(c.events, ev)
	c.mu.Unlock()
}

func (c *collected) ofType(kind string) []events.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []events.Event
	for _, ev := range c.events {
		if ev.Type == kind {
			out = append(out, ev)
		}
	}
	return out
}

// intakeReply answers the intake classification with a fixed grounding contract.
func intakeReply(targets string, fileCount int) func(string) string {
	return func(body string) string {
		if strings.Contains(body, "acceptance_criteria") {
			return fmt.Sprintf(`{"action":"create","file_count":%d,"explicit_targets":[%s],"acceptance_criteria":["it runs"]}`, fileCount, targets)
		}
		return "{}"
	}
}

// TestAgentWritesTheFileItWasAskedFor is the end-to-end loop: classify, call a
// tool, dispatch it against the real filesystem, stream events, and finish.
func TestAgentWritesTheFileItWasAskedFor(t *testing.T) {
	stub := &stubModel{
		plan: intakeReply(`"hello.py"`, 1),
		turns: []string{
			toolTurn("write_file", map[string]any{"path": "hello.py", "content": "print('hello')\n"}),
			textTurn("Created hello.py."),
		},
	}
	a := newAgent(t, stub.start(t))
	dir := tempProject(t, nil)
	got := &collected{}

	conv := a.Run(context.Background(), agent.Request{
		Messages: []model.Message{{Role: "user", Content: "create hello.py that prints hello"}},
		Allowed:  a.ToolNames(),
		Mode:     tools.ModeAuto,
		WorkDir:  dir,
	}, got.emit, nil)
	a.Wait()

	if !exists(dir, "hello.py") {
		t.Fatal("the agent finished without writing the file it was asked for")
	}
	if body(t, dir, "hello.py") != "print('hello')\n" {
		t.Errorf("file content = %q", body(t, dir, "hello.py"))
	}
	if len(got.ofType("tool_call")) == 0 || len(got.ofType("tool_result")) == 0 {
		t.Error("the run streamed no tool activity for the UI to render")
	}
	if len(got.ofType("done")) != 1 {
		t.Errorf("expected exactly one done event, got %d", len(got.ofType("done")))
	}
	if len(got.ofType("error")) != 0 {
		t.Errorf("unexpected error events: %+v", got.ofType("error"))
	}
	if conv[len(conv)-1].Role != "assistant" {
		t.Errorf("the conversation should end on the assistant's answer, got %q", conv[len(conv)-1].Role)
	}
	if stub.planCalls == 0 {
		t.Error("no grounding contract was requested before the run")
	}
}

// TestAgentRefusesAFalseCompletion checks the grounding gate: a model that
// announces success without touching the named target is pushed back, and the
// run ends with an explicit grounding failure rather than a cheerful lie.
func TestAgentRefusesAFalseCompletion(t *testing.T) {
	stub := &stubModel{
		plan:  intakeReply(`"area.c"`, 1),
		turns: []string{textTurn("Done! I have created area.c for you.")},
	}
	a := newAgent(t, stub.start(t))
	dir := tempProject(t, nil)
	got := &collected{}

	a.Run(context.Background(), agent.Request{
		Messages: []model.Message{{Role: "user", Content: "create area.c that computes shape areas"}},
		Allowed:  a.ToolNames(),
		Mode:     tools.ModeAuto,
		WorkDir:  dir,
	}, got.emit, nil)
	a.Wait()

	errs := got.ofType("error")
	if len(errs) == 0 {
		t.Fatal("a false completion was accepted silently")
	}
	if !strings.Contains(errs[len(errs)-1].Message, "grounding failure") {
		t.Errorf("error did not name the grounding failure: %q", errs[len(errs)-1].Message)
	}
	if stub.chatCalls < 3 {
		t.Errorf("the model was nudged only %d times before giving up; the gate should retry", stub.chatCalls-1)
	}
	if exists(dir, "area.c") {
		t.Error("area.c exists, so this was not a false completion after all")
	}
}

// TestAgentPlanModeAnswersWithoutTouchingTheProject checks a plan-mode turn
// cannot change anything even when the model tries.
func TestAgentPlanModeAnswersWithoutTouchingTheProject(t *testing.T) {
	stub := &stubModel{
		turns: []string{
			toolTurn("write_file", map[string]any{"path": "sneaky.py", "content": "x = 1\n"}),
			textTurn("Here is the plan: create sneaky.py, then wire it up."),
		},
	}
	a := newAgent(t, stub.start(t))
	dir := tempProject(t, nil)
	got := &collected{}

	a.Run(context.Background(), agent.Request{
		Messages: []model.Message{{Role: "user", Content: "how would you add a script?"}},
		Allowed:  a.ToolNames(),
		Mode:     tools.ModePlan,
		WorkDir:  dir,
	}, got.emit, nil)
	a.Wait()

	if exists(dir, "sneaky.py") {
		t.Fatal("plan mode wrote a file")
	}
	if stub.planCalls != 0 {
		t.Error("plan mode should skip the intake classification call")
	}
	if len(got.ofType("done")) != 1 {
		t.Errorf("plan-mode turn did not finish cleanly: %+v", got.ofType("done"))
	}
}

// TestAgentChatModeStaysConversational checks the no-project chat path: no
// grounding, no file workflow, one answer.
func TestAgentChatModeStaysConversational(t *testing.T) {
	stub := &stubModel{turns: []string{textTurn("A closure captures its enclosing scope.")}}
	a := newAgent(t, stub.start(t))
	got := &collected{}

	conv := a.Run(context.Background(), agent.Request{
		Messages: []model.Message{{Role: "user", Content: "what is a closure?"}},
		Allowed:  []string{"web_search"},
		Mode:     tools.ModeAuto,
		Chat:     true,
	}, got.emit, nil)
	a.Wait()

	if stub.planCalls != 0 {
		t.Error("chat mode should not run the intake classification")
	}
	if stub.chatCalls != 1 {
		t.Errorf("chat mode took %d model turns for a plain question", stub.chatCalls)
	}
	last := conv[len(conv)-1]
	if last.Role != "assistant" || !strings.Contains(last.Content, "closure") {
		t.Errorf("chat answer = %+v", last)
	}
}

// TestAgentReportsABackendOutage checks that an unreachable model server ends the
// turn with a visible error instead of an empty success.
func TestAgentReportsABackendOutage(t *testing.T) {
	cfg, _ := writeConfig(t, `{"default":"dead","models":[{"name":"dead","model":"dead","host":"127.0.0.1:1"}]}`)
	a, err := agent.New(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	got := &collected{}

	a.Run(context.Background(), agent.Request{
		Messages: []model.Message{{Role: "user", Content: "hello"}},
		Allowed:  a.ToolNames(),
		Mode:     tools.ModeAuto,
		Chat:     true,
	}, got.emit, nil)

	if len(got.ofType("error")) == 0 {
		t.Fatal("an unreachable backend produced no error event")
	}
}

// TestAgentBreaksOutOfARepeatLoop checks the progress breaker: a small model that
// keeps issuing the identical tool call, changing nothing, is stopped instead of
// burning the whole step budget. Without this a degrading 4B model spins forever.
func TestAgentBreaksOutOfARepeatLoop(t *testing.T) {
	stub := &stubModel{
		plan:  intakeReply("", 1),
		turns: []string{toolTurn("list_dir", map[string]any{"path": ".", "depth": 1})},
	}
	a := newAgent(t, stub.start(t))
	a.SetMaxSteps(200)
	dir := tempProject(t, map[string]string{"a.py": "x\n"})
	got := &collected{}

	done := make(chan struct{})
	go func() {
		a.Run(context.Background(), agent.Request{
			Messages: []model.Message{{Role: "user", Content: "look around"}},
			Allowed:  a.ToolNames(),
			Mode:     tools.ModeAuto,
			WorkDir:  dir,
		}, got.emit, nil)
		close(done)
	}()

	select {
	case <-done:
	case <-timeoutAfterSeconds(60):
		t.Fatal("the agent never broke out of a repeated tool call")
	}
	a.Wait()

	if stub.chatCalls >= 200 {
		t.Errorf("the run used %d model turns; the repeat breaker did not fire", stub.chatCalls)
	}
	if stub.chatCalls < 3 {
		t.Errorf("the breaker fired after only %d turns, which would cut off normal work", stub.chatCalls)
	}
}
