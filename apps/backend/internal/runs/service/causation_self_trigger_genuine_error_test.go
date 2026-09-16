package service

// Internal (white-box) test file: checkSelfTriggerAllowance is
// unexported, so exercising its genuine-error handling directly needs
// package-level access, unlike every other test in this directory
// (package service_test).

import (
	"context"
	"expvar"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// launchRefusedCount reads the current office_launch_refused_total value
// for gate, mirroring
// internal/runs/repository/sqlite/claim_shutdown_context_test.go's
// launchCheckFailedCount idiom.
func launchRefusedCount(gate string) int64 {
	v := shared.LaunchRefusedTotal.Get(shared.LaunchSafetyLabel("gate", gate))
	iv, ok := v.(*expvar.Int)
	if !ok {
		return 0
	}
	return iv.Value()
}

// TestCheckSelfTriggerAllowance_GenuineErrorIncrementsLaunchRefusedTotal
// pins AC-OFFICE-BACKPRESSURE-003.1: every enqueue a gate refuses must
// increment LaunchRefusedTotal, labelled by that gate. A genuine (not
// context-canceled, not sql.ErrNoRows) failure reading the self-trigger
// count still refuses the enqueue via *RefusalError, so it must count
// too, the same way the sibling causing_run_unreadable gate in
// applyCausationLineage and this same gate's own exceeded-allowance
// branch already do.
func TestCheckSelfTriggerAllowance_GenuineErrorIncrementsLaunchRefusedTotal(t *testing.T) {
	svc := newShutdownContextTestService(t)
	ctx := context.Background()

	tx, err := svc.repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	// Drop the table CountSelfTriggeredRunsTx queries, inside the same
	// transaction, so the count read fails with a genuine SQL error
	// rather than a context cancellation.
	if _, err := tx.ExecContext(ctx, "DROP TABLE runs"); err != nil {
		t.Fatalf("drop runs table: %v", err)
	}

	rec := &gateOutcomeRecorder{}
	req := QueueRunRequest{
		AgentProfileID: "agent-1",
		Reason:         "self_trigger_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-1",
	}

	before := launchRefusedCount(string(RefusalSelfTrigger))

	err = svc.checkSelfTriggerAllowance(ctx, tx, rec, "agent-1", req, models.ActorKindAgent, "agent-1", "ws-1", "causation-1")
	if err == nil {
		t.Fatal("checkSelfTriggerAllowance returned nil error for a genuinely unreadable count")
	}
	var refusal *RefusalError
	if !isRefusalError(err, &refusal) {
		t.Fatalf("error = %v, want *RefusalError", err)
	}
	if refusal.Gate != RefusalSelfTrigger {
		t.Errorf("gate = %q, want %q", refusal.Gate, RefusalSelfTrigger)
	}

	after := launchRefusedCount(string(RefusalSelfTrigger))
	if after != before+1 {
		t.Errorf("office_launch_refused_total{gate=self_trigger} = %d, want %d (every refusal must increment it, including a genuine gate-read failure)", after, before+1)
	}
}
