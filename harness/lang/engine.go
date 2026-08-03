package lang

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"harness/harness/events"
)

//go:embed templates
var templatesFS embed.FS

// Nest is how a generator writes relative to the project root.
type Nest int

const (
	// NestDot: the generator writes into the current dir (django-admin startproject
	// config .), touching only its own files — run directly in workDir.
	NestDot Nest = iota
	// NestTempMove: the generator forces a ./<name>/ subdir (create-next-app), so
	// generate into an isolated temp subdir and merge up on success only.
	NestTempMove
)

// Cmd is one argv invocation (bin + templated args), run with no shell.
type Cmd struct {
	Bin  string
	Args []string
}

// Post is one ordered step after generate: render a template file OR run a
// command, applied only when its When guard holds.
type Post struct {
	Render string // template path under templates/ (rendered with vars)
	To     string // destination under workDir (templated); required with Render
	Append bool   // append to an existing (generator-produced) file
	Run    *Cmd   // OR: run a command
	When   string // "", "has:redis", "kw:celery|task", "has:redis&kw:celery"
}

// Recipe is one framework's deterministic scaffold. Generator/library versions
// are pinned in the argv for determinism; everything not covered falls back to
// the model path, so a missing/stale recipe degrades, never breaks.
type Recipe struct {
	Framework string
	Requires  []string      // binaries needed to run this recipe
	Install   []Cmd         // prep before generate (venv, pinned installs)
	Generate  *Cmd          // the real generator; nil = template-only framework
	Nest      Nest          //
	Verify    string        // file that must exist after Generate (else fallback)
	Post      []Post        // ordered post steps
	Stack     string        // human label for state.md
	Entry     string        // run command
	Project   string        // default canonical names
	App       string        //
	Settings  string        //
	Layout    []string      // canonical files to advertise (filtered to on-disk)
	Timeout   time.Duration // 0 → default
}

const defaultTimeout = 300 * time.Second

// run executes a recipe end-to-end: toolchain gate, install, generate (with nest
// handling + verify), post steps, then reports the canonical names and the files
// actually on disk. Any failure returns an error so the caller falls back to the
// model path; partial output in a temp dir is always cleaned up.
func (rec Recipe) run(ctx context.Context, r Req) (Result, error) {
	emit := r.Emit
	if emit == nil {
		emit = func(events.Event) {}
	}
	for _, bin := range rec.Requires {
		if !haveAll(bin) {
			return Result{}, fmt.Errorf("%s needs %q (not installed)", rec.Framework, bin)
		}
	}
	timeout := rec.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	data := rec.vars(r)

	// tempmove isolates a nesting generator so a partial/failed run never pollutes
	// workDir; dot generators write straight into workDir.
	genDir := r.WorkDir
	if rec.Generate != nil && rec.Nest == NestTempMove {
		genDir = filepath.Join(r.WorkDir, ".harness_scaffold")
		_ = os.RemoveAll(genDir)
		if err := os.MkdirAll(genDir, 0o755); err != nil {
			return Result{}, err
		}
		defer os.RemoveAll(genDir)
	}

	for _, c := range rec.Install {
		if err := runCmd(ctx, genDir, c.Bin, tmplArgs(c.Args, data)); err != nil {
			return Result{}, err
		}
	}

	if rec.Generate != nil {
		if err := runCmd(ctx, genDir, rec.Generate.Bin, tmplArgs(rec.Generate.Args, data)); err != nil {
			return Result{}, err
		}
		if rec.Verify != "" {
			if _, err := os.Stat(filepath.Join(genDir, rec.Verify)); err != nil {
				return Result{}, fmt.Errorf("generator produced no %s (layout drift)", rec.Verify)
			}
		}
		if genDir != r.WorkDir {
			if err := mergeUp(genDir, r.WorkDir); err != nil {
				return Result{}, err
			}
		}
	}

	for _, p := range rec.Post {
		if !whenMatches(p.When, data, r.Prompt) {
			continue
		}
		if err := rec.runPost(ctx, p, data, r.WorkDir); err != nil {
			return Result{}, err
		}
	}

	res := Result{
		Framework: rec.Framework,
		Stack:     rec.Stack,
		Project:   tmpl(rec.Project, data),
		App:       tmpl(rec.App, data),
		Settings:  tmpl(rec.Settings, data),
		Entry:     rec.Entry,
	}
	for _, f := range rec.Layout {
		rel := tmpl(f, data)
		if _, err := os.Stat(filepath.Join(r.WorkDir, rel)); err == nil {
			res.Layout = append(res.Layout, rel)
		}
	}
	return res, nil
}

// runPost renders a template file or runs a command for one post step.
func (rec Recipe) runPost(ctx context.Context, p Post, data map[string]any, workDir string) error {
	if p.Run != nil {
		return runCmd(ctx, workDir, p.Run.Bin, tmplArgs(p.Run.Args, data))
	}
	if p.Render == "" {
		return nil
	}
	body, err := renderTemplate(p.Render, data)
	if err != nil {
		return err
	}
	dst := filepath.Join(workDir, tmpl(p.To, data))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if p.Append {
		f, err := os.OpenFile(dst, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.WriteString("\n" + body + "\n")
		return err
	}
	return os.WriteFile(dst, []byte(body), 0o644)
}

// vars builds the text/template data: canonical names (recipe defaults, overridden
// by Req.Vars) and has_<service> flags parsed from the provisioned .env.
func (rec Recipe) vars(r Req) map[string]any {
	get := func(k, def string) string {
		if v := r.Vars[k]; v != "" {
			return v
		}
		return def
	}
	return map[string]any{
		"project":      get("project", rec.Project),
		"app":          get("app", rec.App),
		"settings":     get("settings", rec.Settings),
		"module":       get("module", firstNonEmpty(get("project", rec.Project), "app")),
		"has_postgres": strings.Contains(r.Env, "DATABASE_URL=postgres"),
		"has_mysql":    strings.Contains(r.Env, "DATABASE_URL=mysql"),
		"has_redis":    strings.Contains(r.Env, "REDIS_URL="),
		"has_mongo":    strings.Contains(r.Env, "MONGODB_URI="),
	}
}

// whenMatches evaluates a post-step guard: "" always applies; tokens joined by
// "&" must all hold; a token is has:<svc> (a provisioned service) or
// kw:<a|b|c> (the prompt matches any alternative, case-insensitive).
func whenMatches(when string, data map[string]any, prompt string) bool {
	when = strings.TrimSpace(when)
	if when == "" {
		return true
	}
	low := strings.ToLower(prompt)
	for _, tok := range strings.Split(when, "&") {
		tok = strings.TrimSpace(tok)
		switch {
		case strings.HasPrefix(tok, "has:"):
			if v, _ := data["has_"+strings.TrimPrefix(tok, "has:")].(bool); !v {
				return false
			}
		case strings.HasPrefix(tok, "kw:"):
			hit := false
			for _, alt := range strings.Split(strings.TrimPrefix(tok, "kw:"), "|") {
				if alt != "" && strings.Contains(low, strings.ToLower(alt)) {
					hit = true
					break
				}
			}
			if !hit {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// tmplArgs renders each argv element as a template.
func tmplArgs(args []string, data map[string]any) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = tmpl(a, data)
	}
	return out
}

// tmpl renders a single string template; on error it returns the input verbatim.
func tmpl(s string, data map[string]any) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	t, err := template.New("s").Parse(s)
	if err != nil {
		return s
	}
	var b bytes.Buffer
	if t.Execute(&b, data) != nil {
		return s
	}
	return b.String()
}

// renderTemplate renders an embedded template file by path under templates/.
func renderTemplate(name string, data map[string]any) (string, error) {
	raw, err := templatesFS.ReadFile(filepath.ToSlash(filepath.Join("templates", name)))
	if err != nil {
		return "", err
	}
	t, err := template.New(name).Parse(string(raw))
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

// mergeUp moves everything from src into dst, never overwriting an existing file
// (so the provisioned .env and .pilot survive a tempmove merge).
func mergeUp(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil || rel == "." {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		}
		if _, err := os.Stat(target); err == nil {
			return nil // never clobber provisioned files
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.Rename(path, target)
	})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
