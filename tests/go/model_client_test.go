package systemtest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"harness/harness/model"
)

// sseServer replies to /v1/chat/completions with the given SSE lines and answers
// ollama's /api/version probe. It records the last request body it received.
func sseServer(t *testing.T, chunks []string) (*httptest.Server, *string) {
	t.Helper()
	var lastBody string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version":"0.1.0"}`)
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		lastBody = string(raw)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			w.(http.Flusher).Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &lastBody
}

// TestChatReassemblesSplitToolCalls checks the streaming client stitches a tool
// call back together from fragments. Ollama splits write_file arguments across
// many deltas, and a naive reader loses the file body — the exact failure behind
// "the agent produced no files".
func TestChatReassemblesSplitToolCalls(t *testing.T) {
	chunks := []string{
		`{"choices":[{"delta":{"reasoning":"I should write the file. "}}]}`,
		`{"choices":[{"delta":{"content":"Writing app.py"}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"a"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"pp.py\",\"content\":\"print(1)\"}"}}]}}]}`,
		`{"choices":[],"usage":{"total_tokens":812}}`,
	}
	srv, body := sseServer(t, chunks)
	client := model.NewClient()

	msg, tokens, err := client.Chat(context.Background(), srv.URL, "qwen3.5:4b",
		[]model.Message{{Role: "user", Content: "write app.py"}}, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 reassembled tool call, got %d", len(msg.ToolCalls))
	}
	tc := msg.ToolCalls[0]
	if tc.Function.Name != "write_file" {
		t.Errorf("tool name = %q", tc.Function.Name)
	}
	var args map[string]string
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		t.Fatalf("reassembled arguments are not valid JSON: %q", tc.Function.Arguments)
	}
	if args["path"] != "app.py" || args["content"] != "print(1)" {
		t.Errorf("arguments lost content across chunks: %+v", args)
	}
	if msg.Content != "Writing app.py" {
		t.Errorf("assistant text = %q", msg.Content)
	}
	if msg.Reasoning == "" {
		t.Error("reasoning deltas were dropped instead of kept apart from the answer")
	}
	if tokens != 812 {
		t.Errorf("usage tokens = %d, want 812", tokens)
	}

	// The request must ask for a large enough output budget, or long file writes
	// are truncated server-side and the tool call is lost.
	var sent map[string]any
	if err := json.Unmarshal([]byte(*body), &sent); err != nil {
		t.Fatalf("request body was not JSON: %v", err)
	}
	if sent["max_tokens"] == nil || sent["max_tokens"].(float64) < 8192 {
		t.Errorf("max_tokens = %v, want at least 8192", sent["max_tokens"])
	}
	if sent["stream"] != true {
		t.Error("chat should stream")
	}
}

// TestChatStreamsDeltasToTheUI checks tokens reach the caller as they arrive,
// split by kind, which is what the terminal and web UI render live.
func TestChatStreamsDeltasToTheUI(t *testing.T) {
	srv, _ := sseServer(t, []string{
		`{"choices":[{"delta":{"reasoning":"think"}}]}`,
		`{"choices":[{"delta":{"content":"Hel"}}]}`,
		`{"choices":[{"delta":{"content":"lo"}}]}`,
	})

	var content, reasoning strings.Builder
	_, _, err := model.NewClient().Chat(context.Background(), srv.URL, "m", nil, nil,
		func(kind, text string) {
			switch kind {
			case "content":
				content.WriteString(text)
			case "reasoning":
				reasoning.WriteString(text)
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	if content.String() != "Hello" {
		t.Errorf("streamed content = %q, want Hello", content.String())
	}
	if reasoning.String() != "think" {
		t.Errorf("streamed reasoning = %q, want think", reasoning.String())
	}
}

// TestChatSurfacesBackendErrors checks a non-200 from the model server becomes a
// readable error rather than an empty reply the agent would treat as "done".
func TestChatSurfacesBackendErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "grammar not supported", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, _, err := model.NewClient().Chat(context.Background(), srv.URL, "m", nil, nil, nil)
	if err == nil {
		t.Fatal("a 500 from the backend was swallowed")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "grammar not supported") {
		t.Errorf("error hides the cause: %v", err)
	}
}

// TestConstrainedCallSendsASchema checks the grammar-constrained planning call
// carries the JSON schema and returns the raw JSON body.
func TestConstrainedCallSendsASchema(t *testing.T) {
	var lastBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		lastBody = string(raw)
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"{\"action\":\"create\"}"}}],"usage":{"total_tokens":42}}`)
	}))
	defer srv.Close()

	schema := json.RawMessage(`{"type":"object","properties":{"action":{"type":"string"}},"required":["action"]}`)
	out, tokens, err := model.NewClient().CompleteConstrained(context.Background(), srv.URL, "m",
		[]model.Message{{Role: "user", Content: "classify"}}, schema)
	if err != nil {
		t.Fatal(err)
	}
	if out != `{"action":"create"}` {
		t.Errorf("constrained output = %q", out)
	}
	if tokens != 42 {
		t.Errorf("tokens = %d", tokens)
	}
	if !strings.Contains(lastBody, `"response_format"`) || !strings.Contains(lastBody, `"json_schema"`) {
		t.Errorf("request did not carry the schema: %s", lastBody)
	}
	if strings.Contains(lastBody, `"stream":true`) {
		t.Error("a constrained planning call must not stream")
	}
}

// TestReachableProbesTheBackend checks the health probe used to decide whether a
// configured model is actually up.
func TestReachableProbesTheBackend(t *testing.T) {
	srv, _ := sseServer(t, nil)
	client := model.NewClient()

	if !client.Reachable(srv.URL) {
		t.Error("a live backend was reported unreachable")
	}
	if client.Reachable("http://127.0.0.1:1") {
		t.Error("a dead port was reported reachable")
	}
}

// TestRouterFallsBackWhenThePlannerIsUnknown checks ChatWith on an unconfigured
// planner name routes to the active model instead of deadlocking on the
// inference slot it already holds.
func TestRouterFallsBackWhenThePlannerIsUnknown(t *testing.T) {
	srv, _ := sseServer(t, []string{`{"choices":[{"delta":{"content":"ok"}}]}`})
	host := strings.TrimPrefix(srv.URL, "http://")
	cfg, _ := writeConfig(t, `{"default":"live","models":[{"name":"live","model":"qwen3.5:4b","host":"`+host+`"}]}`)
	router := model.NewRouter(cfg, model.NewClient())

	done := make(chan model.Message, 1)
	go func() {
		msg, _, err := router.ChatWith(context.Background(), "not-configured", nil, nil, nil)
		if err != nil {
			t.Errorf("ChatWith: %v", err)
		}
		done <- msg
	}()

	select {
	case msg := <-done:
		if msg.Content != "ok" {
			t.Errorf("fallback reply = %q", msg.Content)
		}
	case <-timeoutAfterSeconds(10):
		t.Fatal("ChatWith deadlocked on an unknown planner model")
	}
	if router.PlannerName() != "live" {
		t.Errorf("planner name = %q", router.PlannerName())
	}
}
