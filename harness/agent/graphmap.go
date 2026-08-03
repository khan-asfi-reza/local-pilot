package agent

import (
	"context"
	"strings"
	"sync"

	"harness/harness/graph"
)

// graphInflight guards against launching more than one background graph build for
// the same working directory at a time.
var graphInflight sync.Map // workDir -> struct{}

// repoDigest returns the project map shown to the model: the ranked tree-sitter
// graph digest when available, else the regex repo map. It also kicks off a
// background (incremental) graph build so the next turn is fresher. It degrades
// safely at every layer — a disabled/absent/broken graph falls back to
// buildRepoMap, so the model always gets a map.
func (a *Agent) repoDigest(req Request) string {
	if !a.graphEnabled() || req.WorkDir == "" {
		return buildRepoMap(req.WorkDir)
	}
	g := graph.Load(req.WorkDir)
	a.scheduleGraphBuild(req.WorkDir)
	if g == nil {
		return buildRepoMap(req.WorkDir) // first run: build is async, use regex until ready
	}
	digest := safeDigest(g, a.graphMaxBytes(), g.PersonalizeFiles(req.Grounding.Targets()))
	if strings.TrimSpace(digest) == "" {
		return buildRepoMap(req.WorkDir)
	}
	return digest
}

// safeDigest renders the digest, recovering from any panic to the regex map.
func safeDigest(g *graph.Graph, maxBytes int, personalize map[graph.NodeID]float64) (out string) {
	defer func() {
		if recover() != nil {
			out = ""
		}
	}()
	return g.Digest(maxBytes, personalize)
}

// scheduleGraphBuild rebuilds the graph on a detached goroutine (tracked by a.bg,
// like memory updates), skipping if a build for this dir is already running.
func (a *Agent) scheduleGraphBuild(workDir string) {
	if !graph.Enabled() || workDir == "" {
		return
	}
	if _, busy := graphInflight.LoadOrStore(workDir, struct{}{}); busy {
		return
	}
	a.bg.Add(1)
	go func() {
		defer a.bg.Done()
		defer graphInflight.Delete(workDir)
		prev := graph.Load(workDir)
		if g, err := graph.Build(context.Background(), workDir, prev); err == nil && g != nil {
			_ = graph.Save(workDir, g)
		}
	}()
}

// graphEnabled reports whether the ranked graph should be used: the binary must
// be built with tree-sitter, and config must not disable it (default on).
func (a *Agent) graphEnabled() bool {
	if !graph.Enabled() {
		return false
	}
	if a.cfg != nil && a.cfg.Graph != nil && a.cfg.Graph.Enabled != nil {
		return *a.cfg.Graph.Enabled
	}
	return true
}

func (a *Agent) graphMaxBytes() int {
	if a.cfg != nil && a.cfg.Graph != nil && a.cfg.Graph.MaxBytes > 0 {
		return a.cfg.Graph.MaxBytes
	}
	return 6000
}
