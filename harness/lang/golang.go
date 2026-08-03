package lang

import (
	"context"
	"time"
)

// golang scaffolds a Go module and, for a named web framework, wires its router;
// bare Go uses net/http from the standard library.
type golang struct{}

func (golang) Lang() string        { return "go" }
func (golang) Toolchain() []string { return []string{"go"} }

func (golang) Frameworks() []Framework {
	// Keep bare-Go detection explicit (\bgolang\b, not \bgo\b) so a prompt like
	// "go build a todo app" does not mis-scaffold a Go project.
	return []Framework{
		{ID: "gin", Priority: 30, Keywords: kw(`\bgin\b`), Markers: []Marker{{File: "go.mod", Contains: "gin-gonic/gin"}}},
		{ID: "fiber", Priority: 30, Keywords: kw(`\bfiber\b`), Markers: []Marker{{File: "go.mod", Contains: "gofiber/fiber"}}},
		{ID: "go", Priority: 5, Keywords: kw(`\bgolang\b`), Markers: []Marker{{File: "go.mod"}}},
	}
}

func (golang) Scaffold(ctx context.Context, r Req) (Result, error) {
	res, err := goRecipe(r.Framework).run(ctx, r)
	if err == nil {
		res.Lang = "go"
	}
	return res, err
}

// Install runs `go get` for each package, then tidies the module.
func (golang) Install(ctx context.Context, workDir string, pkgs []string) error {
	ctx, cancel := context.WithTimeout(ctx, 240*time.Second)
	defer cancel()
	for _, p := range pkgs {
		if err := runCmd(ctx, workDir, "go", []string{"get", p}); err != nil {
			return err
		}
	}
	return runCmd(ctx, workDir, "go", []string{"mod", "tidy"})
}

func goRecipe(framework string) Recipe {
	base := Recipe{
		Requires: []string{"go"}, Timeout: 240 * time.Second,
		Project: "app", App: "app", Settings: "main.go", Entry: "go run .",
		Generate: &Cmd{Bin: "go", Args: []string{"mod", "init", "{{.module}}"}},
		Nest:     NestDot, Verify: "go.mod",
		Layout: []string{"go.mod", "main.go"},
	}
	switch framework {
	case "gin":
		base.Framework, base.Stack = "gin", "Go + Gin"
		base.Post = []Post{
			{Render: "go/main_gin.go.tmpl", To: "main.go"},
			{Run: &Cmd{Bin: "go", Args: []string{"get", "github.com/gin-gonic/gin"}}},
			{Run: &Cmd{Bin: "go", Args: []string{"mod", "tidy"}}},
		}
	case "fiber":
		base.Framework, base.Stack = "fiber", "Go + Fiber"
		base.Post = []Post{
			{Render: "go/main_fiber.go.tmpl", To: "main.go"},
			{Run: &Cmd{Bin: "go", Args: []string{"get", "github.com/gofiber/fiber/v2"}}},
			{Run: &Cmd{Bin: "go", Args: []string{"mod", "tidy"}}},
		}
	default:
		base.Framework, base.Stack = "go", "Go (net/http)"
		base.Post = []Post{
			{Render: "go/main_http.go.tmpl", To: "main.go"},
			{Run: &Cmd{Bin: "go", Args: []string{"mod", "tidy"}}},
		}
	}
	return base
}
