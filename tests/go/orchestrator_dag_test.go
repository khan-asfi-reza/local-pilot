package systemtest

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"harness/harness/events"
	"harness/harness/orchestrator"
)

func plan(tasks ...orchestrator.SubTask) orchestrator.Plan { return orchestrator.Plan{Tasks: tasks} }

func task(id string, deps ...string) orchestrator.SubTask {
	return orchestrator.SubTask{ID: id, Title: "task " + id, Deps: deps}
}

func silent(events.Event) {}

// TestDAGRejectsUnbuildablePlans checks a plan a small model can plausibly emit
// is validated before anything runs: cycles, unknown deps, duplicate and empty
// ids all fail fast so the caller can fall back to sequential execution.
func TestDAGRejectsUnbuildablePlans(t *testing.T) {
	bad := map[string]orchestrator.Plan{
		"cycle":       plan(task("a", "b"), task("b", "a")),
		"self cycle":  plan(task("a", "a")),
		"unknown dep": plan(task("a", "ghost")),
		"duplicate":   plan(task("a"), task("a")),
		"empty id":    plan(task("")),
	}
	for name, p := range bad {
		if _, err := orchestrator.NewDAG(p); err == nil {
			t.Errorf("%s: NewDAG accepted an unbuildable plan", name)
		}
	}
}

// TestDAGOrdersTasksNaturally checks dependency-free tasks launch in natural
// numeric order, so t2 runs before t11 (a plain string sort put t11 first and
// ran later work before its logical predecessors).
func TestDAGOrdersTasksNaturally(t *testing.T) {
	d, err := orchestrator.NewDAG(plan(task("t3"), task("t11"), task("t1"), task("t2"), task("t12"), task("t10")))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"t1", "t2", "t3", "t10", "t11", "t12"}
	got := d.TopoIDs()
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("TopoIDs = %v, want %v", got, want)
		}
	}
}

// TestDAGReadyRespectsDependencies checks the readiness rule the scheduler uses.
func TestDAGReadyRespectsDependencies(t *testing.T) {
	d, err := orchestrator.NewDAG(plan(task("t1"), task("t2", "t1"), task("t3", "t1"), task("t4", "t2", "t3")))
	if err != nil {
		t.Fatal(err)
	}
	none := map[string]bool{}

	if got := d.Ready(none, none, none); len(got) != 1 || got[0] != "t1" {
		t.Fatalf("initially ready = %v, want [t1]", got)
	}
	done := map[string]bool{"t1": true}
	started := map[string]bool{"t1": true}
	got := d.Ready(done, none, started)
	if strings.Join(got, ",") != "t2,t3" {
		t.Fatalf("after t1 ready = %v, want t2 and t3 in parallel", got)
	}
	done["t2"] = true
	started["t2"], started["t3"] = true, true
	if got := d.Ready(done, none, started); len(got) != 0 {
		t.Fatalf("t4 must wait for BOTH deps, got %v", got)
	}

	if deps := d.TransitiveDeps("t4"); len(deps) != 3 {
		t.Errorf("TransitiveDeps(t4) = %v, want t1 t2 t3", deps)
	}
	if dependents := d.TransitiveDependents("t1"); len(dependents) != 3 {
		t.Errorf("TransitiveDependents(t1) = %v, want t2 t3 t4", dependents)
	}
}

// recorder tracks how sub-task execution actually interleaved.
type recorder struct {
	mu       sync.Mutex
	order    []string
	attempts map[string]int
	inFlight int
	peak     int
}

func newRecorder() *recorder { return &recorder{attempts: map[string]int{}} }

func (r *recorder) exec(fail map[string]bool, hold time.Duration) orchestrator.ExecFunc {
	return func(ctx context.Context, t orchestrator.SubTask, mem *orchestrator.Memory) orchestrator.Result {
		r.mu.Lock()
		r.order = append(r.order, t.ID)
		r.attempts[t.ID]++
		r.inFlight++
		if r.inFlight > r.peak {
			r.peak = r.inFlight
		}
		r.mu.Unlock()

		time.Sleep(hold)

		r.mu.Lock()
		r.inFlight--
		r.mu.Unlock()

		if fail[t.ID] {
			return orchestrator.Result{Status: orchestrator.StatusFailed, Summary: "boom"}
		}
		return orchestrator.Result{Status: orchestrator.StatusDone, Summary: "built " + t.ID,
			Written: []string{t.ID + ".py"}}
	}
}

// TestScheduleRunsIndependentTasksInParallel checks the parallelism cap is
// honoured and dependent work still waits its turn.
func TestScheduleRunsIndependentTasksInParallel(t *testing.T) {
	d, err := orchestrator.NewDAG(plan(task("t1"), task("t2"), task("t3"), task("t4"), task("t5", "t1", "t2", "t3", "t4")))
	if err != nil {
		t.Fatal(err)
	}
	rec := newRecorder()
	results := orchestrator.Schedule(context.Background(), d, orchestrator.NewMemory(),
		orchestrator.Policy{MaxParallel: 2}, rec.exec(nil, 20*time.Millisecond), silent)

	if len(results) != 5 {
		t.Fatalf("got %d results, want 5", len(results))
	}
	for id, r := range results {
		if r.Status != orchestrator.StatusDone {
			t.Errorf("%s = %s (%s)", id, r.Status, r.Summary)
		}
	}
	if rec.peak > 2 {
		t.Errorf("%d sub-tasks ran at once, the cap was 2", rec.peak)
	}
	if rec.peak < 2 {
		t.Error("independent sub-tasks did not run in parallel at all")
	}
	if rec.order[len(rec.order)-1] != "t5" {
		t.Errorf("the dependent task did not run last: %v", rec.order)
	}
}

// TestScheduleRetriesAFailingSubTask checks a sub-task is retried up to the
// policy cap and the attempt count is reported.
func TestScheduleRetriesAFailingSubTask(t *testing.T) {
	d, err := orchestrator.NewDAG(plan(task("t1")))
	if err != nil {
		t.Fatal(err)
	}
	rec := newRecorder()
	results := orchestrator.Schedule(context.Background(), d, orchestrator.NewMemory(),
		orchestrator.Policy{MaxParallel: 1, MaxRetries: 2}, rec.exec(map[string]bool{"t1": true}, 0), silent)

	if got := rec.attempts["t1"]; got != 3 {
		t.Errorf("t1 ran %d times, want 3 (one attempt plus two retries)", got)
	}
	r := results["t1"]
	if r.Status != orchestrator.StatusFailed || r.Attempts != 3 {
		t.Errorf("result = %+v", r)
	}
}

// TestScheduleKeepsBuildingAfterAFailure checks a best-effort build: a failed
// sub-task neither stops independent work nor blocks its dependents, but it
// contributes nothing to the shared memory the others read.
func TestScheduleKeepsBuildingAfterAFailure(t *testing.T) {
	d, err := orchestrator.NewDAG(plan(task("t1"), task("t2", "t1"), task("t3")))
	if err != nil {
		t.Fatal(err)
	}
	mem := orchestrator.NewMemory()
	rec := newRecorder()
	results := orchestrator.Schedule(context.Background(), d, mem,
		orchestrator.Policy{MaxParallel: 2}, rec.exec(map[string]bool{"t1": true}, 0), silent)

	if results["t1"].Status != orchestrator.StatusFailed {
		t.Fatalf("t1 should have failed: %+v", results["t1"])
	}
	if results["t2"].Status != orchestrator.StatusDone {
		t.Errorf("a dependent must still build against whatever exists: %+v", results["t2"])
	}
	if results["t3"].Status != orchestrator.StatusDone {
		t.Errorf("an independent branch must continue: %+v", results["t3"])
	}
	if digest := mem.Digest([]string{"t1"}); digest != "" {
		t.Errorf("a failed sub-task left a note in shared memory: %q", digest)
	}
}

// TestScheduleAbortsWhenPolicySaysSo checks AbortOnFail stops the run instead of
// grinding through the rest of the plan.
func TestScheduleAbortsWhenPolicySaysSo(t *testing.T) {
	d, err := orchestrator.NewDAG(plan(task("t1"), task("t2", "t1"), task("t3", "t2")))
	if err != nil {
		t.Fatal(err)
	}
	rec := newRecorder()
	done := make(chan map[string]orchestrator.Result, 1)
	go func() {
		done <- orchestrator.Schedule(context.Background(), d, orchestrator.NewMemory(),
			orchestrator.Policy{MaxParallel: 1, AbortOnFail: true}, rec.exec(map[string]bool{"t1": true}, 0), silent)
	}()

	select {
	case results := <-done:
		if results["t3"].Status == orchestrator.StatusDone {
			t.Error("the run continued to the end despite AbortOnFail")
		}
	case <-timeoutAfterSeconds(10):
		t.Fatal("Schedule hung after an aborting failure")
	}
}

// TestMemoryIsScopedToDependencies checks the only channel between sub-tasks:
// a child sees distilled notes from its own dependencies and nothing else.
func TestMemoryIsScopedToDependencies(t *testing.T) {
	mem := orchestrator.NewMemory()
	mem.Add(orchestrator.Note{TaskID: "t1", Title: "database", Files: []string{"db.py"}, Summary: "created the bookmarks table"})
	mem.Add(orchestrator.Note{TaskID: "t2", Title: "auth", Files: []string{"auth.py"}, Summary: "issued JWTs"})
	mem.Add(orchestrator.Note{TaskID: "t3", Title: "frontend", Files: []string{"App.jsx"}, Summary: "listed bookmarks"})

	digest := mem.Digest([]string{"t1", "t3"})
	if !strings.Contains(digest, "database") || !strings.Contains(digest, "frontend") {
		t.Fatalf("digest lost a dependency: %q", digest)
	}
	if strings.Contains(digest, "auth") {
		t.Errorf("digest leaked an unrelated sub-task: %q", digest)
	}
	if !strings.Contains(digest, "db.py") {
		t.Errorf("digest dropped the files a dependency wrote: %q", digest)
	}
	if mem.Digest(nil) != "" {
		t.Error("a task with no dependencies should see an empty digest")
	}
	if mem.Digest([]string{"never-ran"}) != "" {
		t.Error("an unknown dependency should contribute nothing")
	}
}

// TestMemoryDigestStaysWithinBudget checks the digest is capped, so a wide plan
// cannot push a small model's context over the edge.
func TestMemoryDigestStaysWithinBudget(t *testing.T) {
	mem := orchestrator.NewMemory()
	var ids []string
	for i := 0; i < 200; i++ {
		id := "t" + strings.Repeat("0", 0) + itoa(i)
		ids = append(ids, id)
		mem.Add(orchestrator.Note{TaskID: id, Title: "task " + id, Summary: strings.Repeat("detail ", 40)})
	}
	if n := len(mem.Digest(ids)); n > 4000 {
		t.Fatalf("digest is %d bytes; it must stay capped for a small context window", n)
	}
}

// TestMemoryIsSafeUnderConcurrentWriters checks the shared note store survives
// parallel sub-tasks reporting at once.
func TestMemoryIsSafeUnderConcurrentWriters(t *testing.T) {
	mem := orchestrator.NewMemory()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			mem.Add(orchestrator.Note{TaskID: itoa(i), Title: "t" + itoa(i)})
			_ = mem.Digest([]string{itoa(i)})
		}(i)
	}
	wg.Wait()
	if mem.Digest([]string{"7"}) == "" {
		t.Error("a concurrently added note went missing")
	}
}

// itoa is a tiny local helper so the tests do not depend on strconv formatting.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
