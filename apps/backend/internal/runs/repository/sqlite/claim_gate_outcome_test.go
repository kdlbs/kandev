package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

// TestClaimNextEligibleRun_MissingAgentProfileRecordsGateFailure pins
// AC-OFFICE-LAUNCH-SAFETY-001.8's explicit carve-out from the general
// AC-OFFICE-BACKPRESSURE-003.10 rule: a claiming run naming an agent
// profile that does not exist is named alongside an unreadable ceiling
// input, not alongside an ordinary saturated-pool block, so it must
// record the failure described in AC-OFFICE-BACKPRESSURE-003.3 rather
// than a successful evaluation.
func TestClaimNextEligibleRun_MissingAgentProfileRecordsGateFailure(t *testing.T) {
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
	if state.ConsecutiveFailures != 1 {
		t.Errorf("consecutive_failures = %d, want 1 (a missing agent profile records a gate failure)",
			state.ConsecutiveFailures)
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
