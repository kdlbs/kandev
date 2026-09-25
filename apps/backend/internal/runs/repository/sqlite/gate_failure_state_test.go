package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

// TestRecordGateOutcome_FailureIncrementsConsecutiveCount pins
// AC-OFFICE-BACKPRESSURE-003.8: a failed evaluation increments the
// durable per-(workspace, gate) count.
func TestRecordGateOutcome_FailureIncrementsConsecutiveCount(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.RecordGateOutcome(ctx, "ws1", "agent_ceiling", false); err != nil {
		t.Fatalf("record 1: %v", err)
	}
	if err := repo.RecordGateOutcome(ctx, "ws1", "agent_ceiling", false); err != nil {
		t.Fatalf("record 2: %v", err)
	}

	state, err := repo.GetGateFailureState(ctx, "ws1", "agent_ceiling")
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.ConsecutiveFailures != 2 {
		t.Errorf("consecutive_failures = %d, want 2", state.ConsecutiveFailures)
	}
}

// TestRecordGateOutcome_SuccessResetsConsecutiveCount pins
// AC-OFFICE-BACKPRESSURE-003.8: reset by exactly one successful
// evaluation, and by nothing else.
func TestRecordGateOutcome_SuccessResetsConsecutiveCount(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	for range 3 {
		if err := repo.RecordGateOutcome(ctx, "ws1", "workspace_ceiling", false); err != nil {
			t.Fatalf("record failure: %v", err)
		}
	}
	if err := repo.RecordGateOutcome(ctx, "ws1", "workspace_ceiling", true); err != nil {
		t.Fatalf("record success: %v", err)
	}

	state, err := repo.GetGateFailureState(ctx, "ws1", "workspace_ceiling")
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d, want 0 after success", state.ConsecutiveFailures)
	}
}

// TestRecordGateOutcome_DifferentGatesAndWorkspacesAreIndependent pins
// AC-OFFICE-BACKPRESSURE-003.8's "held per (workspace, gate) pair": an
// evaluation for one pair must not affect another gate or another
// workspace's count for the same gate.
func TestRecordGateOutcome_DifferentGatesAndWorkspacesAreIndependent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.RecordGateOutcome(ctx, "ws1", "agent_ceiling", false); err != nil {
		t.Fatalf("record ws1/agent_ceiling: %v", err)
	}
	if err := repo.RecordGateOutcome(ctx, "ws1", "instance_ceiling", false); err != nil {
		t.Fatalf("record ws1/instance_ceiling: %v", err)
	}
	if err := repo.RecordGateOutcome(ctx, "ws2", "agent_ceiling", false); err != nil {
		t.Fatalf("record ws2/agent_ceiling: %v", err)
	}

	agentWS1, err := repo.GetGateFailureState(ctx, "ws1", "agent_ceiling")
	if err != nil {
		t.Fatalf("get ws1/agent_ceiling: %v", err)
	}
	if agentWS1.ConsecutiveFailures != 1 {
		t.Errorf("ws1/agent_ceiling = %d, want 1", agentWS1.ConsecutiveFailures)
	}

	instanceWS1, err := repo.GetGateFailureState(ctx, "ws1", "instance_ceiling")
	if err != nil {
		t.Fatalf("get ws1/instance_ceiling: %v", err)
	}
	if instanceWS1.ConsecutiveFailures != 1 {
		t.Errorf("ws1/instance_ceiling = %d, want 1", instanceWS1.ConsecutiveFailures)
	}

	agentWS2, err := repo.GetGateFailureState(ctx, "ws2", "agent_ceiling")
	if err != nil {
		t.Fatalf("get ws2/agent_ceiling: %v", err)
	}
	if agentWS2.ConsecutiveFailures != 1 {
		t.Errorf("ws2/agent_ceiling = %d, want 1", agentWS2.ConsecutiveFailures)
	}
}

// TestRecordGateOutcome_EscalatesAtThreshold pins
// AC-OFFICE-BACKPRESSURE-003.5: reaching the configured threshold stamps
// last_escalation_at, the durable escalation record.
func TestRecordGateOutcome_EscalatesAtThreshold(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	repo.SetGateFailureThreshold(3)

	before := time.Now().UTC().Add(-time.Second)
	for range 2 {
		if err := repo.RecordGateOutcome(ctx, "ws1", "routine_budget", false); err != nil {
			t.Fatalf("record failure: %v", err)
		}
	}
	state, err := repo.GetGateFailureState(ctx, "ws1", "routine_budget")
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.LastEscalationAt != nil {
		t.Fatalf("last_escalation_at = %v, want nil below threshold", state.LastEscalationAt)
	}

	if err := repo.RecordGateOutcome(ctx, "ws1", "routine_budget", false); err != nil {
		t.Fatalf("record failure 3: %v", err)
	}
	state, err = repo.GetGateFailureState(ctx, "ws1", "routine_budget")
	if err != nil {
		t.Fatalf("get state after threshold: %v", err)
	}
	if state.ConsecutiveFailures != 3 {
		t.Errorf("consecutive_failures = %d, want 3", state.ConsecutiveFailures)
	}
	if state.LastEscalationAt == nil {
		t.Fatal("last_escalation_at = nil, want a stamp at threshold")
	}
	if state.LastEscalationAt.Before(before) {
		t.Errorf("last_escalation_at = %v, want at or after %v", state.LastEscalationAt, before)
	}
}

// TestRecordGateOutcome_DoesNotReescalateWithinOneHour pins
// AC-OFFICE-BACKPRESSURE-003.9: the record is written at most once per
// hour per (workspace, gate) pair, even though the count keeps climbing.
func TestRecordGateOutcome_DoesNotReescalateWithinOneHour(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	repo.SetGateFailureThreshold(1)

	if err := repo.RecordGateOutcome(ctx, "ws1", "workspace_budget", false); err != nil {
		t.Fatalf("record failure 1: %v", err)
	}
	first, err := repo.GetGateFailureState(ctx, "ws1", "workspace_budget")
	if err != nil {
		t.Fatalf("get state after first escalation: %v", err)
	}
	if first.LastEscalationAt == nil {
		t.Fatal("last_escalation_at = nil, want a stamp after the first escalation")
	}
	firstStamp := *first.LastEscalationAt

	if err := repo.RecordGateOutcome(ctx, "ws1", "workspace_budget", false); err != nil {
		t.Fatalf("record failure 2: %v", err)
	}
	second, err := repo.GetGateFailureState(ctx, "ws1", "workspace_budget")
	if err != nil {
		t.Fatalf("get state after second failure: %v", err)
	}
	if second.ConsecutiveFailures != 2 {
		t.Errorf("consecutive_failures = %d, want 2 (count keeps climbing)", second.ConsecutiveFailures)
	}
	if !second.LastEscalationAt.Equal(firstStamp) {
		t.Errorf("last_escalation_at changed to %v, want unchanged at %v (within the 1hr throttle)",
			second.LastEscalationAt, firstStamp)
	}
}

// TestRecordGateOutcome_ThresholdBelowOneUsesDefault pins the operator
// clamp: a configured threshold under 1 falls back to the documented
// default rather than escalating on every single failure.
func TestRecordGateOutcome_ThresholdBelowOneUsesDefault(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	repo.SetGateFailureThreshold(0)

	if err := repo.RecordGateOutcome(ctx, "ws1", "instance_ceiling", false); err != nil {
		t.Fatalf("record failure: %v", err)
	}
	state, err := repo.GetGateFailureState(ctx, "ws1", "instance_ceiling")
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.LastEscalationAt != nil {
		t.Fatalf("last_escalation_at = %v, want nil (1 failure must not escalate under the default threshold)",
			state.LastEscalationAt)
	}
}

// TestGetGateFailureState_NoRowsIsNotAnError pins the "never seen this
// pair" case as a normal empty state rather than requiring every caller
// to special-case sql.ErrNoRows.
func TestGetGateFailureState_NoRowsIsNotAnError(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if _, err := repo.GetGateFailureState(ctx, "unseen-ws", "unseen-gate"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("err = %v, want sql.ErrNoRows for a never-recorded pair", err)
	}
}
