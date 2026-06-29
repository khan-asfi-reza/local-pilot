package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"harness/harness/events"
)

// ignoredDirs are directories the discovery tools skip so the model does not
// wade through build output and version control internals.
var ignoredDirs = map[string]bool{
	".git": true, "node_modules": true, ".venv": true, "venv": true,
	"__pycache__": true, "dist": true, "build": true, "target": true,
	".harness": true,
}

// safePath resolves a caller-supplied path against the working directory and
// refuses anything that would escape it. This is the file-access boundary.
func safePath(workDir, rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("path is empty")
	}
	clean := filepath.Clean(rel)
	abs := clean
	if !filepath.IsAbs(clean) {
		abs = filepath.Join(workDir, clean)
	}
	wd, err := filepath.Abs(workDir)
	if err != nil {
		return "", err
	}
	ap, err := filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	relToWd, err := filepath.Rel(wd, ap)
	if err != nil || relToWd == ".." || strings.HasPrefix(relToWd, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the working directory", rel)
	}
	return ap, nil
}

// searchTool locates text across files with ripgrep.
func searchTool() *Tool {
	return &Tool{
		Name:        "search",
		Description: "Search the project for a text pattern across files, like ripgrep. Use to locate code, a symbol, or a string when you do not know which file it is in. Returns matching lines with file path and line number.",
		Params:      json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Text or regex to search for."},"path":{"type":"string","description":"Optional subdirectory to limit the search to."},"glob":{"type":"string","description":"Optional file filter, e.g. *.py."},"max_results":{"type":"integer","description":"Cap on matches returned.","default":50}},"required":["query"]}`),
		Run: func(env Env, args Args) (any, *events.Diff, error) {
			query := args.Str("query")
			if query == "" {
				return nil, nil, fmt.Errorf("query is required")
			}
			max := args.Int("max_results", 50)
			target := args.StrOr("path", ".")
			glob := args.Str("glob")

			out, code, err := runRipgrep(env, query, target, glob, false)
			if err != nil {
				return nil, nil, fmt.Errorf("ripgrep: %w", err)
			}
			// Exit 2 means the query was not a valid regex. Retry it as a literal
			// string, which is what a plain text search usually wants anyway.
			if code == 2 {
				out, code, err = runRipgrep(env, query, target, glob, true)
				if err != nil {
					return nil, nil, fmt.Errorf("ripgrep: %w", err)
				}
			}
			// Exit 1 (no matches) or a residual error is treated as no matches, not
			// a failure, so the model gets a clean empty result to reason over.
			if code != 0 {
				return map[string]any{"matches": []any{}, "truncated": false}, nil, nil
			}
			type match struct {
				Path string `json:"path"`
				Line int    `json:"line"`
				Text string `json:"text"`
			}
			var matches []match
			truncated := false
			for _, ln := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
				if ln == "" {
					continue
				}
				if len(matches) >= max {
					truncated = true
					break
				}
				path, lineNo, text := splitRgLine(ln)
				matches = append(matches, match{Path: path, Line: lineNo, Text: text})
			}
			return map[string]any{"matches": matches, "truncated": truncated}, nil, nil
		},
	}
}

// runRipgrep runs ripgrep and returns its output and exit code. fixed uses
// literal-string matching instead of regex. A non-ExitError (rg missing, killed)
// is returned as an error.
func runRipgrep(env Env, query, target, glob string, fixed bool) ([]byte, int, error) {
	args := []string{"--line-number", "--no-heading", "--color", "never"}
	if fixed {
		args = append(args, "--fixed-strings")
	}
	if glob != "" {
		args = append(args, "--glob", glob)
	}
	args = append(args, "--", query, target)
	cmd := exec.CommandContext(env.Ctx, "rg", args...)
	cmd.Dir = env.WorkDir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return out, ee.ExitCode(), nil
		}
		return nil, -1, err
	}
	return out, 0, nil
}

// splitRgLine parses a ripgrep line of the form path:line:text.
func splitRgLine(ln string) (path string, lineNo int, text string) {
	first := strings.IndexByte(ln, ':')
	if first < 0 {
		return ln, 0, ""
	}
	rest := ln[first+1:]
	second := strings.IndexByte(rest, ':')
	if second < 0 {
		return ln[:first], 0, rest
	}
	n, _ := strconv.Atoi(rest[:second])
	return ln[:first], n, rest[second+1:]
}

// listDirTool shows the structure of a directory without reading file contents.
func listDirTool() *Tool {
	return &Tool{
		Name:        "list_dir",
		Description: "List the files and folders in a directory to explore project structure. Does not read file contents.",
		Params:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Directory to list.","default":"."},"depth":{"type":"integer","description":"How many levels deep.","default":1}},"required":[]}`),
		Run: func(env Env, args Args) (any, *events.Diff, error) {
			rel := args.StrOr("path", ".")
			depth := args.Int("depth", 1)
			if depth < 1 {
				depth = 1
			}
			root, err := safePath(env.WorkDir, rel)
			if err != nil {
				return nil, nil, err
			}
			type entry struct {
				Name string `json:"name"`
				Type string `json:"type"`
			}
			var entries []entry
			var walk func(dir string, level int, prefix string) error
			walk = func(dir string, level int, prefix string) error {
				items, err := os.ReadDir(dir)
				if err != nil {
					return err
				}
				for _, it := range items {
					if it.IsDir() && ignoredDirs[it.Name()] {
						continue
					}
					name := prefix + it.Name()
					kind := "file"
					if it.IsDir() {
						kind = "dir"
					}
					entries = append(entries, entry{Name: name, Type: kind})
					if it.IsDir() && level < depth {
						if err := walk(filepath.Join(dir, it.Name()), level+1, name+"/"); err != nil {
							return err
						}
					}
				}
				return nil
			}
			if err := walk(root, 1, ""); err != nil {
				return nil, nil, err
			}
			return map[string]any{"entries": entries}, nil, nil
		},
	}
}

const maxReadBytes = 200_000

// readFileTool reads a file, optionally a line range, and truncates very large
// files from the tail.
func readFileTool() *Tool {
	return &Tool{
		Name:        "read_file",
		Description: "Read a file's contents. Read a file before editing it. Locate the file with search or list_dir first if you do not know the path. Optionally read only a line range.",
		Params:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File to read."},"start_line":{"type":"integer","description":"Optional first line (1-based)."},"end_line":{"type":"integer","description":"Optional last line (inclusive)."}},"required":["path"]}`),
		Run: func(env Env, args Args) (any, *events.Diff, error) {
			p, err := safePath(env.WorkDir, args.Str("path"))
			if err != nil {
				return nil, nil, err
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return nil, nil, err
			}
			markSeen(env, p)
			content := string(raw)
			start := args.Int("start_line", 0)
			end := args.Int("end_line", 0)
			if start > 0 || end > 0 {
				lines := splitLines(content)
				if start < 1 {
					start = 1
				}
				if end < 1 || end > len(lines) {
					end = len(lines)
				}
				if start > len(lines) {
					start = len(lines)
				}
				content = strings.Join(lines[start-1:end], "\n")
			}
			truncated := false
			if len(content) > maxReadBytes {
				content = content[:maxReadBytes]
				truncated = true
			}
			return map[string]any{"path": args.Str("path"), "content": content, "truncated": truncated}, nil, nil
		},
	}
}

// writeFileTool creates a new file or fully replaces one, and reports the change
// as an all-added (or replacement) diff.
func writeFileTool() *Tool {
	return &Tool{
		Name:        "write_file",
		Description: "Create a NEW file (or fully replace a tiny one) with the given content, which becomes the entire file. To change part of an existing file, use edit_file instead; it is far cheaper than rewriting the whole file. The path must be a full file path with filename and extension, for example backend/main.py, never a bare folder name. Parent folders are created for you. Mutating: in ask mode this pauses for approval.",
		Params:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File to write."},"content":{"type":"string","description":"Full content of the file."}},"required":["path","content"]}`),
		Mutating:    true,
		Preview: func(env Env, args Args) (string, *events.Diff, error) {
			p, err := safePath(env.WorkDir, args.Str("path"))
			if err != nil {
				return "", nil, err
			}
			old := readIfExists(p)
			diff := buildDiff(args.Str("path"), old, args.Str("content"))
			return fmt.Sprintf("write %s (+%d -%d lines)", args.Str("path"), diff.Added, diff.Removed), diff, nil
		},
		Run: func(env Env, args Args) (any, *events.Diff, error) {
			p, err := safePath(env.WorkDir, args.Str("path"))
			if err != nil {
				return nil, nil, err
			}
			if err := requireSeen(env, p, args.Str("path")); err != nil {
				return nil, nil, err
			}
			old := readIfExists(p)
			content := args.Str("content")
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return nil, nil, parentDirError(args.Str("path"), err)
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				return nil, nil, err
			}
			markSeen(env, p)
			diff := buildDiff(args.Str("path"), old, content)
			return map[string]any{"ok": true, "path": args.Str("path"), "bytes_written": len(content)}, diff, nil
		},
	}
}

// editSpec is one anchor edit: replace old_text with new_text.
type editSpec struct {
	OldText string
	NewText string
}

// editFileTool makes anchor-based edits and returns a positional diff. The model
// never gives line numbers; the harness resolves each anchor to exact positions.
func editFileTool() *Tool {
	return &Tool{
		Name:        "edit_file",
		Description: "Make targeted edits to an existing file by anchor text, not by line number. Each edit gives the exact existing text to replace (old_text) and the new text (new_text). old_text must appear exactly once in the file; include a few surrounding lines to make it unique. To insert, set old_text to a nearby unique line and put that line plus the new lines in new_text. To delete, set new_text to empty. Mutating: in ask mode this pauses for approval and shows the diff first.",
		Params:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File to edit."},"edits":{"type":"array","description":"One or more edits applied in order.","items":{"type":"object","properties":{"old_text":{"type":"string","description":"Exact existing text to replace, unique in the file."},"new_text":{"type":"string","description":"Replacement text. Empty string deletes."}},"required":["old_text","new_text"]}}},"required":["path","edits"]}`),
		Mutating:    true,
		Preview: func(env Env, args Args) (string, *events.Diff, error) {
			p, err := safePath(env.WorkDir, args.Str("path"))
			if err != nil {
				return "", nil, err
			}
			old, err := os.ReadFile(p)
			if err != nil {
				return "", nil, err
			}
			edits, err := parseEdits(args)
			if err != nil {
				return "", nil, err
			}
			updated, err := applyEdits(string(old), edits)
			if err != nil {
				return "", nil, err
			}
			diff := buildDiff(args.Str("path"), string(old), updated)
			return fmt.Sprintf("edit %s (+%d -%d lines)", args.Str("path"), diff.Added, diff.Removed), diff, nil
		},
		Run: func(env Env, args Args) (any, *events.Diff, error) {
			p, err := safePath(env.WorkDir, args.Str("path"))
			if err != nil {
				return nil, nil, err
			}
			if err := requireSeen(env, p, args.Str("path")); err != nil {
				return nil, nil, err
			}
			old, err := os.ReadFile(p)
			if err != nil {
				return nil, nil, err
			}
			edits, err := parseEdits(args)
			if err != nil {
				return nil, nil, err
			}
			updated, err := applyEdits(string(old), edits)
			if err != nil {
				return nil, nil, err
			}
			if err := os.WriteFile(p, []byte(updated), 0o644); err != nil {
				return nil, nil, err
			}
			markSeen(env, p)
			diff := buildDiff(args.Str("path"), string(old), updated)
			return map[string]any{
				"ok": true, "path": args.Str("path"), "applied": len(edits),
				"added": diff.Added, "removed": diff.Removed, "diff": diff,
			}, diff, nil
		},
	}
}

// parseEdits pulls the edits array out of the tool arguments.
func parseEdits(args Args) ([]editSpec, error) {
	rawEdits, ok := args["edits"].([]any)
	if !ok || len(rawEdits) == 0 {
		return nil, fmt.Errorf("edits is required and must be a non-empty array")
	}
	var edits []editSpec
	for _, r := range rawEdits {
		m, ok := r.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("each edit must be an object with old_text and new_text")
		}
		ot, _ := m["old_text"].(string)
		nt, _ := m["new_text"].(string)
		edits = append(edits, editSpec{OldText: ot, NewText: nt})
	}
	return edits, nil
}

// applyEdits resolves each anchor and applies it. It first tries an exact
// unique match; if that fails it retries with a whitespace-flexible match
// (comparing lines with leading/trailing whitespace ignored), which tolerates
// the indentation and trailing-space differences small models commonly get
// wrong. A match that is missing or ambiguous is an error and nothing is
// written, so an edit is always safe.
func applyEdits(content string, edits []editSpec) (string, error) {
	for i, e := range edits {
		if e.OldText == "" {
			return "", fmt.Errorf("edit %d: old_text is empty; to insert, anchor on a nearby unique line", i+1)
		}
		switch strings.Count(content, e.OldText) {
		case 1:
			content = strings.Replace(content, e.OldText, e.NewText, 1)
			continue
		case 0:
			// fall through to flexible matching
		default:
			return "", fmt.Errorf("edit %d: old_text matches more than once; include more surrounding context to make it unique", i+1)
		}
		start, end, n := flexibleFind(content, e.OldText)
		if n == 1 {
			// The match ignored whitespace, so re-indent new_text to the file's
			// real indentation instead of the model's, keeping the code valid.
			fileIndent := leadingWS(firstLineText(content[start:end]))
			modelIndent := leadingWS(firstLineText(e.OldText))
			content = content[:start] + reindent(e.NewText, modelIndent, fileIndent) + content[end:]
			continue
		}
		if n > 1 {
			return "", fmt.Errorf("edit %d: old_text matches more than once; include more surrounding context to make it unique", i+1)
		}
		return "", fmt.Errorf("edit %d: old_text not found. Copy the lines exactly from the file (indentation may differ, but the text must match).", i+1)
	}
	return content, nil
}

// flexibleFind locates old within content comparing line by line with leading
// and trailing whitespace ignored. It returns the byte span of the matched
// region in the original content and how many times it matched.
func flexibleFind(content, old string) (start, end, count int) {
	type span struct{ start, end int }
	// Index the content lines with their byte offsets.
	var lines []span
	off := 0
	for _, ln := range strings.SplitAfter(content, "\n") {
		text := strings.TrimSuffix(ln, "\n")
		lines = append(lines, span{off, off + len(text)})
		off += len(ln)
	}
	want := strings.Split(strings.Trim(old, "\n"), "\n")
	for i := range want {
		want[i] = strings.TrimSpace(want[i])
	}
	if len(want) == 0 {
		return 0, 0, 0
	}
	for i := 0; i+len(want) <= len(lines); i++ {
		ok := true
		for j := 0; j < len(want); j++ {
			lineText := content[lines[i+j].start:lines[i+j].end]
			if strings.TrimSpace(lineText) != want[j] {
				ok = false
				break
			}
		}
		if ok {
			count++
			start = lines[i].start
			end = lines[i+len(want)-1].end
		}
	}
	return start, end, count
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + "..."
	}
	return s
}

// firstLineText returns the first line of s, without any marker.
func firstLineText(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// leadingWS returns the leading spaces and tabs of s.
func leadingWS(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
}

// reindent shifts each line of text so a leading fromIndent becomes toIndent,
// preserving any deeper indentation. Lines without the fromIndent prefix are
// left as-is apart from empty lines.
func reindent(text, fromIndent, toIndent string) string {
	if fromIndent == toIndent {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		if fromIndent != "" && strings.HasPrefix(ln, fromIndent) {
			lines[i] = toIndent + ln[len(fromIndent):]
		} else {
			lines[i] = toIndent + ln
		}
	}
	return strings.Join(lines, "\n")
}

// parentDirError turns an opaque mkdir failure into an actionable message when a
// parent path component is a file rather than a directory, which happens when an
// earlier write used a bare folder name.
func parentDirError(path string, err error) error {
	if strings.Contains(err.Error(), "not a directory") {
		return fmt.Errorf("cannot create %s because a parent path already exists as a FILE, not a folder. An earlier write likely used a bare name. Remove that file with shell_run, then write to a full path like dir/file.ext", path)
	}
	return err
}

// markSeen records that the model has read or written a file this turn.
func markSeen(env Env, absPath string) {
	if env.Seen != nil {
		env.Seen[absPath] = true
	}
}

// requireSeen forces read-before-modify: if a file already exists and the model
// has not looked at it this turn, it must read it first. This stops the model
// from editing based on a guess of the contents (the blind-edit loop). New files
// are exempt.
func requireSeen(env Env, absPath, relPath string) error {
	if env.Seen == nil {
		return nil
	}
	if _, err := os.Stat(absPath); err != nil {
		return nil // new file, nothing to read
	}
	if env.Seen[absPath] {
		return nil
	}
	return fmt.Errorf("Read the file first. Read %s with read_file before you change it, so your change matches its real contents.", relPath)
}

func readIfExists(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(raw)
}

// truncateMiddle keeps the head and tail of long output and drops the middle,
// where errors are least likely to be.
func truncateMiddle(s string, max int) string {
	if len(s) <= max {
		return s
	}
	half := max / 2
	var b bytes.Buffer
	b.WriteString(s[:half])
	b.WriteString("\n... [truncated] ...\n")
	b.WriteString(s[len(s)-half:])
	return b.String()
}
