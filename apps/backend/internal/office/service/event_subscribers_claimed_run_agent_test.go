package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// TestSchedulerTick_AgentCompletedForNonLatestClaimReleasesTheCompletingAgentsOwnRun
// pins the Review round-4 BLOCKING FINDING 2 fix. ClaimNextEligibleRun's
// busy-lock is scoped per-agent, not per-task (see
// internal/runs/repository/sqlite/runs.go), so two different agents' runs
// on the SAME task can both be 'claimed' simultaneously. Before this fix,
// handleAgentCompleted resolved the completing run via the unscoped
// GetClaimedRunByTaskID, which prefers the most-recently-claimed row
// regardless of which agent actually sent the AgentCompleted event. When
// agent A's claimed row happened to be newer than the completing agent B's,
// the handler released A's checkout (a no-op, since B actually holds it -
// leaking B's checkout forever) and finished A's still-live run, even
// though A never completed anything.
func TestSchedulerTick_AgentCompletedForNonLatestClaimReleasesTheCompletingAgentsOwnRun(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	svc.SetSyncHandlers(true)
	ctx := context.Background()
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}

	agentA := &models.AgentInstance{
		ID:          "agent-claimed-a",
		WorkspaceID: "ws-1",
		Name:        "claimed-a",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, agentA); err != nil {
		t.Fatalf("create agent A: %v", err)
	}
	agentB := &models.AgentInstance{
		ID:          "agent-claimed-b",
		WorkspaceID: "ws-1",
		Name:        "claimed-b",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, agentB); err != nil {
		t.Fatalf("create agent B: %v", err)
	}

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-claimed-race-1', 'ws-1', 'Claimed Race Task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	base := time.Now().UTC().Add(-time.Hour)

	// Agent B's run: claimed first (older claimed_at), and B is the one
	// that actually acquired the checkout and is now completing.
	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, requested_at, claimed_at
		) VALUES (
			'run-claimed-b', ?, 'task_assigned', '{"task_id":"task-claimed-race-1"}',
			'claimed', 1, '{}', ?, ?
		)
	`, agentB.ID, base, base)
	ok, err := svc.CheckoutTask(ctx, "task-claimed-race-1", agentB.ID)
	if err != nil || !ok {
		t.Fatalf("seed B's checkout: ok=%v err=%v", ok, err)
	}

	// Agent A's run: claimed AFTER B's (newer claimed_at), but A never
	// actually reached checkoutTask and is not the run that is completing.
	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, requested_at, claimed_at
		) VALUES (
			'run-claimed-a', ?, 'task_assigned', '{"task_id":"task-claimed-race-1"}',
			'claimed', 1, '{}', ?, ?
		)
	`, agentA.ID, base, base.Add(time.Minute))

	event := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          "task-claimed-race-1",
		"session_id":       "session-claimed-b",
		"agent_profile_id": agentB.ID,
	})
	if err := eb.Publish(ctx, events.AgentCompleted, event); err != nil {
		t.Fatalf("publish agent completed: %v", err)
	}

	// B's checkout must be released: a third agent must now be able to
	// take it. Before the fix this stayed leaked, because the handler
	// tried to release A's checkout (a no-op) instead of B's.
	stillHeld, err := svc.CheckoutTask(ctx, "task-claimed-race-1", "agent-third")
	if err != nil {
		t.Fatalf("checkout after completion: %v", err)
	}
	if !stillHeld {
		t.Fatal("expected task checkout to be released after B's AgentCompleted, but it is still held (leaked)")
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	byID := make(map[string]models.Run, len(runs))
	for _, r := range runs {
		byID[r.ID] = *r
	}
	runA, okA := byID["run-claimed-a"]
	if !okA {
		t.Fatalf("expected run-claimed-a to still exist, got: %v", runs)
	}
	runB, okB := byID["run-claimed-b"]
	if !okB {
		t.Fatalf("expected run-claimed-b to still exist, got: %v", runs)
	}
	if runB.Status != service.RunStatusFinished {
		t.Fatalf("run-claimed-b status = %q, want finished (it is the run that actually completed)", runB.Status)
	}
	if runA.Status != service.RunStatusClaimed {
		t.Fatalf("run-claimed-a status = %q, want claimed (untouched - it never completed)", runA.Status)
	}
}
