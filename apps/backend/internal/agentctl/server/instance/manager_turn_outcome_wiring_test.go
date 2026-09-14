package instance

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

// TestCreateInstanceWiresTurnOutcomeRecorder pins the end-to-end wiring
// CreateInstance is responsible for (AC-EXECUTORS-SURVIVAL-004.1): a real
// terminal event sent through the process manager it constructs must reach
// this instance's retained-outcome slot, peekable via PeekTurnOutcome. Only
// exported process.Manager API is used (SendErrorEvent) so this exercises
// the production path -- forwardUpdates/sendUpdateBlocking calling back into
// recordTerminalOutcome -- rather than reaching into unexported internals.
func TestCreateInstanceWiresTurnOutcomeRecorder(t *testing.T) {
	log := newTestLogger(t)
	mgr := NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 0, Max: 0},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, log)
	t.Cleanup(func() { _ = mgr.Shutdown(context.Background()) })

	var captured *process.Manager
	mgr.SetServerFactory(func(cfg *config.InstanceConfig, procMgr *process.Manager, log *logger.Logger) http.Handler {
		captured = procMgr
		return http.NotFoundHandler()
	})

	resp, err := mgr.CreateInstance(context.Background(), &CreateRequest{WorkspacePath: t.TempDir()})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	t.Cleanup(func() { _ = mgr.StopInstance(context.Background(), resp.ID) })

	if captured == nil {
		t.Fatal("server factory was never invoked; no process.Manager captured")
	}

	if _, hasOutcome, found := mgr.PeekTurnOutcome(resp.ID); !found || hasOutcome {
		t.Fatalf("hasOutcome = %v, found = %v before any terminal event, want found=true hasOutcome=false", hasOutcome, found)
	}

	captured.SendErrorEvent("boom", 1)

	deadline := time.After(time.Second)
	for {
		outcome, hasOutcome, found := mgr.PeekTurnOutcome(resp.ID)
		if !found {
			t.Fatal("instance not found while polling for the retained outcome")
		}
		if hasOutcome {
			if outcome.Event.Error != "boom" {
				t.Fatalf("retained outcome error = %q, want boom", outcome.Event.Error)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for SendErrorEvent to reach the retained outcome slot")
		case <-time.After(time.Millisecond):
		}
	}
}
