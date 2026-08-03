package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ModelEntry struct {
	Name     string `json:"name"`
	File     string `json:"file,omitempty"`
	Port     int    `json:"port"`
	ToolMode string `json:"tool_mode,omitempty"`
	Template string `json:"template,omitempty"`
	// Base is the ollama model tag this derived (-tools) model was built from.
	Base string `json:"base,omitempty"`
	// Host is the ollama server this model lives on, e.g. http://192.168.1.50:11434.
	// Empty means the local server on Port.
	Host string `json:"host,omitempty"`
}

// NormalizeHost turns a host like "192.168.1.5", "192.168.1.5:11434", or a full
// URL into a base URL with a scheme and port.
func NormalizeHost(h string) string {
	h = strings.TrimSpace(h)
	if h == "" {
		return ""
	}
	if !strings.Contains(h, "://") {
		h = "http://" + h
	}
	rest := h[strings.Index(h, "://")+3:]
	if !strings.Contains(rest, ":") {
		h += ":11434"
	}
	return strings.TrimRight(h, "/")
}

// ToolModeNative and ToolModeJSON name the two tool-calling strategies.
const (
	ToolModeNative = "native"
	ToolModeJSON   = "json"
)

// Config is the model registry, loaded from models/models.json. It is the single
type Config struct {
	AssetsDir     string `json:"assets_dir,omitempty"`
	ContextTokens int    `json:"context_tokens"`
	// ContextLength is the saved OLLAMA_CONTEXT_LENGTH override; 0 means auto-size
	// from the machine's hardware at launch.
	ContextLength int    `json:"ollama_context_length,omitempty"`
	Default       string `json:"default"`
	// DefaultPlanner is the model used for the planning phase (intake, scaffold,
	// decompose). Empty falls back to the active/default model.
	DefaultPlanner string       `json:"default_planner,omitempty"`
	Models         []ModelEntry `json:"models"`
	// Suggested is the list of base ollama models the start menu offers; the first
	// is the default choice.
	Suggested []string `json:"suggested,omitempty"`
	// Graph configures the tree-sitter repo graph; nil means default (on when the
	// binary was built with tree-sitter).
	Graph *GraphConfig `json:"graph,omitempty"`

	active string // runtime selection
	dir    string // directory of the config file, for resolving assets
}

// GraphConfig tunes the repo memory graph.
type GraphConfig struct {
	Enabled  *bool `json:"enabled,omitempty"` // nil = default on
	MaxBytes int   `json:"max_bytes,omitempty"`
	MaxFiles int   `json:"max_files,omitempty"`
}

// AddModel adds or replaces a model entry by name.
func (c *Config) AddModel(e ModelEntry) {
	for i, m := range c.Models {
		if m.Name == e.Name {
			c.Models[i] = e
			return
		}
	}
	c.Models = append(c.Models, e)
}

// Save writes the registry back to path as indented JSON.
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// LoadConfig reads the registry and selects the default model as active.
func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if len(c.Models) == 0 {
		return nil, fmt.Errorf("config has no models")
	}
	if c.ContextTokens <= 0 {
		c.ContextTokens = 30000
	}
	abs, _ := filepath.Abs(path)
	c.dir = filepath.Dir(abs)

	c.active = c.Default
	if c.active == "" {
		c.active = c.Models[0].Name
	}
	if _, ok := c.entry(c.active); !ok {
		return nil, fmt.Errorf("default model %q is not in the models list", c.active)
	}
	return &c, nil
}

func (c *Config) entry(name string) (ModelEntry, bool) {
	for _, e := range c.Models {
		if e.Name == name {
			return e, true
		}
	}
	return ModelEntry{}, false
}

// ActiveName returns the name of the active model.
func (c *Config) ActiveName() string { return c.active }

// PlannerName returns the model used for planning, falling back to the active
// model when no dedicated planner is configured.
func (c *Config) PlannerName() string {
	if c.DefaultPlanner != "" {
		if _, ok := c.entry(c.DefaultPlanner); ok {
			return c.DefaultPlanner
		}
	}
	return c.active
}

// SetPlanner sets the planner model (empty clears it, falling back to default).
func (c *Config) SetPlanner(name string) error {
	if name != "" {
		if _, ok := c.entry(name); !ok {
			return fmt.Errorf("unknown model %q; configured models are %v", name, c.Names())
		}
	}
	c.DefaultPlanner = name
	return nil
}

// DefaultEntry returns the configured default model entry.
func (c *Config) DefaultEntry() (ModelEntry, bool) { return c.entry(c.active) }

func (c *Config) Dir() string { return c.dir }

func (c *Config) ToolMode() string {
	e, ok := c.entry(c.active)
	if !ok || e.ToolMode == "" {
		return ToolModeNative
	}
	return e.ToolMode
}

func (c *Config) TemplatePath(name string) string {
	e, ok := c.entry(name)
	if !ok || e.Template == "" {
		return ""
	}
	return filepath.Join(c.dir, e.Template)
}

func (c *Config) Active() (name, url string, err error) {
	e, ok := c.entry(c.active)
	if !ok {
		return "", "", fmt.Errorf("active model %q is not configured", c.active)
	}
	return e.Name, entryURL(e), nil
}

// entryURL is the base URL for a model: its Host if set (a remote server on the
// network), otherwise the local server on its port.
func entryURL(e ModelEntry) string {
	if e.Host != "" {
		return NormalizeHost(e.Host)
	}
	port := e.Port
	if port == 0 {
		port = 11434
	}
	return URLForPort(port)
}

func (c *Config) SetActive(name string) error {
	if _, ok := c.entry(name); !ok {
		return fmt.Errorf("unknown model %q; configured models are %v", name, c.Names())
	}
	c.active = name
	return nil
}

func (c *Config) Names() []string {
	out := make([]string, 0, len(c.Models))
	for _, e := range c.Models {
		out = append(out, e.Name)
	}
	return out
}

func (c *Config) URLFor(name string) (string, bool) {
	e, ok := c.entry(name)
	if !ok {
		return "", false
	}
	return entryURL(e), true
}

func (c *Config) AssetPath(name string) (string, bool) {
	e, ok := c.entry(name)
	if !ok {
		return "", false
	}
	return filepath.Join(c.dir, c.AssetsDir, e.File), true
}

func URLForPort(port int) string {
	return fmt.Sprintf("http://localhost:%d", port)
}
