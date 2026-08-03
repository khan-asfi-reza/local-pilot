package graph

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	jsImportRe = regexp.MustCompile(`(?m)(?:from\s+|import\s+|require\(\s*)['"]([^'"]+)['"]`)
	pyFromRe   = regexp.MustCompile(`(?m)^\s*from\s+([.\w]+)\s+import\b`)
	pyImportRe = regexp.MustCompile(`(?m)^\s*import\s+([.\w][.\w]*)`)
)

// importSpecs extracts raw import specifiers from a file's source, so resolution
// can run later (from persisted metadata) without re-reading the file.
func importSpecs(lang string, src []byte) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	text := string(src)
	switch lang {
	case "javascript", "typescript", "tsx":
		for _, m := range jsImportRe.FindAllStringSubmatch(text, -1) {
			if strings.HasPrefix(m[1], ".") { // repo-local only
				add(m[1])
			}
		}
	case "python":
		for _, m := range pyFromRe.FindAllStringSubmatch(text, -1) {
			add(m[1])
		}
		for _, m := range pyImportRe.FindAllStringSubmatch(text, -1) {
			add(m[1])
		}
	}
	return out
}

// resolveSpec maps one import specifier from file rel to a repo-relative file in
// the known set, or "" if it points outside the repo (stdlib/third-party).
func resolveSpec(lang, rel, spec string, known map[string]bool) string {
	switch lang {
	case "javascript", "typescript", "tsx":
		if !strings.HasPrefix(spec, ".") {
			return ""
		}
		base := filepath.Clean(filepath.Join(filepath.Dir(rel), spec))
		cands := []string{base, base + ".ts", base + ".tsx", base + ".js", base + ".jsx", base + ".mjs",
			filepath.Join(base, "index.ts"), filepath.Join(base, "index.tsx"),
			filepath.Join(base, "index.js"), filepath.Join(base, "index.jsx")}
		for _, c := range cands {
			if known[c] {
				return c
			}
		}
	case "python":
		mod := spec
		rels := 0
		for strings.HasPrefix(mod, ".") {
			rels++
			mod = mod[1:]
		}
		parts := strings.Split(mod, ".")
		var base string
		if rels > 0 {
			base = filepath.Dir(rel)
			for i := 1; i < rels; i++ {
				base = filepath.Dir(base)
			}
			base = filepath.Join(append([]string{base}, parts...)...)
		} else {
			base = filepath.Join(parts...)
		}
		for _, c := range []string{base + ".py", filepath.Join(base, "__init__.py")} {
			if known[c] {
				return c
			}
		}
	}
	return ""
}
