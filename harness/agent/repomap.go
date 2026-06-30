package agent

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ignoredDirs are directories left out of the repo map, matching what the
// discovery tools skip.
var ignoredDirs = map[string]bool{
	".git": true, "node_modules": true, ".venv": true, "venv": true,
	"__pycache__": true, "dist": true, "build": true, "target": true,
	".harness": true,
}

const (
	maxMapFiles        = 150
	maxMapBytes        = 6000
	maxSymbolsPerFile  = 25
	maxFileReadForMap  = 512 * 1024
	maxSymbolLineChars = 100
)

// Line patterns that name a top-level definition, per file extension. This is a
// cheap, dependency-free stand-in for tree-sitter: it catches the common
// declaration forms in each language, which is enough for the model to see what
// lives where without a parser.
var (
	pyPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*(async\s+)?def\s+\w+`),
		regexp.MustCompile(`^\s*class\s+\w+`),
	}
	goPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^func\s`),
		regexp.MustCompile(`^type\s+\w+`),
	}
	jsPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*(export\s+)?(default\s+)?(async\s+)?function\s+\w+`),
		regexp.MustCompile(`^\s*(export\s+)?(abstract\s+)?class\s+\w+`),
		regexp.MustCompile(`^\s*(export\s+)?const\s+\w+\s*=\s*(async\s*)?\(`),
		regexp.MustCompile(`^\s*(export\s+)?const\s+\w+\s*=\s*(async\s+)?function`),
	}
	rsPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*(pub\s+)?(async\s+)?fn\s+\w+`),
		regexp.MustCompile(`^\s*(pub\s+)?(struct|enum|trait)\s+\w+`),
		regexp.MustCompile(`^\s*impl\s`),
	}
	javaPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*(public|private|protected).*\b(class|interface|enum)\b`),
		regexp.MustCompile(`^\s*(public|private|protected)\s+[\w<>\[\].]+\s+\w+\s*\(`),
	}
)

var symbolPatterns = map[string][]*regexp.Regexp{
	".py":   pyPatterns,
	".go":   goPatterns,
	".js":   jsPatterns,
	".jsx":  jsPatterns,
	".mjs":  jsPatterns,
	".ts":   jsPatterns,
	".tsx":  jsPatterns,
	".rs":   rsPatterns,
	".java": javaPatterns,
}

// buildRepoMap returns a compact map of the project: each file path followed by
// its top-level definitions, so the model can often guess where code lives and
// confirm with a read instead of searching. It is bounded so a large repo does
// not blow up the prompt.
func buildRepoMap(workDir string) string {
	var b strings.Builder
	files := 0
	truncated := false

	_ = filepath.WalkDir(workDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != workDir && (ignoredDirs[name] || strings.HasPrefix(name, ".")) {
				return fs.SkipDir
			}
			return nil
		}
		if files >= maxMapFiles || b.Len() >= maxMapBytes {
			truncated = true
			return fs.SkipAll
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		rel, relErr := filepath.Rel(workDir, path)
		if relErr != nil {
			return nil
		}
		b.WriteString(rel)
		b.WriteByte('\n')
		for _, s := range symbolsFor(path) {
			b.WriteString("  " + s + "\n")
		}
		files++
		return nil
	})

	out := strings.TrimRight(b.String(), "\n")
	if truncated {
		out += "\n... (map truncated)"
	}
	return out
}

// symbolsFor reads a code file and returns its top-level definition lines, or
// nil for non-code or oversized files (which appear as a path only).
func symbolsFor(path string) []string {
	pats := symbolPatterns[strings.ToLower(filepath.Ext(path))]
	if pats == nil {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxFileReadForMap {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		if len(out) >= maxSymbolsPerFile {
			break
		}
		for _, p := range pats {
			if p.MatchString(line) {
				out = append(out, truncate(strings.TrimRight(strings.TrimSpace(line), "{"), maxSymbolLineChars))
				break
			}
		}
	}
	return out
}
