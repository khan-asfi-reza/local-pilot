package systemtest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"harness/harness/orchestrator"
)

// TestGenerateContractNormalisesWhatTheModelReturns checks the REST contract both
// sides build against is cleaned up before use: verbs upper-cased, every path
// under /api, query strings stripped, duplicates dropped, and the flat
// "name:type" strings a small model can actually produce parsed into fields.
func TestGenerateContractNormalisesWhatTheModelReturns(t *testing.T) {
	p := &fakePlanner{replies: []string{`{"endpoints":[
      {"method":"get","path":"bookmarks","summary":"list","auth":false,"request":"","response":"id:int, url:string, title:string","response_list":true},
      {"method":"POST","path":"/api/bookmarks/","summary":"create","auth":true,"request":"url:string, title:string","response":"id:int","response_list":false},
      {"method":"GET","path":"/api/bookmarks?tag={tag}","summary":"filter","auth":false,"request":"","response":"id:int","response_list":true},
      {"method":"GET","path":"/api/tags","summary":"tags","auth":false,"request":"none","response":"id:int, name:string","response_list":true}
    ]}`}}

	c, err := orchestrator.GenerateContract(context.Background(), p, "a bookmarks app")
	if err != nil {
		t.Fatal(err)
	}

	byKey := map[string]orchestrator.APIEndpoint{}
	for _, e := range c.Endpoints {
		byKey[e.Method+" "+e.Path] = e
	}
	if _, ok := byKey["GET /api/bookmarks"]; !ok {
		t.Fatalf("a bare path was not lifted under /api and upper-cased: %v", keysOf(byKey))
	}
	if _, ok := byKey["POST /api/bookmarks"]; !ok {
		t.Errorf("a trailing slash was not trimmed: %v", keysOf(byKey))
	}
	// "GET /api/bookmarks?tag={tag}" collapses onto "GET /api/bookmarks" and is
	// dropped as a duplicate, so four raw endpoints become three.
	if len(c.Endpoints) != 3 {
		t.Errorf("expected 3 endpoints after normalisation, got %d: %v", len(c.Endpoints), keysOf(byKey))
	}

	list := byKey["GET /api/bookmarks"]
	if !list.ResponseList || len(list.Response) != 3 || list.Response[1].Name != "url" {
		t.Errorf("response fields not parsed: %+v", list.Response)
	}
	create := byKey["POST /api/bookmarks"]
	if !create.Auth || len(create.Request) != 2 {
		t.Errorf("request fields or auth flag lost: %+v", create)
	}
	if tags := byKey["GET /api/tags"]; len(tags.Request) != 0 {
		t.Errorf(`"none" should parse as an empty body, got %+v`, tags.Request)
	}
}

// TestGenerateContractRetriesAndThenReportsFailure checks the contract call is
// retried and, when the backend cannot do constrained output at all, reports the
// failure instead of returning an empty contract that silently disables the
// contract-first build.
func TestGenerateContractRetriesAndThenReportsFailure(t *testing.T) {
	p := &fakePlanner{replies: []string{`{"endpoints":[]}`, "junk", `{"endpoints":[{"method":"GET","path":"/api/x","summary":"","auth":false,"request":"","response":"id:int","response_list":false}]}`}}
	c, err := orchestrator.GenerateContract(context.Background(), p, "spec")
	if err != nil || len(c.Endpoints) != 1 {
		t.Fatalf("retry path failed: c=%+v err=%v (calls=%d)", c, err, p.calls())
	}

	broken := &fakePlanner{replies: []string{"", "", ""}}
	if _, err := orchestrator.GenerateContract(context.Background(), broken, "spec"); err == nil {
		t.Fatal("an unusable backend produced no error")
	}
	if broken.calls() != 3 {
		t.Errorf("attempts = %d, want 3", broken.calls())
	}
}

// TestGenerateDataModelParsesColumnFlags checks the data model the migration and
// model generators consume: types, nullability, uniqueness, indexes, defaults and
// foreign keys, with the built-in users/auth tables excluded.
func TestGenerateDataModelParsesColumnFlags(t *testing.T) {
	p := &fakePlanner{replies: []string{`{"entities":[
      {"name":"bookmark","table":"bookmarks","fields":"url:string:notnull:unique, notes:text:null, tag_id:int:fk=tags.id:index, archived:bool:default=false"},
      {"name":"tag","table":"","fields":"name:string:notnull"},
      {"name":"user","table":"users","fields":"email:string"},
      {"name":"empty","table":"empties","fields":""}
    ]}`}}

	ents := orchestrator.GenerateDataModel(context.Background(), p, "a bookmarks app", orchestrator.APIContract{})

	byName := map[string]orchestrator.Entity{}
	for _, e := range ents {
		byName[e.Name] = e
	}
	if _, ok := byName["user"]; ok {
		t.Error("the built-in users table must not be re-modelled")
	}
	if _, ok := byName["empty"]; ok {
		t.Error("an entity with no fields should be dropped")
	}
	if tag, ok := byName["tag"]; !ok || tag.Table != "tags" {
		t.Errorf("a missing table name should be pluralised, got %+v", tag)
	}

	bm, ok := byName["bookmark"]
	if !ok {
		t.Fatalf("bookmark entity missing: %+v", ents)
	}
	fields := map[string]orchestrator.EntityField{}
	for _, f := range bm.Fields {
		fields[f.Name] = f
	}
	if f := fields["url"]; f.Type != "string" || f.Nullable || !f.Unique {
		t.Errorf("url flags wrong: %+v", f)
	}
	if f := fields["notes"]; f.Type != "text" || !f.Nullable {
		t.Errorf("notes flags wrong: %+v", f)
	}
	if f := fields["tag_id"]; f.Ref != "tags.id" || !f.Index {
		t.Errorf("foreign key not parsed: %+v", f)
	}
	if f := fields["archived"]; f.Type != "bool" || f.Default != "false" {
		t.Errorf("default not parsed: %+v", f)
	}
	if _, present := fields["id"]; present {
		t.Error("id is added by the generator and must not be listed")
	}
}

// TestProjectStateRendersCompactly checks the shared state block injected into
// planning and every child: key/value lines, not JSON, so it costs few tokens.
func TestProjectStateRendersCompactly(t *testing.T) {
	s := orchestrator.State{
		Initialized: true, Stack: "fastapi", Project: "bookmarks", App: "api",
		Settings: "app/config.py", Entry: "uvicorn app.main:app",
		Layout: []string{"app/main.py", "app/config.py"},
	}
	out := s.Render()

	for _, want := range []string{"initialized: true", "stack: fastapi", "project=bookmarks", "app=api", "run: uvicorn app.main:app", "app/main.py"} {
		if !strings.Contains(out, want) {
			t.Errorf("state render missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "{") {
		t.Errorf("state should render as plain lines, not JSON:\n%s", out)
	}
	if empty := (orchestrator.State{}).Render(); empty != "initialized: false" {
		t.Errorf("empty state = %q", empty)
	}
}

// TestOSFilesChecksTargetsUnderTheProject checks the structural verification the
// grounding gate depends on.
func TestOSFilesChecksTargetsUnderTheProject(t *testing.T) {
	dir := tempProject(t, map[string]string{"app/main.py": "x\n"})
	fc := orchestrator.OSFiles{}

	if !fc.Exists(dir, "app/main.py") {
		t.Error("an existing target was reported missing")
	}
	if !fc.Exists(dir, "  app/main.py  ") {
		t.Error("a padded path should be trimmed before checking")
	}
	if fc.Exists(dir, "app/missing.py") {
		t.Error("a missing target was reported present")
	}
	if err := os.MkdirAll(filepath.Join(dir, "static"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func keysOf(m map[string]orchestrator.APIEndpoint) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
