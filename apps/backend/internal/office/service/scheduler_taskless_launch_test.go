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

// TestSchedulerTick_TasklessRunFailsInsteadOfFinishing is the primary WO-35
// regression test. wakeup/dispatcher.go's createFreshRun inserts a
// `Payload: "{}"` run for every lightweight-routine fire (the pre-installed
// coordinator heartbeat, among others) — no task_id, ever. Before the fix,
// SchedulerIntegration.launchOrLog returned true for taskID=="" without
// calling the task starter, and prepareAndLaunch's fall-through called
// finishRun, so the run reached status=finished with an empty session_id
// and an empty error_message: a run that never launched an agent, reported
// as a success. That is exactly the shape of the card's "323 consecutive
// successful runs, zero agent sessions" measurement.
//
// The scheduler cannot currently launch a taskless run at all (that
// requires a taskless session seam — task_sessions.task_id is NOT NULL —
// which is a follow-up feature, not part of this card). So the correct
// terminal state today is a loud, immediate failure, not silent success.
func TestSchedulerTick_TasklessRunFailsInsteadOfFinishing(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "coordinator-wo35",
		WorkspaceID:        "ws-1",
		Name:               "coordinator-wo35",
		Role:               models.AgentRoleCEO,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Mirrors wakeup/dispatcher.go's createFreshRun: reason from the
	// routine trigger, payload literally "{}" (no task_id).
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if mock.callCount() != 0 {
		t.Fatalf("expected 0 StartTask calls for a taskless run, got %d", mock.callCount())
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
	if runs[0].Status != service.RunStatusFailed {
		t.Fatalf("run status = %q, want %q — an un-launched run must not report success",
			runs[0].Status, service.RunStatusFailed)
	}
	if runs[0].ErrorMessage == "" {
		t.Error("expected a non-empty error_message explaining the run could not launch")
	}
	if runs[0].SessionID != "" {
		t.Errorf("session_id = %q, want empty — no agent was launched", runs[0].SessionID)
	}
}

// TestSchedulerTick_TaskBoundRunStillLaunches is the regression guard
// alongside the taskless-failure fix above: an ordinary task-bound run with
// a wired task starter must still launch normally and stay `claimed` (not
// reach a terminal state synchronously) — proving launchAgent's new
// taskID=="" / taskStarter==nil failure branches did not swallow the
// legitimate launch path.
func TestSchedulerTick_TaskBoundRunStillLaunches(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "worker-wo35",
		WorkspaceID:        "ws-1",
		Name:               "worker-wo35",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-wo35-1', 'ws-1', 'Build API', 'Implement endpoint', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-wo35-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if mock.callCount() != 1 {
		t.Fatalf("expected 1 StartTask call, got %d", mock.callCount())
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
	if runs[0].Status != service.RunStatusClaimed {
		t.Fatalf("run status = %q, want %q — a launched run stays claimed for the async completion subscriber",
			runs[0].Status, service.RunStatusClaimed)
	}
}

// TestSchedulerTick_TasklessRunsDoNotAutoPauseAgent is the WO-35 Review
// round 1 regression test. A taskless run is a scheduler capability gap
// (the taskless-launch seam does not exist yet — see failTasklessRun's
// SCOPE-1 decision comment in scheduler_integration.go), not an agent
// failure, so failing it must not touch the agent's consecutive-failure
// counter. The pre-installed "Coordinator heartbeat" routine is taskless
// by design and fires every 5 minutes
// (routines/service.go:133,155): if taskless failures counted toward
// DefaultAgentFailureThreshold (3), every default install would
// auto-pause its coordinator within ~15 minutes of a fresh boot. Once
// paused, scheduler_integration.go's !isAgentActive branch silently
// FinishRuns every subsequent run for that agent — including task-bound,
// event-driven ones that work today — which is exactly the
// reports-success-but-does-nothing pathology this card exists to kill,
// now applied to the path the card's own SYMPTOM section says is
// unaffected.
func TestSchedulerTick_TasklessRunsDoNotAutoPauseAgent(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "coordinator-wo35-pause",
		WorkspaceID:        "ws-1",
		Name:               "coordinator-wo35-pause",
		Role:               models.AgentRoleCEO,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// DefaultAgentFailureThreshold consecutive taskless heartbeat fires —
	// mirrors 3 ticks of the pre-installed coordinator heartbeat routine.
	const firesAtThreshold = 3
	for i := 0; i < firesAtThreshold; i++ {
		if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
			t.Fatalf("queue taskless run %d: %v", i, err)
		}
		service.RunSchedulerTick(svc, ctx)
	}

	got, err := svc.GetAgentInstance(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.Status == models.AgentStatusPaused {
		t.Fatalf("agent auto-paused after %d taskless failures — a taskless run is a scheduler "+
			"capability gap, not an agent failure, and must not count toward auto-pause", firesAtThreshold)
	}
	if got.PauseReason != "" {
		t.Fatalf("pause_reason = %q, want empty", got.PauseReason)
	}
	if got.ConsecutiveFailures != 0 {
		t.Fatalf("consecutive_failures = %d, want 0 — taskless failures must not accumulate", got.ConsecutiveFailures)
	}

	// The event-driven path must still work after repeated taskless
	// failures: a task-bound run queued afterwards must still launch.
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-wo35-pause-1', 'ws-1', 'Build API', 'Implement endpoint', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-wo35-pause-1"}`, ""); err != nil {
		t.Fatalf("queue task-bound run: %v", err)
	}
	service.RunSchedulerTick(svc, ctx)

	if mock.callCount() != 1 {
		t.Fatalf("expected 1 StartTask call for the task-bound run after taskless failures, got %d — "+
			"the event-driven path must survive repeated taskless failures", mock.callCount())
	}
}

// TestSchedulerTick_TasklessRunFailure_PublishesResolvableWorkspaceEvent is
// the WO-35 Review round 2, Finding 1 regression test. A taskless run has
// no task_id, so the WS gateway's workspaceForEvent (office_notifications.go)
// cannot resolve a workspace for it the way it does for task-bound runs;
// without an explicit workspace_id in the payload, BroadcastToWorkspaceOrDrop
// silently drops the event and the failed run never reaches the UI — the
// same "reports nothing to the user" pathology the card exists to kill,
// just moved from the DB row to the live notification. failTasklessRun must
// publish OfficeRunProcessed with workspace_id set to the launching agent's
// own workspace.
func TestSchedulerTick_TasklessRunFailure_PublishesResolvableWorkspaceEvent(t *testing.T) {
	mock := &mockTaskStarter{}
	eb := bus.NewMemoryEventBus(logger.Default())
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock, EventBus: eb})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "coordinator-wo35-event",
		WorkspaceID:        "ws-1",
		Name:               "coordinator-wo35-event",
		Role:               models.AgentRoleCEO,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	got := make(chan *bus.Event, 1)
	sub, err := eb.Subscribe(events.OfficeRunProcessed, func(_ context.Context, e *bus.Event) error {
		got <- e
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	service.RunSchedulerTick(svc, ctx)

	select {
	case e := <-got:
		data, ok := e.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("event data not a map: %T", e.Data)
		}
		if data["status"] != service.RunStatusFailed {
			t.Errorf("status = %v, want %s", data["status"], service.RunStatusFailed)
		}
		if data["workspace_id"] != "ws-1" {
			t.Errorf("workspace_id = %v, want ws-1 — the WS gateway cannot resolve one from "+
				"task_id on a taskless run, so it must be set explicitly", data["workspace_id"])
		}
		if errorMessage, _ := data["error_message"].(string); errorMessage == "" {
			t.Error("expected a non-empty error_message in the processed event")
		}
	default:
		t.Fatalf("expected OfficeRunProcessed event for the failed taskless run; got none")
	}
}

// TestSchedulerTick_UnlaunchableRun_PublishesResolvableWorkspaceEvent is the
// WO-35 Review round 2, Finding 1 regression test for the sibling failure
// path: a task-bound run that cannot launch because no task starter is
// wired. This run DOES carry a task_id, so the pre-fix finishRun published
// OfficeRunProcessed for it and the WS gateway resolved its workspace via
// task_id — that live update must survive the fail-fast rewrite.
func TestSchedulerTick_UnlaunchableRun_PublishesResolvableWorkspaceEvent(t *testing.T) {
	eb := bus.NewMemoryEventBus(logger.Default())
	svc := newTestService(t, service.ServiceOptions{EventBus: eb}) // no TaskStarter wired
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "worker-wo35-unlaunchable",
		WorkspaceID:        "ws-1",
		Name:               "worker-wo35-unlaunchable",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-wo35-unlaunchable', 'ws-1', 'Build API', 'Implement endpoint', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	got := make(chan *bus.Event, 1)
	sub, err := eb.Subscribe(events.OfficeRunProcessed, func(_ context.Context, e *bus.Event) error {
		got <- e
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-wo35-unlaunchable"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	service.RunSchedulerTick(svc, ctx)

	select {
	case e := <-got:
		data, ok := e.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("event data not a map: %T", e.Data)
		}
		if data["status"] != service.RunStatusFailed {
			t.Errorf("status = %v, want %s", data["status"], service.RunStatusFailed)
		}
		if data["task_id"] != "task-wo35-unlaunchable" {
			t.Errorf("task_id = %v, want task-wo35-unlaunchable", data["task_id"])
		}
		if data["workspace_id"] != "ws-1" {
			t.Errorf("workspace_id = %v, want ws-1", data["workspace_id"])
		}
	default:
		t.Fatalf("expected OfficeRunProcessed event for the unlaunchable run; got none")
	}
}

// TestSchedulerTick_RepeatTasklessFailures_OnlyFirstStaysInInbox is the
// WO-35 Review round 2, Finding 2 regression test. The pre-installed
// "Coordinator heartbeat" routine fires every 5 minutes and is taskless by
// design, so every fire now writes a durable failed run (288/day/install,
// forever). Without capping the inbox view, ListFailedRunsForInbox's
// LIMIT 200 (ORDER BY failed_at DESC) fills with heartbeat noise within
// ~17 hours and evicts genuine failures. Only the agent's first taskless
// failure must stay visible; the rest are auto-dismissed via the existing
// "_auto" sentinel.
func TestSchedulerTick_RepeatTasklessFailures_OnlyFirstStaysInInbox(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "coordinator-wo35-inbox",
		WorkspaceID:        "ws-1",
		Name:               "coordinator-wo35-inbox",
		Role:               models.AgentRoleCEO,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	const fires = 3
	var firstRunID string
	for i := 0; i < fires; i++ {
		if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
			t.Fatalf("queue taskless run %d: %v", i, err)
		}
		service.RunSchedulerTick(svc, ctx)
		if i == 0 {
			runs, err := svc.ListRuns(ctx, "ws-1")
			if err != nil {
				t.Fatalf("list first run: %v", err)
			}
			if len(runs) != 1 {
				t.Fatalf("run count after first fire = %d, want 1", len(runs))
			}
			firstRunID = runs[0].ID
		}
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != fires {
		t.Fatalf("run count = %d, want %d — every fire must still leave its own failed row", len(runs), fires)
	}
	for _, r := range runs {
		if r.Status != service.RunStatusFailed {
			t.Errorf("run %s status = %q, want %q", r.ID, r.Status, service.RunStatusFailed)
		}
	}

	rows, err := svc.ListFailedRunInboxRows(ctx, "ws-1", "some-user-who-dismissed-nothing")
	if err != nil {
		t.Fatalf("list inbox rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("inbox rows = %d, want 1 — only the first taskless failure should stay "+
			"visible, the rest auto-dismissed", len(rows))
	}
	if rows[0].RunID != firstRunID {
		t.Fatalf("inbox run ID = %s, want first failed run %s", rows[0].RunID, firstRunID)
	}
}

// TestSchedulerTick_RepeatTasklessFailures_StayVisiblePerRoutineScope proves
// that one routine cannot hide another routine's first failure when both
// routines run on the same agent. ContinuationScope is persisted at run
// creation in production. The test updates it after QueueRun to model two
// routine wakeups without starting the full wakeup dispatcher.
func TestSchedulerTick_RepeatTasklessFailures_StayVisiblePerRoutineScope(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "coordinator-wo35-scopes",
		WorkspaceID:        "ws-1",
		Name:               "coordinator-wo35-scopes",
		Role:               models.AgentRoleCEO,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	queueRoutineFailure := func(scope, idempotencyKey string) string {
		t.Helper()
		if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, idempotencyKey); err != nil {
			t.Fatalf("queue routine %s: %v", scope, err)
		}
		runs, err := svc.ListRuns(ctx, "ws-1")
		if err != nil {
			t.Fatalf("list runs for routine %s: %v", scope, err)
		}
		var runID string
		for _, run := range runs {
			if run.IdempotencyKey != nil && *run.IdempotencyKey == idempotencyKey {
				runID = run.ID
				break
			}
		}
		if runID == "" {
			t.Fatalf("queued run for routine %s not found", scope)
		}
		svc.ExecSQL(t, `UPDATE runs SET continuation_scope = ? WHERE id = ?`, scope, runID)
		service.RunSchedulerTick(svc, ctx)
		return runID
	}

	firstRoutineA := queueRoutineFailure("routine:routine-a", "routine-a-1")
	firstRoutineB := queueRoutineFailure("routine:routine-b", "routine-b-1")
	repeatRoutineA := queueRoutineFailure("routine:routine-a", "routine-a-2")

	rows, err := svc.ListFailedRunInboxRows(ctx, "ws-1", "some-user-who-dismissed-nothing")
	if err != nil {
		t.Fatalf("list inbox rows: %v", err)
	}
	visible := make(map[string]bool, len(rows))
	for _, row := range rows {
		visible[row.RunID] = true
	}
	if len(rows) != 2 {
		t.Fatalf("inbox rows = %d, want one visible failure per routine", len(rows))
	}
	if !visible[firstRoutineA] || !visible[firstRoutineB] {
		t.Fatalf("visible run IDs = %v, want first failures %s and %s", visible, firstRoutineA, firstRoutineB)
	}
	if visible[repeatRoutineA] {
		t.Fatalf("repeat failure %s remains visible", repeatRoutineA)
	}
	dismissed, err := svc.IsInboxItemDismissed(ctx, "_auto", service.InboxKindAgentRunFailed, repeatRoutineA)
	if err != nil {
		t.Fatalf("check repeat dismissal: %v", err)
	}
	if !dismissed {
		t.Fatalf("repeat failure %s was not auto-dismissed", repeatRoutineA)
	}
}
