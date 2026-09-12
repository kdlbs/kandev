package main

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

// fakeUnownedReaper is a test double for unownedReaper, letting the gate
// decision be asserted without exercising the real reaper goroutine's
// lifecycle (already covered directly by the api package's own tests).
type fakeUnownedReaper struct {
	started bool
	stopped bool
}

func (f *fakeUnownedReaper) StartUnownedReaper() { f.started = true }
func (f *fakeUnownedReaper) StopUnownedReaper()  { f.stopped = true }

// TestStartUnownedReaperIfEnabled pins Layer 5.9's gate on
// StartUnownedReaper's own caller contract: unconditionally starting it on
// every agentctl launch would self-terminate a healthy attached instance
// once the period elapses, since nothing renews ownership without the
// agent-survival capability and its backend-side claim loop.
func TestStartUnownedReaperIfEnabled(t *testing.T) {
	t.Run("enabled starts the reaper", func(t *testing.T) {
		reaper := &fakeUnownedReaper{}
		stop := startUnownedReaperIfEnabled(&config.Config{AgentSurvivalEnabled: true}, reaper)
		if !reaper.started {
			t.Fatal("StartUnownedReaper was not called with the capability enabled")
		}
		stop()
		if !reaper.stopped {
			t.Fatal("the returned stop function did not call StopUnownedReaper")
		}
	})

	t.Run("disabled never starts the reaper but stop stays safe", func(t *testing.T) {
		reaper := &fakeUnownedReaper{}
		stop := startUnownedReaperIfEnabled(&config.Config{AgentSurvivalEnabled: false}, reaper)
		if reaper.started {
			t.Fatal("StartUnownedReaper was called with the capability disabled")
		}
		stop()
		if !reaper.stopped {
			t.Fatal("the returned stop function did not call StopUnownedReaper even when disabled")
		}
	})
}
