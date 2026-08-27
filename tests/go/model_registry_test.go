package systemtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"harness/harness/model"
)

// writeConfig writes a models.json and loads it.
func writeConfig(t *testing.T, jsonText string) (*model.Config, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	if err := os.WriteFile(path, []byte(jsonText), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := model.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return cfg, path
}

const twoHostConfig = `{
  "context_tokens": 0,
  "default": "qwen3.5:4b",
  "models": [
    {"name": "qwen3.5:4b", "port": 11434},
    {"name": "qwen3.5:9b (192.168.10.99)", "model": "qwen3.5:9b", "host": "192.168.10.99"}
  ]
}`

// TestLoadConfigAppliesDefaults checks the registry defaults: the active model is
// the configured default and a missing context budget is filled in.
func TestLoadConfigAppliesDefaults(t *testing.T) {
	cfg, _ := writeConfig(t, twoHostConfig)

	if cfg.ActiveName() != "qwen3.5:4b" {
		t.Errorf("active model = %q, want the configured default", cfg.ActiveName())
	}
	if cfg.ContextTokens != 30000 {
		t.Errorf("context_tokens = %d, want the 30000 default", cfg.ContextTokens)
	}
	if cfg.ToolMode() != model.ToolModeNative {
		t.Errorf("tool mode = %q, want native by default", cfg.ToolMode())
	}
	// With no dedicated planner, planning runs on the active model.
	if cfg.PlannerName() != "qwen3.5:4b" {
		t.Errorf("planner = %q, want the active model", cfg.PlannerName())
	}
}

// TestLoadConfigRejectsBrokenRegistries checks the two ways a registry can be
// unusable are caught at load, not at the first inference.
func TestLoadConfigRejectsBrokenRegistries(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"no models":       `{"default":"x","models":[]}`,
		"unknown default": `{"default":"missing","models":[{"name":"qwen3.5:4b","port":11434}]}`,
		"not json":        `{`,
	}
	for name, text := range cases {
		p := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".json")
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := model.LoadConfig(p); err == nil {
			t.Errorf("%s: LoadConfig accepted a broken registry", name)
		}
	}
	if _, err := model.LoadConfig(filepath.Join(dir, "absent.json")); err == nil {
		t.Error("LoadConfig accepted a missing file")
	}
}

// TestRemoteEntryResolvesToItsHost checks the local-vs-remote URL rule and that
// the ollama tag sent on the wire is the entry's Model, not its display label.
// A regression here silently sends every request to the wrong machine.
func TestRemoteEntryResolvesToItsHost(t *testing.T) {
	cfg, _ := writeConfig(t, twoHostConfig)

	local, ok := cfg.URLFor("qwen3.5:4b")
	if !ok || local != "http://localhost:11434" {
		t.Errorf("local URL = %q, want http://localhost:11434", local)
	}
	remote, ok := cfg.URLFor("qwen3.5:9b (192.168.10.99)")
	if !ok || remote != "http://192.168.10.99:11434" {
		t.Errorf("remote URL = %q, want http://192.168.10.99:11434", remote)
	}
	if tag := cfg.TagFor("qwen3.5:9b (192.168.10.99)"); tag != "qwen3.5:9b" {
		t.Errorf("wire tag = %q, want the bare ollama tag", tag)
	}
	if tag := cfg.TagFor("qwen3.5:4b"); tag != "qwen3.5:4b" {
		t.Errorf("a label-only entry should send its own name, got %q", tag)
	}
	if _, ok := cfg.URLFor("nope"); ok {
		t.Error("URLFor returned ok for an unknown model")
	}
}

// TestNormalizeHostAcceptsWhatUsersType checks the host field tolerates a bare
// IP, a host:port and a full URL.
func TestNormalizeHostAcceptsWhatUsersType(t *testing.T) {
	cases := map[string]string{
		"192.168.1.5":             "http://192.168.1.5:11434",
		"192.168.1.5:11434":       "http://192.168.1.5:11434",
		"http://192.168.1.5:8080": "http://192.168.1.5:8080",
		"https://box.local:443/":  "https://box.local:443",
		"  10.0.0.2  ":            "http://10.0.0.2:11434",
		"":                        "",
	}
	for in, want := range cases {
		if got := model.NormalizeHost(in); got != want {
			t.Errorf("NormalizeHost(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestDeriveNameKeepsLabelsUnique checks that pulling the same ollama tag on a
// second host produces a distinct registry label instead of overwriting the
// first entry.
func TestDeriveNameKeepsLabelsUnique(t *testing.T) {
	cfg, _ := writeConfig(t, twoHostConfig)

	if got := cfg.DeriveName("llama3:8b", ""); got != "llama3:8b" {
		t.Errorf("a free tag should be used as-is, got %q", got)
	}
	got := cfg.DeriveName("qwen3.5:4b", "192.168.10.99")
	if got == "qwen3.5:4b" {
		t.Fatal("DeriveName reused a taken label")
	}
	if !strings.Contains(got, "192.168.10.99") {
		t.Errorf("derived label should name the host, got %q", got)
	}
	if local := cfg.DeriveName("qwen3.5:4b", ""); !strings.Contains(local, "local") {
		t.Errorf("a taken tag on the local server should be labelled local, got %q", local)
	}
}

// TestRemoveModelReassignsDefaults checks removing the active/default model
// repoints the registry instead of leaving it dangling, and that the last model
// cannot be removed.
func TestRemoveModelReassignsDefaults(t *testing.T) {
	cfg, _ := writeConfig(t, twoHostConfig)
	remote := "qwen3.5:9b (192.168.10.99)"
	if err := cfg.SetPlanner(remote); err != nil {
		t.Fatal(err)
	}
	if cfg.PlannerName() != remote {
		t.Fatalf("planner not set: %q", cfg.PlannerName())
	}

	if err := cfg.Remove("nope"); err == nil {
		t.Error("removing an unknown model should error")
	}
	if err := cfg.Remove("qwen3.5:4b"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if cfg.Default != remote || cfg.ActiveName() != remote {
		t.Errorf("default/active not reassigned: default=%q active=%q", cfg.Default, cfg.ActiveName())
	}
	if err := cfg.Remove(remote); err == nil {
		t.Error("removing the only remaining model should be refused")
	}
}

// TestSetPlannerRejectsUnknownModel checks a typo cannot silently route planning
// nowhere.
func TestSetPlannerRejectsUnknownModel(t *testing.T) {
	cfg, _ := writeConfig(t, twoHostConfig)
	if err := cfg.SetPlanner("does-not-exist"); err == nil {
		t.Fatal("SetPlanner accepted an unknown model")
	}
	if err := cfg.SetPlanner(""); err != nil {
		t.Fatalf("clearing the planner should work: %v", err)
	}
	if cfg.PlannerName() != cfg.ActiveName() {
		t.Error("a cleared planner should fall back to the active model")
	}
}

// TestAddAndSaveRoundTrip checks a registry edit survives a save/load cycle, the
// path every model-management command takes.
func TestAddAndSaveRoundTrip(t *testing.T) {
	cfg, path := writeConfig(t, twoHostConfig)
	cfg.AddModel(model.ModelEntry{Name: "llama3:8b", Port: 11434, ToolMode: model.ToolModeJSON})
	cfg.AddModel(model.ModelEntry{Name: "llama3:8b", Port: 11500, ToolMode: model.ToolModeJSON}) // replace, not duplicate
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}

	reloaded, err := model.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(reloaded.Names()); n != 3 {
		t.Fatalf("registry has %d models, want 3 (AddModel must replace by name)", n)
	}
	e, ok := reloaded.EntryFor("llama3:8b")
	if !ok || e.Port != 11500 {
		t.Fatalf("entry not replaced: %+v", e)
	}
	if err := reloaded.SetActive("llama3:8b"); err != nil {
		t.Fatal(err)
	}
	if reloaded.ToolMode() != model.ToolModeJSON {
		t.Errorf("tool mode = %q, want json", reloaded.ToolMode())
	}
	if err := reloaded.SetActive("ghost"); err == nil {
		t.Error("SetActive accepted an unknown model")
	}
}
