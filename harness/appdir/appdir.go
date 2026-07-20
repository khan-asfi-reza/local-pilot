// Package appdir resolves the global local-pilot data directory and seeds it with
// the embedded defaults, so pilot can run from anywhere and keep its config and
// skills in one user-owned place.
package appdir

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"

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

// Ensure creates the data dir and populates it from the embedded defaults,
// mirroring the repo layout so the config and skill loaders resolve unchanged.
// The local dir holds only config and model info: models.json is user state,
// seeded only if missing and never clobbered. The system prompt is NOT placed
// here — it is loaded from the binary's embedded models/prompt.json. Skills are
// shipped content, refreshed whenever the embedded version changes so fixes
// propagate on upgrade. Returns models.json.
func Ensure() (string, error) {
	dir := Dir()
	if err := place(dir, "models/models.json", false); err != nil {
		return "", err
	}

	want := embeddedVersion()
	verPath := filepath.Join(dir, ".defaults-version")
	have, _ := os.ReadFile(verPath)
	refresh := string(have) != want

	if err := placeTree(dir, "skills", refresh); err != nil {
		return "", err
	}
	if refresh {
		_ = os.WriteFile(verPath, []byte(want), 0o644)
	}
	return filepath.Join(dir, "models", "models.json"), nil
}

// ConfigPath returns the models.json path without seeding.
func ConfigPath() string { return filepath.Join(Dir(), "models", "models.json") }

// place writes an embedded file to dir/rel. When overwrite is false it is kept
// if it already exists (used for user state); when true it is always rewritten.
func place(dir, rel string, overwrite bool) error {
	dst := filepath.Join(dir, filepath.FromSlash(rel))
	if !overwrite {
		if _, err := os.Stat(dst); err == nil {
			return nil
		}
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

// placeTree copies an embedded directory subtree into dir, file by file.
func placeTree(dir, rel string, overwrite bool) error {
	return fs.WalkDir(root.Defaults, rel, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		return place(dir, p, overwrite)
	})
}

// embeddedVersion is a content hash of the shipped prompt and skills, so any edit
// to them changes the version and triggers a refresh of the data dir on upgrade.
func embeddedVersion() string {
	var files []string
	files = append(files, "models/prompt.json")
	_ = fs.WalkDir(root.Defaults, "skills", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	h := sha256.New()
	for _, f := range files {
		b, _ := root.Defaults.ReadFile(f)
		h.Write([]byte(f))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
