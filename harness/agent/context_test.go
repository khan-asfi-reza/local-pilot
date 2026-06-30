package agent

import (
	"strings"
	"testing"

	"harness/harness/model"
)

// TestCompactStaysUnderBudget checks that a long conversation is trimmed to fit
// the token budget while keeping the system prompt, the original task, and the
// most recent message.
func TestCompactStaysUnderBudget(t *testing.T) {
	conv := []model.Message{{Role: "user", Content: "ORIGINAL TASK: build the thing"}}
	for i := 0; i < 200; i++ {
		conv = append(conv, model.Message{Role: "assistant", Content: strings.Repeat("x", 400)})
		conv = append(conv, model.Message{Role: "user", Content: strings.Repeat("y", 400)})
	}

	budget := 5000
	out := compact("SYSTEM PROMPT", conv, budget)

	total := 0
	for _, m := range out {
		total += msgTokens(m)
	}
	if total > budget {
		t.Fatalf("compacted context is %d tokens, over budget %d", total, budget)
	}
	if out[0].Role != "system" {
		t.Fatalf("system prompt is not first")
	}
	found := false
	for _, m := range out {
		if strings.Contains(m.Content, "ORIGINAL TASK") {
			found = true
		}
	}
	if !found {
		t.Fatalf("original task was dropped during compaction")
	}
	if out[len(out)-1].Content != conv[len(conv)-1].Content {
		t.Fatalf("most recent message was not kept")
	}
}

// TestElideHistory checks that old bulky messages are shortened while the recent
// ones stay verbatim, and the caller's slice is not mutated.
func TestElideHistory(t *testing.T) {
	big := strings.Repeat("x", 2000)
	var conv []model.Message
	for i := 0; i < 12; i++ {
		conv = append(conv, model.Message{Role: "user", Content: big})
	}

	out := elideHistory(conv, recentFullMessages)

	if len(out[0].Content) >= 2000 {
		t.Fatalf("old message was not elided: %d chars", len(out[0].Content))
	}
	if out[len(out)-1].Content != big {
		t.Fatalf("most recent message was wrongly elided")
	}
	if conv[0].Content != big {
		t.Fatalf("caller's conversation was mutated")
	}
}

// TestCompactKeepsEverythingWhenSmall checks that a short conversation is passed
// through untouched apart from the system prompt.
func TestCompactKeepsEverythingWhenSmall(t *testing.T) {
	conv := []model.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	out := compact("SYS", conv, 100000)
	if len(out) != len(conv)+1 {
		t.Fatalf("expected system + %d messages, got %d", len(conv), len(out))
	}
}
