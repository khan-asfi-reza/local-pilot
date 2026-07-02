package main

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"harness/harness/agent"
	"harness/harness/model"
)

// TestUILaunches drives the terminal against a simulated screen (no real TTY) to
// confirm it builds its layout, draws, cycles the mode, and stops cleanly.
func TestUILaunches(t *testing.T) {
	cfg := &model.Config{
		AssetsDir:     "assets",
		ContextTokens: 30000,
		Default:       "m",
		Models:        []model.ModelEntry{{Name: "m", File: "m.gguf", Port: 1}},
	}
	ag, err := agent.New(cfg, "")
	if err != nil {
		t.Fatal(err)
	}

	u := newUI(ag, &Session{Mode: "ask"}, t.TempDir())
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	u.app.SetScreen(screen)

	done := make(chan error, 1)
	go func() { done <- u.app.Run() }()
	time.Sleep(150 * time.Millisecond)

	u.app.QueueUpdateDraw(func() { u.cycleMode() })
	time.Sleep(100 * time.Millisecond)
	u.app.Stop()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("app run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("app did not stop")
	}
	if u.session.Mode != "auto" {
		t.Fatalf("mode cycle from ask expected auto, got %s", u.session.Mode)
	}
}
