// Package projects is the unified code-project registry shared with the web
// backend and Telegram. It reads and writes the same projects.json in the global
// config dir, so a folder opened in the terminal shows up everywhere (and vice
// versa). Bare minimum per project: a stable id, absolute path, name, source,
// and timestamps.
package projects

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"harness/harness/appdir"
)

// Project is one registered code project. JSON field names match the Python side.
type Project struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Name       string `json:"name"`
	Source     string `json:"source"`
	CreatedAt  string `json:"created_at"`
	LastOpened string `json:"last_opened"`
}

func storePath() string { return filepath.Join(appdir.Dir(), "projects.json") }

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Load returns the registered projects (empty if the file is missing).
func Load() []Project {
	raw, err := os.ReadFile(storePath())
	if err != nil {
		return nil
	}
	var out []Project
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

func save(items []Project) error {
	dir := appdir.Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	tmp := storePath() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, storePath()) // atomic; a concurrent reader never sees a half file
}

// Upsert registers (or refreshes) a project by absolute path. The id is stable
// per path, so the terminal and the web/Telegram sides converge on one record.
func Upsert(path, source string) (Project, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Project{}, err
	}
	if resolved, e := filepath.EvalSymlinks(abs); e == nil {
		abs = resolved
	}
	items := Load()
	for i := range items {
		if items[i].Path == abs {
			items[i].LastOpened = now()
			return items[i], save(items)
		}
	}
	p := Project{
		ID:         newID(),
		Path:       abs,
		Name:       filepath.Base(abs),
		Source:     source,
		CreatedAt:  now(),
		LastOpened: now(),
	}
	items = append(items, p)
	return p, save(items)
}
