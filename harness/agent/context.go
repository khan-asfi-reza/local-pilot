package agent

import (
	"strings"

	"harness/harness/model"
)

// recentFullMessages is how many of the most recent messages are kept verbatim.
// Older messages are elided to their opening lines, which strips stale bulk like
// full file contents that were read or written many steps ago but no longer need
// to sit in context. This is the core of the context engineering: the model sees
// its recent working set in full and older steps as short summaries.
const recentFullMessages = 8

// elideHead is how much of an old, bulky message to keep. The opening carries the
// useful part (the reasoning, the tool, the file path); the rest is usually a
// file dump that is redundant once the step is done.
const elideHead = 300

// estTokens roughly estimates the token count of a string. A tokenizer is not
// available in the harness, so this uses the common four-characters-per-token
// approximation, which is close enough to keep the conversation within budget.
func estTokens(s string) int {
	return len(s)/4 + 1
}

// msgTokens estimates the tokens a single message costs, including any tool-call
// arguments it carries.
func msgTokens(m model.Message) int {
	n := estTokens(m.Content)
	for _, tc := range m.ToolCalls {
		n += estTokens(tc.Function.Arguments)
	}
	return n
}

// compact builds the message list sent to the planner so it fits within a token
// budget. It keeps the system prompt and the largest suffix of recent messages
// that fits, drops the older middle, and leaves a short note plus the original
// task in its place. This is what makes the conversation feel unlimited: instead
// of hitting a hard token error, older turns are quietly summarized away.
func compact(system string, conv []model.Message, budget int) []model.Message {
	// Elide stale bulk (old file reads and writes) before fitting to budget, so
	// the recent working set survives instead of being crowded out. This operates
	// on a copy; the caller's history stays complete for the session.
	conv = elideHistory(conv, recentFullMessages)

	sys := model.Message{Role: "system", Content: system}
	avail := budget - estTokens(system) - 512
	if avail < 1000 {
		avail = 1000
	}

	// Choose the largest suffix of the conversation that fits. The most recent
	// message is always kept, truncated if it alone is too large.
	used := 0
	start := len(conv)
	for i := len(conv) - 1; i >= 0; i-- {
		t := msgTokens(conv[i])
		if i < len(conv)-1 && used+t > avail {
			break
		}
		used += t
		start = i
	}

	// A tool message must follow its assistant turn; if the suffix would start on
	// an orphaned tool message, skip past it so the sequence stays valid.
	for start < len(conv) && conv[start].Role == "tool" {
		start++
	}

	out := make([]model.Message, 0, len(conv)-start+3)
	out = append(out, sys)
	if start > 0 {
		// Older turns were dropped. Keep the original task so the goal survives,
		// and mark that the middle was compacted.
		if conv[0].Role == "user" {
			out = append(out, capMsg(conv[0], avail))
		}
		out = append(out, model.Message{Role: "user", Content: "[Earlier steps were compacted to save context. Continue the task from the recent messages below.]"})
	}
	for _, m := range conv[start:] {
		out = append(out, capMsg(m, avail))
	}
	return out
}

// elideHistory returns a copy of the conversation with older bulky messages
// shortened to their opening lines. The most recent keepRecent messages, and any
// short message, are kept verbatim. The caller's slice is not modified.
func elideHistory(conv []model.Message, keepRecent int) []model.Message {
	out := make([]model.Message, len(conv))
	copy(out, conv)
	cut := len(conv) - keepRecent
	for i := 0; i < cut; i++ {
		out[i] = elideMessage(out[i])
	}
	return out
}

// elideMessage keeps a small message as-is; a large one is cut to its head with
// a marker, dropping the stale bulk (usually file content) that follows.
func elideMessage(m model.Message) model.Message {
	if len(m.Content) <= elideHead*2 {
		return m
	}
	head := m.Content[:elideHead]
	// Cut at the last newline in the head so the summary ends on a clean line.
	if nl := strings.LastIndexByte(head, '\n'); nl > 0 {
		head = head[:nl]
	}
	m.Content = head + "\n… [earlier detail elided]"
	return m
}

// capMsg truncates a message's content from the middle if it alone exceeds the
// available budget, so a single huge tool result cannot overflow the window.
func capMsg(m model.Message, availTokens int) model.Message {
	if estTokens(m.Content) <= availTokens {
		return m
	}
	max := availTokens * 4
	half := max / 2
	m.Content = m.Content[:half] + "\n... [truncated to fit context] ...\n" + m.Content[len(m.Content)-half:]
	return m
}
