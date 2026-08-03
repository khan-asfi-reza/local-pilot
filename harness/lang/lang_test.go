package lang

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectByKeyword(t *testing.T) {
	cases := []struct {
		prompt string
		want   string
	}{
		{"Build a Django REST API for a blog", "django"},
		{"Make a FastAPI service with postgres", "fastapi"},
		{"A Next.js dashboard", "nextjs"},
		{"Simple express.js server", "express"},
		{"A gin web service in golang", "gin"},
		{"CLI in plain C program", "c"},
		{"A C++ number cruncher", "cpp"},
	}
	for _, c := range cases {
		_, fw, score := Detect(c.prompt, "")
		if fw != c.want || score < 0 {
			t.Errorf("Detect(%q) = %q (score %d), want %q", c.prompt, fw, score, c.want)
		}
	}
}

func TestDetectMarkerBeatsKeyword(t *testing.T) {
	// A prompt mentioning react in a dir already carrying next.config.ts must pick
	// nextjs (marker) — and a marker outscores any keyword-only match.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "next.config.ts"), []byte("export default {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, fw, score := Detect("add a react component", dir)
	if fw != "nextjs" {
		t.Fatalf("marker detection = %q (score %d), want nextjs", fw, score)
	}
	if score < 1000 {
		t.Fatalf("marker score = %d, want >= 1000", score)
	}
}

func TestWhenMatches(t *testing.T) {
	data := map[string]any{"has_redis": true, "has_postgres": false}
	cases := []struct {
		when   string
		prompt string
		want   bool
	}{
		{"", "anything", true},
		{"has:redis", "x", true},
		{"has:postgres", "x", false},
		{"kw:celery|worker", "add a celery worker", true},
		{"kw:celery", "no match here", false},
		{"has:redis&kw:celery", "use celery", true},
		{"has:postgres&kw:celery", "use celery", false},
	}
	for _, c := range cases {
		if got := whenMatches(c.when, data, c.prompt); got != c.want {
			t.Errorf("whenMatches(%q, %q) = %v, want %v", c.when, c.prompt, got, c.want)
		}
	}
}

func TestScaffoldTemplateOnly(t *testing.T) {
	// C++ scaffolding needs no toolchain (templates only), so it runs hermetically.
	dir := t.TempDir()
	res, err := (cpp{}).Scaffold(context.Background(), Req{Framework: "cpp", WorkDir: dir})
	if err != nil {
		t.Fatalf("scaffold cpp: %v", err)
	}
	if res.Lang != "cpp" || res.Framework != "cpp" {
		t.Fatalf("result = %+v", res)
	}
	for _, want := range []string{"CMakeLists.txt", "main.cpp"} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("expected %s on disk: %v", want, err)
		}
	}
	if len(res.Layout) != 2 {
		t.Errorf("layout = %v, want 2 files", res.Layout)
	}
	// The CMake project name must be templated from the canonical project var.
	body, _ := os.ReadFile(filepath.Join(dir, "CMakeLists.txt"))
	if !contains(string(body), "project(app CXX)") {
		t.Errorf("CMakeLists not templated: %s", body)
	}
}

func TestScaffoldEnvWiring(t *testing.T) {
	// With a provisioned postgres .env, the FastAPI recipe's has_postgres guard
	// must fire and render db.py. This needs python3; skip if absent.
	if !haveAll("python3") {
		t.Skip("python3 not installed")
	}
	dir := t.TempDir()
	res, err := (python{}).Scaffold(context.Background(), Req{
		Framework: "fastapi", WorkDir: dir,
		Env: "DATABASE_URL=postgresql://pilot:pilot@localhost:5432/app\n",
	})
	if err != nil {
		t.Fatalf("scaffold fastapi: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "db.py")); err != nil {
		t.Errorf("db.py should be rendered when postgres is provisioned: %v", err)
	}
	if res.Entry == "" {
		t.Errorf("entry should be set")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
