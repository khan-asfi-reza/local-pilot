// Package lang holds per-language handlers that deterministically initialize a
// project: detect its framework, run the real generator (or lay templates),
// install libraries, and wire the provisioned .env — all model-free, so the
// harness never spends tokens hand-writing boilerplate. It imports only leaf
// packages (events + stdlib), so both the orchestrator (scaffold step) and the
// tools package (install_deps) can use it without an import cycle.
package lang

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"harness/harness/events"
)

// Handler owns everything language-specific: which frameworks it recognizes, how
// to scaffold one, and how to install libraries into a project of that language.
type Handler interface {
	Lang() string            // "python", "javascript", "go", "c", "cpp", ...
	Toolchain() []string     // runtime binaries that must be on PATH to scaffold
	Frameworks() []Framework // frameworks this language handles (detection data)
	Scaffold(ctx context.Context, r Req) (Result, error)
	Install(ctx context.Context, workDir string, pkgs []string) error
}

// Framework is one detectable stack: a keyword regex + marker files + a priority
// so a specific meta-framework (next) beats the base library (react) on a tie.
type Framework struct {
	ID       string
	Keywords *regexp.Regexp
	Priority int
	Markers  []Marker
}

// Marker is an existing-file signal: the file's presence (or its containing a
// substring) means the project already uses this framework.
type Marker struct {
	File     string
	Contains string
}

// Req is the input to a scaffold: the chosen framework, where to build, the spec
// text (for keyword-gated add-ons), and the provisioned .env text so generated
// code reads DB/cache config from the running containers.
type Req struct {
	Framework string
	WorkDir   string
	Prompt    string
	Env       string
	Vars      map[string]string
	Emit      func(events.Event)
}

// Result is a successful scaffold's canonical names + the files actually created,
// which the caller maps into its own project-state representation.
type Result struct {
	Lang      string
	Framework string
	Stack     string
	Project   string
	App       string
	Settings  string
	Entry     string
	Layout    []string
}

// handlers is the registry, one per language. Order does not matter — Detect
// scores every framework across all handlers and picks the best.
var handlers = []Handler{
	python{}, javascript{}, golang{}, rust{}, java{}, ruby{}, php{}, clang{}, cpp{},
}

// Handlers returns the registered language handlers.
func Handlers() []Handler { return handlers }

// Detect picks the highest-scoring framework across all handlers: a marker file
// on disk beats a keyword in the prompt, and a higher framework priority wins a
// tie. Returns the owning handler, the framework id, and the score (score < 0
// means nothing matched, so the caller falls back to the model path).
func Detect(prompt, workDir string) (Handler, string, int) {
	low := strings.ToLower(prompt)
	var bestH Handler
	var bestFW string
	best := -1
	for _, h := range handlers {
		for _, fw := range h.Frameworks() {
			score := -1
			if anyMarker(workDir, fw.Markers) {
				score = fw.Priority + 1000 // a file on disk is the strongest signal
			} else if fw.Keywords != nil && fw.Keywords.MatchString(low) {
				score = fw.Priority
			}
			if score > best {
				best, bestH, bestFW = score, h, fw.ID
			}
		}
	}
	return bestH, bestFW, best
}

// HandlerFor returns the handler whose language matches, or nil.
func HandlerFor(language string) Handler {
	for _, h := range handlers {
		if h.Lang() == language {
			return h
		}
	}
	return nil
}

// HandlerForFramework returns the handler that owns a framework id, or nil.
func HandlerForFramework(framework string) Handler {
	for _, h := range handlers {
		for _, fw := range h.Frameworks() {
			if fw.ID == framework {
				return h
			}
		}
	}
	return nil
}

// StackPlan is what to scaffold: a single framework (one of Backend/Frontend set),
// or a full-stack split (both set → backend/ + frontend/ subdirs).
type StackPlan struct {
	Backend  string
	Frontend string
}

// FullStack reports whether both sides are set.
func (p StackPlan) FullStack() bool { return p.Backend != "" && p.Frontend != "" }

// Empty reports whether nothing was detected.
func (p StackPlan) Empty() bool { return p.Backend == "" && p.Frontend == "" }

var frontendIDs = map[string]bool{"nextjs": true, "react": true}

// namedBackendIDs are the frameworks whose mere presence signals a real backend.
// The generic language catches (node/go/rust/…) are excluded here so a UI spec
// that merely says "TypeScript" is not turned into a full-stack build.
var namedBackendIDs = map[string]bool{
	"django": true, "fastapi": true, "flask": true, "express": true, "nestjs": true,
	"gin": true, "fiber": true, "rails": true, "laravel": true, "spring-boot": true,
	"axum": true, "actix": true,
}

var (
	frontendSignal = kw(`frontend|front-end|web app|web application|dashboard|storefront|\bui\b|user interface|single[- ]page|\bspa\b|admin panel|landing page|web client|customer[- ]facing|\breact\b`)
	backendSignal  = kw(`\brest api\b|\bapi\b|\bbackend\b|\bserver\b|endpoints?|\bdatabase\b|postgres|mysql|mongo|authentication|\bauth\b|microservice|\bcrud\b`)
)

// bestWhere returns the highest-scoring framework whose id satisfies want.
func bestWhere(prompt, workDir string, want func(string) bool) (string, int) {
	low := strings.ToLower(prompt)
	var bestFW string
	best := -1
	for _, h := range handlers {
		for _, fw := range h.Frameworks() {
			if !want(fw.ID) {
				continue
			}
			score := -1
			if anyMarker(workDir, fw.Markers) {
				score = fw.Priority + 1000
			} else if fw.Keywords != nil && fw.Keywords.MatchString(low) {
				score = fw.Priority
			}
			if score > best {
				best, bestFW = score, fw.ID
			}
		}
	}
	return bestFW, best
}

// DetectStack decides the scaffold layout: Next.js is full-stack on its own; a
// spec that needs both a UI and a server becomes a backend/ + frontend/ split;
// otherwise a single framework (falling back to plain Detect for languages like
// Go/Rust). Returns an empty plan when nothing is recognized.
func DetectStack(prompt, workDir string) StackPlan {
	low := strings.ToLower(prompt)

	// Next.js already covers UI + API routes → single.
	if fw, s := bestWhere(prompt, workDir, func(id string) bool { return id == "nextjs" }); s >= 0 && fw == "nextjs" {
		return StackPlan{Frontend: "nextjs"}
	}

	fe := ""
	if fw, s := bestWhere(prompt, workDir, func(id string) bool { return id == "react" }); s >= 0 {
		fe = fw
	} else if frontendSignal.MatchString(low) {
		fe = "react"
	}

	be := ""
	if fw, s := bestWhere(prompt, workDir, func(id string) bool { return namedBackendIDs[id] }); s >= 0 {
		be = fw
	} else if backendSignal.MatchString(low) {
		be = "node"
	}

	switch {
	case fe != "" && be != "":
		return StackPlan{Backend: be, Frontend: fe}
	case be != "":
		return StackPlan{Backend: be}
	case fe != "":
		return StackPlan{Frontend: fe}
	}
	// No full-stack signal: single detection (covers go/rust/python/etc.).
	if _, fw, s := Detect(prompt, workDir); s >= 0 {
		if frontendIDs[fw] {
			return StackPlan{Frontend: fw}
		}
		return StackPlan{Backend: fw}
	}
	return StackPlan{}
}

// langMarkers maps a manifest file to its language, for language-level detection
// (install_deps) where the framework may be unknown but the language is clear.
var langMarkers = []struct{ file, lang string }{
	{"manage.py", "python"}, {"requirements.txt", "python"}, {"pyproject.toml", "python"}, {"setup.py", "python"},
	{"package.json", "javascript"},
	{"go.mod", "go"},
	{"Cargo.toml", "rust"},
	{"pom.xml", "java"}, {"build.gradle", "java"},
	{"Gemfile", "ruby"},
	{"composer.json", "php"},
	{"CMakeLists.txt", "cpp"},
}

// DetectLanguage picks a handler from the project's manifest files, for cases
// (install_deps) where the framework is unknown but the language is clear.
func DetectLanguage(workDir string) Handler {
	if workDir == "" {
		return nil
	}
	for _, m := range langMarkers {
		if _, err := os.Stat(filepath.Join(workDir, m.file)); err == nil {
			return HandlerFor(m.lang)
		}
	}
	return nil
}

// kw compiles a case-insensitive keyword regex, matching agent/skilldetect.go.
func kw(pat string) *regexp.Regexp { return regexp.MustCompile(`(?i)` + pat) }

// anyMarker reports whether any marker file exists (and, if set, contains its
// substring), matching agent/skilldetect.go's anyMarker.
func anyMarker(workDir string, markers []Marker) bool {
	if workDir == "" {
		return false
	}
	for _, m := range markers {
		p := filepath.Join(workDir, m.File)
		if m.Contains == "" {
			if _, err := os.Stat(p); err == nil {
				return true
			}
			continue
		}
		if b, err := os.ReadFile(p); err == nil && strings.Contains(string(b), m.Contains) {
			return true
		}
	}
	return false
}
