package terminal

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"harness/harness/model"
)

// Session is one conversation in a project's .pilot directory. Each has a unique
// id and its own model choice, so a project can hold several and resume any with
// the model it was using.
type Session struct {
	ID        string          `json:"id"`
	Model     string          `json:"model,omitempty"`
	Mode      string          `json:"mode"`
	Title     string          `json:"title,omitempty"`
	Messages  []model.Message `json:"messages"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

// sessionsDir is where a project's sessions live.
func sessionsDir(workDir string) string {
	return filepath.Join(workDir, ".pilot", "sessions")
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// newSession makes a fresh session with a unique id, defaulting to ask mode.
func newSession() *Session {
	now := time.Now().UTC().Format(time.RFC3339)
	return &Session{ID: newID(), Mode: "ask", CreatedAt: now, UpdatedAt: now}
}

// loadSession resumes the most recently updated session in the working
// directory, or creates a fresh one if there are none.
func loadSession(workDir string) *Session {
	all := listSessions(workDir)
	if len(all) == 0 {
		return newSession()
	}
	s := all[0]
	if s.Mode == "" {
		s.Mode = "ask"
	}
	return s
}

// listSessions returns every session in the working directory, newest first.
func listSessions(workDir string) []*Session {
	entries, err := os.ReadDir(sessionsDir(workDir))
	if err != nil {
		return nil
	}
	var out []*Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(sessionsDir(workDir), e.Name()))
		if err != nil {
			continue
		}
		var s Session
		if json.Unmarshal(raw, &s) == nil && s.ID != "" {
			cp := s
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out
}

// save writes the session to .pilot/sessions/<id>.json, stamping timestamps and
// a title from the first user message.
func (s *Session) save(workDir string) error {
	if s.ID == "" {
		s.ID = newID()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if s.CreatedAt == "" {
		s.CreatedAt = now
	}
	s.UpdatedAt = now
	if s.Title == "" {
		s.Title = deriveTitle(s.Messages)
	}
	dir := sessionsDir(workDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, s.ID+".json"), raw, 0o644)
}

// deriveTitle uses the first user message as a short session title.
func deriveTitle(msgs []model.Message) string {
	for _, m := range msgs {
		if m.Role == "user" && strings.TrimSpace(m.Content) != "" {
			t := strings.TrimSpace(m.Content)
			if len(t) > 60 {
				t = t[:60]
			}
			return t
		}
	}
	return ""
}
