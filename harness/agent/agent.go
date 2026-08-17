// Package agent is the harness brain: it runs the ReAct loop over one model,
// dispatches tools, and shapes the context the model sees. It is stateless per
// request; history lives in the caller.
package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

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
	log           *auditLog
	bg            sync.WaitGroup // tracks detached background work (memory updates)
}

// Wait blocks until any background work (async memory updates) has finished. The
// headless runner calls it before exiting so a detached update is not killed
// mid-write; the long-lived TUI/server can ignore it.
func (a *Agent) Wait() { a.bg.Wait() }

// Request is one turn of work. It carries everything the harness needs, since
// the harness remembers nothing between calls.
type Request struct {
	Messages     []model.Message
	Allowed      []string
	Mode         string
	WorkDir      string
	Sandbox      bool       // web path: run code_run in an isolated sandbox, not the project
	Chat         bool       // conversational mode: answer inline, no project/file workflow
	InjectSkills []string   // skill names to inject silently regardless of detection
	Grounding    *Grounding // pinned named target(s); set by intake or the eval harness
	noTriage     bool       // internal: a child sub-task run — skip intake/orchestration/memory
}

// New builds an agent from a config and an optional skills directory. Alongside
// that directory's shipped "default" skills, it also scans a sibling
// "skills_local" directory for user-installed skills (e.g. added via
// `pilot skill add`), which is never touched by upgrades.
func New(cfg *model.Config, skillsDir string) (*Agent, error) {
	client := model.NewClient()
	var localDir string
	if skillsDir != "" {
		localDir = filepath.Join(filepath.Dir(skillsDir), "skills_local")
	}
	skills, err := scanSkills(skillsDir, localDir)
	if err != nil {
		return nil, err
	}
	contextTokens := cfg.ContextTokens
	if contextTokens <= 0 {
		contextTokens = 30000
	}
	prompt := LoadPrompt()
	reg := tools.NewRegistry(skills.names)
	reg.SetDescriptions(prompt.Tools)
	return &Agent{
		cfg:           cfg,
		router:        model.NewRouter(cfg, client),
		reg:           reg,
		skills:        skills,
		prompt:        prompt,
		maxSteps:      2000,
		contextTokens: contextTokens,
		log:           newAuditLog(),
	}, nil
}

// Reload re-reads the model registry from disk so models added by `pilot models
// add` appear without restarting. Rebuilds the config and router; tools, skills,
// and step limits are unchanged.
func (a *Agent) Reload(cfgPath string) error {
	cfg, err := model.LoadConfig(cfgPath)
	if err != nil {
		return err
	}
	a.cfg = cfg
	a.router = model.NewRouter(cfg, model.NewClient())
	a.prompt = LoadPrompt()
	a.reg.SetDescriptions(a.prompt.Tools)
	return nil
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

// DefaultModel returns the configured default model name.
func (a *Agent) DefaultModel() string { return a.cfg.Default }

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
	// Cache the installed list per host, so models on the same server are one query.
	installedAt := map[string]map[string]bool{}
	var out []ModelStatus
	for _, name := range a.cfg.Names() {
		url, _ := a.cfg.URLFor(name)
		set, ok := installedAt[url]
		if !ok {
			set = map[string]bool{}
			for _, n := range a.router.InstalledModelsAt(url) {
				set[n] = true
			}
			installedAt[url] = set
		}
		out = append(out, ModelStatus{
			Name:    name,
			URL:     url,
			Running: set[a.cfg.TagFor(name)],
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

// UseSessionModel activates a session's preferred model when it is configured
// and installed, otherwise falls back to the default. Returns the model in use.
func (a *Agent) UseSessionModel(preferred string) string {
	def := a.cfg.Default
	fallback := func() string {
		if def != "" {
			_ = a.cfg.SetActive(def)
			return def
		}
		return a.cfg.ActiveName()
	}
	if preferred == "" {
		return fallback()
	}
	if err := a.cfg.SetActive(preferred); err != nil {
		return fallback()
	}
	url, _ := a.cfg.URLFor(preferred)
	for _, n := range a.router.InstalledModelsAt(url) {
		if n == preferred {
			return preferred
		}
	}
	return fallback()
}

// UseModel switches the active model and persists it as the default. If the name
// is not registered but is installed in ollama, it is added first. Returns an
// error when the model is neither registered nor installed.
func (a *Agent) UseModel(name string) error {
	if err := a.cfg.SetActive(name); err != nil {
		installed := false
		for _, n := range a.router.InstalledModels() {
			if n == name {
				installed = true
				break
			}
		}
		if !installed {
			return fmt.Errorf("%q is not a known or installed model; run `pilot models add %s` first", name, name)
		}
		a.cfg.AddModel(model.ModelEntry{Name: name, ToolMode: model.ToolModeNative, Port: 11434})
		_ = a.cfg.SetActive(name)
	}
	a.cfg.Default = name
	return a.cfg.Save(filepath.Join(a.cfg.Dir(), "models.json"))
}

// modelsPath is the on-disk registry the model-management methods persist to.
func (a *Agent) modelsPath() string { return filepath.Join(a.cfg.Dir(), "models.json") }

// modelHosts returns the distinct backend URLs the registry uses (plus the
// active one), so installed-model queries cover every server in play.
func (a *Agent) modelHosts() []string {
	seen := map[string]bool{}
	var hosts []string
	add := func(name string) {
		if url, ok := a.cfg.URLFor(name); ok && !seen[url] {
			seen[url] = true
			hosts = append(hosts, url)
		}
	}
	for _, name := range a.cfg.Names() {
		add(name)
	}
	add(a.cfg.ActiveName())
	return hosts
}

// AvailableModels returns ollama-installed model tags that are NOT yet registered
// ON THEIR HOST — the source for an "add model" autocomplete. A tag registered on
// one server is still offered for another (that is the whole point of decoupling
// the label from the tag), so a local copy of a remote model can be added.
func (a *Agent) AvailableModels() []string {
	registeredHostTag := map[string]bool{}
	for _, name := range a.cfg.Names() {
		if url, ok := a.cfg.URLFor(name); ok {
			registeredHostTag[url+"|"+a.cfg.TagFor(name)] = true
		}
	}
	added := map[string]bool{}
	var out []string
	for _, url := range a.modelHosts() {
		for _, tag := range a.router.InstalledModelsAt(url) {
			if registeredHostTag[url+"|"+tag] || added[tag] {
				continue
			}
			added[tag] = true
			out = append(out, tag)
		}
	}
	sort.Strings(out)
	return out
}

// RegisterInstalled adds an already-installed ollama model to the registry
// (native tool-calls). modelTag is the ollama tag; host is optional (a remote
// server URL, empty = local); name is an optional display label (a unique one is
// derived when blank, so the same tag on two servers never collides). It verifies
// the tag is installed on that host before registering.
func (a *Agent) RegisterInstalled(modelTag, host, name string) error {
	modelTag = strings.TrimSpace(modelTag)
	if modelTag == "" {
		return fmt.Errorf("model tag is required")
	}
	url := "http://localhost:11434"
	entry := model.ModelEntry{ToolMode: model.ToolModeNative, Port: 11434}
	if h := strings.TrimSpace(host); h != "" {
		url = model.NormalizeHost(h)
		entry.Host = url
		entry.Port = 0
	}
	installed := false
	for _, t := range a.router.InstalledModelsAt(url) {
		if t == modelTag {
			installed = true
			break
		}
	}
	if !installed {
		return fmt.Errorf("%q is not installed on %s; pull it first", modelTag, url)
	}
	regName := strings.TrimSpace(name)
	if regName == "" {
		regName = a.cfg.DeriveName(modelTag, host)
	}
	for _, n := range a.cfg.Names() {
		if n == regName {
			return fmt.Errorf("the name %q is already in use; choose another", regName)
		}
	}
	entry.Name = regName
	if regName != modelTag {
		entry.Model = modelTag // label differs from the tag → record the real tag
	}
	a.cfg.AddModel(entry)
	return a.cfg.Save(a.modelsPath())
}

// PullModel downloads a model on the local backend, streaming progress through
// emit. It does NOT touch the registry — call RegisterInstalled after it
// succeeds (under the caller's lock), so a long download never blocks runs.
func (a *Agent) PullModel(ctx context.Context, name string, emit func(status string, completed, total int64)) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("model name is required")
	}
	return a.router.PullModel(ctx, name, emit)
}

// RemoveModel drops a model from the registry and deletes it from ollama to free
// disk (local models only; a remote model's files are not ours to delete).
func (a *Agent) RemoveModel(name string) error {
	entry, ok := a.cfg.EntryFor(name)
	url, _ := a.cfg.URLFor(name)
	if err := a.cfg.Remove(name); err != nil {
		return err
	}
	if err := a.cfg.Save(a.modelsPath()); err != nil {
		return err
	}
	// Free disk for a local model: delete its ollama tag (and the base it derived
	// from, for a -tools variant). A remote model lives on someone else's server.
	// Skip deletion if another registry entry still points at the same tag on the
	// same host, so removing one label never pulls the model out from under another.
	if ok && entry.Host == "" && url != "" {
		tag := entry.Model
		if tag == "" {
			tag = entry.Name
		}
		if !a.tagStillUsed(url, tag) {
			_ = a.router.DeleteModelAt(url, tag)
		}
		if base := entry.Base; base != "" && base != tag && !a.tagStillUsed(url, base) {
			_ = a.router.DeleteModelAt(url, base)
		}
	}
	return nil
}

// tagStillUsed reports whether any remaining registry entry uses this ollama tag
// on this host — so its files must not be deleted.
func (a *Agent) tagStillUsed(url, tag string) bool {
	for _, n := range a.cfg.Names() {
		if u, ok := a.cfg.URLFor(n); ok && u == url && a.cfg.TagFor(n) == tag {
			return true
		}
	}
	return false
}

// SetDefaultModel makes a registered model the persistent default (and active).
func (a *Agent) SetDefaultModel(name string) error {
	if err := a.cfg.SetActive(name); err != nil {
		return err
	}
	a.cfg.Default = name
	return a.cfg.Save(a.modelsPath())
}
