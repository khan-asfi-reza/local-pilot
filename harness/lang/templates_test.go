package lang

import (
	"path/filepath"
	"testing"
)

// allRecipes returns every framework recipe, so tests can validate them without a
// network or toolchain.
func allRecipes() []Recipe {
	var rs []Recipe
	for _, fw := range []string{"django", "flask", "fastapi"} {
		rs = append(rs, pyRecipe(fw))
	}
	for _, fw := range []string{"nextjs", "react", "nestjs", "express", "node"} {
		rs = append(rs, jsRecipe(fw))
	}
	for _, fw := range []string{"gin", "fiber", "go"} {
		rs = append(rs, goRecipe(fw))
	}
	rs = append(rs, rustRecipe("axum"), rustRecipe("actix"), rustRecipe("rust"),
		javaRecipe(), rubyRecipe(), phpRecipe(), cRecipe(), cppRecipe())
	return rs
}

// TestRecipeTemplatesExist guards against a recipe referencing a template file
// that was never created — the render fails at scaffold time and silently drops
// to the LLM path, which is hard to spot. This catches it at test time instead.
func TestRecipeTemplatesExist(t *testing.T) {
	for _, rec := range allRecipes() {
		for _, p := range rec.Post {
			if p.Render == "" {
				continue
			}
			path := filepath.ToSlash(filepath.Join("templates", p.Render))
			if _, err := templatesFS.ReadFile(path); err != nil {
				t.Errorf("recipe %q references missing template %q: %v", rec.Framework, p.Render, err)
			}
		}
	}
}
