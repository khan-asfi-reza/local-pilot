package lang

import (
	"context"
	"time"
)

// ruby scaffolds a Rails application with the real `rails new` generator. It is
// gated on the rails binary being installed, so it cleanly falls back otherwise.
type ruby struct{}

func (ruby) Lang() string        { return "ruby" }
func (ruby) Toolchain() []string { return []string{"ruby", "rails"} }

func (ruby) Frameworks() []Framework {
	return []Framework{
		{ID: "rails", Priority: 40, Keywords: kw(`\brails\b|ruby on rails`), Markers: []Marker{
			{File: "bin/rails"}, {File: "Gemfile", Contains: "rails"},
		}},
	}
}

func (ruby) Scaffold(ctx context.Context, r Req) (Result, error) {
	res, err := rubyRecipe().run(ctx, r)
	if err == nil {
		res.Lang = "ruby"
	}
	return res, err
}

// Install runs `bundle add` for each gem.
func (ruby) Install(ctx context.Context, workDir string, pkgs []string) error {
	if len(pkgs) == 0 || !haveAll("bundle") {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 240*time.Second)
	defer cancel()
	return runCmd(ctx, workDir, "bundle", append([]string{"add"}, pkgs...))
}

func rubyRecipe() Recipe {
	// rails new . writes into the current dir; --force lets it coexist with the
	// provisioned .env, and --skip-bundle keeps the generator fast (deps install
	// on first run). Postgres is selected when it was provisioned.
	return Recipe{
		Framework: "rails", Requires: []string{"ruby", "rails"}, Timeout: 300 * time.Second,
		Project: "app", App: "app", Settings: "config/application.rb",
		Stack: "Ruby on Rails", Entry: "bin/rails server",
		Generate: &Cmd{Bin: "rails", Args: []string{"new", ".", "--force", "--skip-git", "--skip-bundle"}},
		Nest:     NestDot, Verify: "Gemfile",
		Layout: []string{"Gemfile", "config/application.rb", "config/routes.rb"},
	}
}
