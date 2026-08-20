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

// TestSchedulerTick_AgentFailedForNonLatestClaimReleasesTheFailingAgentsOwnRun
// is the failure-path sibling of
// TestSchedulerTick_AgentCompletedForNonLatestClaimReleasesTheCompletingAgentsOwnRun.
// handleAgentFailed resolves the failing run via the same
// GetClaimedRunByTaskAndAgent scoping fix as handleAgentCompleted, so it
// needs the identical two-agent regression pin: an unscoped lookup could
// resolve to a different agent's still-live claimed run, releasing that
// agent's checkout and failing its run out from under it.
func TestSchedulerTick_AgentFailedForNonLatestClaimReleasesTheFailingAgentsOwnRun(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	svc.SetSyncHandlers(true)
	ctx := context.Background()
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}

	agentA := &models.AgentInstance{
		ID:          "agent-failed-a",
		WorkspaceID: "ws-1",
		Name:        "failed-a",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, agentA); err != nil {
		t.Fatalf("create agent A: %v", err)
	}
	agentB := &models.AgentInstance{
		ID:          "agent-failed-b",
		WorkspaceID: "ws-1",
		Name:        "failed-b",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, agentB); err != nil {
		t.Fatalf("create agent B: %v", err)
	}

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-failed-race-1', 'ws-1', 'Failed Race Task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	base := time.Now().UTC().Add(-time.Hour)

	// Agent B's run: claimed first (older claimed_at), and B is the one
	// that actually acquired the checkout and is now failing.
	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, requested_at, claimed_at
		) VALUES (
			'run-failed-b', ?, 'task_assigned', '{"task_id":"task-failed-race-1"}',
			'claimed', 1, '{}', ?, ?
		)
	`, agentB.ID, base, base)
	ok, err := svc.CheckoutTask(ctx, "task-failed-race-1", agentB.ID)
	if err != nil || !ok {
		t.Fatalf("seed B's checkout: ok=%v err=%v", ok, err)
	}

	// Agent A's run: claimed AFTER B's (newer claimed_at), but A never
	// actually reached checkoutTask and is not the run that is failing.
	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, requested_at, claimed_at
		) VALUES (
			'run-failed-a', ?, 'task_assigned', '{"task_id":"task-failed-race-1"}',
			'claimed', 1, '{}', ?, ?
		)
	`, agentA.ID, base, base.Add(time.Minute))

	event := bus.NewEvent(events.AgentFailed, "test", map[string]string{
		"task_id":          "task-failed-race-1",
		"session_id":       "session-failed-b",
		"agent_profile_id": agentB.ID,
		"error_message":    "boom",
	})
	if err := eb.Publish(ctx, events.AgentFailed, event); err != nil {
		t.Fatalf("publish agent failed: %v", err)
	}

	// B's checkout must be released: a third agent must now be able to
	// take it. An unscoped lookup would instead have tried to release A's
	// checkout (a no-op) and left B's leaked.
	stillHeld, err := svc.CheckoutTask(ctx, "task-failed-race-1", "agent-third")
	if err != nil {
		t.Fatalf("checkout after failure: %v", err)
	}
	if !stillHeld {
		t.Fatal("expected task checkout to be released after B's AgentFailed, but it is still held (leaked)")
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	byID := make(map[string]models.Run, len(runs))
	for _, r := range runs {
		byID[r.ID] = *r
	}
	runA, okA := byID["run-failed-a"]
	if !okA {
		t.Fatalf("expected run-failed-a to still exist, got: %v", runs)
	}
	runB, okB := byID["run-failed-b"]
	if !okB {
		t.Fatalf("expected run-failed-b to still exist, got: %v", runs)
	}
	if runB.Status != service.RunStatusFailed {
		t.Fatalf("run-failed-b status = %q, want failed (it is the run that actually failed)", runB.Status)
	}
	if runA.Status != service.RunStatusClaimed {
		t.Fatalf("run-failed-a status = %q, want claimed (untouched - it never failed)", runA.Status)
	}
}

// TestSchedulerTick_StaleLifecycleEventDoesNotFinishSuccessorRun pins the
// immutable run identity carried by lifecycle events. When a predecessor
// execution is stopped while a successor is already claimed, the predecessor
// event can arrive after the successor claim. Resolving only by task and agent
// would finish and release the successor instead of the run that emitted the
// event.
func TestSchedulerTick_StaleLifecycleEventDoesNotFinishSuccessorRun(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	svc.SetSyncHandlers(true)
	ctx := context.Background()
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}

	agent := &models.AgentInstance{
		ID:          "agent-lifecycle-race",
		WorkspaceID: "ws-1",
		Name:        "lifecycle-race",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES ('task-lifecycle-race', 'ws-1', 'Lifecycle Race Task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	base := time.Now().UTC().Add(-time.Hour)
	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, requested_at, claimed_at
		) VALUES (
			'run-lifecycle-predecessor', ?, 'task_assigned', '{"task_id":"task-lifecycle-race"}',
			'claimed', 1, '{}', ?, ?
		)`, agent.ID, base, base)
	svc.ExecSQL(t, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, coalesced_count,
			context_snapshot, requested_at, claimed_at
		) VALUES (
			'run-lifecycle-successor', ?, 'task_assigned', '{"task_id":"task-lifecycle-race"}',
			'claimed', 1, '{}', ?, ?
		)`, agent.ID, base, base.Add(time.Minute))
	if ok, err := svc.CheckoutTaskForRun(ctx, "task-lifecycle-race", agent.ID, "run-lifecycle-successor"); err != nil || !ok {
		t.Fatalf("seed successor checkout: ok=%v err=%v", ok, err)
	}

	event := bus.NewEvent(events.AgentStopped, "test", map[string]string{
		"task_id":          "task-lifecycle-race",
		"session_id":       "session-predecessor",
		"agent_profile_id": agent.ID,
		"run_id":           "run-lifecycle-predecessor",
	})
	if err := eb.Publish(ctx, events.AgentStopped, event); err != nil {
		t.Fatalf("publish stale agent stopped: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	byID := make(map[string]models.Run, len(runs))
	for _, run := range runs {
		byID[run.ID] = *run
	}
	if got := byID["run-lifecycle-predecessor"].Status; got != service.RunStatusFinished {
		t.Fatalf("predecessor status = %q, want finished", got)
	}
	if got := byID["run-lifecycle-successor"].Status; got != service.RunStatusClaimed {
		t.Fatalf("successor status = %q, want claimed", got)
	}
	if ok, err := svc.CheckoutTask(ctx, "task-lifecycle-race", "agent-third"); err != nil {
		t.Fatalf("successor checkout probe: %v", err)
	} else if ok {
		t.Fatal("stale predecessor event released the successor checkout")
	}
}
