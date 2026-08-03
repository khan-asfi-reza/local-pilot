package lang

import (
	"context"
	"time"
)

// php scaffolds a Laravel application with `composer create-project`, the
// canonical generator, into a temp dir merged up on success.
type php struct{}

func (php) Lang() string        { return "php" }
func (php) Toolchain() []string { return []string{"php", "composer"} }

func (php) Frameworks() []Framework {
	return []Framework{
		{ID: "laravel", Priority: 40, Keywords: kw(`\blaravel\b`), Markers: []Marker{
			{File: "artisan"}, {File: "composer.json", Contains: "laravel/framework"},
		}},
	}
}

func (php) Scaffold(ctx context.Context, r Req) (Result, error) {
	res, err := phpRecipe().run(ctx, r)
	if err == nil {
		res.Lang = "php"
	}
	return res, err
}

// Install runs `composer require` for each package.
func (php) Install(ctx context.Context, workDir string, pkgs []string) error {
	if len(pkgs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	return runCmd(ctx, workDir, "composer", append([]string{"require", "--no-interaction"}, pkgs...))
}

func phpRecipe() Recipe {
	return Recipe{
		Framework: "laravel", Requires: []string{"php", "composer"}, Timeout: 420 * time.Second,
		Project: "app", App: "app", Settings: "config/app.php",
		Stack: "Laravel (PHP)", Entry: "php artisan serve",
		Generate: &Cmd{Bin: "composer", Args: []string{"create-project", "laravel/laravel", "{{.scaffold_dir}}", "--no-interaction"}},
		Nest:     NestTempMove, Verify: "artisan",
		Layout: []string{"artisan", "composer.json", "routes/web.php"},
	}
}
