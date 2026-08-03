package model

import (
	"context"
	"encoding/json"
)

// Router forwards requests to the backend of the active model. Nothing in the
type Router struct {
	cfg    *Config
	client *Client
}

// NewRouter builds a router over a config and a shared client.
func NewRouter(cfg *Config, client *Client) *Router {
	return &Router{cfg: cfg, client: client}
}

// Constrained forwards a grammar-constrained completion to the active model,
func (r *Router) Constrained(ctx context.Context, msgs []Message, schema json.RawMessage) (string, int, error) {
	name, url, err := r.cfg.Active()
	if err != nil {
		return "", 0, err
	}
	return r.client.CompleteConstrained(ctx, url, r.cfg.TagFor(name), msgs, schema)
}

// Chat forwards a native tool-calling turn to the active model, streaming tokens
// through onDelta (may be nil) and returning the assembled message.
func (r *Router) Chat(ctx context.Context, msgs []Message, defs []ToolDef, onDelta func(kind, text string)) (Message, int, error) {
	name, url, err := r.cfg.Active()
	if err != nil {
		return Message{}, 0, err
	}
	return r.client.Chat(ctx, url, r.cfg.TagFor(name), msgs, defs, onDelta)
}

// ChatWith runs a native tool-calling turn on a SPECIFIC model (not the active
// one), used to route planning through a dedicated planner model. Falls back to
// the active model if the name is unknown.
func (r *Router) ChatWith(ctx context.Context, modelName string, msgs []Message, defs []ToolDef, onDelta func(kind, text string)) (Message, int, error) {
	url, ok := r.cfg.URLFor(modelName)
	if !ok {
		return r.Chat(ctx, msgs, defs, onDelta)
	}
	return r.client.Chat(ctx, url, r.cfg.TagFor(modelName), msgs, defs, onDelta)
}

// PlannerName returns the configured planner model (or the active model).
func (r *Router) PlannerName() string { return r.cfg.PlannerName() }

// ToolMode returns the active model's tool-calling strategy.
func (r *Router) ToolMode() string { return r.cfg.ToolMode() }

// Active returns the active model name, for display.
func (r *Router) Active() string { return r.cfg.ActiveName() }

// Reachable reports whether the backend at a URL is up.
func (r *Router) Reachable(url string) bool { return r.client.Reachable(url) }

// InstalledModels returns the model tags installed on the active backend.
func (r *Router) InstalledModels() []string {
	_, url, err := r.cfg.Active()
	if err != nil {
		return nil
	}
	return r.InstalledModelsAt(url)
}

// InstalledModelsAt returns the model tags installed on a specific backend URL.
func (r *Router) InstalledModelsAt(url string) []string {
	names, err := r.client.InstalledModels(url)
	if err != nil {
		return nil
	}
	return names
}

// PullModel downloads a model on the active backend, streaming progress.
func (r *Router) PullModel(ctx context.Context, name string, progress func(status string, completed, total int64)) error {
	_, url, err := r.cfg.Active()
	if err != nil {
		return err
	}
	return r.client.PullModel(ctx, url, name, progress)
}

// DeleteModelAt removes a model from the ollama backend at a specific URL.
func (r *Router) DeleteModelAt(url, name string) error { return r.client.DeleteModel(url, name) }
