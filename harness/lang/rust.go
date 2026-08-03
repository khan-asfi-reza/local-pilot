package lang

import (
	"context"
	"time"
)

// rust scaffolds a Cargo binary crate and, for a named web framework, adds it via
// `cargo add`.
type rust struct{}

func (rust) Lang() string        { return "rust" }
func (rust) Toolchain() []string { return []string{"cargo"} }

func (rust) Frameworks() []Framework {
	return []Framework{
		{ID: "axum", Priority: 30, Keywords: kw(`\baxum\b`), Markers: []Marker{{File: "Cargo.toml", Contains: "axum"}}},
		{ID: "actix", Priority: 30, Keywords: kw(`\bactix\b`), Markers: []Marker{{File: "Cargo.toml", Contains: "actix"}}},
		{ID: "rust", Priority: 5, Keywords: kw(`\brust\b|\bcargo\b`), Markers: []Marker{{File: "Cargo.toml"}}},
	}
}

func (rust) Scaffold(ctx context.Context, r Req) (Result, error) {
	res, err := rustRecipe(r.Framework).run(ctx, r)
	if err == nil {
		res.Lang = "rust"
	}
	return res, err
}

// Install runs `cargo add` for the packages.
func (rust) Install(ctx context.Context, workDir string, pkgs []string) error {
	if len(pkgs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 240*time.Second)
	defer cancel()
	return runCmd(ctx, workDir, "cargo", append([]string{"add"}, pkgs...))
}

func rustRecipe(framework string) Recipe {
	base := Recipe{
		Requires: []string{"cargo"}, Timeout: 300 * time.Second,
		Project: "app", App: "app", Settings: "Cargo.toml", Entry: "cargo run",
		Generate: &Cmd{Bin: "cargo", Args: []string{"init", "--bin", "."}},
		Nest:     NestDot, Verify: "Cargo.toml",
		Layout: []string{"Cargo.toml", "src/main.rs"},
	}
	switch framework {
	case "axum":
		base.Framework, base.Stack = "axum", "Rust + Axum"
		base.Post = []Post{
			{Run: &Cmd{Bin: "cargo", Args: []string{"add", "axum", "tokio", "--features", "tokio/full"}}},
			{Render: "rust/main_axum.rs.tmpl", To: "src/main.rs"},
		}
	case "actix":
		base.Framework, base.Stack = "actix", "Rust + Actix Web"
		base.Post = []Post{
			{Run: &Cmd{Bin: "cargo", Args: []string{"add", "actix-web"}}},
			{Render: "rust/main_actix.rs.tmpl", To: "src/main.rs"},
		}
	default:
		base.Framework, base.Stack = "rust", "Rust (binary crate)"
	}
	return base
}
