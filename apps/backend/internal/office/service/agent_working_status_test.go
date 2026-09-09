package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// DR-14: "working" is a defined agent status that nothing ever assigned, so
// every Office agent read "idle" permanently — including while a run was in
// flight. These tests cover the write at the launch boundary and the reset
// on all three terminal paths (success, failure, cancellation).

// launchedWorkingAgent seeds an agent + task, ticks the scheduler so the run
// is actually handed to the adapter, and returns the agent. On return the
// agent is mid-run: its run row is `claimed` and no terminal event has been
// delivered yet.
func launchedWorkingAgent(
	t *testing.T, svc *service.Service, ctx context.Context, name, taskID string,
) *models.AgentInstance {
	t.Helper()
	agent := makeAgent(name, models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// The task needs a project carrying an executor config: executor
	// resolution runs before the launch, and a run that fails there is
	// retried without ever reaching the adapter.
	project := &models.Project{
		WorkspaceID:    "ws-1",
		Name:           "Project " + name,
		ExecutorConfig: `{"type":"local_pc"}`,
	}
	if err := svc.CreateProject(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, project_id, title, created_at, updated_at)
		VALUES (?, 'ws-1', ?, 'Working status task', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		taskID, project.ID)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"`+taskID+`"}`, ""); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	service.RunSchedulerTick(svc, ctx)
	return agent
}

func assertAgentStatus(
	t *testing.T, svc *service.Service, ctx context.Context,
	agentID string, want models.AgentStatus, when string,
) {
	t.Helper()
	got, err := svc.GetAgentFromConfig(ctx, agentID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.Status != want {
		t.Fatalf("status %s = %q, want %q", when, got.Status, want)
	}
}

// TestAgentStatus_WorkingWhileRunInFlight is the core DR-14 regression pin:
// before this change the assertion below read "idle" for an agent whose run
// had just been handed to the adapter, making a busy workspace and a stalled
// one indistinguishable.
func TestAgentStatus_WorkingWhileRunInFlight(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := launchedWorkingAgent(t, svc, ctx, "worker-inflight", "task-working-inflight")

	if mock.callCount() != 1 {
		t.Fatalf("StartTask calls = %d, want 1: the run must actually launch "+
			"for the status to mean anything", mock.callCount())
	}
	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusWorking, "while the run is in flight")
}

// TestAgentStatus_ReturnsToIdleOnCompletion covers the success path:
// AgentCompleted -> handleAgentCompleted -> stampRunFinished.
func TestAgentStatus_ReturnsToIdleOnCompletion(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	svc.SetSyncHandlers(true)
	ctx := context.Background()
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}

	agent := launchedWorkingAgent(t, svc, ctx, "worker-completes", "task-working-complete")
	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusWorking, "before completion")

	publishLifecycle(t, ctx, eb, events.AgentCompleted, "task-working-complete", agent.ID)

	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusIdle, "after completion")
}

// TestAgentStatus_ReturnsToIdleOnFailure covers the failure path:
// AgentFailed -> handleAgentFailed -> HandleAgentFailure. A failed run that
// left the agent pinned to "working" would be the worst outcome of this
// feature — it reads as progress that is not happening.
func TestAgentStatus_ReturnsToIdleOnFailure(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	svc.SetSyncHandlers(true)
	ctx := context.Background()
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}

	agent := launchedWorkingAgent(t, svc, ctx, "worker-fails", "task-working-fail")
	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusWorking, "before failure")

	event := bus.NewEvent(events.AgentFailed, "test", map[string]string{
		"task_id":          "task-working-fail",
		"session_id":       "session-fail",
		"agent_profile_id": agent.ID,
		"error_message":    "boom",
	})
	if err := eb.Publish(ctx, events.AgentFailed, event); err != nil {
		t.Fatalf("publish agent failed: %v", err)
	}

	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusIdle, "after failure")
}

// TestAgentStatus_ReturnsToIdleOnStop covers the cancellation path.
// AgentStopped is wired to handleAgentCompleted (event_subscribers.go), so
// this shares the reset seam with the success path — pinned separately
// because a future split of those subscribers must not drop the reset.
func TestAgentStatus_ReturnsToIdleOnStop(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	svc.SetSyncHandlers(true)
	ctx := context.Background()
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}

	agent := launchedWorkingAgent(t, svc, ctx, "worker-stopped", "task-working-stop")
	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusWorking, "before stop")

	publishLifecycle(t, ctx, eb, events.AgentStopped, "task-working-stop", agent.ID)

	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusIdle, "after stop")
}

// TestAgentStatus_NotLeftWorkingWhenLaunchNeverHappened pins the branch that
// has no terminal event to clean up after it: launchAgent returned false, so
// no AgentCompleted/AgentFailed/AgentStopped will ever arrive. Without the
// explicit clear in prepareAndLaunch the agent would sit at "working"
// forever, permanently misreporting a run that never started.
func TestAgentStatus_NotLeftWorkingWhenLaunchNeverHappened(t *testing.T) {
	// No TaskStarter configured: launchAgent bails at its
	// "no task starter configured" guard, via failUnlaunchableRun.
	svc := newTestService(t)
	ctx := context.Background()

	agent := launchedWorkingAgent(t, svc, ctx, "worker-unlaunchable", "task-working-unlaunchable")

	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusIdle,
		"after a run that never reached the adapter")
}

// TestAgentStatus_ReturnsToIdleWhenNoClaimedRunResolves is the regression
// test for DR-14 review round 1 Finding 3: resolveLifecycleRun only ever
// looks up claimed runs, so a cancellation that already marked the run
// terminal, or a late/duplicate delivery, makes it resolve to sql.ErrNoRows.
// handleAgentCompleted (shared by AgentCompleted and AgentStopped) must
// still clear "working" on that exit — it never reaches stampRunFinished.
// The event carries the run's own id so the run-scoped clear (see
// TestAgentStatus_StaleEventDoesNotClobberSuccessorRun) still matches it.
func TestAgentStatus_ReturnsToIdleWhenNoClaimedRunResolves(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	svc.SetSyncHandlers(true)
	ctx := context.Background()
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}

	taskID := "task-working-no-claim"
	agent := launchedWorkingAgent(t, svc, ctx, "worker-no-claimed-run", taskID)
	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusWorking, "before the event")
	run := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonTaskAssigned)

	// The run already left `claimed` via another path (e.g.
	// ReapStaleCheckouts), so resolveLifecycleRun's GetClaimedRunByID(run.ID)
	// returns sql.ErrNoRows even though the event names this exact run.
	svc.ExecSQL(t, `UPDATE runs SET status = 'finished' WHERE id = ?`, run.ID)

	event := bus.NewEvent(events.AgentStopped, "test", map[string]string{
		"task_id":          taskID,
		"run_id":           run.ID,
		"session_id":       "session-" + agent.ID,
		"agent_profile_id": agent.ID,
	})
	if err := eb.Publish(ctx, events.AgentStopped, event); err != nil {
		t.Fatalf("publish agent stopped: %v", err)
	}

	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusIdle,
		"after an AgentStopped event whose own run no longer resolves as claimed")
}

// TestAgentStatus_StaleEventDoesNotClobberSuccessorRun is the interleaving
// regression: a stale/duplicate terminal event for a run that already left
// "claimed" must not reset an agent a SUCCESSOR run has since marked
// working. Before the working/idle CAS was run-scoped, this exact sequence
// flipped a live run's agent back to "idle" mid-run.
func TestAgentStatus_StaleEventDoesNotClobberSuccessorRun(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	svc.SetSyncHandlers(true)
	ctx := context.Background()
	eb := bus.NewMemoryEventBus(logger.Default())
	if err := svc.RegisterEventSubscribers(eb); err != nil {
		t.Fatalf("register subscribers: %v", err)
	}

	taskID := "task-working-stale-vs-successor"
	agent := launchedWorkingAgent(t, svc, ctx, "worker-stale-vs-successor", taskID)
	runA := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonTaskAssigned)
	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusWorking, "after run A launches")

	// Run A leaves `claimed` via another path, and a successor run B has
	// since been marked working for the same agent. Exercising the full
	// requeue/re-claim/re-launch path is covered by
	// TestAgentStatus_ReturnsToIdleWhenRequeuedRunHitsPreLaunchGate; here the
	// successor's ownership is seeded directly so this test isolates the
	// stale-event race.
	svc.ExecSQL(t, `UPDATE runs SET status = 'finished' WHERE id = ?`, runA.ID)
	svc.ExecSQL(t, `UPDATE agent_profiles SET working_run_id = 'run-b-successor' WHERE id = ?`, agent.ID)

	// Run A's own late/duplicate terminal event now arrives.
	event := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":          taskID,
		"run_id":           runA.ID,
		"session_id":       "session-" + agent.ID,
		"agent_profile_id": agent.ID,
	})
	if err := eb.Publish(ctx, events.AgentCompleted, event); err != nil {
		t.Fatalf("publish agent completed: %v", err)
	}

	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusWorking,
		"after run A's stale event: run B's working status must survive")
}

// TestAgentStatus_ReturnsToIdleWhenRequeuedRunHitsPreLaunchGate is the
// regression test for DR-14 review round 2 Finding 1: a launched run that
// gets requeued (e.g. a post-start provider fallback calling
// RequeueRunForNextCandidate) sets the run back to "queued" without ever
// clearing the agent's "working" status, since that clear only happens on
// the launch/complete cycle the requeue bypassed. The next scheduler pass
// for that run must not assume prepareAndLaunch will be reached again — it
// can terminate at any pre-launch gate (task-tree hold, budget, idle-skip,
// staleness, retry exhaustion) and must still release the agent.
//
// This reproduces the exact chain Review drove against the real scheduler:
// routed launch -> manual requeue (mirroring RequeueRunForNextCandidate) ->
// a task-tree hold now gates the run's task -> a second scheduler tick.
func TestAgentStatus_ReturnsToIdleWhenRequeuedRunHitsPreLaunchGate(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	taskID := "task-requeued-tree-held"
	agent := launchedWorkingAgent(t, svc, ctx, "worker-requeued-tree-held", taskID)
	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusWorking, "after the first launch")

	run := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonTaskAssigned)

	// Mirror RequeueRunForNextCandidate (repository/sqlite/run_routing.go):
	// a post-start provider fallback puts the run back in the queue without
	// touching agent status.
	svc.ExecSQL(t, `UPDATE runs SET status = 'queued', session_id = '',
		claimed_at = NULL, finished_at = NULL WHERE id = ?`, run.ID)

	// A task-tree hold now gates the task, so the re-dispatch pass will
	// terminate at checkoutTask, before prepareAndLaunch ever re-marks the
	// agent (or clears it).
	svc.ExecSQL(t, `INSERT INTO office_task_tree_holds
		(id, workspace_id, root_task_id, mode, created_at)
		VALUES ('hold-requeued-tree-held', 'ws-1', ?, ?, CURRENT_TIMESTAMP)`,
		taskID, models.TreeHoldModePause)
	svc.ExecSQL(t, `INSERT INTO office_task_tree_hold_members
		(hold_id, task_id, depth) VALUES ('hold-requeued-tree-held', ?, 0)`, taskID)

	service.RunSchedulerTick(svc, ctx)

	got, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != "finished" || got.Outcome == nil || *got.Outcome != service.RunOutcomeTaskTreeHeld {
		t.Fatalf("run status/outcome = %q/%v, want finished/%q",
			got.Status, got.Outcome, service.RunOutcomeTaskTreeHeld)
	}
	assertAgentStatus(t, svc, ctx, agent.ID, models.AgentStatusIdle,
		"after the requeued run was terminated at a pre-launch gate")
}

func publishLifecycle(
	t *testing.T, ctx context.Context, eb bus.EventBus,
	topic, taskID, agentID string,
) {
	t.Helper()
	event := bus.NewEvent(topic, "test", map[string]string{
		"task_id":          taskID,
		"session_id":       "session-" + agentID,
		"agent_profile_id": agentID,
	})
	if err := eb.Publish(ctx, topic, event); err != nil {
		t.Fatalf("publish %s: %v", topic, err)
	}
}
