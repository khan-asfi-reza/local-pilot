package systemtest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"harness/harness/lang"
)

// TestDetectLanguageFromManifest checks language detection from a project's
// manifest file, which is how install_deps knows which package manager to use
// when the framework is unknown.
func TestDetectLanguageFromManifest(t *testing.T) {
	cases := map[string]string{
		"requirements.txt": "python",
		"manage.py":        "python",
		"pyproject.toml":   "python",
		"package.json":     "javascript",
		"go.mod":           "go",
		"Cargo.toml":       "rust",
		"pom.xml":          "java",
		"Gemfile":          "ruby",
		"composer.json":    "php",
		"CMakeLists.txt":   "cpp",
	}
	for manifest, wantLang := range cases {
		dir := tempProject(t, map[string]string{manifest: "\n"})
		h := lang.DetectLanguage(dir)
		if h == nil {
			t.Errorf("%s: no handler detected", manifest)
			continue
		}
		if h.Lang() != wantLang {
			t.Errorf("%s: language = %q, want %q", manifest, h.Lang(), wantLang)
		}
	}
	if lang.DetectLanguage(t.TempDir()) != nil {
		t.Error("an empty directory should not resolve to a language")
	}
	if lang.DetectLanguage("") != nil {
		t.Error("an empty path should not resolve to a language")
	}
}

// TestEveryFrameworkResolvesToAHandler checks the registry is internally
// consistent: each advertised framework is owned by exactly one handler, and
// each handler's language is unique.
func TestEveryFrameworkResolvesToAHandler(t *testing.T) {
	handlers := lang.Handlers()
	if len(handlers) < 8 {
		t.Fatalf("only %d language handlers registered", len(handlers))
	}
	seenLang := map[string]bool{}
	seenFW := map[string]string{}
	for _, h := range handlers {
		if seenLang[h.Lang()] {
			t.Errorf("two handlers claim language %q", h.Lang())
		}
		seenLang[h.Lang()] = true
		if lang.HandlerFor(h.Lang()) == nil {
			t.Errorf("HandlerFor(%q) returned nil", h.Lang())
		}
		for _, fw := range h.Frameworks() {
			if prev, dup := seenFW[fw.ID]; dup {
				t.Errorf("framework %q claimed by both %s and %s", fw.ID, prev, h.Lang())
			}
			seenFW[fw.ID] = h.Lang()
			if owner := lang.HandlerForFramework(fw.ID); owner == nil || owner.Lang() != h.Lang() {
				t.Errorf("HandlerForFramework(%q) did not return its owner", fw.ID)
			}
		}
	}
	if lang.HandlerFor("cobol") != nil || lang.HandlerForFramework("cobol-web") != nil {
		t.Error("an unknown language/framework should resolve to nil")
	}
}

// TestExistingProjectBeatsPromptKeywords checks a marker file on disk outranks
// whatever the user typed, so a follow-up request never re-scaffolds a project
// in a different stack.
func TestExistingProjectBeatsPromptKeywords(t *testing.T) {
	django := tempProject(t, map[string]string{"manage.py": "# django\n"})

	_, fw, score := lang.Detect("add a FastAPI endpoint for orders", django)
	if fw != "django" {
		t.Errorf("detected %q for an existing Django project, want django", fw)
	}
	if score < 1000 {
		t.Errorf("a marker file should score above any keyword match, got %d", score)
	}

	_, fw, _ = lang.Detect("add a FastAPI endpoint for orders", t.TempDir())
	if fw != "fastapi" {
		t.Errorf("on an empty directory the prompt decides, got %q", fw)
	}
}

// TestDetectStackChoosesTheLayout checks the scaffold layout decision: a
// full-stack spec splits into backend/ + frontend/, a UI-only spec does not get
// a server, and Next.js covers both on its own.
func TestDetectStackChoosesTheLayout(t *testing.T) {
	cases := []struct {
		name, prompt, wantBE, wantFE string
	}{
		{"full stack", "A React storefront UI plus an Express REST API with postgres", "express", "react"},
		{"api only", "A FastAPI REST API with JWT auth and postgres", "fastapi", ""},
		{"ui only", "A landing page with a hero section and a pricing table", "", "react"},
		{"nextjs", "A Next.js dashboard with API routes", "", "nextjs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := lang.DetectStack(tc.prompt, t.TempDir())
			if p.Backend != tc.wantBE || p.Frontend != tc.wantFE {
				t.Fatalf("stack = {backend:%q frontend:%q}, want {%q %q}", p.Backend, p.Frontend, tc.wantBE, tc.wantFE)
			}
			if tc.wantBE != "" && tc.wantFE != "" && !p.FullStack() {
				t.Error("FullStack() disagrees with the plan")
			}
			if p.Empty() {
				t.Error("plan reported empty although a framework was chosen")
			}
		})
	}
	if !lang.DetectStack("", t.TempDir()).Empty() {
		t.Error("an empty prompt on an empty directory should detect nothing")
	}
}

// apiSpec is a small two-table data model with a foreign key and an
// authenticated write, matching what the contract stage produces.
func apiSpec(backendRoot string) lang.BuildSpec {
	return lang.BuildSpec{
		BackendRoot: backendRoot,
		Entities: []lang.BuildEntity{
			{Name: "tag", Table: "tags", Fields: []lang.BuildField{{Name: "name", Type: "string", Unique: true}}},
			{Name: "bookmark", Table: "bookmarks", Fields: []lang.BuildField{
				{Name: "url", Type: "string"},
				{Name: "notes", Type: "text", Nullable: true},
				{Name: "tag_id", Type: "int", Ref: "tags.id", Index: true},
			}},
		},
		Endpoints: []lang.BuildEndpoint{
			{Method: "GET", Path: "/api/bookmarks", List: true},
			{Method: "POST", Path: "/api/bookmarks", Auth: true},
			{Method: "GET", Path: "/api/tags", List: true},
		},
	}
}

// TestGeneratedFastAPICodeIsValidPython checks the deterministic API builder
// emits code that actually parses. The whole point of generating the plumbing is
// that the model only fills marked blanks, so the skeleton must never be the
// thing that breaks the build.
func TestGeneratedFastAPICodeIsValidPython(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not on PATH")
	}
	dir := t.TempDir()

	res, err := lang.GenerateAPI(context.Background(), "fastapi", dir, apiSpec(""), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) == 0 {
		t.Fatal("no files generated")
	}
	if len(res.Blanks) == 0 {
		t.Error("no logic blanks were reported, so the model is not told what to fill")
	}

	checked := 0
	for _, rel := range res.Files {
		if !strings.HasSuffix(rel, ".py") {
			continue
		}
		checked++
		out, err := exec.Command(python, "-m", "py_compile", filepath.Join(dir, rel)).CombinedOutput()
		if err != nil {
			t.Errorf("generated %s does not parse:\n%s", rel, out)
		}
	}
	if checked == 0 {
		t.Fatal("the FastAPI builder generated no Python files")
	}
}

// TestGenerateAPIIsRerunnable checks a second pass neither errors nor overwrites
// work the model has already done, which is what makes repair passes safe.
func TestGenerateAPIIsRerunnable(t *testing.T) {
	dir := t.TempDir()
	spec := apiSpec("backend")

	first, err := lang.GenerateAPI(context.Background(), "express", dir, spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Files) == 0 {
		t.Fatal("no files generated")
	}
	route := filepath.Join(dir, first.Files[0])
	filled := "// filled in by the model\n"
	if err := os.WriteFile(route, []byte(filled), 0o644); err != nil {
		t.Fatal(err)
	}

	second, err := lang.GenerateAPI(context.Background(), "express", dir, spec, nil)
	if err != nil {
		t.Fatalf("a re-run must not fail: %v", err)
	}
	if len(second.Files) != 0 {
		t.Errorf("a re-run rewrote %d existing files", len(second.Files))
	}
	raw, err := os.ReadFile(route)
	if err != nil || string(raw) != filled {
		t.Errorf("the model's work was clobbered: %q", string(raw))
	}
	if lang.Supports("rails") {
		t.Error("Supports should be false for a framework with no builder")
	}
	if _, err := lang.GenerateAPI(context.Background(), "rails", dir, spec, nil); err == nil {
		t.Error("an unsupported framework must error so the caller falls back to hand-writing")
	}
}
