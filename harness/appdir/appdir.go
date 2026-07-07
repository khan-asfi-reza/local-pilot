// Package appdir resolves the global local-pilot data directory and seeds it with
// the embedded defaults, so pilot can run from anywhere and keep its config and
// skills in one user-owned place.
package appdir

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	root "harness"
)

// Dir returns the per-OS data directory. macOS uses ~/.localpilot, Linux uses
// $XDG_DATA_HOME or ~/.local/share/localpilot, Windows uses %LOCALAPPDATA%.
func Dir() string {
	switch runtime.GOOS {
	case "windows":
		if b := os.Getenv("LOCALAPPDATA"); b != "" {
			return filepath.Join(b, "localpilot")
		}
		if b := os.Getenv("APPDATA"); b != "" {
			return filepath.Join(b, "localpilot")
		}
	case "darwin":
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, ".localpilot")
		}
	default:
		if b := os.Getenv("XDG_DATA_HOME"); b != "" {
			return filepath.Join(b, "localpilot")
		}
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, ".local", "share", "localpilot")
		}
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".localpilot")
}

// Ensure creates the data dir and seeds models/models.json, models/prompt.json,
// and skills/ from the embedded defaults where they are missing. It mirrors the
// repo layout so the config, prompt, and skill loaders resolve unchanged. It is
// idempotent and never overwrites an existing file. Returns the models.json path.
func Ensure() (string, error) {
	dir := Dir()
	if err := seedFile(dir, "models/models.json"); err != nil {
		return "", err
	}
	if err := seedFile(dir, "models/prompt.json"); err != nil {
		return "", err
	}
	if err := seedTree(dir, "skills"); err != nil {
		return "", err
	}
	return filepath.Join(dir, "models", "models.json"), nil
}

// ConfigPath returns the models.json path without seeding.
func ConfigPath() string { return filepath.Join(Dir(), "models", "models.json") }

// seedFile writes one embedded file to dir/rel if it does not already exist.
func seedFile(dir, rel string) error {
	dst := filepath.Join(dir, filepath.FromSlash(rel))
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	data, err := root.Defaults.ReadFile(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// seedTree copies an embedded directory subtree into dir, file by file.
func seedTree(dir, rel string) error {
	return fs.WalkDir(root.Defaults, rel, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		return seedFile(dir, p)
	})
}
