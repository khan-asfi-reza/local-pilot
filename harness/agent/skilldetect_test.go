package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"harness/harness/model"
)

func writeSkill(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestInternalSkillsHidden confirms internal skills stay out of the visible
// catalog and the load_skill enum, while a normal skill is listed.
func TestInternalSkillsHidden(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "nextjs", "---\nname: nextjs\ndescription: next\ninternal: true\n---\nNEXTJS_BODY")
	writeSkill(t, dir, "app-builder", "---\nname: app-builder\ndescription: build\n---\nAPP_BODY")

	set, err := scanSkills(dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(set.catalog, "nextjs") {
		t.Errorf("internal skill leaked into catalog: %q", set.catalog)
	}
	if !strings.Contains(set.catalog, "app-builder") {
		t.Errorf("visible skill missing from catalog: %q", set.catalog)
	}
	for _, n := range set.names {
		if n == "nextjs" {
			t.Errorf("internal skill leaked into load_skill enum")
		}
	}
	if set.bodies["nextjs"] == "" || !set.internal["nextjs"] {
		t.Errorf("internal skill body/flag not retained for silent injection")
	}
}

// TestDetectByKeyword confirms a framework named in the prompt is injected, and
// an unrelated language is not.
func TestDetectByKeyword(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "nextjs", "---\nname: nextjs\ndescription: next\ninternal: true\n---\nNEXTJS_BODY")
	writeSkill(t, dir, "python", "---\nname: python\ndescription: py\ninternal: true\n---\nPYTHON_BODY")
	set, _ := scanSkills(dir)

	g := detectInternalSkills("", []model.Message{{Role: "user", Content: "build a next.js dashboard"}}, set)
	if !strings.Contains(g, "NEXTJS_BODY") {
		t.Errorf("nextjs not detected from prompt; got %q", g)
	}
	if strings.Contains(g, "PYTHON_BODY") {
		t.Errorf("unrelated python injected for a next.js task")
	}
}

// TestDetectGameAndFramework confirms newer skills resolve: a web-game library
// by keyword, and that a specific framework outranks the base react library.
func TestDetectGameAndFramework(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "phaser", "---\nname: phaser\ndescription: p\ninternal: true\n---\nPHASER_BODY")
	writeSkill(t, dir, "remix", "---\nname: remix\ndescription: r\ninternal: true\n---\nREMIX_BODY")
	writeSkill(t, dir, "react", "---\nname: react\ndescription: rc\ninternal: true\n---\nREACT_BODY")
	set, _ := scanSkills(dir)

	if g := detectInternalSkills("", []model.Message{{Role: "user", Content: "make a phaser platformer game"}}, set); !strings.Contains(g, "PHASER_BODY") {
		t.Errorf("phaser not detected; got %q", g)
	}
	// A Remix project's package.json lists both @remix-run and react; remix wins.
	wd := t.TempDir()
	os.WriteFile(filepath.Join(wd, "package.json"), []byte(`{"dependencies":{"@remix-run/node":"2","react":"18"}}`), 0o644)
	g := detectInternalSkills(wd, []model.Message{{Role: "user", Content: "add a route"}}, set)
	if !strings.Contains(g, "REMIX_BODY") || strings.Contains(g, "REACT_BODY") {
		t.Errorf("expected remix to outrank react; got %q", g)
	}
}

// TestDetectByFileMarker confirms a project file (manage.py) triggers the django
// skill even with no keyword in the prompt.
func TestDetectByFileMarker(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "django", "---\nname: django\ndescription: dj\ninternal: true\n---\nDJANGO_BODY")
	set, _ := scanSkills(dir)

	wd := t.TempDir()
	if err := os.WriteFile(filepath.Join(wd, "manage.py"), []byte("#!/usr/bin/env python"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := detectInternalSkills(wd, []model.Message{{Role: "user", Content: "add an endpoint"}}, set)
	if !strings.Contains(g, "DJANGO_BODY") {
		t.Errorf("django not detected from manage.py; got %q", g)
	}
}
