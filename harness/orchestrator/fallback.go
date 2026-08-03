package orchestrator

import "strings"

// inferServices scans the spec text for backing services in the catalog. It is
// the deterministic fallback when the planner pick fails or returns nothing, so
// provisioning never silently no-ops on a spec that clearly names its infra.
func inferServices(prompt string) []string {
	low := strings.ToLower(prompt)
	var out []string
	seen := map[string]bool{}
	add := func(k string) {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	if strings.Contains(low, "postgres") || strings.Contains(low, "postgresql") {
		add("postgres")
	}
	if strings.Contains(low, "mysql") || strings.Contains(low, "mariadb") {
		add("mysql")
	}
	if strings.Contains(low, "mongo") {
		add("mongodb")
	}
	if strings.Contains(low, "redis") {
		add("redis")
	}
	return out
}

// primaryDB returns the main database the spec names, so the scaffold can be
// grounded to it instead of a small model defaulting to the wrong store.
func primaryDB(prompt string) string {
	low := strings.ToLower(prompt)
	switch {
	case strings.Contains(low, "postgres"), strings.Contains(low, "postgresql"):
		return "PostgreSQL"
	case strings.Contains(low, "mysql"), strings.Contains(low, "mariadb"):
		return "MySQL"
	case strings.Contains(low, "mongo"):
		return "MongoDB"
	}
	return ""
}

// sectionPlan is the DETERMINISTIC decomposition used whenever the planner call
// fails or yields nothing. It maps each substantive spec section to one parallel
// sub-task, so the build NEVER collapses to a single whole-PRD agent run. The
// tasks declare no target files — they are verified by whatever files the child
// actually writes (see runAndVerify).
func sectionPlan(sections []Section, c *Contract) Plan {
	var tasks []SubTask
	n := 0
	for _, s := range sections {
		if len(strings.TrimSpace(s.Body)) < 40 {
			continue // skip thin intro/prose sections with nothing to build
		}
		n++
		title := s.Title
		if title == "" {
			title = "Part " + itoa(n)
		}
		tasks = append(tasks, SubTask{
			ID:    "s" + itoa(n),
			Title: title,
			Description: "Build everything this part of the specification describes. Create all files it " +
				"implies, at conventional root-relative paths, matching the project's canonical names, layout " +
				"and stack. Wire your files to the rest of the project by those shared names.",
			Deps:        nil, // all parallel; the scaffold already ran in initialize()
			TargetFiles: nil, // verified by files actually written
			Acceptance:  c.AcceptanceCriteria,
			SectionIdx:  s.Index,
		})
	}
	// Guarantee at least two tasks so the build never degrades to one agent.
	if len(tasks) == 0 {
		tasks = append(tasks, SubTask{
			ID: "s1", Title: "Core implementation", SectionIdx: -1, Acceptance: c.AcceptanceCriteria,
			Description: "Implement the core application described by the specification: data layer, business " +
				"logic and API endpoints, wired to the project's canonical names and stack.",
		})
	}
	if len(tasks) == 1 {
		tasks = append(tasks, SubTask{
			ID: "s" + itoa(len(tasks)+1), Title: "Tests & verification", SectionIdx: -1, Acceptance: c.AcceptanceCriteria,
			Description: "Create the test/verification files the acceptance criteria require, covering the " +
				"core behaviors of the application.",
		})
	}
	return Plan{Tasks: tasks}
}
