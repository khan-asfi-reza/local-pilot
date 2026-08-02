package orchestrator

import (
	"fmt"
	"sort"
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
	sort.Strings(ids)
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
		sort.Strings(ready)
		queue = append(queue, ready...)
	}
	if len(out) != len(ids) {
		return nil, fmt.Errorf("dependency cycle among sub-tasks")
	}
	return out, nil
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
			if failed[dep] || !done[dep] {
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
