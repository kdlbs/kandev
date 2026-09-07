package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// TestRequeueClaimedRun_RequeuesAndClearsClaimedAt proves a claimed run
// blocked by a gate discovered post-claim (e.g. a workspace pause) goes
// back to queued with claimed_at cleared, so it is picked up again like
// any other queued run once the gate lifts.
func TestRequeueClaimedRun_RequeuesAndClearsClaimedAt(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	run := mustCreateRun(t, repo, &models.Run{
		ID:             "run-requeue-1",
		AgentProfileID: "a1",
		Reason:         "task_assigned",
		Payload:        `{}`,
		Status:         "queued",
		CoalescedCount: 1,
	})
	claimed, err := repo.ClaimRun(ctx, run.AgentProfileID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != run.ID {
		t.Fatalf("claimed %q, want %q", claimed.ID, run.ID)
	}

	ok, err := repo.RequeueClaimedRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("requeue: %v", err)
	}
	if !ok {
		t.Fatal("expected requeue to report true")
	}

	got, err := repo.GetRunByID(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != "queued" {
		t.Fatalf("status = %q, want queued", got.Status)
	}
	if got.ClaimedAt != nil {
		t.Fatalf("claimed_at = %v, want nil", got.ClaimedAt)
	}
}

// TestRequeueClaimedRun_NoOpWhenNotClaimed proves the CAS only fires
// against a currently-claimed run: a queued, finished, or already-requeued
// run reports false rather than being mutated.
func TestRequeueClaimedRun_NoOpWhenNotClaimed(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	run := mustCreateRun(t, repo, &models.Run{
		ID:             "run-requeue-2",
		AgentProfileID: "a1",
		Reason:         "task_assigned",
		Payload:        `{}`,
		Status:         "queued",
		CoalescedCount: 1,
	})

	ok, err := repo.RequeueClaimedRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("requeue: %v", err)
	}
	if ok {
		t.Fatal("expected requeue of an already-queued run to report false")
	}

	ok, err = repo.RequeueClaimedRun(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("requeue nonexistent: %v", err)
	}
	if ok {
		t.Fatal("expected requeue of a nonexistent run to report false")
	}
}
