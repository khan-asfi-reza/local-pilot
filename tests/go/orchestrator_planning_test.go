package systemtest

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"harness/harness/orchestrator"
)

// fakePlanner stands in for the local model's grammar-constrained JSON call. It
// replays scripted answers in order and records every prompt it was given, so a
// test can assert both the output handling and what the harness actually sent.
type fakePlanner struct {
	mu       sync.Mutex
	replies  []string
	errs     []error
	systems  []string
	users    []string
	schemas  []string
	callsMax int
}

func (f *fakePlanner) PlanJSON(ctx context.Context, system, user string, schema json.RawMessage) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.systems = append(f.systems, system)
	f.users = append(f.users, user)
	f.schemas = append(f.schemas, string(schema))
	i := len(f.users) - 1
	if i < len(f.errs) && f.errs[i] != nil {
		return "", f.errs[i]
	}
	if i < len(f.replies) {
		return f.replies[i], nil
	}
	if f.callsMax > 0 && i >= f.callsMax {
		return "", context.Canceled
	}
	return "", nil
}

func (f *fakePlanner) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.users)
}

const samplePRD = `Build a bookmarks service.

# Backend
A FastAPI service with a bookmarks table and CRUD endpoints under /api/bookmarks.

# Frontend
A React page that lists bookmarks.
It also lets the user add one, and uses react-router for navigation.

## Notes
`

// TestSplitSectionsNeverShowsTheWholePRD checks the chunking that guarantees no
// single model call sees the entire specification.
func TestSplitSectionsNeverShowsTheWholePRD(t *testing.T) {
	secs := orchestrator.SplitSections(samplePRD)
	if len(secs) < 3 {
		t.Fatalf("expected an intro plus two real sections, got %d: %+v", len(secs), secs)
	}
	if secs[0].Title != "" || !strings.Contains(secs[0].Body, "bookmarks service") {
		t.Errorf("text before the first heading should be an untitled intro: %+v", secs[0])
	}
	if secs[1].Title != "Backend" || !strings.Contains(secs[1].Body, "FastAPI") {
		t.Errorf("bad backend section: %+v", secs[1])
	}
	for i, s := range secs {
		if s.Index != i {
			t.Errorf("section %d carries index %d", i, s.Index)
		}
	}

	// A numbered heading (a .docx exported to text) splits too, but a long
	// numbered list item must not be mistaken for one.
	if got := orchestrator.SplitSections("1. Overview\nvision\n2. Roles\nadmin, user"); len(got) != 2 {
		t.Errorf("numbered headings did not split: %d sections", len(got))
	}
	long := "1. Media assets: at least three images converted to webp on upload plus one short demo video"
	if got := orchestrator.SplitSections(long); len(got) != 1 {
		t.Errorf("a long numbered list item was treated as a heading: %d sections", len(got))
	}
	if got := orchestrator.SplitSections("no headings at all"); len(got) != 1 {
		t.Errorf("a headingless document should be a single section, got %d", len(got))
	}
}

// TestOutlineIsTitlesAndPreviewsOnly checks the decompose call is fed an outline,
// not the section bodies.
func TestOutlineIsTitlesAndPreviewsOnly(t *testing.T) {
	secs := orchestrator.SplitSections(samplePRD)
	outline := orchestrator.Outline(secs)

	if !strings.Contains(outline, "Backend") || !strings.Contains(outline, "Frontend") {
		t.Fatalf("outline lost section titles: %s", outline)
	}
	if strings.Contains(outline, "react-router") {
		t.Errorf("outline leaked past the first line of a section body: %s", outline)
	}
	if body := orchestrator.SectionBody(secs, 2); !strings.Contains(body, "react-router") {
		t.Errorf("SectionBody(2) lost the rest of the section: %q", body)
	}
	if body := orchestrator.SectionBody(secs, 1); !strings.Contains(body, "FastAPI") {
		t.Errorf("SectionBody(1) = %q", body)
	}
	if orchestrator.SectionBody(secs, 99) != "" {
		t.Error("SectionBody should be empty for an out-of-range index")
	}
}

// TestIntakeBuildsAGroundingContract checks request classification: the contract
// the rest of the run is verified against.
func TestIntakeBuildsAGroundingContract(t *testing.T) {
	p := &fakePlanner{replies: []string{
		`{"action":"fix","file_count":1,"explicit_targets":["calc.py"],"acceptance_criteria":["tests pass"]}`,
	}}

	c, err := orchestrator.Intake(context.Background(), p, "fix the subtraction bug in calc.py")
	if err != nil {
		t.Fatal(err)
	}
	if c.Action != "fix" || c.FileCount != 1 {
		t.Errorf("contract = %+v", c)
	}
	if len(c.ExplicitTargets) != 1 || c.ExplicitTargets[0] != "calc.py" {
		t.Errorf("explicit targets = %v", c.ExplicitTargets)
	}
	if !strings.Contains(p.schemas[0], `"action"`) {
		t.Errorf("intake did not constrain the answer to a schema: %s", p.schemas[0])
	}
}

// TestIntakeFailsLoudlyOnABrokenBackend checks a model server that cannot do
// grammar-constrained output surfaces an error instead of a silent empty
// contract. This is the failure mode that made builds stall with no explanation.
func TestIntakeFailsLoudlyOnABrokenBackend(t *testing.T) {
	p := &fakePlanner{errs: []error{context.DeadlineExceeded}}
	if _, err := orchestrator.Intake(context.Background(), p, "build a shop"); err == nil {
		t.Fatal("Intake swallowed a backend failure")
	}
}

// TestDecomposeRetriesThenNormalises checks the decompose call is retried when
// the small model returns junk, and that the accepted plan is cleaned up:
// duplicate ids dropped, dangling and self-referencing deps removed.
func TestDecomposeRetriesThenNormalises(t *testing.T) {
	good := `{"tasks":[
      {"id":"t1","title":"scaffold","description":"","deps":[],"target_files":["main.py"],"acceptance":[],"packages":[],"exposes":[],"section_idx":0},
      {"id":"t1","title":"duplicate","description":"","deps":[],"target_files":[],"acceptance":[],"packages":[],"exposes":[],"section_idx":0},
      {"id":"t2","title":"api","description":"","deps":["t1","ghost","t2"],"target_files":["api.py"],"acceptance":[],"packages":["fastapi"],"exposes":["GET /api/bookmarks"],"section_idx":1}
    ]}`
	p := &fakePlanner{replies: []string{"not json at all", good}}
	c := &orchestrator.Contract{Action: "create", AcceptanceCriteria: []string{"it runs"}}

	plan, err := orchestrator.Decompose(context.Background(), p, c, "0. Backend\n1. Frontend", "initialized: true")
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if p.calls() != 2 {
		t.Errorf("expected one retry after the unparseable answer, got %d calls", p.calls())
	}
	if len(plan.Tasks) != 2 {
		t.Fatalf("duplicate id was not dropped: %+v", plan.Tasks)
	}
	deps := plan.Tasks[1].Deps
	if len(deps) != 1 || deps[0] != "t1" {
		t.Errorf("deps not normalised (dangling/self refs must go): %v", deps)
	}
	if !strings.Contains(p.users[1], "PROJECT STATE") {
		t.Error("decompose did not carry the project state, so tasks cannot reuse existing names")
	}
}

// TestDecomposeGivesUpAfterThreeAttempts checks the retry loop is bounded, so a
// broken backend fails the run instead of spinning.
func TestDecomposeGivesUpAfterThreeAttempts(t *testing.T) {
	p := &fakePlanner{replies: []string{"junk", "junk", "junk"}}
	c := &orchestrator.Contract{Action: "create"}

	if _, err := orchestrator.Decompose(context.Background(), p, c, "0. Thing", ""); err == nil {
		t.Fatal("Decompose accepted three unparseable answers")
	}
	if p.calls() != 3 {
		t.Errorf("attempts = %d, want 3", p.calls())
	}
}

// TestEnrichExpandsAVagueRequest checks the spec-expansion call.
func TestEnrichExpandsAVagueRequest(t *testing.T) {
	p := &fakePlanner{replies: []string{`{"spec":"# Backend\nFastAPI service\n\n# Frontend\nReact page"}`}}

	spec, err := orchestrator.Enrich(context.Background(), p, "make me a bookmarks app")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(spec, "# Backend") {
		t.Errorf("enriched spec = %q", spec)
	}
	if len(orchestrator.SplitSections(spec)) != 2 {
		t.Error("an enriched spec must be section-organised so it can be built part by part")
	}
}

// TestProvisionPicksBackingServices checks the service pick is constrained to
// the dockerisable catalogue, and that a backend failure is reported rather than
// returned as "this project needs nothing".
func TestProvisionPicksBackingServices(t *testing.T) {
	p := &fakePlanner{replies: []string{`{"services":["postgres","redis"]}`}}

	svcs, err := orchestrator.Provision(context.Background(), p,
		"A Django API backed by PostgreSQL with Redis for caching.")
	if err != nil {
		t.Fatal(err)
	}
	if len(svcs) != 2 || svcs[0] != "postgres" || svcs[1] != "redis" {
		t.Fatalf("services = %v", svcs)
	}
	if !strings.Contains(p.schemas[0], "mongodb") {
		t.Errorf("the pick was not restricted to the catalogue: %s", p.schemas[0])
	}

	broken := &fakePlanner{errs: []error{context.DeadlineExceeded}}
	if _, err := orchestrator.Provision(context.Background(), broken, "postgres app"); err == nil {
		t.Fatal("a failed provisioning call must be reported, not read as no services")
	}
}
