package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// TestUpdateRunReasonIfQueued_QueuedRow_Promotes is the WO-46.1 R2-F1
// regression test: the conditional UPDATE must land, and report that it
// landed, while the row is still queued.
func TestUpdateRunReasonIfQueued_QueuedRow_Promotes(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	run := mustCreateRun(t, repo, &models.Run{
		AgentProfileID: "a1",
		Reason:         shared.RunReasonRoutineDispatchCron,
		Payload:        "{}",
		Status:         "queued",
		CoalescedCount: 1,
	})

	promoted, err := repo.UpdateRunReasonIfQueued(ctx, run.ID, shared.RunReasonRoutineDispatchEvent)
	if err != nil {
		t.Fatalf("UpdateRunReasonIfQueued: %v", err)
	}
	if !promoted {
		t.Fatal("expected promoted=true for a still-queued run")
	}
	got := mustGetRun(t, repo, run.ID)
	if got.Reason != shared.RunReasonRoutineDispatchEvent {
		t.Errorf("reason = %q, want promoted value", got.Reason)
	}
}

// TestUpdateRunReasonIfQueued_ClaimedRow_LeavesReasonUntouched proves the
// fix for R2-F1: once the scheduler has claimed the row, the promotion
// must not land — the affected-row count, not a stale in-memory status
// read earlier, is what decides. A claimed run is already in
// processRun's hands with the reason it captured at claim time; writing
// a new reason here would race that decision without anyone re-reading
// it.
func TestUpdateRunReasonIfQueued_ClaimedRow_LeavesReasonUntouched(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	run := mustCreateRun(t, repo, &models.Run{
		AgentProfileID: "a1",
		Reason:         shared.RunReasonRoutineDispatchCron,
		Payload:        "{}",
		Status:         "queued",
		CoalescedCount: 1,
	})
	setStatus(t, repo, run.ID, "claimed", timePtr(time.Now().UTC()), nil)

	promoted, err := repo.UpdateRunReasonIfQueued(ctx, run.ID, shared.RunReasonRoutineDispatchEvent)
	if err != nil {
		t.Fatalf("UpdateRunReasonIfQueued: %v", err)
	}
	if promoted {
		t.Fatal("expected promoted=false for an already-claimed run")
	}
	got := mustGetRun(t, repo, run.ID)
	if got.Reason != shared.RunReasonRoutineDispatchCron {
		t.Errorf("reason = %q, want unchanged (claimed rows must not be mutated)", got.Reason)
	}
}
