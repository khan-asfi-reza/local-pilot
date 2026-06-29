package tools

import (
	"encoding/json"
	"fmt"

	"harness/harness/events"
)

// loadSkillTool loads a skill's full instructions on demand. The parameter enum
// is generated from the known skill names, so the grammar prevents the model
// from naming a skill that does not exist.
func loadSkillTool(names []string) *Tool {
	enum, _ := json.Marshal(names)
	params := fmt.Sprintf(`{"type":"object","properties":{"name":{"type":"string","enum":%s,"description":"Skill to load."}},"required":["name"]}`, enum)
	return &Tool{
		Name:        "load_skill",
		Description: "Load the full instructions for a skill by name when the current task matches it. Only names shown in the skill catalog are valid.",
		Params:      json.RawMessage(params),
		Run: func(env Env, args Args) (any, *events.Diff, error) {
			name := args.Str("name")
			body, ok := env.Skills[name]
			if !ok {
				return nil, nil, fmt.Errorf("unknown skill %q", name)
			}
			return map[string]any{"name": name, "body": body}, nil, nil
		},
	}
}
