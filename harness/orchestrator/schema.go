package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const intakeSchema = `{"type":"object","properties":{` +
	`"action":{"type":"string","enum":["create","edit","fix","question","other"]},` +
	`"file_count":{"type":"integer"},` +
	`"explicit_targets":{"type":"array","items":{"type":"string"}},` +
	`"acceptance_criteria":{"type":"array","items":{"type":"string"}}` +
	`},"required":["action","file_count","explicit_targets","acceptance_criteria"]}`

const enrichSchema = `{"type":"object","properties":{"spec":{"type":"string"}},"required":["spec"]}`

const decomposeSchema = `{"type":"object","properties":{"tasks":{"type":"array","items":{"type":"object","properties":{` +
	`"id":{"type":"string"},` +
	`"title":{"type":"string"},` +
	`"description":{"type":"string"},` +
	`"deps":{"type":"array","items":{"type":"string"}},` +
	`"target_files":{"type":"array","items":{"type":"string"}},` +
	`"acceptance":{"type":"array","items":{"type":"string"}},` +
	`"section_idx":{"type":"integer"}` +
	`},"required":["id","title","description","deps","target_files","acceptance","section_idx"]}}},"required":["tasks"]}`

// Intake classifies a request into a grounding contract with one stateless call.
func Intake(ctx context.Context, p Planner, prompt string) (*Contract, error) {
	sys := "You are a task classifier. Read the user's request and extract a grounding contract. " +
		"explicit_targets = the EXACT file names or paths the user literally named (e.g. \"area.c\", " +
		"\"backend/main.py\"); use [] if the user named no specific file. Never invent a target. " +
		"action = create|edit|fix for a coding change, question for a pure question, other otherwise. " +
		"file_count = your best estimate of how many distinct files or components building this task would take " +
		"(count the files it implies, even if not named). " +
		"acceptance_criteria = the concrete things that must hold for the task to be done. Output ONLY the JSON."
	raw, err := p.PlanJSON(ctx, sys, clip(prompt, 8000), json.RawMessage(intakeSchema))
	if err != nil {
		return nil, err
	}
	var c Contract
	if json.Unmarshal([]byte(raw), &c) != nil {
		return nil, err
	}
	return &c, nil
}

// Enrich expands a vague request into a concrete, section-organized spec.
func Enrich(ctx context.Context, p Planner, prompt string) (string, error) {
	sys := "You expand a short or vague software request into a concrete, buildable specification. " +
		"Make sensible concrete choices (stack, files, features) and organize it under Markdown headings, " +
		"one per independent part, so it can be built section by section. Real file names and behaviors, no " +
		"fluff. Output ONLY the JSON with a single 'spec' string."
	raw, err := p.PlanJSON(ctx, sys, clip(prompt, 8000), json.RawMessage(enrichSchema))
	if err != nil {
		return "", err
	}
	var out struct {
		Spec string `json:"spec"`
	}
	if json.Unmarshal([]byte(raw), &out) != nil {
		return "", err
	}
	return out.Spec, nil
}

// Decompose turns a contract plus a section outline into a sub-task DAG. It is
// fed the outline only, never full section bodies.
func Decompose(ctx context.Context, p Planner, c *Contract, outline, stateText string) (Plan, error) {
	sys := "You break a software project into a DAG of INDEPENDENT sub-tasks a small model can each finish alone. " +
		"Follow these rules for a good plan:\n" +
		"1. Prefer FEW, coherent sub-tasks. Group tightly-coupled files into ONE task (a web app's models, " +
		"serializers, views and routes belong together; a config/settings module is one task; the test suite is " +
		"one task; each frontend screen can be its own task). Do NOT split coupled files across tasks or make many " +
		"tiny tasks.\n" +
		"2. Keep dependencies MINIMAL so tasks run in parallel. Never create one giant foundational task that " +
		"everything else depends on; a small scaffold task plus mostly-parallel feature tasks is better.\n" +
		"3. Use a FLAT, conventional layout with files at predictable paths from the project root.\n" +
		"4. Choose concrete, consistent names up front (project/package name, app name, config/settings module " +
		"path) and use those SAME names in every task's description and target_files, so independently-built files " +
		"import and wire together.\n" +
		"5. Include a task that creates the test/verification file(s) the acceptance criteria require.\n" +
		"6. COVER THE WHOLE PROJECT: across all tasks, target_files must include EVERY file needed to satisfy the " +
		"acceptance criteria — entrypoint/config, data layer, business logic, API endpoints, the test file, and any " +
		"frontend. A file only exists if it is listed under PROJECT STATE; assume nothing else exists yet. Do NOT " +
		"recreate files listed as existing unless the task must change them.\n" +
		"Each sub-task: short id (t1, t2, ...), a title, a description precise enough to build the files ALONE " +
		"(name the exact modules, routes, symbols and imports it must expose or use), deps (ids), target_files " +
		"(exact paths), acceptance, and section_idx (the outline number, or -1). No cycles. Output ONLY the JSON."
	user := "CONTRACT:\naction=" + c.Action + "\nacceptance=" + strings.Join(c.AcceptanceCriteria, "; ")
	if stateText != "" {
		user += "\n\nPROJECT STATE (already scaffolded — plan FEATURE tasks on top; reuse these names/layout):\n" + stateText
	}
	user += "\n\nSECTION OUTLINE:\n" + outline

	// The tool-call plan is occasionally empty/unparseable on a small model; retry
	// a few times before giving up so a transient miss doesn't collapse to one task.
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		raw, err := p.PlanJSON(ctx, sys, clip(user, 8000), json.RawMessage(decomposeSchema))
		if err != nil {
			lastErr = err
			continue
		}
		var plan Plan
		if json.Unmarshal([]byte(raw), &plan) == nil {
			if plan = normalizePlan(plan); len(plan.Tasks) > 0 {
				return plan, nil
			}
		}
	}
	if lastErr != nil {
		return Plan{}, lastErr
	}
	return Plan{}, fmt.Errorf("decompose produced no tasks")
}

func normalizePlan(p Plan) Plan {
	seen := map[string]bool{}
	var out []SubTask
	for _, t := range p.Tasks {
		if strings.TrimSpace(t.ID) == "" || seen[t.ID] {
			continue
		}
		seen[t.ID] = true
		out = append(out, t)
	}
	for i := range out {
		var deps []string
		for _, d := range out[i].Deps {
			if seen[d] && d != out[i].ID {
				deps = append(deps, d)
			}
		}
		out[i].Deps = deps
	}
	return Plan{Tasks: out}
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
