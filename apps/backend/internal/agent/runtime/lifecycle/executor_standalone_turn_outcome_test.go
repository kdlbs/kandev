package lifecycle

import (
	"context"
	"testing"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// TestFetchTurnOutcomeWithRetryReturnsNilWhenNothingRetained pins
// AC-EXECUTORS-SURVIVAL-004.5's "nothing retained" case: a successful read
// with no outcome returns (nil, nil), never an error.
func TestFetchTurnOutcomeWithRetryReturnsNilWhenNothingRetained(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	exec := control.executor(t)

	outcome, err := exec.fetchTurnOutcomeWithRetry(context.Background(), "inst-1")
	if err != nil {
		t.Fatalf("fetchTurnOutcomeWithRetry() error = %v, want nil", err)
	}
	if outcome != nil {
		t.Fatalf("fetchTurnOutcomeWithRetry() = %+v, want nil", outcome)
	}
	if control.turnOutcomeAttempts["inst-1"] != 1 {
		t.Fatalf("attempts = %d, want 1 (no retry needed on success)", control.turnOutcomeAttempts["inst-1"])
	}
}

// TestFetchTurnOutcomeWithRetryReturnsRetainedOutcome pins the "outcome
// present" case, including that the turn identifier and event round-trip
// intact.
func TestFetchTurnOutcomeWithRetryReturnsRetainedOutcome(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.turnOutcomes["inst-1"] = agentctlclient.TurnOutcome{
		TurnID: 42,
		Event:  streams.AgentEvent{Type: streams.EventTypeComplete, Text: "done"},
	}
	exec := control.executor(t)

	outcome, err := exec.fetchTurnOutcomeWithRetry(context.Background(), "inst-1")
	if err != nil {
		t.Fatalf("fetchTurnOutcomeWithRetry() error = %v", err)
	}
	if outcome == nil {
		t.Fatal("fetchTurnOutcomeWithRetry() = nil, want the retained outcome")
	}
	if outcome.TurnID != 42 || outcome.Event.Text != "done" {
		t.Fatalf("outcome = %+v, want TurnID 42 and Event.Text %q", outcome, "done")
	}
}

// TestFetchTurnOutcomeWithRetryRetriesTransientFailure pins
// AC-EXECUTORS-SURVIVAL-004.5's bound: the read is retried the configured
// number of times before giving up.
func TestFetchTurnOutcomeWithRetryRetriesTransientFailure(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.turnOutcomeFailures["inst-1"] = 1
	control.turnOutcomes["inst-1"] = agentctlclient.TurnOutcome{
		TurnID: 7,
		Event:  streams.AgentEvent{Type: streams.EventTypeError},
	}
	exec := control.executor(t)
	exec.SetRecoveryRetryConfig(0, 2)

	outcome, err := exec.fetchTurnOutcomeWithRetry(context.Background(), "inst-1")
	if err != nil {
		t.Fatalf("fetchTurnOutcomeWithRetry() error = %v, want success after retry", err)
	}
	if outcome == nil || outcome.TurnID != 7 {
		t.Fatalf("outcome = %+v, want TurnID 7", outcome)
	}
	if control.turnOutcomeAttempts["inst-1"] != 2 {
		t.Fatalf("attempts = %d, want 2 (one failure, one success)", control.turnOutcomeAttempts["inst-1"])
	}
}

// TestFetchTurnOutcomeWithRetryExhaustsConfiguredRetries pins the exact
// attempt count when every attempt fails: AC-EXECUTORS-SURVIVAL-004.5's
// read-failed case, which the recovery loop must treat as "not-re-tracked,"
// never as "nothing retained."
func TestFetchTurnOutcomeWithRetryExhaustsConfiguredRetries(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.turnOutcomeFailures["inst-1"] = 100
	exec := control.executor(t)
	exec.SetRecoveryRetryConfig(0, 2)

	outcome, err := exec.fetchTurnOutcomeWithRetry(context.Background(), "inst-1")
	if err == nil {
		t.Fatal("fetchTurnOutcomeWithRetry() error = nil, want an error after exhausting retries")
	}
	if outcome != nil {
		t.Fatalf("outcome = %+v, want nil on failure", outcome)
	}
	if control.turnOutcomeAttempts["inst-1"] != 3 {
		t.Fatalf("attempts = %d, want 3 (1 initial + 2 retries)", control.turnOutcomeAttempts["inst-1"])
	}
}

// TestAckTurnOutcomeSendsInstanceAndTurnID pins the wire shape of the
// acknowledgement call.
func TestAckTurnOutcomeSendsInstanceAndTurnID(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	exec := control.executor(t)

	if err := exec.ackTurnOutcome(context.Background(), "inst-1", 42); err != nil {
		t.Fatalf("ackTurnOutcome() error = %v", err)
	}
	if len(control.ackedTurnOutcomes) != 1 {
		t.Fatalf("acked calls = %d, want 1", len(control.ackedTurnOutcomes))
	}
	got := control.ackedTurnOutcomes[0]
	if got.instanceID != "inst-1" || got.turnID != 42 {
		t.Fatalf("acked = %+v, want {inst-1 42}", got)
	}
}
