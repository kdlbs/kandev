package main

import (
	"os"
	"syscall"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

// TestSIGPIPEDispositionFollowsTheSurvivalCapability pins that ignoring
// SIGPIPE is part of the survival capability rather than an unconditional
// property of the binary. With the capability off, an inherited stdout whose
// reader has gone away must still be able to end this process, because that
// is one of the paths by which a departing backend takes its agentctl with
// it; removing it for every launch would leave a detached agentctl behind on
// installations that never enabled survival at all.
func TestSIGPIPEDispositionFollowsTheSurvivalCapability(t *testing.T) {
	t.Run("disabled leaves the default disposition intact", func(t *testing.T) {
		var ignored []os.Signal
		applySIGPIPEDisposition(&config.Config{AgentSurvivalEnabled: false}, func(s ...os.Signal) {
			ignored = append(ignored, s...)
		})
		if len(ignored) != 0 {
			t.Fatalf("signals ignored = %v, want none with the capability disabled", ignored)
		}
	})

	t.Run("enabled ignores SIGPIPE", func(t *testing.T) {
		var ignored []os.Signal
		applySIGPIPEDisposition(&config.Config{AgentSurvivalEnabled: true}, func(s ...os.Signal) {
			ignored = append(ignored, s...)
		})
		if len(ignored) != 1 || ignored[0] != syscall.SIGPIPE {
			t.Fatalf("signals ignored = %v, want exactly [SIGPIPE]", ignored)
		}
	})
}
