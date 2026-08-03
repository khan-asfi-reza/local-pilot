package graph

import (
	"embed"
	"path/filepath"
	"strings"
)

//go:embed queries
var queriesFS embed.FS

// extLang maps a file extension to a grammar language name.
var extLang = map[string]string{
	".go":   "go",
	".py":   "python",
	".js":   "javascript",
	".jsx":  "javascript",
	".mjs":  "javascript",
	".cjs":  "javascript",
	".ts":   "typescript",
	".tsx":  "tsx",
	".rs":   "rust",
	".java": "java",
	".rb":   "ruby",
	".c":    "c",
	".h":    "c",
	".cpp":  "cpp",
	".cc":   "cpp",
	".cxx":  "cpp",
	".hpp":  "cpp",
}

// langForPath returns the grammar language for a path, or "" if unsupported.
func langForPath(path string) string {
	return extLang[strings.ToLower(filepath.Ext(path))]
}

// queryFileByLang names the embedded tag-query file for a language.
var queryFileByLang = map[string]string{
	"go":         "go-tags.scm",
	"python":     "python-tags.scm",
	"javascript": "javascript-tags.scm",
	"typescript": "typescript-tags.scm",
	"tsx":        "typescript-tags.scm",
	"rust":       "rust-tags.scm",
	"java":       "java-tags.scm",
	"ruby":       "ruby-tags.scm",
	"c":          "c-tags.scm",
	"cpp":        "cpp-tags.scm",
}

// queryFor returns the tag-query source for a language, or "".
func queryFor(lang string) string {
	name := queryFileByLang[lang]
	if name == "" {
		return ""
	}
	b, err := queriesFS.ReadFile("queries/" + name)
	if err != nil {
		return ""
	}
	return string(b)
}
