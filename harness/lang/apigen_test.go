package lang

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleSpec() BuildSpec {
	return BuildSpec{
		BackendRoot: "backend",
		Entities: []BuildEntity{
			{Name: "book", Table: "books", Fields: []BuildField{
				{Name: "title", Type: "string", Nullable: false},
				{Name: "author_id", Type: "int", Ref: "authors.id", Index: true, Nullable: false},
			}},
			{Name: "author", Table: "authors", Fields: []BuildField{
				{Name: "name", Type: "string", Nullable: false, Unique: true},
				{Name: "bio", Type: "text", Nullable: true},
			}},
		},
		Endpoints: []BuildEndpoint{
			{Method: "GET", Path: "/api/books", List: true},
			{Method: "POST", Path: "/api/books", Auth: true},
			{Method: "GET", Path: "/api/authors", List: true},
		},
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
	return string(b)
}

// markersBalanced asserts every LOGIC block is opened and closed exactly once.
func markersBalanced(t *testing.T, src, path string) {
	t.Helper()
	open := strings.Count(src, "=== LOGIC:")
	closed := strings.Count(src, "=== END LOGIC ===")
	if open == 0 {
		t.Errorf("%s: no LOGIC blocks", path)
	}
	if open != closed {
		t.Errorf("%s: unbalanced LOGIC markers (%d open, %d close)", path, open, closed)
	}
}

func TestSupports(t *testing.T) {
	for _, fw := range []string{"express", "node", "fastapi"} {
		if !Supports(fw) {
			t.Errorf("Supports(%q) = false, want true", fw)
		}
	}
	if Supports("rails") {
		t.Errorf("Supports(rails) = true, want false (fallback framework)")
	}
}

func TestGenerateExpress(t *testing.T) {
	dir := t.TempDir()
	res, err := GenerateAPI(context.Background(), "express", dir, sampleSpec(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) == 0 || len(res.Blanks) == 0 {
		t.Fatalf("empty result: %+v", res)
	}

	// FK topo order: authors (010) before books (011).
	authorsMig := read(t, filepath.Join(dir, "backend/migrations/010_authors.sql"))
	if !strings.Contains(authorsMig, "CREATE TABLE IF NOT EXISTS authors") {
		t.Errorf("authors migration missing CREATE TABLE:\n%s", authorsMig)
	}
	if !strings.Contains(authorsMig, "name TEXT NOT NULL UNIQUE") {
		t.Errorf("authors migration missing unique name column:\n%s", authorsMig)
	}
	if !strings.Contains(authorsMig, "created_at TIMESTAMPTZ NOT NULL DEFAULT now()") {
		t.Errorf("authors migration missing created_at:\n%s", authorsMig)
	}
	booksMig := read(t, filepath.Join(dir, "backend/migrations/011_books.sql"))
	if !strings.Contains(booksMig, "REFERENCES authors(id) ON DELETE CASCADE") {
		t.Errorf("books migration missing FK:\n%s", booksMig)
	}
	if !strings.Contains(booksMig, "CREATE INDEX IF NOT EXISTS idx_books_author_id ON books(author_id)") {
		t.Errorf("books migration missing FK index:\n%s", booksMig)
	}

	model := read(t, filepath.Join(dir, "backend/src/models/book.ts"))
	if !strings.Contains(model, "export interface Book {") || !strings.Contains(model, "export interface NewBook {") {
		t.Errorf("book model missing interfaces:\n%s", model)
	}

	// The POST /api/books endpoint is auth:true → the router imports requireAuth.
	route := read(t, filepath.Join(dir, "backend/src/routes/books.ts"))
	markersBalanced(t, route, "books.ts")
	if !strings.Contains(route, "import { requireAuth } from '../auth'") {
		t.Errorf("books route missing requireAuth import (POST is auth:true):\n%s", route)
	}
	if !strings.Contains(route, "export default router;") {
		t.Errorf("books route missing default export")
	}
	if !strings.Contains(route, "INSERT INTO books (title, author_id) VALUES ($1, $2) RETURNING *") {
		t.Errorf("books route missing correct INSERT:\n%s", route)
	}
	// authors POST is not auth → no requireAuth.
	authorsRoute := read(t, filepath.Join(dir, "backend/src/routes/authors.ts"))
	if strings.Contains(authorsRoute, "requireAuth") {
		t.Errorf("authors route should NOT import requireAuth:\n%s", authorsRoute)
	}
}

func TestGenerateFastAPI(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateAPI(context.Background(), "fastapi", dir, BuildSpec{
		Entities:  sampleSpec().Entities,
		Endpoints: sampleSpec().Endpoints,
	}, nil); err != nil {
		t.Fatal(err)
	}
	// No BackendRoot → written at root (monolith python layout).
	model := read(t, filepath.Join(dir, "app/models/book.py"))
	if !strings.Contains(model, "class Book(Base):") || !strings.Contains(model, `ForeignKey("authors.id")`) {
		t.Errorf("book model wrong:\n%s", model)
	}
	if !strings.Contains(model, "class BookIn(BaseModel):") || !strings.Contains(model, "class BookOut(BookIn):") {
		t.Errorf("book model missing pydantic schemas:\n%s", model)
	}
	route := read(t, filepath.Join(dir, "app/routes/books.py"))
	markersBalanced(t, route, "books.py")
	if !strings.Contains(route, `router = APIRouter(prefix="/api/books"`) {
		t.Errorf("books route missing APIRouter prefix:\n%s", route)
	}
	if !strings.Contains(route, "from app.db import get_db") {
		t.Errorf("books route missing get_db import:\n%s", route)
	}
}

func TestUnsupportedFramework(t *testing.T) {
	if _, err := GenerateAPI(context.Background(), "rails", t.TempDir(), sampleSpec(), nil); err == nil {
		t.Errorf("expected error for unsupported framework, got nil (must fall back to hand-write)")
	}
}

func TestNoClobber(t *testing.T) {
	dir := t.TempDir()
	routePath := filepath.Join(dir, "backend/src/routes/books.ts")
	if err := os.MkdirAll(filepath.Dir(routePath), 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := "// FILLED BY THE MODEL — do not overwrite\n"
	if err := os.WriteFile(routePath, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateAPI(context.Background(), "express", dir, sampleSpec(), nil); err != nil {
		t.Fatal(err)
	}
	if got := read(t, routePath); got != sentinel {
		t.Errorf("no-clobber violated: existing route file was overwritten")
	}
}
