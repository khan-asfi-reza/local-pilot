package orchestrator

import (
	"context"

	"harness/harness/events"
)

type ExecFunc func(ctx context.Context, t SubTask, mem *Memory) Result

// Schedule runs the DAG honoring deps, up to Policy.MaxParallel at once. One
// owner goroutine holds all state; workers only report on a channel. A failed
// task cascade-skips its dependents; independent branches continue.
func Schedule(ctx context.Context, d *DAG, mem *Memory, pol Policy, exec ExecFunc, emit func(events.Event)) map[string]Result {
	if pol.MaxParallel < 1 {
		pol.MaxParallel = 1
	}
	results := map[string]Result{}
	done := map[string]bool{}
	failed := map[string]bool{}
	started := map[string]bool{}
	inflight := 0
	completions := make(chan Result)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	all := d.TopoIDs()
	total := len(all)
	settled := 0

	launch := func(t SubTask) {
		started[t.ID] = true
		inflight++
		emit(events.Text("\n▸ " + t.ID + ": " + t.Title + "\n"))
		go func() { completions <- runWithRetry(ctx, t, mem, pol, exec) }()
	}

	skipRest := func() {
		for _, id := range all {
			if !started[id] && !done[id] && !failed[id] {
				results[id] = skipped(d.Task(id))
				failed[id] = true
				started[id] = true
				settled++
			}
		}
	}

	for settled < total {
		for _, id := range d.Ready(done, failed, started) {
			if inflight >= pol.MaxParallel {
				break
			}
			launch(d.Task(id))
		}
		if inflight == 0 {
			skipRest()
			break
		}
		select {
		case <-ctx.Done():
			return results
		case r := <-completions:
			inflight--
			settled++
			results[r.Task.ID] = r
			if r.Status == StatusDone {
				done[r.Task.ID] = true
				mem.Add(Note{TaskID: r.Task.ID, Title: r.Task.Title, Files: r.Written, Summary: r.Summary})
				emit(events.Text("✓ " + r.Task.ID + " done\n"))
			} else {
				failed[r.Task.ID] = true
				emit(events.Text("✗ " + r.Task.ID + " failed: " + r.Summary + "\n"))
				for _, dep := range d.TransitiveDependents(r.Task.ID) {
					if !started[dep] && !done[dep] && !failed[dep] {
						results[dep] = skipped(d.Task(dep))
						failed[dep] = true
						started[dep] = true
						settled++
					}
				}
				if pol.AbortOnFail {
					cancel()
				}
			}
		}
	}
	return results
}

func runWithRetry(ctx context.Context, t SubTask, mem *Memory, pol Policy, exec ExecFunc) Result {
	attempts := pol.MaxRetries + 1
	if attempts < 1 {
		attempts = 1
	}
	var last Result
	for i := 0; i < attempts; i++ {
		select {
		case <-ctx.Done():
			return Result{Task: t, Status: StatusFailed, Attempts: i, Summary: "canceled"}
		default:
		}
		last = exec(ctx, t, mem)
		last.Task = t
		last.Attempts = i + 1
		if last.Status == StatusDone {
			return last
		}
	}
	if last.Status == "" {
		last.Status = StatusFailed
	}
	return last
}

func skipped(t SubTask) Result {
	return Result{Task: t, Status: StatusSkipped, Summary: "skipped (a dependency failed)"}
}
