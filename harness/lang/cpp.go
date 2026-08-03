package lang

import (
	"context"
	"time"
)

// cpp scaffolds a C++ project with a CMake build.
type cpp struct{}

func (cpp) Lang() string        { return "cpp" }
func (cpp) Toolchain() []string { return nil }

func (cpp) Frameworks() []Framework {
	return []Framework{
		{ID: "cpp", Priority: 10, Keywords: kw(`\bc\+\+|\bcpp\b`), Markers: []Marker{{File: "CMakeLists.txt"}}},
	}
}

func (cpp) Scaffold(ctx context.Context, r Req) (Result, error) {
	res, err := cppRecipe().run(ctx, r)
	if err == nil {
		res.Lang = "cpp"
	}
	return res, err
}

func (cpp) Install(ctx context.Context, workDir string, pkgs []string) error {
	return cppInstall(ctx, workDir, pkgs)
}

// cppInstall routes C/C++ dependency installs to vcpkg or conan when present.
// With no C/C++ package manager on PATH it is a no-op; the caller surfaces that.
func cppInstall(ctx context.Context, workDir string, pkgs []string) error {
	if len(pkgs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	switch {
	case haveAll("vcpkg"):
		return runCmd(ctx, workDir, "vcpkg", append([]string{"install"}, pkgs...))
	case haveAll("conan"):
		return runCmd(ctx, workDir, "conan", append([]string{"install"}, pkgs...))
	default:
		return nil
	}
}

func cppRecipe() Recipe {
	return Recipe{
		Framework: "cpp", Timeout: 30 * time.Second,
		Project: "app", App: "app", Settings: "CMakeLists.txt",
		Stack: "C++ (CMake)", Entry: "cmake -S . -B build && cmake --build build && ./build/app",
		Post: []Post{
			{Render: "cpp/CMakeLists.txt.tmpl", To: "CMakeLists.txt"},
			{Render: "cpp/main.cpp.tmpl", To: "main.cpp"},
		},
		Layout: []string{"CMakeLists.txt", "main.cpp"},
	}
}
