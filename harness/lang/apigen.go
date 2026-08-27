package lang

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"harness/harness/events"
)

// apigen is the deterministic API/DB builder: given a data model (entities) + the
// framework, it writes migrations, models, and CRUD route skeletons so a small model
// only fills the marked LOGIC blanks instead of hand-writing all the plumbing. A
// framework with no builder returns Supports()==false and the caller falls back to
// the existing hand-write path. Generated CRUD handlers always compile and boot with
// a naive default, so an unfilled blank never breaks the app.

// BuildSpec is the generator input (a lang-local copy of the orchestrator contract,
// so lang stays a leaf package). BackendRoot is "" for a monolith or "backend" for a
// monorepo split; every generated path is written under workDir/BackendRoot.
type BuildSpec struct {
	Entities    []BuildEntity
	Endpoints   []BuildEndpoint
	BackendRoot string
}

type BuildEntity struct {
	Name   string // singular snake_case
	Table  string // plural
	Fields []BuildField
}

type BuildField struct {
	Name     string
	Type     string // logical: string|text|int|number|bool|date|datetime|id|uuid|email|json
	Nullable bool
	Unique   bool
	Default  string
	Ref      string // "table.column"
	Index    bool
}

type BuildEndpoint struct {
	Method string
	Path   string
	Auth   bool
	List   bool
}

// GenFile is one generated file: Path is relative to the backend root.
type GenFile struct {
	Path    string
	Content string
}

// BuildResult reports what was generated: the files written (relative to workDir) and
// human descriptors of the logic blanks left for the model to fill.
type BuildResult struct {
	Files  []string
	Blanks []string
}

// Builder is one framework's data-driven generator. Any emitter may be nil (e.g.
// FastAPI has no migration files — its models create the schema at boot).
type Builder struct {
	Framework  string
	Migrations func(BuildSpec) []GenFile
	Models     func(BuildSpec) []GenFile
	Routes     func(BuildSpec) ([]GenFile, []string)
}

var builders = map[string]*Builder{}

func registerBuilder(b *Builder) { builders[b.Framework] = b }

// Supports reports whether a deterministic API builder exists for the framework.
func Supports(framework string) bool {
	_, ok := builders[framework]
	return ok
}

// GenerateAPI runs the framework's builder and writes its files under workDir/
// BackendRoot, never clobbering an existing file (so a filled blank or a model edit
// survives a re-run). Returns the inventory + logic-blank descriptors.
func GenerateAPI(ctx context.Context, framework, workDir string, spec BuildSpec, emit func(events.Event)) (BuildResult, error) {
	_ = ctx
	b := builders[framework]
	if b == nil {
		return BuildResult{}, fmt.Errorf("no API builder for framework %q", framework)
	}
	if emit == nil {
		emit = func(events.Event) {}
	}
	var res BuildResult
	write := func(files []GenFile) {
		for _, f := range files {
			rel := filepath.Join(spec.BackendRoot, f.Path)
			abs := filepath.Join(workDir, rel)
			if _, err := os.Stat(abs); err == nil {
				continue // no-clobber
			}
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				continue
			}
			if err := os.WriteFile(abs, []byte(f.Content), 0o644); err == nil {
				res.Files = append(res.Files, filepath.ToSlash(rel))
			}
		}
	}
	if b.Migrations != nil {
		write(b.Migrations(spec))
	}
	if b.Models != nil {
		write(b.Models(spec))
	}
	if b.Routes != nil {
		files, blanks := b.Routes(spec)
		write(files)
		res.Blanks = append(res.Blanks, blanks...)
	}
	emit(events.Text(fmt.Sprintf("\n[api-builder: generated %d files for %d entities (%s)]\n", len(res.Files), len(spec.Entities), framework)))
	return res, nil
}

// ---- shared helpers ----

// orderByFK returns entities in an order where a referenced table precedes the one
// that references it, so migrations can run in file order without FK failures.
func orderByFK(entities []BuildEntity) []BuildEntity {
	idx := map[string]int{}
	for i, e := range entities {
		idx[e.Table] = i
	}
	state := map[string]int{} // 0 unvisited, 1 visiting, 2 done
	var out []BuildEntity
	var visit func(e BuildEntity)
	visit = func(e BuildEntity) {
		if state[e.Table] != 0 {
			return
		}
		state[e.Table] = 1
		for _, f := range e.Fields {
			if f.Ref == "" {
				continue
			}
			rt := strings.SplitN(f.Ref, ".", 2)[0]
			if rt != e.Table {
				if j, ok := idx[rt]; ok {
					visit(entities[j])
				}
			}
		}
		state[e.Table] = 2
		out = append(out, e)
	}
	for _, e := range entities {
		visit(e)
	}
	return out
}

func refTarget(ref string) (table, col string) {
	parts := strings.SplitN(ref, ".", 2)
	table = parts[0]
	col = "id"
	if len(parts) == 2 && parts[1] != "" {
		col = parts[1]
	}
	return
}

// pgType maps a logical field type to a Postgres column type.
func pgType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "int", "integer", "id":
		return "INTEGER"
	case "number", "float", "decimal":
		return "NUMERIC"
	case "bool", "boolean":
		return "BOOLEAN"
	case "date":
		return "DATE"
	case "datetime", "timestamp":
		return "TIMESTAMPTZ"
	case "uuid":
		return "UUID"
	case "json":
		return "JSONB"
	case "text":
		return "TEXT"
	default: // string, email, and anything unknown
		return "TEXT"
	}
}

// tsFieldType maps a logical type to a TypeScript type.
func tsFieldType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "int", "integer", "number", "float", "decimal", "id":
		return "number"
	case "bool", "boolean":
		return "boolean"
	case "json":
		return "any"
	default:
		return "string"
	}
}

// pySAType maps a logical type to a SQLAlchemy column type expression.
func pySAType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "int", "integer", "id":
		return "Integer"
	case "number", "float", "decimal":
		return "Float"
	case "bool", "boolean":
		return "Boolean"
	case "date":
		return "Date"
	case "datetime", "timestamp":
		return "DateTime"
	case "text":
		return "Text"
	default:
		return "String"
	}
}

// pyFieldType maps a logical type to a Python/Pydantic type.
func pyFieldType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "int", "integer", "id":
		return "int"
	case "number", "float", "decimal":
		return "float"
	case "bool", "boolean":
		return "bool"
	case "json":
		return "dict"
	default:
		return "str"
	}
}

func title(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "-", "_"), ".", "_")
	var b strings.Builder
	for _, part := range strings.Split(s, "_") {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return b.String()
}
