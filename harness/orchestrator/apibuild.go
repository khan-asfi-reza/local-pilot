package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"harness/harness/events"
	"harness/harness/lang"
)

// buildAPI runs the deterministic API/DB builder. It derives the data model from the
// contract (refined by the model), persists it into apispec.json, and — when a
// builder exists for the backend framework — generates migrations, models, and CRUD
// route skeletons so feature tasks only fill the marked LOGIC blanks. Returns
// covered=false and an empty note (writing nothing) when there is no contract, no
// data model, or no builder for the framework, so the existing hand-write path runs
// unchanged. The note is appended to the shared stateText, so it reaches both the
// decomposer and every child.
func (o *Orchestrator) buildAPI(ctx context.Context, prompt, workDir string, c *APIContract, emit func(events.Event)) (bool, string) {
	if c == nil || len(c.Endpoints) == 0 {
		return false, ""
	}
	// 1. Derive the data model from the endpoints (free), refine with the model, persist.
	entities := deriveEntitiesFromContract(*c)
	if refined := GenerateDataModel(ctx, o.planner, prompt, *c); len(refined) > 0 {
		entities = mergeEntities(entities, refined)
	}
	if len(entities) == 0 {
		return false, ""
	}
	c.Entities = entities
	if raw, e := json.MarshalIndent(*c, "", "  "); e == nil {
		_ = os.WriteFile(filepath.Join(workDir, "apispec.json"), raw, 0o644)
	}
	emit(events.Text(fmt.Sprintf("\n[data model: %d entities designed → apispec.json]\n", len(entities))))

	// 2. Which backend framework? Fall back to hand-write if there is no builder.
	fw, backendRoot := backendFrameworkAndRoot(workDir)
	if fw == "" || !lang.Supports(fw) {
		emit(events.Text("\n[api-builder: no generator for backend framework '" + fw + "' — feature tasks will hand-write the data layer]\n"))
		return false, ""
	}

	// 3. Generate migrations + models + CRUD routes.
	res, err := lang.GenerateAPI(ctx, fw, workDir, buildSpecFrom(*c, backendRoot), emit)
	if err != nil || len(res.Files) == 0 {
		emit(events.Text("\n[api-builder: produced no files — falling back to hand-written data layer]\n"))
		return false, ""
	}
	return true, generatedLayerNote(res)
}

// backendFrameworkAndRoot finds the backend sub-app and returns its framework and its
// path relative to workDir ("" for a monolith, "backend" for a monorepo split).
func backendFrameworkAndRoot(workDir string) (string, string) {
	for _, a := range detectNodeApps(workDir) {
		if a.kind == "backend" {
			rel, err := filepath.Rel(workDir, a.dir)
			if err != nil || rel == "." {
				rel = ""
			}
			return a.framework, filepath.ToSlash(rel)
		}
	}
	return "", ""
}

// buildSpecFrom maps the orchestrator's APIContract into the leaf-package lang.BuildSpec
// the generator consumes (the seam that keeps lang free of an orchestrator import).
func buildSpecFrom(c APIContract, backendRoot string) lang.BuildSpec {
	spec := lang.BuildSpec{BackendRoot: backendRoot}
	for _, e := range c.Entities {
		be := lang.BuildEntity{Name: e.Name, Table: e.Table}
		for _, f := range e.Fields {
			be.Fields = append(be.Fields, lang.BuildField{
				Name: f.Name, Type: f.Type, Nullable: f.Nullable, Unique: f.Unique,
				Default: f.Default, Ref: f.Ref, Index: f.Index,
			})
		}
		spec.Entities = append(spec.Entities, be)
	}
	for _, ep := range c.Endpoints {
		spec.Endpoints = append(spec.Endpoints, lang.BuildEndpoint{
			Method: ep.Method, Path: ep.Path, Auth: ep.Auth, List: ep.ResponseList,
		})
	}
	return spec
}

// generatedLayerNote is injected into stateText (so the decomposer and every child
// see it): it tells the model the data layer already exists and to implement ONLY the
// business logic inside the marked LOGIC blocks, plus how to add anything not covered.
func generatedLayerNote(res lang.BuildResult) string {
	if len(res.Files) == 0 {
		return ""
	}
	note := "\n\nGENERATED DATA LAYER — the database migrations, models, and CRUD routers listed below ALREADY EXIST " +
		"(generated deterministically from the API contract). Do NOT create tasks to write migrations, models, or basic " +
		"CRUD for these entities — that work is done. For each generated router, IMPLEMENT ONLY the business logic " +
		"BETWEEN the `=== LOGIC: ... ===` and `=== END LOGIC ===` comment markers (they are `//` in TypeScript and `#` " +
		"in Python): add filtering, sorting, pagination, validation, ownership/auth scoping, and relations. NEVER edit " +
		"code outside those markers and NEVER recreate these files. Generated files:\n"
	for _, f := range res.Files {
		note += "  - " + f + "\n"
	}
	note += "For an endpoint that is NOT plain CRUD (search, aggregate, nested/custom routes), or a NEW table not listed " +
		"above, add it yourself: a new numbered migration in backend/migrations/ using the 900+ range (e.g. 901_reviews.sql) " +
		"so it runs AFTER the generated ones, plus its router/model wired the same way the generated files are."
	return note
}
