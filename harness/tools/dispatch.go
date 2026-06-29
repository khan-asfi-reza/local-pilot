package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"harness/harness/events"
	"harness/harness/model"
)

// Mode names. The mode is a permission policy enforced here in dispatch, not by
// the model.
const (
	ModePlan = "plan"
	ModeAsk  = "ask"
	ModeAuto = "auto"
)

// Registry holds the tool set and preserves a stable order for display.
type Registry struct {
	tools map[string]*Tool
	order []string
}

// NewRegistry builds the v1 tool set. load_skill is only registered when skills
// exist, so its name enum is never empty.
func NewRegistry(skillNames []string) *Registry {
	r := &Registry{tools: map[string]*Tool{}}
	add := func(t *Tool) {
		r.tools[t.Name] = t
		r.order = append(r.order, t.Name)
	}
	add(searchTool())
	add(listDirTool())
	add(readFileTool())
	add(writeFileTool())
	add(editFileTool())
	add(shellRunTool())
	add(serveTool())
	add(codeRunTool())
	add(webSearchTool())
	if len(skillNames) > 0 {
		add(loadSkillTool(skillNames))
	}
	return r
}

// Get returns a tool by name, or nil.
func (r *Registry) Get(name string) *Tool { return r.tools[name] }

// Names returns every registered tool name in order.
func (r *Registry) Names() []string { return append([]string(nil), r.order...) }

// Defs builds the native tool definitions to send to the model in the tools
// array, limited to the allowed set. An empty allowed set means every tool is
// offered. When includeMutating is false (plan mode), mutating tools are left
// out, so the model cannot even attempt a change and loop on refusals.
func (r *Registry) Defs(allowed []string, includeMutating bool) []model.ToolDef {
	allow := toSet(allowed)
	var defs []model.ToolDef
	for _, name := range r.order {
		if len(allow) > 0 && !allow[name] {
			continue
		}
		t := r.tools[name]
		if !includeMutating && t.Mutating {
			continue
		}
		defs = append(defs, model.ToolDef{
			Type: "function",
			Function: model.ToolFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Params,
			},
		})
	}
	return defs
}

// SetDescriptions overrides tool descriptions from an external prompt file, so
// the tool docs can be tuned without a rebuild. Names not present are left with
// their built-in description; unknown names are ignored.
func (r *Registry) SetDescriptions(desc map[string]string) {
	for name, d := range desc {
		if t := r.tools[name]; t != nil && strings.TrimSpace(d) != "" {
			t.Description = d
		}
	}
}

// Describe returns human-readable docs for the allowed tools, to put in the
// planner prompt, and the list of allowed tool names, to build the action
// schema enum. An empty allowed set describes every tool. When includeMutating
// is false (plan mode), mutating tools are left out entirely, so the model is
// not even offered a way to change things and cannot loop on refusals.
func (r *Registry) Describe(allowed []string, includeMutating bool) (docs string, names []string) {
	allow := toSet(allowed)
	var b strings.Builder
	for _, name := range r.order {
		if len(allow) > 0 && !allow[name] {
			continue
		}
		t := r.tools[name]
		if !includeMutating && t.Mutating {
			continue
		}
		names = append(names, name)
		fmt.Fprintf(&b, "- %s: %s\n  arguments (JSON schema): %s\n", t.Name, t.Description, string(t.Params))
	}
	return strings.TrimRight(b.String(), "\n"), names
}

// Dispatch runs one tool call through the two gates and returns the result as
// the string that becomes the tool message, plus any diff to render.
//
// The first gate is the allowed set: even if the model asks for a tool, dispatch
// refuses one the request did not permit. The second gate is the mode: a
// mutating tool is refused in plan mode and paused for approval in ask mode.
func (r *Registry) Dispatch(tc model.ToolCall, allowed []string, mode string, env Env, confirm ConfirmFunc) (string, *events.Diff) {
	name := tc.Function.Name
	if allow := toSet(allowed); len(allow) > 0 && !allow[name] {
		return errString("This tool is not allowed for this request."), nil
	}
	tool := r.tools[name]
	if tool == nil {
		return errString("Unknown tool."), nil
	}

	args, err := parseArgs(tc.Function.Arguments)
	if err != nil {
		return errString("Could not parse tool arguments: " + err.Error()), nil
	}

	mutating := tool.Mutating
	if tool.MutatingWhen != nil {
		mutating = tool.MutatingWhen(args)
	}
	// A call reaching outside the working directory always needs approval.
	escapes := tool.EscapesSandbox != nil && tool.EscapesSandbox(env, args)

	if mutating && mode == ModePlan {
		return errString("Plan mode is read-only. This action was not run; include it in the plan instead."), nil
	}
	if escapes || (mutating && mode == ModeAsk) {
		if confirm != nil {
			summary, diff := previewOf(tool, env, args)
			if escapes {
				summary = "runs OUTSIDE the working directory — " + summary
			}
			decision, feedback := confirm(tool.Name, summary, diff)
			if decision == Decline {
				msg := "The user declined this action."
				if feedback != "" {
					msg += " Feedback from the user: " + feedback
				}
				return errString(msg), nil
			}
		} else if escapes {
			return errString("This command operates outside the working directory, which needs the user's approval that is not available in this run. Keep all paths inside the working directory."), nil
		}
	}

	result, diff, err := tool.Run(env, args)
	if err != nil {
		return errString(err.Error()), nil
	}
	return resultString(result), diff
}

// previewOf produces the confirmation summary and diff for a mutating tool,
// falling back to a generic summary if the tool has no preview.
func previewOf(tool *Tool, env Env, args Args) (string, *events.Diff) {
	if tool.Preview != nil {
		summary, diff, err := tool.Preview(env, args)
		if err == nil {
			return summary, diff
		}
		return tool.Name + ": " + err.Error(), nil
	}
	return "run " + tool.Name, nil
}

func parseArgs(raw string) (Args, error) {
	if strings.TrimSpace(raw) == "" {
		return Args{}, nil
	}
	var a Args
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		return nil, err
	}
	return a, nil
}

func resultString(result any) string {
	buf, err := json.Marshal(result)
	if err != nil {
		return errString("Could not encode tool result: " + err.Error())
	}
	return string(buf)
}

func errString(msg string) string {
	buf, _ := json.Marshal(map[string]string{"error": msg})
	return string(buf)
}

func toSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}
