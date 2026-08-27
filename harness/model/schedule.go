package model

import (
	"context"
	"sync"
)

// RunSched is the process-wide, per-project ROUND-ROBIN scheduler for model
// inference. It is acquired around EACH model call (not once per whole run), so a
// long build and a short chat interleave call-by-call: every project gets a turn,
// none is starved by a busy peer. Model-management ops (add/remove/activate) also
// acquire it, so they never mutate the active model mid-inference.
var RunSched = newRRScheduler()

// runCtx carries the current run's scheduling key (its project) plus the model and
// log dir to apply for the duration of each of its inferences. Threaded via context
// from the /run handler down to the router, so every inference — agent loop,
// planner, orchestrator child — is gated and runs under the right model.
type runCtx struct {
	key    string
	model  string
	logDir string
}

type schedCtxKey struct{}

// WithRun tags a context with the run's project key, preferred model, and log dir.
func WithRun(ctx context.Context, key, mdl, logDir string) context.Context {
	return context.WithValue(ctx, schedCtxKey{}, runCtx{key: key, model: mdl, logDir: logDir})
}

func runInfoFrom(ctx context.Context) runCtx {
	if v, ok := ctx.Value(schedCtxKey{}).(runCtx); ok {
		return v
	}
	return runCtx{}
}

// rrScheduler grants a single inference slot round-robin across keys (projects):
// FIFO within a key, rotating across keys so a flood from one cannot starve others.
type rrScheduler struct {
	mu     sync.Mutex
	held   bool
	queues map[string][]chan struct{}
	order  []string
	cursor int
}

func newRRScheduler() *rrScheduler {
	return &rrScheduler{queues: map[string][]chan struct{}{}}
}

// Acquire blocks until this key holds the single slot.
func (s *rrScheduler) Acquire(key string) {
	s.mu.Lock()
	if !s.held {
		s.held = true
		s.mu.Unlock()
		return
	}
	ch := make(chan struct{})
	if len(s.queues[key]) == 0 {
		s.order = append(s.order, key)
	}
	s.queues[key] = append(s.queues[key], ch)
	s.mu.Unlock()
	<-ch
}

// Release hands the slot to the next key in round-robin order.
func (s *rrScheduler) Release() {
	s.mu.Lock()
	if len(s.order) == 0 {
		s.held = false
		s.mu.Unlock()
		return
	}
	if s.cursor >= len(s.order) {
		s.cursor = 0
	}
	key := s.order[s.cursor]
	s.cursor++
	ch := s.queues[key][0]
	s.queues[key] = s.queues[key][1:]
	if len(s.queues[key]) == 0 {
		delete(s.queues, key)
		s.order = append(s.order[:s.cursor-1], s.order[s.cursor:]...)
		s.cursor--
	}
	s.mu.Unlock()
	close(ch)
}

// QueueDepth reports how many inferences are waiting across all projects.
func (s *rrScheduler) QueueDepth() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, q := range s.queues {
		n += len(q)
	}
	return n
}
