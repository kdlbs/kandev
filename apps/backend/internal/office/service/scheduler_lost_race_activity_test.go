package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// ignoreFinishedWrite installs a trigger that makes the given run's
// terminal-transition write to status='finished' a no-op, simulating
// another writer having already claimed the terminal-transition race.
func ignoreFinishedWrite(t *testing.T, svc *service.Service, runID string) {
	t.Helper()
	svc.ExecSQL(t, fmt.Sprintf(`
		CREATE TRIGGER ignore_finished_write
		BEFORE UPDATE OF status ON runs
		WHEN NEW.status = 'finished' AND OLD.id = '%s'
		BEGIN
			SELECT RAISE(IGNORE);
		END;
	`, runID))
}

// A heartbeat run that loses the idle-skip terminal-transition race (another
// writer already moved it out of 'claimed') must not log a false
// run_idle_skipped activity entry: the run's real terminal state came from
// whichever writer actually won, and a stray activity entry misreports why
// it ended.
func TestSchedulerIntegration_IdleSkipLostRaceDoesNotLogFalseActivity(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "idle-worker-race",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Worker defaults to skip_idle_runs=true, no tasks assigned.

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonHeartbeat, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs: %v (%d)", err, len(runs))
	}
	ignoreFinishedWrite(t, svc, runs[0].ID)

	service.RunSchedulerTick(svc, ctx)

	if mock.callCount() != 0 {
		t.Errorf("expected 0 StartTask calls, got %d", mock.callCount())
	}

	entries, err := svc.RepoForTest().ListActivityEntriesByAction(ctx, "ws-1", "run_idle_skipped", 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("run_idle_skipped activity entries = %d, want 0 for a lost terminal-transition race", len(entries))
	}
}

// A run that loses the budget-blocked terminal-transition race has the same
// obligation: no false run_budget_blocked activity entry.
func TestSchedulerIntegration_BudgetBlockedLostRaceDoesNotLogFalseActivity(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "budget-worker-race",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "agent",
		ScopeID:           agent.ID,
		LimitSubcents:     100,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "pause_agent",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	insertTestTask(t, svc, "task-budget-race", "ws-1")
	insertTestCostEvent(t, svc, agent.ID, "task-budget-race", int64(600))

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-budget-race"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs: %v (%d)", err, len(runs))
	}
	ignoreFinishedWrite(t, svc, runs[0].ID)

	service.RunSchedulerTick(svc, ctx)

	entries, err := svc.RepoForTest().ListActivityEntriesByAction(ctx, "ws-1", "run_budget_blocked", 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("run_budget_blocked activity entries = %d, want 0 for a lost terminal-transition race", len(entries))
	}
}
