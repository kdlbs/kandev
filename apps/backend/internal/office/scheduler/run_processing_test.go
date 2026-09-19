package scheduler_test

// Covers docs/specs/task-delivery-ledger/spec.md, "Office run outcome":
// SchedulerService.FinishRun and .FailRun (run_processing.go) are a dormant
// parallel copy of the service-layer FinishRun/FailRun with no production
// caller today, but the spec names them explicitly (scenarios at lines
// 2077-2085) because they reach the same runs table through a different
// path than transitionRunTerminal: outcome must be written NULL on both,
// never one of the eight-value vocabulary invented for a path with no
// established semantic label.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/scheduler"
)

func seedRunForProcessing(t *testing.T, repo interface {
	CreateRun(ctx context.Context, req *models.Run) error
}, agentID, reason string) *models.Run {
	t.Helper()
	run := &models.Run{
		ID:             uuid.New().String(),
		AgentProfileID: agentID,
		Reason:         reason,
		Payload:        `{}`,
		Status:         scheduler.RunStatusClaimed,
		CoalescedCount: 1,
		RequestedAt:    time.Now().UTC(),
	}
	if err := repo.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	return run
}

// TestSchedulerService_FinishRun_WritesFinishedWithNilOutcome covers
// run_processing.go:35-37. This path has no established semantic label for
// what happened, so it must never be bucketed as "processed" or any other
// value from the eight-value vocabulary.
func TestSchedulerService_FinishRun_WritesFinishedWithNilOutcome(t *testing.T) {
	repo := newTestRepoSched(t)
	ss := buildScheduler(t, repo, newFakeTaskStarter())
	ctx := context.Background()

	run := seedRunForProcessing(t, repo, testAgentID, scheduler.RunReasonTaskAssigned)

	if err := ss.FinishRun(ctx, run.ID); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	got, err := repo.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != scheduler.RunStatusFinished {
		t.Fatalf("status = %q, want %q", got.Status, scheduler.RunStatusFinished)
	}
	if got.Outcome != nil {
		t.Fatalf("outcome = %q, want nil", *got.Outcome)
	}
}

// TestSchedulerService_FailRun_WritesFailedWithNilOutcome covers
// run_processing.go:39-43 — bucketed on status alone, same as the
// service-layer FailRun.
func TestSchedulerService_FailRun_WritesFailedWithNilOutcome(t *testing.T) {
	repo := newTestRepoSched(t)
	ss := buildScheduler(t, repo, newFakeTaskStarter())
	ctx := context.Background()

	run := seedRunForProcessing(t, repo, testAgentID, scheduler.RunReasonTaskAssigned)

	if err := ss.FailRun(ctx, run.ID); err != nil {
		t.Fatalf("fail run: %v", err)
	}

	got, err := repo.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != scheduler.RunStatusFailed {
		t.Fatalf("status = %q, want %q", got.Status, scheduler.RunStatusFailed)
	}
	if got.Outcome != nil {
		t.Fatalf("outcome = %q, want nil", *got.Outcome)
	}
}

// TestSchedulerService_ClaimNextRun_ReturnsQueuedRun exercises ClaimNextRun
// (run_processing.go:15-28), the third method in this file, so the whole
// dormant-copy surface has direct coverage rather than only the two
// outcome-bearing terminal methods.
func TestSchedulerService_ClaimNextRun_ReturnsQueuedRun(t *testing.T) {
	repo := newTestRepoSched(t)
	ss := buildScheduler(t, repo, newFakeTaskStarter())
	ctx := context.Background()

	settingsAgent := &models.AgentInstance{
		ID:          testAgentID,
		WorkspaceID: testWorkspaceID,
		Name:        "claim-next-agent",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := repo.CreateAgentInstance(ctx, settingsAgent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	queued := &models.Run{
		ID:             uuid.New().String(),
		AgentProfileID: testAgentID,
		Reason:         scheduler.RunReasonTaskAssigned,
		Payload:        `{}`,
		Status:         scheduler.RunStatusQueued,
		CoalescedCount: 1,
		RequestedAt:    time.Now().UTC(),
	}
	if err := repo.CreateRun(ctx, queued); err != nil {
		t.Fatalf("seed queued run: %v", err)
	}

	claimed, err := ss.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed == nil {
		t.Fatal("expected a claimed run")
	}
	if claimed.ID != queued.ID {
		t.Fatalf("claimed id = %q, want %q", claimed.ID, queued.ID)
	}
	if claimed.Status != scheduler.RunStatusClaimed {
		t.Fatalf("status = %q, want %q", claimed.Status, scheduler.RunStatusClaimed)
	}

	next, err := ss.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim again: %v", err)
	}
	if next != nil {
		t.Fatalf("expected nil on empty queue, got %+v", next)
	}
}
