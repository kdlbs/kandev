package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

// TestClaimNextEligibleRun_MissingAgentProfileRecordsSuccessfulGateOutcome
// pins AC-OFFICE-BACKPRESSURE-003.10: a gate that correctly blocks a
// candidate (here, agent_ceiling deferring for a missing agent_profiles
// row) is a *successful* evaluation for failure-tracking purposes, not a
// failure — a readable "no profile" answer is not the same as an
// unreadable input, so it must not move the durable failure count.
func TestClaimNextEligibleRun_MissingAgentProfileRecordsSuccessfulGateOutcome(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	workspace := claimTestWorkspace("ghost-agent")

	queueRunAt(t, repo, "no-profile", "ghost-agent", time.Now().UTC())

	if _, err := repo.ClaimNextEligibleRun(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("claim err = %v, want sql.ErrNoRows", err)
	}

	state, err := repo.GetGateFailureState(ctx, workspace, "agent_ceiling")
	if err != nil {
		t.Fatalf("get gate failure state: %v", err)
	}
	if state.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d, want 0 (a correctly-blocking gate is a success)", state.ConsecutiveFailures)
	}
	if state.LastEscalationAt != nil {
		t.Errorf("last_escalation_at = %v, want nil", state.LastEscalationAt)
	}
}

// TestClaimNextEligibleRun_SuccessfulClaimResetsGateFailureState pins the
// other half of AC-OFFICE-BACKPRESSURE-003.8: a real claim going through
// ClaimNextEligibleRun (not the standalone RecordGateOutcome used by the
// unit tests in gate_failure_state_test.go) exercises the wired-in
// RecordGateOutcomeTx calls in claim.go, and its clean pass through every
// gate for the candidate's workspace must reset a pre-existing failure
// streak to 0.
func TestClaimNextEligibleRun_SuccessfulClaimResetsGateFailureState(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	seedClaimAgent(t, repo, "a1")
	workspace := claimTestWorkspace("a1")

	if err := repo.RecordGateOutcome(ctx, workspace, "agent_ceiling", false); err != nil {
		t.Fatalf("seed failure: %v", err)
	}
	if err := repo.RecordGateOutcome(ctx, workspace, "agent_ceiling", false); err != nil {
		t.Fatalf("seed failure: %v", err)
	}
	seeded, err := repo.GetGateFailureState(ctx, workspace, "agent_ceiling")
	if err != nil {
		t.Fatalf("get seeded state: %v", err)
	}
	if seeded.ConsecutiveFailures != 2 {
		t.Fatalf("seeded consecutive_failures = %d, want 2", seeded.ConsecutiveFailures)
	}

	queueRunAt(t, repo, "claimable", "a1", time.Now().UTC())
	if _, err := repo.ClaimNextEligibleRun(ctx); err != nil {
		t.Fatalf("claim: %v", err)
	}

	state, err := repo.GetGateFailureState(ctx, workspace, "agent_ceiling")
	if err != nil {
		t.Fatalf("get gate failure state after claim: %v", err)
	}
	if state.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d, want 0 (reset by the claim's successful gate evaluation)", state.ConsecutiveFailures)
	}
}
