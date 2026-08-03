package orchestrator

import (
	"fmt"
	"sort"
	"strings"
)

type DAG struct {
	tasks      map[string]SubTask
	order      []string
	dependents map[string][]string
}

// NewDAG validates a plan (unique ids, resolvable deps, no cycle) and returns the
// graph, or an error so the caller can fall back to sequential execution.
func NewDAG(p Plan) (*DAG, error) {
	tasks := make(map[string]SubTask, len(p.Tasks))
	var ids []string
	for _, t := range p.Tasks {
		if t.ID == "" {
			return nil, fmt.Errorf("sub-task with empty id")
		}
		if _, dup := tasks[t.ID]; dup {
			return nil, fmt.Errorf("duplicate sub-task id %q", t.ID)
		}
		tasks[t.ID] = t
		ids = append(ids, t.ID)
	}
	sortIDs(ids)
	dependents := map[string][]string{}
	for _, id := range ids {
		for _, dep := range tasks[id].Deps {
			if _, ok := tasks[dep]; !ok {
				return nil, fmt.Errorf("sub-task %q depends on unknown %q", id, dep)
			}
			dependents[dep] = append(dependents[dep], id)
		}
	}
	order, err := topoOrder(tasks, ids)
	if err != nil {
		return nil, err
	}
	return &DAG{tasks: tasks, order: order, dependents: dependents}, nil
}

func topoOrder(tasks map[string]SubTask, ids []string) ([]string, error) {
	indeg := map[string]int{}
	for _, id := range ids {
		indeg[id] = len(uniqueDeps(tasks[id].Deps))
	}
	var queue []string
	for _, id := range ids {
		if indeg[id] == 0 {
			queue = append(queue, id)
		}
	}
	var out []string
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		out = append(out, n)
		var ready []string
		for _, id := range ids {
			for _, dep := range uniqueDeps(tasks[id].Deps) {
				if dep == n {
					indeg[id]--
					if indeg[id] == 0 {
						ready = append(ready, id)
					}
				}
			}
		}
		sortIDs(ready)
		queue = append(queue, ready...)
	}
	if len(out) != len(ids) {
		return nil, fmt.Errorf("dependency cycle among sub-tasks")
	}
	return out, nil
}

// sortIDs orders task ids naturally so "t2" precedes "t11". A plain string sort
// puts "t11" before "t2" (because '1' < '2'), which launched sub-tasks out of
// order — t11 ran before its logical predecessors.
func sortIDs(ids []string) {
	sort.Slice(ids, func(i, j int) bool { return lessNatural(ids[i], ids[j]) })
}

// lessNatural compares two strings with embedded digit runs compared numerically.
func lessNatural(a, b string) bool {
	for len(a) > 0 && len(b) > 0 {
		ad := a[0] >= '0' && a[0] <= '9'
		bd := b[0] >= '0' && b[0] <= '9'
		if ad && bd {
			ai, bi := 0, 0
			for ai < len(a) && a[ai] >= '0' && a[ai] <= '9' {
				ai++
			}
			for bi < len(b) && b[bi] >= '0' && b[bi] <= '9' {
				bi++
			}
			an := strings.TrimLeft(a[:ai], "0")
			bn := strings.TrimLeft(b[:bi], "0")
			if len(an) != len(bn) {
				return len(an) < len(bn)
			}
			if an != bn {
				return an < bn
			}
			a, b = a[ai:], b[bi:]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func uniqueDeps(deps []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range deps {
		if d != "" && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

func (d *DAG) TopoIDs() []string { return append([]string(nil), d.order...) }

func (d *DAG) Task(id string) SubTask { return d.tasks[id] }

// Ready returns sub-tasks whose deps are all done, none failed, not yet started.
func (d *DAG) Ready(done, failed, started map[string]bool) []string {
	var ready []string
	for _, id := range d.order {
		if started[id] || done[id] || failed[id] {
			continue
		}
		ok := true
		for _, dep := range uniqueDeps(d.tasks[id].Deps) {
			// A failed dep is marked done too (best-effort build), so only block on
			// a dep that has not finished at all.
			if !done[dep] {
				ok = false
				break
			}
		}
		if ok {
			ready = append(ready, id)
		}
	}
	return ready
}

func (d *DAG) TransitiveDeps(id string) []string {
	seen := map[string]bool{}
	var walk func(string)
	walk = func(cur string) {
		for _, dep := range uniqueDeps(d.tasks[cur].Deps) {
			if !seen[dep] {
				seen[dep] = true
				walk(dep)
			}
		}
	}
	walk(id)
	return sortedSet(seen)
}

func (d *DAG) TransitiveDependents(id string) []string {
	seen := map[string]bool{}
	var walk func(string)
	walk = func(cur string) {
		for _, dep := range d.dependents[cur] {
			if !seen[dep] {
				seen[dep] = true
				walk(dep)
			}
		}
	}
	walk(id)
	return sortedSet(seen)
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
