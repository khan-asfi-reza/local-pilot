package orchestrator

import (
	"strings"
	"testing"

	"harness/harness/lang"
)

func TestDeriveEntitiesFromContract(t *testing.T) {
	c := APIContract{Endpoints: []APIEndpoint{
		{Method: "GET", Path: "/api/bookmarks", ResponseList: true, Response: []APIField{
			{Name: "id", Type: "int"}, {Name: "url", Type: "string"}, {Name: "title", Type: "string"}, {Name: "tag_id", Type: "int"},
		}},
		{Method: "POST", Path: "/api/bookmarks", Auth: true, Request: []APIField{
			{Name: "url", Type: "string"}, {Name: "title", Type: "string"}, {Name: "notes", Type: "text"}, {Name: "tag_id", Type: "int"},
		}},
		{Method: "GET", Path: "/api/tags", ResponseList: true, Response: []APIField{
			{Name: "id", Type: "int"}, {Name: "name", Type: "string"},
		}},
		{Method: "POST", Path: "/api/auth/login", Request: []APIField{{Name: "email", Type: "string"}}},
	}}
	ents := deriveEntitiesFromContract(c)

	byTable := map[string]Entity{}
	for _, e := range ents {
		byTable[e.Table] = e
	}
	if _, ok := byTable["auth"]; ok {
		t.Errorf("auth must not become an entity")
	}
	bm, ok := byTable["bookmarks"]
	if !ok {
		t.Fatalf("expected a bookmarks entity, got %v", ents)
	}
	if bm.Name != "bookmark" {
		t.Errorf("bookmarks entity name = %q, want bookmark", bm.Name)
	}
	var tagField *EntityField
	names := map[string]bool{}
	for i := range bm.Fields {
		names[bm.Fields[i].Name] = true
		if bm.Fields[i].Name == "tag_id" {
			tagField = &bm.Fields[i]
		}
	}
	if names["id"] {
		t.Errorf("id must not be a listed field (auto-added)")
	}
	for _, want := range []string{"url", "title", "notes", "tag_id"} {
		if !names[want] {
			t.Errorf("bookmarks missing field %q (have %v)", want, names)
		}
	}
	if tagField == nil || tagField.Ref != "tags.id" || !tagField.Index {
		t.Errorf("tag_id should be an indexed FK to tags.id, got %+v", tagField)
	}
	if _, ok := byTable["tags"]; !ok {
		t.Errorf("expected a tags entity")
	}
}

func TestParseEntityFields(t *testing.T) {
	fs := parseEntityFields("title:string:notnull, done:bool:default=false, owner_id:int:fk=users.id:index, body:text:null")
	if len(fs) != 4 {
		t.Fatalf("want 4 fields, got %d: %+v", len(fs), fs)
	}
	byName := map[string]EntityField{}
	for _, f := range fs {
		byName[f.Name] = f
	}
	if f := byName["title"]; f.Type != "string" || f.Nullable {
		t.Errorf("title: want type=string notnull, got %+v", f)
	}
	if f := byName["done"]; f.Type != "bool" || f.Default != "false" || !f.Nullable {
		t.Errorf("done: want bool default=false nullable, got %+v", f)
	}
	if f := byName["owner_id"]; f.Ref != "users.id" || !f.Index {
		t.Errorf("owner_id: want fk=users.id index, got %+v", f)
	}
	if f := byName["body"]; f.Type != "text" || !f.Nullable {
		t.Errorf("body: want text nullable, got %+v", f)
	}
}

func TestBuildSpecFrom(t *testing.T) {
	c := APIContract{
		Endpoints: []APIEndpoint{{Method: "POST", Path: "/api/notes", Auth: true, ResponseList: false}},
		Entities: []Entity{{Name: "note", Table: "notes", Fields: []EntityField{
			{Name: "body", Type: "text", Nullable: true},
			{Name: "user_id", Type: "int", Ref: "users.id", Index: true},
		}}},
	}
	spec := buildSpecFrom(c, "backend")
	if spec.BackendRoot != "backend" {
		t.Errorf("BackendRoot = %q, want backend", spec.BackendRoot)
	}
	if len(spec.Entities) != 1 || spec.Entities[0].Table != "notes" || len(spec.Entities[0].Fields) != 2 {
		t.Fatalf("entity mapping wrong: %+v", spec.Entities)
	}
	if spec.Entities[0].Fields[1].Ref != "users.id" {
		t.Errorf("field FK not mapped: %+v", spec.Entities[0].Fields[1])
	}
	if len(spec.Endpoints) != 1 || !spec.Endpoints[0].Auth {
		t.Errorf("endpoint mapping wrong: %+v", spec.Endpoints)
	}
}

func TestGeneratedLayerNote(t *testing.T) {
	if note := generatedLayerNote(lang.BuildResult{}); note != "" {
		t.Errorf("empty result should give empty note, got %q", note)
	}
	note := generatedLayerNote(lang.BuildResult{Files: []string{"backend/migrations/010_notes.sql", "backend/src/routes/notes.ts"}})
	for _, want := range []string{"GENERATED DATA LAYER", "=== LOGIC", "backend/src/routes/notes.ts", "900+"} {
		if !strings.Contains(note, want) {
			t.Errorf("note missing %q:\n%s", want, note)
		}
	}
}
