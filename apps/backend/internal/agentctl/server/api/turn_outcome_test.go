package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

// newTurnOutcomeTestServer wires a real instance.Manager with one real
// instance through a real ControlServer, mirroring the ListInstances
// end-to-end pattern (control_instance_list_test.go): the wire contract for
// turn-outcome retain/peek/ack must be proven against real JSON encoding and
// decoding on both ends, not a hand-written fixture.
func newTurnOutcomeTestServer(t *testing.T) (*instance.Manager, *agentctl.ControlClient, string) {
	t.Helper()
	log := logger.Default()
	mgr := instance.NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 0, Max: 0},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, log)
	t.Cleanup(func() { _ = mgr.Shutdown(t.Context()) })
	mgr.SetServerFactory(func(*config.InstanceConfig, *process.Manager, *logger.Logger) http.Handler {
		return http.NotFoundHandler()
	})

	created, err := mgr.CreateInstance(t.Context(), &instance.CreateRequest{
		WorkspacePath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	t.Cleanup(func() { _ = mgr.StopInstance(t.Context(), created.ID) })

	cs := NewControlServer(&config.Config{}, mgr, log)
	server := httptest.NewServer(cs.Router())
	t.Cleanup(server.Close)
	host, port := parseHostPort(t, server.URL)
	client := agentctl.NewControlClient(host, port, log)

	return mgr, client, created.ID
}

// TestGetTurnOutcomeReturns404ForUnknownInstance pins that the wire contract
// distinguishes "no such instance" from "instance exists, nothing retained".
func TestGetTurnOutcomeReturns404ForUnknownInstance(t *testing.T) {
	_, client, _ := newTurnOutcomeTestServer(t)

	if _, err := client.GetTurnOutcome(t.Context(), "does-not-exist"); err == nil {
		t.Fatal("GetTurnOutcome(unknown instance) = nil error, want an error")
	}
}

// TestGetTurnOutcomeReturnsNilWhenNothingRetained pins
// AC-EXECUTORS-SURVIVAL-004.5: a real, live instance with no retained
// terminal outcome yet must answer "nothing retained", not an error.
func TestGetTurnOutcomeReturnsNilWhenNothingRetained(t *testing.T) {
	_, client, instanceID := newTurnOutcomeTestServer(t)

	outcome, err := client.GetTurnOutcome(t.Context(), instanceID)
	if err != nil {
		t.Fatalf("GetTurnOutcome: %v", err)
	}
	if outcome != nil {
		t.Fatalf("outcome = %+v, want nil (nothing retained yet)", outcome)
	}
}

// TestGetTurnOutcomeRoundTripsARetainedOutcomeRepeatedly pins
// AC-EXECUTORS-SURVIVAL-004.2/.6 end to end: a retained outcome round-trips
// through the real HTTP handler and real client, and repeated GETs return
// the identical turn ID and event without discarding it.
func TestGetTurnOutcomeRoundTripsARetainedOutcomeRepeatedly(t *testing.T) {
	mgr, client, instanceID := newTurnOutcomeTestServer(t)

	turnID, ok := mgr.RetainTurnOutcome(instanceID, streams.AgentEvent{
		Type:      streams.EventTypeComplete,
		SessionID: "sess-xyz",
	})
	if !ok {
		t.Fatal("RetainTurnOutcome() ok = false, want true")
	}

	for i := 0; i < 2; i++ {
		outcome, err := client.GetTurnOutcome(t.Context(), instanceID)
		if err != nil {
			t.Fatalf("iteration %d: GetTurnOutcome: %v", i, err)
		}
		if outcome == nil {
			t.Fatalf("iteration %d: outcome = nil, want the retained outcome", i)
		}
		if outcome.TurnID != turnID {
			t.Fatalf("iteration %d: TurnID = %d, want %d", i, outcome.TurnID, turnID)
		}
		if outcome.Event.SessionID != "sess-xyz" {
			t.Fatalf("iteration %d: Event.SessionID = %q, want %q", i, outcome.Event.SessionID, "sess-xyz")
		}
	}
}

// TestAckTurnOutcomeClearsTheMatchingOutcome pins the ack half of
// AC-EXECUTORS-SURVIVAL-004.6 end to end: acking the retained turn ID
// discards it, and a subsequent GET reports nothing retained.
func TestAckTurnOutcomeClearsTheMatchingOutcome(t *testing.T) {
	mgr, client, instanceID := newTurnOutcomeTestServer(t)

	turnID, _ := mgr.RetainTurnOutcome(instanceID, streams.AgentEvent{Type: streams.EventTypeComplete})

	if err := client.AckTurnOutcome(t.Context(), instanceID, turnID); err != nil {
		t.Fatalf("AckTurnOutcome: %v", err)
	}

	outcome, err := client.GetTurnOutcome(t.Context(), instanceID)
	if err != nil {
		t.Fatalf("GetTurnOutcome after ack: %v", err)
	}
	if outcome != nil {
		t.Fatalf("outcome after ack = %+v, want nil", outcome)
	}
}

// TestAckTurnOutcomeWithUnrelatedIDIsSafeAndChangesNothing pins that acking
// an identifier the server never issued (or already discarded) is accepted
// -- so a retried acknowledgement is always safe -- and does not disturb a
// different outcome that is still retained.
func TestAckTurnOutcomeWithUnrelatedIDIsSafeAndChangesNothing(t *testing.T) {
	mgr, client, instanceID := newTurnOutcomeTestServer(t)

	turnID, _ := mgr.RetainTurnOutcome(instanceID, streams.AgentEvent{Type: streams.EventTypeComplete})

	if err := client.AckTurnOutcome(t.Context(), instanceID, turnID+999); err != nil {
		t.Fatalf("AckTurnOutcome(unrelated id): %v", err)
	}

	outcome, err := client.GetTurnOutcome(t.Context(), instanceID)
	if err != nil {
		t.Fatalf("GetTurnOutcome: %v", err)
	}
	if outcome == nil {
		t.Fatal("outcome = nil after an unrelated ack, want the original outcome still retained")
	}
	if outcome.TurnID != turnID {
		t.Fatalf("TurnID = %d, want %d (unaffected by the unrelated ack)", outcome.TurnID, turnID)
	}
}
