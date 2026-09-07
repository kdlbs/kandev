package scheduler_test

// Regression coverage for a live production bug: models.PriorityClass's Go
// zero value is PriorityClassHuman (0), the *highest* claim-order
// preference — not PriorityClassEvent (2), the intended default for
// ordinary wakes. SchedulerService.QueueRun/QueueRunCtx build a
// models.Run{} literal directly (this package bypasses
// runs/service.resolveCausation entirely) and previously omitted
// PriorityClass, so every run enqueued through it — including the
// approval_adapter.go and reactivity.go production call sites — silently
// shipped as PriorityClassHuman, undermining
// AC-OFFICE-BACKPRESSURE-001.1/.3's claim-ordering guarantee.

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/scheduler"
	"github.com/kandev/kandev/internal/office/service"
)

// newPriorityClassTestScheduler wires a real *service.Service (not nil):
// QueueRun's guardAgentStatus calls svc.GetAgentFromConfig, which panics
// on a nil Service.
func newPriorityClassTestScheduler(t *testing.T, repo *officesqlite.Repository) *scheduler.SchedulerService {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	svc := service.NewService(service.ServiceOptions{Repo: repo, Logger: log})
	return scheduler.NewSchedulerService(repo, log, svc)
}

func TestQueueRun_StampsEventPriorityClass(t *testing.T) {
	repo := newTestRepoSched(t)
	ss := newPriorityClassTestScheduler(t, repo)
	ctx := context.Background()

	if err := repo.CreateAgentInstance(ctx, &models.AgentInstance{
		ID:                    testAgentID,
		WorkspaceID:           testWorkspaceID,
		Name:                  testAgentID,
		Status:                models.AgentStatusIdle,
		MaxConcurrentSessions: 1,
	}); err != nil {
		t.Fatalf("create agent instance: %v", err)
	}

	if err := ss.QueueRun(ctx, testAgentID, scheduler.RunReasonTaskAssigned, `{}`, ""); err != nil {
		t.Fatalf("QueueRun: %v", err)
	}

	claimed, err := ss.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed == nil {
		t.Fatal("expected a claimable run")
	}
	if claimed.PriorityClass != models.PriorityClassEvent {
		t.Fatalf("priority class = %v, want %v (PriorityClassHuman zero-value regression)",
			claimed.PriorityClass, models.PriorityClassEvent)
	}
}

func TestQueueRunCtx_StampsEventPriorityClass(t *testing.T) {
	repo := newTestRepoSched(t)
	ss := newPriorityClassTestScheduler(t, repo)
	ctx := context.Background()

	if err := repo.CreateAgentInstance(ctx, &models.AgentInstance{
		ID:                    testAgentID,
		WorkspaceID:           testWorkspaceID,
		Name:                  testAgentID,
		Status:                models.AgentStatusIdle,
		MaxConcurrentSessions: 1,
	}); err != nil {
		t.Fatalf("create agent instance: %v", err)
	}

	c := scheduler.RunContext{
		Reason: scheduler.RunReasonApprovalResolved,
		TaskID: "task-1",
	}
	if err := ss.QueueRunCtx(ctx, testAgentID, c); err != nil {
		t.Fatalf("QueueRunCtx: %v", err)
	}

	claimed, err := ss.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed == nil {
		t.Fatal("expected a claimable run")
	}
	if claimed.PriorityClass != models.PriorityClassEvent {
		t.Fatalf("priority class = %v, want %v (PriorityClassHuman zero-value regression)",
			claimed.PriorityClass, models.PriorityClassEvent)
	}
}
