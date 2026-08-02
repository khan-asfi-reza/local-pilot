package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"harness/harness/events"
	"harness/harness/model"
	"harness/harness/tools"
)

func noopEmit(events.Event) {}

func TestSplitSections(t *testing.T) {
	secs := SplitSections("intro text\n\n# Backend\nbuild api\n\n## Frontend\nbuild ui")
	if len(secs) != 3 {
		t.Fatalf("want 3 sections, got %d: %+v", len(secs), secs)
	}
	if secs[1].Title != "Backend" || !strings.Contains(secs[1].Body, "build api") {
		t.Errorf("bad backend section: %+v", secs[1])
	}
	if one := SplitSections("no headings here"); len(one) != 1 {
		t.Errorf("headingless doc should be one section, got %d", len(one))
	}
}

func TestNewDAGCycle(t *testing.T) {
	_, err := NewDAG(Plan{Tasks: []SubTask{
		{ID: "a", Deps: []string{"b"}},
		{ID: "b", Deps: []string{"a"}},
	}})
	if err == nil {
		t.Fatal("expected a cycle error")
	}
	d, err := NewDAG(Plan{Tasks: []SubTask{
		{ID: "a"}, {ID: "b", Deps: []string{"a"}},
	}})
	if err != nil {
		t.Fatalf("valid plan errored: %v", err)
	}
	if got := d.TransitiveDeps("b"); len(got) != 1 || got[0] != "a" {
		t.Errorf("TransitiveDeps(b) = %v", got)
	}
}

func TestScheduleDepsAndCascadeSkip(t *testing.T) {
	dag, _ := NewDAG(Plan{Tasks: []SubTask{
		{ID: "t1"},
		{ID: "t2", Deps: []string{"t1"}},
		{ID: "t3", Deps: []string{"t2"}},
		{ID: "t4"},
	}})
	var mu sync.Mutex
	ran := map[string]bool{}
	exec := func(ctx context.Context, tk SubTask, mem *Memory) Result {
		mu.Lock()
		ran[tk.ID] = true
		mu.Unlock()
		if tk.ID == "t2" {
			return Result{Status: StatusFailed, Summary: "boom"}
		}
		return Result{Status: StatusDone, Written: []string{tk.ID + ".txt"}}
	}
	res := Schedule(context.Background(), dag, NewMemory(), Policy{MaxParallel: 4, MaxRetries: 0}, exec, noopEmit)

	if res["t1"].Status != StatusDone || res["t4"].Status != StatusDone {
		t.Errorf("t1/t4 should be done: %v %v", res["t1"].Status, res["t4"].Status)
	}
	if res["t2"].Status != StatusFailed {
		t.Errorf("t2 should be failed: %v", res["t2"].Status)
	}
	if res["t3"].Status != StatusSkipped {
		t.Errorf("t3 should be skipped (dep failed): %v", res["t3"].Status)
	}
	if ran["t3"] {
		t.Error("t3 must never execute when its dep failed")
	}
}

func TestScheduleParallelBound(t *testing.T) {
	dag, _ := NewDAG(Plan{Tasks: []SubTask{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}})
	var mu sync.Mutex
	cur, max := 0, 0
	exec := func(ctx context.Context, tk SubTask, mem *Memory) Result {
		mu.Lock()
		cur++
		if cur > max {
			max = cur
		}
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		cur--
		mu.Unlock()
		return Result{Status: StatusDone}
	}
	Schedule(context.Background(), dag, NewMemory(), Policy{MaxParallel: 2}, exec, noopEmit)
	if max > 2 {
		t.Errorf("parallelism exceeded bound: observed %d", max)
	}
	if max < 2 {
		t.Errorf("parallelism never reached the bound: observed %d", max)
	}
}

type fakePlanner struct{ plan Plan }

func (f fakePlanner) PlanJSON(ctx context.Context, system, user string, schema json.RawMessage) (string, error) {
	s := string(schema)
	switch {
	case strings.Contains(s, `"size"`):
		return `{"size":"large","is_question":false,"action":"create","explicit_targets":["a.txt"],"acceptance_criteria":[]}`, nil
	case strings.Contains(s, `"spec"`):
		return `{"spec":"# Part A\nbody"}`, nil
	case strings.Contains(s, `"scaffold"`):
		return `{"stack":"test","project":"config","app":"api","settings":"config.settings","entry":"run","scaffold":[{"path":"manage.py","purpose":"entry"}]}`, nil
	default:
		raw, _ := json.Marshal(f.plan)
		return string(raw), nil
	}
}

type recExecutor struct {
	mu    sync.Mutex
	calls int
}

func (e *recExecutor) RunChild(ctx context.Context, prompt string, spec ChildSpec, emit func(events.Event), confirm tools.ConfirmFunc) []model.Message {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()
	return nil
}

type fakeFC struct{ all bool }

func (f fakeFC) Exists(workDir, rel string) bool { return f.all }

func TestExecuteLargeDecomposes(t *testing.T) {
	plan := Plan{Tasks: []SubTask{
		{ID: "t1", TargetFiles: []string{"a.txt"}},
		{ID: "t2", Deps: []string{"t1"}, TargetFiles: []string{"b.txt"}},
	}}
	ex := &recExecutor{}
	o := New(fakePlanner{plan: plan}, ex, fakeFC{all: true}, Policy{MaxParallel: 2, MaxRetries: 0})
	sum := o.Execute(context.Background(), "a big prompt",
		&Contract{Action: "create", ExplicitTargets: []string{"a.txt", "b.txt"}},
		ChildSpec{WorkDir: t.TempDir()}, noopEmit, nil)
	// 1 init child (uninitialized dir) + 2 sub-task children
	if ex.calls != 3 {
		t.Errorf("expected 3 child runs (init + 2 sub-tasks), got %d", ex.calls)
	}
	if len(sum.Failed) != 0 {
		t.Errorf("no task should fail when targets exist: %v", sum.Failed)
	}
}

func TestExecuteEmptyPlanFallsBackToSingle(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644) // initialized -> skip init
	ex := &recExecutor{}
	o := New(fakePlanner{}, ex, fakeFC{all: true}, Policy{MaxParallel: 2})
	o.Execute(context.Background(), "big doc", &Contract{Action: "create"}, ChildSpec{WorkDir: dir}, noopEmit, nil)
	if ex.calls != 1 {
		t.Errorf("empty plan should fall back to one child run, got %d", ex.calls)
	}
}

func TestExecuteMissingTargetFails(t *testing.T) {
	plan := Plan{Tasks: []SubTask{{ID: "t1", TargetFiles: []string{"never.txt"}}}}
	o := New(fakePlanner{plan: plan}, &recExecutor{}, fakeFC{all: false}, Policy{MaxParallel: 1, MaxRetries: 0})
	sum := o.Execute(context.Background(), "big",
		&Contract{Action: "create"}, ChildSpec{WorkDir: t.TempDir()}, noopEmit, nil)
	if len(sum.Failed) != 1 {
		t.Errorf("a sub-task whose target never appears must fail: %+v", sum.Results)
	}
}
