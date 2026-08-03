package model

import "testing"

func newTestConfig() *Config {
	c := &Config{
		Default:        "a",
		DefaultPlanner: "b",
		Models:         []ModelEntry{{Name: "a"}, {Name: "b"}, {Name: "c"}},
	}
	c.active = "a"
	return c
}

func TestRemoveReassignsDefault(t *testing.T) {
	c := newTestConfig()
	if err := c.Remove("a"); err != nil { // "a" is Default + active
		t.Fatal(err)
	}
	if len(c.Models) != 2 {
		t.Fatalf("models = %d, want 2", len(c.Models))
	}
	if c.Default == "a" || c.Default == "" {
		t.Errorf("Default not reassigned: %q", c.Default)
	}
	if c.active != c.Default {
		t.Errorf("active %q should follow Default %q", c.active, c.Default)
	}
	if _, ok := c.entry("a"); ok {
		t.Error("removed model still present")
	}
}

func TestRemoveClearsPlanner(t *testing.T) {
	c := newTestConfig()
	if err := c.Remove("b"); err != nil { // "b" is DefaultPlanner
		t.Fatal(err)
	}
	if c.DefaultPlanner != "" {
		t.Errorf("DefaultPlanner should be cleared, got %q", c.DefaultPlanner)
	}
	if c.Default != "a" || c.active != "a" {
		t.Errorf("removing a non-default must not move Default/active (%q/%q)", c.Default, c.active)
	}
}

func TestRemoveUnknown(t *testing.T) {
	c := newTestConfig()
	if err := c.Remove("nope"); err == nil {
		t.Error("expected error removing unknown model")
	}
}

func TestTagForBackCompat(t *testing.T) {
	// An old-style entry (Name = the tag, no Model) must send its Name as the tag —
	// so existing configs are unaffected by the new Model field.
	c := &Config{Models: []ModelEntry{
		{Name: "qwen3.5:9b"},                                  // legacy: no Model
		{Name: "qwen3.5:9b (192.168.10.99)", Model: "qwen3.5:9b"}, // new: label != tag
	}}
	if got := c.TagFor("qwen3.5:9b"); got != "qwen3.5:9b" {
		t.Errorf("legacy TagFor = %q, want the name itself", got)
	}
	if got := c.TagFor("qwen3.5:9b (192.168.10.99)"); got != "qwen3.5:9b" {
		t.Errorf("labeled TagFor = %q, want the real tag qwen3.5:9b", got)
	}
}

func TestDeriveNameDisambiguates(t *testing.T) {
	c := &Config{Models: []ModelEntry{{Name: "qwen3.5:9b"}}}
	if got := c.DeriveName("llama3:8b", ""); got != "llama3:8b" {
		t.Errorf("free tag should keep its name, got %q", got)
	}
	if got := c.DeriveName("qwen3.5:9b", "http://192.168.10.99:11434"); got != "qwen3.5:9b (192.168.10.99)" {
		t.Errorf("remote collision = %q, want host-labeled", got)
	}
	if got := c.DeriveName("qwen3.5:9b", ""); got != "qwen3.5:9b (local)" {
		t.Errorf("local collision = %q, want (local)", got)
	}
}

func TestRemoveLastRefused(t *testing.T) {
	c := &Config{Default: "only", Models: []ModelEntry{{Name: "only"}}}
	c.active = "only"
	if err := c.Remove("only"); err == nil {
		t.Error("expected error removing the only model")
	}
	if len(c.Models) != 1 {
		t.Error("the only model must survive a refused remove")
	}
}
