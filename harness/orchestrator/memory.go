package orchestrator

import (
	"sort"
	"strings"
	"sync"
)

const digestBudget = 1500

// Memory is the only channel between sub-tasks: distilled outcomes, never
// transcripts. A child reads a deps-scoped, budget-capped Digest.
type Memory struct {
	mu    sync.Mutex
	notes map[string]Note
}

func NewMemory() *Memory { return &Memory{notes: map[string]Note{}} }

func (m *Memory) Add(n Note) {
	m.mu.Lock()
	m.notes[n.TaskID] = n
	m.mu.Unlock()
}

// Digest summarizes only the given dependency tasks, capped to a token budget.
func (m *Memory) Digest(depIDs []string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(depIDs) == 0 {
		return ""
	}
	ids := append([]string(nil), depIDs...)
	sort.Strings(ids)
	var b strings.Builder
	for _, id := range ids {
		n, ok := m.notes[id]
		if !ok {
			continue
		}
		b.WriteString("- " + n.Title)
		if len(n.Files) > 0 {
			b.WriteString(" (" + strings.Join(n.Files, ", ") + ")")
		}
		if n.Summary != "" {
			b.WriteString(": " + n.Summary)
		}
		b.WriteString("\n")
		if b.Len() > digestBudget {
			break
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
