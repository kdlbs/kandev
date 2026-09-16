package service

// Internal (white-box) test file: checkSelfTriggerTotalAllowance is
// unexported, so exercising its genuine-error and context-cancellation
// handling directly needs package-level access, unlike every other test
// in this directory (package service_test). Mirrors
// causation_self_trigger_genuine_error_test.go and the self-trigger
// cases in causation_shutdown_context_test.go for the sibling
// per-reason gate.

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// TestCheckSelfTriggerTotalAllowance_GenuineErrorIncrementsLaunchRefusedTotal
// pins AC-OFFICE-LAUNCH-SAFETY-004.9 for the total gate: a genuine (not
// context-canceled, not sql.ErrNoRows) failure reading the total
// self-trigger count still refuses the enqueue via *RefusalError, labelled
// self_trigger_total, and counts toward the durable gate-failure record.
func TestCheckSelfTriggerTotalAllowance_GenuineErrorIncrementsLaunchRefusedTotal(t *testing.T) {
	svc := newShutdownContextTestService(t)
	ctx := context.Background()

	tx, err := svc.repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

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

	before := launchRefusedCount(string(RefusalSelfTriggerTotal))

	err = svc.checkSelfTriggerTotalAllowance(ctx, tx, rec, "agent-1", req, "ws-1", "causation-1", 0)
	if err == nil {
		t.Fatal("checkSelfTriggerTotalAllowance returned nil error for a genuinely unreadable count")
	}
	var refusal *RefusalError
	if !isRefusalError(err, &refusal) {
		t.Fatalf("error = %v, want *RefusalError", err)
	}
	if refusal.Gate != RefusalSelfTriggerTotal {
		t.Errorf("gate = %q, want %q", refusal.Gate, RefusalSelfTriggerTotal)
	}

	after := launchRefusedCount(string(RefusalSelfTriggerTotal))
	if after != before+1 {
		t.Errorf("office_launch_refused_total{gate=self_trigger_total} = %d, want %d", after, before+1)
	}
}

// TestCheckSelfTriggerTotalAllowance_ContextCanceledDefersWithoutRecordingFailure
// is the total-gate counterpart of
// TestCheckSelfTriggerAllowance_ContextCanceledDefersWithoutRecordingFailure:
// a canceled context reading the total count must still refuse the wake
// (AC-OFFICE-LAUNCH-SAFETY-004.9's "refused whatever cancelled it") but
// must not be recorded as a gate failure, so a restart is not mistaken
// for this gate failing closed.
func TestCheckSelfTriggerTotalAllowance_ContextCanceledDefersWithoutRecordingFailure(t *testing.T) {
	svc := newShutdownContextTestService(t)
	ctx := context.Background()

	tx, err := svc.repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	rec := &gateOutcomeRecorder{}
	req := QueueRunRequest{
		AgentProfileID: "agent-1",
		Reason:         "self_trigger_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-1",
	}

	err = svc.checkSelfTriggerTotalAllowance(canceledCtx, tx, rec, "agent-1", req, "ws-1", "causation-1", 0)
	if err == nil {
		t.Fatal("checkSelfTriggerTotalAllowance returned nil error for a context-canceled read")
	}
	var refusal *RefusalError
	if !isRefusalError(err, &refusal) {
		t.Fatalf("error = %v, want *RefusalError", err)
	}
	if refusal.Gate != RefusalSelfTriggerTotal {
		t.Errorf("gate = %q, want %q", refusal.Gate, RefusalSelfTriggerTotal)
	}
	if len(rec.outcomes) != 0 {
		t.Errorf("rec.outcomes = %+v, want empty (a shutdown cancellation must not record a gate outcome)", rec.outcomes)
	}
}
