// Package agent is the harness brain: it runs the ReAct loop over one model,
// dispatches tools, and shapes the context the model sees. It is stateless per
// request; history lives in the caller.
package agent

import (
	"harness/harness/model"
	"harness/harness/tools"
)

// Agent wires the router and tool registry together. One Agent serves many
// requests; it stores no conversation between them.
type Agent struct {
	cfg           *model.Config
	router        *model.Router
	reg           *tools.Registry
	skills        skillSet
	prompt        *Prompt
	maxSteps      int
	contextTokens int
}

// Request is one turn of work. It carries everything the harness needs, since
// the harness remembers nothing between calls.
type Request struct {
	Messages []model.Message
	Allowed  []string
	Mode     string
	WorkDir  string
}

// New builds an agent from a config and an optional skills directory.
func New(cfg *model.Config, skillsDir string) (*Agent, error) {
	client := model.NewClient()
	skills, err := scanSkills(skillsDir)
	if err != nil {
		return nil, err
	}
	contextTokens := cfg.ContextTokens
	if contextTokens <= 0 {
		contextTokens = 30000
	}
	prompt := LoadPrompt(cfg.Dir())
	reg := tools.NewRegistry(skills.names)
	reg.SetDescriptions(prompt.Tools)
	return &Agent{
		cfg:           cfg,
		router:        model.NewRouter(cfg, client),
		reg:           reg,
		skills:        skills,
		prompt:        prompt,
		maxSteps:      25,
		contextTokens: contextTokens,
	}, nil
}

// ToolNames returns every tool the harness knows, which the terminal uses as the
// full allowed set.
func (a *Agent) ToolNames() []string { return a.reg.Names() }

// SetMaxSteps raises or lowers the per-request step cap (non-positive ignored).
func (a *Agent) SetMaxSteps(n int) {
	if n > 0 {
		a.maxSteps = n
	}
}

// ActiveModel returns the model currently in use, for the status line.
func (a *Agent) ActiveModel() string { return a.router.Active() }

// ModelStatus describes a registered model and whether its backend is running.
type ModelStatus struct {
	Name    string
	URL     string
	Running bool
	Active  bool
}

// Models returns every registered model with a live check of whether its
// backend is up, so the terminal can show what is available and switchable.
func (a *Agent) Models() []ModelStatus {
	active := a.cfg.ActiveName()
	installed := map[string]bool{}
	for _, n := range a.router.InstalledModels() {
		installed[n] = true
	}
	var out []ModelStatus
	for _, name := range a.cfg.Names() {
		url, _ := a.cfg.URLFor(name)
		out = append(out, ModelStatus{
			Name:    name,
			URL:     url,
			Running: a.router.Reachable(url) && installed[name],
			Active:  name == active,
		})
	}
	return out
}

// Reachable reports whether the active model's backend is up.
func (a *Agent) Reachable(name string) bool {
	url, ok := a.cfg.URLFor(name)
	return ok && a.router.Reachable(url)
}

// SetModel switches the active model to another registered name.
func (a *Agent) SetModel(name string) error { return a.cfg.SetActive(name) }
