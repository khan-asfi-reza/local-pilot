package lang

import (
	"context"
	"time"
)

// clang scaffolds a C project with a CMake build. There is no universal C package
// manager, so Install routes to vcpkg/conan only when one is present.
type clang struct{}

func (clang) Lang() string        { return "c" }
func (clang) Toolchain() []string { return nil } // templates only; cmake needed to build, not scaffold

func (clang) Frameworks() []Framework {
	// Keep C detection explicit so plain prose "go build a C-level cache" does not
	// match; \bc\+\+ is deliberately excluded (that is the cpp handler).
	return []Framework{
		{ID: "c", Priority: 5, Keywords: kw(`\bc\s+program\b|\bc\s+language\b|\bplain c\b|\bansi c\b`)},
	}
}

func (clang) Scaffold(ctx context.Context, r Req) (Result, error) {
	res, err := cRecipe().run(ctx, r)
	if err == nil {
		res.Lang = "c"
	}
	return res, err
}

// Install uses vcpkg or conan if available; otherwise it is a no-op (the caller
// reports that C has no standard package manager).
func (clang) Install(ctx context.Context, workDir string, pkgs []string) error {
	return cppInstall(ctx, workDir, pkgs)
}

func cRecipe() Recipe {
	return Recipe{
		Framework: "c", Timeout: 30 * time.Second,
		Project: "app", App: "app", Settings: "CMakeLists.txt",
		Stack: "C (CMake)", Entry: "cmake -S . -B build && cmake --build build && ./build/app",
		Post: []Post{
			{Render: "c/CMakeLists.txt.tmpl", To: "CMakeLists.txt"},
			{Render: "c/main.c.tmpl", To: "main.c"},
		},
		Layout: []string{"CMakeLists.txt", "main.c"},
	}
}
