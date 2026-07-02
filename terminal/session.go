package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"harness/harness/model"
)

// Session is the conversation and client state the terminal owns and persists.
// The harness itself stores nothing, so this file is the memory of the project.
type Session struct {
	Messages []model.Message `json:"messages"`
	Mode     string          `json:"mode"`
	Model    string          `json:"model"`
}

// sessionPath is where a project's session lives, inside the working directory.
func sessionPath(workDir string) string {
	return filepath.Join(workDir, ".harness", "session.json")
}

// loadSession reads the saved session, or returns a fresh one defaulting to ask
// mode if none exists yet.
func loadSession(workDir string) *Session {
	raw, err := os.ReadFile(sessionPath(workDir))
	if err != nil {
		return &Session{Mode: "ask"}
	}
	var s Session
	if json.Unmarshal(raw, &s) != nil {
		return &Session{Mode: "ask"}
	}
	if s.Mode == "" {
		s.Mode = "ask"
	}
	return &s
}

// save writes the session to disk, creating the .harness directory as needed.
func (s *Session) save(workDir string) error {
	dir := filepath.Dir(sessionPath(workDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sessionPath(workDir), raw, 0o644)
}
