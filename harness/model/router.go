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
	return r.client.CompleteConstrained(ctx, url, name, msgs, schema)
}

// Chat forwards a native tool-calling turn to the active model, returning the
func (r *Router) Chat(ctx context.Context, msgs []Message, defs []ToolDef) (Message, int, error) {
	name, url, err := r.cfg.Active()
	if err != nil {
		return Message{}, 0, err
	}
	return r.client.Chat(ctx, url, name, msgs, defs)
}

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
	names, err := r.client.InstalledModels(url)
	if err != nil {
		return nil
	}
	return names
}
