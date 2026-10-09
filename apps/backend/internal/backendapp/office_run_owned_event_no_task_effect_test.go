package backendapp

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	officecosts "github.com/kandev/kandev/internal/office/costs"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officeservice "github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// runOwnedEventHarness bundles a routineCronHarness (real taskSvc/workflowSvc/
// orchestrator.Service on one bus) with a real office/service.Service wired
// onto that SAME bus (h.eventBus), so a lifecycle event this test publishes
// is observed by the real orchestrator exactly as it would be in production —
// unlike TestRoutine_CronFire_LightweightReachesSession, which stands up its
// own private, disconnected bus and therefore never exercises the
// orchestrator's reaction to a taskless event at all.
type runOwnedEventHarness struct {
	h         *routineCronHarness
	officeSvc *officeservice.Service
	launcher  *lightweightRunLauncher
	agent     *officemodels.AgentInstance
}

func newRunOwnedEventHarness(t *testing.T) *runOwnedEventHarness {
	t.Helper()
	h := newRoutineCronHarness(t)
	ctx := context.Background()

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	officeSvc := officeservice.NewService(officeservice.ServiceOptions{
		Repo: h.officeRepo, Logger: log, EventBus: h.eventBus,
	})
	officeSvc.SetSyncHandlers(true)
	if err := officeSvc.RegisterEventSubscribers(h.eventBus); err != nil {
		t.Fatalf("register event subscribers: %v", err)
	}
	activity := shared.NewActivityLogger(h.officeRepo, log)
	officeSvc.SetBudgetChecker(officecosts.NewCostService(h.officeRepo, log, activity, officeSvc, officeSvc))
	launcher := &lightweightRunLauncher{svc: officeSvc}
	officeSvc.SetRunSessionLauncher(launcher)

	// agent_profiles.agent_id is a NOT NULL FK to agents (CLI tool
	// registrations); newRoutineCronHarness opens its DB with FK enforcement
	// on, so CreateAgentInstance's DefaultAgentID fallback needs a seeded row.
	if _, err := h.db.Exec(
		`INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"cli-agent-run-owned", "CLI Agent",
	); err != nil {
		t.Fatalf("seed agents row: %v", err)
	}

	agent := &officemodels.AgentInstance{
		WorkspaceID: h.workspaceID, Name: "run-owned-event-assignee",
		// CEO defaults to SkipIdleRuns=false, sidestepping the idle-skip gate
		// entirely; this test is about the SessionID=="" guard, not idle-skip.
		Role: officemodels.AgentRoleCEO, Status: officemodels.AgentStatusIdle,
		ExecutorPreference: `{"type":"local_pc"}`,
	}
	if err := officeSvc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	return &runOwnedEventHarness{h: h, officeSvc: officeSvc, launcher: launcher, agent: agent}
}

// launchTasklessRun queues and launches one real taskless run through the
// production scheduler path, returning the run and the office_run_sessions
// row the launcher bound to it — genuine run-owned identity, not a
// hand-crafted one.
func (r *runOwnedEventHarness) launchTasklessRun(
	t *testing.T, ctx context.Context,
) (*officemodels.Run, *officemodels.RunSession) {
	t.Helper()
	wantCalls := len(r.launcher.calls) + 1
	if _, err := r.officeSvc.QueueRun(ctx, r.agent.ID, shared.RunReasonRoutineDispatchEvent, `{}`, ""); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	awaitLightweightLaunchCount(t, ctx, r.officeSvc, r.launcher, wantCalls)

	runs, err := r.officeSvc.ListRuns(ctx, r.h.workspaceID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	var run *officemodels.Run
	for _, candidate := range runs {
		if candidate.SessionID != "" {
			session, sessErr := r.officeSvc.RepoForTest().GetRunSession(ctx, candidate.SessionID)
			if sessErr == nil && session != nil && session.State != officemodels.RunSessionStateFinished &&
				session.State != officemodels.RunSessionStateInterrupted {
				run = candidate
			}
		}
	}
	if run == nil {
		t.Fatalf("no launched, still-open run found among %#v", runs)
	}
	session, err := r.officeSvc.RepoForTest().GetRunSession(ctx, run.SessionID)
	if err != nil || session == nil {
		t.Fatalf("get run session %q: %v", run.SessionID, err)
	}
	return run, session
}

// decoyTaskSession creates a real task and a real, hand-inserted RUNNING
// task_session under it — a row that WOULD be read or mutated by
// handleAgentCompletedLocked/handleAgentFailedLocked if either ever
// resolved a task session for a run-owned (SessionID=="") lifecycle event.
func decoyTaskSession(t *testing.T, ctx context.Context, h *routineCronHarness) *taskmodels.TaskSession {
	t.Helper()
	workflows, err := h.taskSvc.ListWorkflows(ctx, h.workspaceID, true)
	if err != nil || len(workflows) == 0 {
		t.Fatalf("ListWorkflows: workflows=%d err=%v", len(workflows), err)
	}
	steps, err := h.workflowSvc.ListStepsByWorkflow(ctx, workflows[0].ID)
	if err != nil || len(steps) == 0 {
		t.Fatalf("ListStepsByWorkflow: steps=%d err=%v", len(steps), err)
	}
	taskResult, err := h.taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID: h.workspaceID, WorkflowID: workflows[0].ID,
		WorkflowStepID: steps[0].ID, Title: "Decoy task for run-owned event guard",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	now := time.Now().UTC()
	session := &taskmodels.TaskSession{
		ID: "decoy-session-" + taskResult.Task.ID, TaskID: taskResult.Task.ID,
		AgentExecutionID: "decoy-execution", State: taskmodels.TaskSessionStateRunning,
		StartedAt: now, UpdatedAt: now,
	}
	if err := h.taskRepo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("create decoy task session: %v", err)
	}
	stored, err := h.taskRepo.GetTaskSession(ctx, session.ID)
	if err != nil || stored == nil {
		t.Fatalf("get decoy task session: %v", err)
	}
	return stored
}

// subscribeTaskMutationEvents counts task.updated/task.moved publications on
// bus for the duration of the calling test, per the repo's own rule
// (apps/backend/CLAUDE.md "Task lifecycle events") that any task-row
// mutation must publish one of these — so a zero count after the event under
// test is a repo-convention-aligned proxy for "no task mutation occurred".
func subscribeTaskMutationEvents(t *testing.T, eb bus.EventBus) *int32 {
	t.Helper()
	var count int32
	handler := func(_ context.Context, _ *bus.Event) error {
		atomic.AddInt32(&count, 1)
		return nil
	}
	for _, subject := range []string{events.TaskUpdated, events.TaskMoved} {
		sub, err := eb.Subscribe(subject, handler)
		if err != nil {
			t.Fatalf("subscribe %q: %v", subject, err)
		}
		t.Cleanup(func() { _ = sub.Unsubscribe() })
	}
	return &count
}

// TestRunOwnedLifecycleEvent_AgentCompleted_NoTaskSessionEffect pins item 3 of
// task-06's completion half: a run-owned AgentCompleted event (SessionID=="",
// carrying real RunID/RunSessionID/RunAttempt from a genuine taskless launch)
// must leave every existing task_session untouched and trigger no
// task.updated/task.moved workflow-transition event. handleAgentCompleted's
// `if data.SessionID == ""` branch (internal/orchestrator/event_handlers_agent.go)
// routes straight to handleAgentCompletedLocked with a nil guard, which then
// fails to resolve any task session for SessionID=="" and returns before any
// workflow evaluation — this test proves that behavior against the real
// orchestrator wired on the same bus the real Office service publishes on,
// not just by reading the source.
func TestRunOwnedLifecycleEvent_AgentCompleted_NoTaskSessionEffect(t *testing.T) {
	ctx := context.Background()
	r := newRunOwnedEventHarness(t)
	decoy := decoyTaskSession(t, ctx, r.h)
	run, session := r.launchTasklessRun(t, ctx)

	mutationCount := subscribeTaskMutationEvents(t, r.h.eventBus)

	if err := r.h.eventBus.Publish(ctx, events.AgentCompleted, bus.NewEvent(events.AgentCompleted, "test",
		lifecycle.AgentEventPayload{
			AgentExecutionID: session.ExecutionID, AgentID: "test-adapter", AgentProfileID: r.agent.ID,
			RunID: run.ID, RunSessionID: session.ID, RunAttempt: session.Attempt,
			OwnerKind: lifecycle.ExecutionOwnerRun, WorkspaceID: r.h.workspaceID, Status: "COMPLETED",
		})); err != nil {
		t.Fatalf("publish run-owned agent completed: %v", err)
	}

	after, err := r.h.taskRepo.GetTaskSession(ctx, decoy.ID)
	if err != nil || after == nil {
		t.Fatalf("get decoy task session after event: %v", err)
	}
	if after.State != decoy.State {
		t.Errorf("decoy session state = %q, want unchanged %q", after.State, decoy.State)
	}
	if !after.UpdatedAt.Equal(decoy.UpdatedAt) {
		t.Errorf("decoy session updated_at = %v, want unchanged %v", after.UpdatedAt, decoy.UpdatedAt)
	}
	if got := atomic.LoadInt32(mutationCount); got != 0 {
		t.Errorf("task.updated/task.moved publications = %d, want 0", got)
	}
}

// TestRunOwnedLifecycleEvent_AgentFailed_NoTaskSessionEffect is the failure
// counterpart: handleAgentFailed's `if data.SessionID == ""` branch dispatches
// straight to handleAgentFailedLocked, whose only session/task-scoped
// branches (shouldDropSessionFailure, handleTransientFailure,
// routeDynamicAgentFailure, handleRecoverableFailureLockedState) all guard on
// data.SessionID != "" and are skipped; the no-session fallback tail
// (scheduler.HandleTaskCompleted/RetryTask/writeTaskReviewState) operates on
// data.TaskID, which is likewise "" for a run-owned event, so it resolves no
// real task either.
func TestRunOwnedLifecycleEvent_AgentFailed_NoTaskSessionEffect(t *testing.T) {
	ctx := context.Background()
	r := newRunOwnedEventHarness(t)
	decoy := decoyTaskSession(t, ctx, r.h)
	run, session := r.launchTasklessRun(t, ctx)

	mutationCount := subscribeTaskMutationEvents(t, r.h.eventBus)

	if err := r.h.eventBus.Publish(ctx, events.AgentFailed, bus.NewEvent(events.AgentFailed, "test",
		lifecycle.AgentEventPayload{
			AgentExecutionID: session.ExecutionID, AgentID: "test-adapter", AgentProfileID: r.agent.ID,
			RunID: run.ID, RunSessionID: session.ID, RunAttempt: session.Attempt,
			OwnerKind: lifecycle.ExecutionOwnerRun, WorkspaceID: r.h.workspaceID, Status: "FAILED",
			ErrorMessage: "run-owned provider failure",
		})); err != nil {
		t.Fatalf("publish run-owned agent failed: %v", err)
	}

	after, err := r.h.taskRepo.GetTaskSession(ctx, decoy.ID)
	if err != nil || after == nil {
		t.Fatalf("get decoy task session after event: %v", err)
	}
	if after.State != decoy.State {
		t.Errorf("decoy session state = %q, want unchanged %q", after.State, decoy.State)
	}
	if !after.UpdatedAt.Equal(decoy.UpdatedAt) {
		t.Errorf("decoy session updated_at = %v, want unchanged %v", after.UpdatedAt, decoy.UpdatedAt)
	}
	if got := atomic.LoadInt32(mutationCount); got != 0 {
		t.Errorf("task.updated/task.moved publications = %d, want 0", got)
	}
}
