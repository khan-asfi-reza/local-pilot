package lang

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

// python scaffolds Django (with DRF/Celery add-ons), FastAPI, and Flask, always
// inside a project-local .venv so a managed system Python is never touched.
type python struct{}

func (python) Lang() string        { return "python" }
func (python) Toolchain() []string { return []string{"python3"} }

func (python) Frameworks() []Framework {
	return []Framework{
		{ID: "django", Priority: 50, Keywords: kw(`\bdjango\b`), Markers: []Marker{
			{File: "manage.py"}, {File: "requirements.txt", Contains: "Django"}, {File: "pyproject.toml", Contains: "django"},
		}},
		{ID: "fastapi", Priority: 40, Keywords: kw(`\bfastapi\b`), Markers: []Marker{
			{File: "requirements.txt", Contains: "fastapi"}, {File: "pyproject.toml", Contains: "fastapi"},
		}},
		{ID: "flask", Priority: 40, Keywords: kw(`\bflask\b`), Markers: []Marker{
			{File: "requirements.txt", Contains: "Flask"}, {File: "requirements.txt", Contains: "flask"},
		}},
	}
}

func (python) Scaffold(ctx context.Context, r Req) (Result, error) {
	res, err := pyRecipe(r.Framework).run(ctx, r)
	if err == nil {
		res.Lang = "python"
	}
	return res, err
}

// Install ensures a venv exists, then pip-installs the packages into it.
func (python) Install(ctx context.Context, workDir string, pkgs []string) error {
	if len(pkgs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 240*time.Second)
	defer cancel()
	if _, err := os.Stat(filepath.Join(workDir, ".venv")); err != nil {
		if err := runCmd(ctx, workDir, "python3", []string{"-m", "venv", ".venv"}); err != nil {
			return err
		}
	}
	return runCmd(ctx, workDir, ".venv/bin/pip", append([]string{"install", "--quiet"}, pkgs...))
}

func pyRecipe(framework string) Recipe {
	venv := Cmd{Bin: "python3", Args: []string{"-m", "venv", ".venv"}}
	switch framework {
	case "django":
		return Recipe{
			Framework: "django", Requires: []string{"python3"}, Timeout: 300 * time.Second,
			Project: "config", App: "core", Settings: "config/settings.py",
			Stack: "Django (Python)", Entry: ".venv/bin/python manage.py runserver",
			Install:  []Cmd{venv, {Bin: ".venv/bin/pip", Args: []string{"install", "--quiet", "django==5.1.*", "dj-database-url", "psycopg[binary]"}}},
			Generate: &Cmd{Bin: ".venv/bin/django-admin", Args: []string{"startproject", "{{.project}}", "."}},
			Nest:     NestDot, Verify: "manage.py",
			Post: []Post{
				{Run: &Cmd{Bin: ".venv/bin/python", Args: []string{"manage.py", "startapp", "{{.app}}"}}},
				{Run: &Cmd{Bin: ".venv/bin/pip", Args: []string{"install", "--quiet", "djangorestframework"}}, When: "kw:rest|drf|restful|api endpoint"},
				{Run: &Cmd{Bin: ".venv/bin/pip", Args: []string{"install", "--quiet", "celery", "redis"}}, When: "has:redis&kw:celery|worker|background task|task queue"},
				{Render: "python/django_settings.py.tmpl", To: "{{.project}}/settings.py", Append: true},
				{Render: "python/django_requirements.txt.tmpl", To: "requirements.txt"},
				{Render: "addons/celery/celery_app.py.tmpl", To: "{{.project}}/celery.py", When: "has:redis&kw:celery|worker|background task|task queue"},
			},
			Layout: []string{"manage.py", "{{.project}}/settings.py", "{{.project}}/urls.py", "{{.app}}/models.py"},
		}
	case "flask":
		return Recipe{
			Framework: "flask", Requires: []string{"python3"}, Timeout: 180 * time.Second,
			Project: "app", App: "app", Settings: "app.py",
			Stack: "Flask (Python)", Entry: ".venv/bin/flask --app app run --debug",
			Install: []Cmd{venv, {Bin: ".venv/bin/pip", Args: []string{"install", "--quiet", "flask"}}},
			Post: []Post{
				{Run: &Cmd{Bin: ".venv/bin/pip", Args: []string{"install", "--quiet", "SQLAlchemy", "psycopg[binary]"}}, When: "has:postgres"},
				{Render: "python/flask_app.py.tmpl", To: "app.py"},
				{Render: "python/flask_requirements.txt.tmpl", To: "requirements.txt"},
			},
			Layout: []string{"app.py", "requirements.txt"},
		}
	default: // fastapi
		return Recipe{
			Framework: "fastapi", Requires: []string{"python3"}, Timeout: 180 * time.Second,
			Project: "app", App: "app", Settings: "app/main.py",
			Stack: "FastAPI (Python)", Entry: ".venv/bin/uvicorn app.main:app --reload",
			Install: []Cmd{venv, {Bin: ".venv/bin/pip", Args: []string{"install", "--quiet", "fastapi", "uvicorn[standard]"}}},
			Post: []Post{
				{Run: &Cmd{Bin: ".venv/bin/pip", Args: []string{"install", "--quiet", "psycopg[binary]"}}, When: "has:postgres"},
				// sqlalchemy whenever there is a DB layer (postgres OR an auth module that
				// needs the User model) — db.py falls back to sqlite so it works with no docker.
				{Run: &Cmd{Bin: ".venv/bin/pip", Args: []string{"install", "--quiet", "sqlalchemy"}}, When: "has:postgres"},
				{Run: &Cmd{Bin: ".venv/bin/pip", Args: []string{"install", "--quiet", "sqlalchemy"}}, When: "kw:jwt|auth|login|password|token|register|signup"},
				{Run: &Cmd{Bin: ".venv/bin/pip", Args: []string{"install", "--quiet", "bcrypt", "python-jose[cryptography]", "python-multipart"}}, When: "kw:jwt|auth|login|password|token|register|signup"},
				// FastAPI code lives in an app/ package (the layout models naturally build
				// into) so generated feature modules import `from app.db/app.auth import ...`.
				{Render: "python/fastapi_init.py.tmpl", To: "app/__init__.py"},
				{Render: "python/fastapi_main.py.tmpl", To: "app/main.py"},
				{Render: "python/fastapi_requirements.txt.tmpl", To: "requirements.txt"},
				{Render: "python/fastapi_db.py.tmpl", To: "app/db.py", When: "has:postgres"},
				{Render: "python/fastapi_db.py.tmpl", To: "app/db.py", When: "kw:jwt|auth|login|password|token|register|signup"},
				{Render: "python/fastapi_auth.py.tmpl", To: "app/auth.py", When: "kw:jwt|auth|login|password|token|register|signup"},
			},
			Layout: []string{"app/main.py", "requirements.txt"},
		}
	}
}
