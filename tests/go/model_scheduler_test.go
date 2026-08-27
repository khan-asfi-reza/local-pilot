package systemtest

import (
	"sync"
	"testing"
	"time"

	"harness/harness/model"
)

// waitDepth blocks until the scheduler queue reaches n, so a test can enqueue
// waiters in a known order without sleeping on guesses.
func waitDepth(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if model.RunSched.QueueDepth() == n {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("queue depth never reached %d (stuck at %d)", n, model.RunSched.QueueDepth())
}

// TestSchedulerIsFairAcrossProjects checks the round-robin inference slot: two
// queued calls from a busy project must not starve one call from another
// project. This is the guarantee behind running a Telegram chat and a web build
// on one machine at the same time.
func TestSchedulerIsFairAcrossProjects(t *testing.T) {
	model.RunSched.Acquire("holder") // occupy the single slot

	var mu sync.Mutex
	var got []string
	var wg sync.WaitGroup

	enqueue := func(key, label string, depth int) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			model.RunSched.Acquire(key)
			mu.Lock()
			got = append(got, label)
			mu.Unlock()
			model.RunSched.Release()
		}()
		waitDepth(t, depth)
	}

	enqueue("projectA", "A1", 1)
	enqueue("projectA", "A2", 2)
	enqueue("projectB", "B1", 3)

	model.RunSched.Release() // hand the slot on; the queue drains itself
	wg.Wait()

	if len(got) != 3 {
		t.Fatalf("expected 3 completions, got %v", got)
	}
	if got[0] != "A1" {
		t.Errorf("FIFO within a project broken: first served was %q, want A1", got[0])
	}
	if got[1] != "B1" {
		t.Errorf("project B was starved behind project A: order was %v, want A1 B1 A2", got)
	}
	if got[2] != "A2" {
		t.Errorf("round-robin order wrong: %v", got)
	}
	if d := model.RunSched.QueueDepth(); d != 0 {
		t.Errorf("queue not drained, depth = %d", d)
	}
}

// TestSchedulerSerialisesInference checks only one inference holds the slot at a
// time, which is what keeps a 4B model on a laptop from thrashing.
func TestSchedulerSerialisesInference(t *testing.T) {
	var mu sync.Mutex
	inFlight, peak := 0, 0
	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "p1"
			if i%2 == 0 {
				key = "p2"
			}
			model.RunSched.Acquire(key)
			mu.Lock()
			inFlight++
			if inFlight > peak {
				peak = inFlight
			}
			mu.Unlock()

			time.Sleep(time.Millisecond)

			mu.Lock()
			inFlight--
			mu.Unlock()
			model.RunSched.Release()
		}(i)
	}
	wg.Wait()

	if peak != 1 {
		t.Fatalf("%d inferences ran at once; the slot must be exclusive", peak)
	}
}
