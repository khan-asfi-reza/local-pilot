// Package events defines the newline-delimited JSON events that every layer of
package events

type Op string

const (
	OpContext Op = "context"
	OpAdd     Op = "add"
	OpRemove  Op = "remove"
)

type DiffLine struct {
	Op   Op     `json:"op"`
	Old  int    `json:"old,omitempty"`
	New  int    `json:"new,omitempty"`
	Text string `json:"text"`
}

type Hunk struct {
	OldStart int        `json:"old_start"`
	OldCount int        `json:"old_count"`
	NewStart int        `json:"new_start"`
	NewCount int        `json:"new_count"`
	Lines    []DiffLine `json:"lines"`
}

type Diff struct {
	Path    string `json:"path"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Hunks   []Hunk `json:"hunks"`
}

type Event struct {
	Type     string `json:"type"`
	Content  string `json:"content,omitempty"`
	Tool     string `json:"tool,omitempty"`
	Info     string `json:"info,omitempty"`
	ID       string `json:"id,omitempty"`
	Summary  string `json:"summary,omitempty"`
	Diff     *Diff  `json:"diff,omitempty"`
	Decision string `json:"decision,omitempty"`
	Message  string `json:"message,omitempty"`
	Data string `json:"data,omitempty"`
	Tokens int `json:"tokens,omitempty"`
}

func Text(content string) Event { return Event{Type: "text", Content: content} }

func ToolCall(tool, info string) Event { return Event{Type: "tool_call", Tool: tool, Info: info} }

func ToolResult(tool, info, data string, diff *Diff) Event {
	return Event{Type: "tool_result", Tool: tool, Info: info, Data: data, Diff: diff}
}

func Confirm(id, tool, summary string, diff *Diff) Event {
	return Event{Type: "confirm", ID: id, Tool: tool, Summary: summary, Diff: diff}
}

func Error(message string) Event { return Event{Type: "error", Message: message} }

func Done() Event { return Event{Type: "done"} }

func Usage(total int) Event { return Event{Type: "usage", Tokens: total} }
