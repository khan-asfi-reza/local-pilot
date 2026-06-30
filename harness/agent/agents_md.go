package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// discoverAgentsMD walks from the git root down to the working directory and
// combines every AGENTS.md on the path, root first and nearest last, so the
// nearest file overrides. If there is no git root, only the working directory is
// considered.
func discoverAgentsMD(workDir string) string {
	abs, err := filepath.Abs(workDir)
	if err != nil {
		return ""
	}
	root := findGitRoot(abs)

	var dirs []string
	if root == "" {
		dirs = []string{abs}
	} else {
		dirs = append(dirs, root)
		if rel, err := filepath.Rel(root, abs); err == nil && rel != "." {
			cur := root
			for _, part := range strings.Split(rel, string(filepath.Separator)) {
				cur = filepath.Join(cur, part)
				dirs = append(dirs, cur)
			}
		}
	}

	var parts []string
	for _, d := range dirs {
		if raw, err := os.ReadFile(filepath.Join(d, "AGENTS.md")); err == nil {
			parts = append(parts, strings.TrimSpace(string(raw)))
		}
	}
	return strings.Join(parts, "\n\n")
}

// findGitRoot walks up from a directory looking for a .git entry, returning the
// directory that holds it, or empty if none is found.
func findGitRoot(dir string) string {
	cur := dir
	for {
		if _, err := os.Stat(filepath.Join(cur, ".git")); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}
