package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
)

type OSFiles struct{}

func (OSFiles) Exists(workDir, rel string) bool {
	_, err := os.Stat(filepath.Join(workDir, strings.TrimSpace(rel)))
	return err == nil
}

// verifyStructural splits a sub-task's targets into those on disk and those missing.
func verifyStructural(fc FileChecker, workDir string, t SubTask) (written, missing []string) {
	for _, f := range t.TargetFiles {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if fc.Exists(workDir, f) {
			written = append(written, f)
		} else {
			missing = append(missing, f)
		}
	}
	return written, missing
}
