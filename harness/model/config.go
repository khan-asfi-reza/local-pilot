package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ModelEntry struct {
	Name     string `json:"name"`
	File     string `json:"file,omitempty"`
	Port     int    `json:"port"`
	ToolMode string `json:"tool_mode,omitempty"`
	Template string `json:"template,omitempty"`
	// Base is the ollama model tag to pull and derive this local model from.
	Base string `json:"base,omitempty"`
	// NumCtx is the runtime context window baked into the created model.
	NumCtx int `json:"num_ctx,omitempty"`
}

// ToolModeNative and ToolModeJSON name the two tool-calling strategies.
const (
	ToolModeNative = "native"
	ToolModeJSON   = "json"
)

// Config is the model registry, loaded from models/models.json. It is the single
type Config struct {
	AssetsDir     string       `json:"assets_dir,omitempty"`
	ContextTokens int          `json:"context_tokens"`
	Default       string       `json:"default"`
	Models        []ModelEntry `json:"models"`

	active string // runtime selection
	dir    string // directory of the config file, for resolving assets
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
	return e.Name, URLForPort(e.Port), nil
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
	return URLForPort(e.Port), true
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
