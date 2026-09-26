package backendapp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepository "github.com/kandev/kandev/internal/workflow/repository"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// pluginMoveErrorBindingHarness wires the same two repositories and services
// production boots (internal/task's own workflow table for
// validateTaskMove's GetWorkflow/workspace checks, plus the separate
// internal/workflow package for step lookups via workflowStepGetterAdapter),
// so the error text classifyPluginMoveError matches against is whatever the
// real validators produce today, not a hand-written stand-in.
type pluginMoveErrorBindingHarness struct {
	adapter     pluginsTaskWriterAdapter
	repo        *sqliterepo.Repository
	workflowSvc *workflowservice.Service
	db          *sqlx.DB
	eventBus    *bus.MemoryEventBus
}

func newPluginMoveErrorBindingHarness(t *testing.T) *pluginMoveErrorBindingHarness {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "plugin-move-error-binding.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	taskRepo, cleanup, err := repository.Provide(database, database, nil)
	if err != nil {
		t.Fatalf("task repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	workflowRepo, err := workflowrepository.NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("workflow repository: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	eventBus := bus.NewMemoryEventBus(log)
	taskSvc := taskservice.NewService(taskservice.Repos{
		Workspaces:       taskRepo,
		Tasks:            taskRepo,
		TaskRepos:        taskRepo,
		Workflows:        taskRepo,
		Messages:         taskRepo,
		Turns:            taskRepo,
		Sessions:         taskRepo,
		GitSnapshots:     taskRepo,
		RepoEntities:     taskRepo,
		Executors:        taskRepo,
		Environments:     taskRepo,
		TaskEnvironments: taskRepo,
		Reviews:          taskRepo,
		ResourceCleanups: taskRepo,
	}, eventBus, log, taskservice.RepositoryDiscoveryConfig{})
	workflowSvc := workflowservice.NewService(workflowRepo, log)
	taskSvc.SetWorkflowStepGetter(&workflowStepGetterAdapter{svc: workflowSvc})
	// Matches production wiring (backendapp.wireServices calls
	// taskSvc.SetStepHistoryRecorder(workflowSvc)) so a plugin move against a
	// task with a live session writes the ADR 0015 session_step_history audit
	// row the same way it does at runtime.
	taskSvc.SetStepHistoryRecorder(workflowSvc)
	t.Cleanup(func() { _ = workflowSvc.Close() })

	ctx := context.Background()
	require.NoError(t, taskRepo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "ws-1", Name: "Workspace 1"}))
	require.NoError(t, taskRepo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "ws-2", Name: "Workspace 2"}))
	require.NoError(t, taskRepo.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "wf-home", WorkspaceID: "ws-1", Name: "Home"}))
	require.NoError(t, taskRepo.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "wf-target", WorkspaceID: "ws-1", Name: "Target"}))
	require.NoError(t, taskRepo.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "wf-other-workspace", WorkspaceID: "ws-2", Name: "Other"}))
	require.NoError(t, workflowSvc.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "step-target", WorkflowID: "wf-target", Name: "Target", Position: 0}))
	require.NoError(t, workflowSvc.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "step-home", WorkflowID: "wf-home", Name: "Home", Position: 0}))

	return &pluginMoveErrorBindingHarness{
		adapter:     pluginsTaskWriterAdapter{svc: taskSvc},
		repo:        taskRepo,
		workflowSvc: workflowSvc,
		db:          database,
		eventBus:    eventBus,
	}
}

func (h *pluginMoveErrorBindingHarness) createTask(t *testing.T, id string, archived bool) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, h.repo.CreateTask(ctx, &taskmodels.Task{
		ID:             id,
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-home",
		WorkflowStepID: "step-home",
		Title:          id,
		State:          v1.TaskStateTODO,
	}))
	if archived {
		require.NoError(t, h.repo.ArchiveTask(ctx, id))
	}
}

// createSession attaches a primary task session in the given state, letting
// tests exercise MoveTaskWithOptions' session-resolution and ADR 0015
// session_step_history recording paths.
func (h *pluginMoveErrorBindingHarness) createSession(t *testing.T, id, taskID string, state taskmodels.TaskSessionState) {
	t.Helper()
	require.NoError(t, h.repo.CreateTaskSession(context.Background(), &taskmodels.TaskSession{
		ID: id, TaskID: taskID, State: state, IsPrimary: true,
	}))
}

// lastSessionStepHistoryTrigger reads back the most recently written ADR 0015
// session_step_history.trigger for a session, flushing the workflow
// service's async history-writer queue first (EnqueueStepTransition is
// fire-and-forget; Close drains it and waits for the writer goroutine to
// exit before returning).
func (h *pluginMoveErrorBindingHarness) lastSessionStepHistoryTrigger(t *testing.T, sessionID string) string {
	t.Helper()
	require.NoError(t, h.workflowSvc.Close())
	var trigger string
	err := h.db.Get(&trigger,
		`SELECT trigger FROM session_step_history WHERE session_id = ? ORDER BY id DESC LIMIT 1`, sessionID)
	require.NoError(t, err, "expected a session_step_history row for session %q", sessionID)
	return trigger
}

// requireTaskUnmoved reads taskID back from the real repository and asserts
// its workflow/step are still exactly h.createTask's defaults (wf-home /
// step-home). Every rejected-move subtest below drives a real task through
// h.adapter.MoveTask and only ever asserts the returned error — never that
// the DB row was left untouched. A regression that validates then writes
// unconditionally (or writes before validating) would still pass every
// existing assertion in this file; this closes that gap by proving a
// rejection has zero persisted side effects, not just a non-nil error.
func requireTaskUnmoved(t *testing.T, h *pluginMoveErrorBindingHarness, taskID string) {
	t.Helper()
	task, err := h.repo.GetTask(context.Background(), taskID)
	require.NoError(t, err)
	require.Equal(t, "wf-home", task.WorkflowID, "a rejected move must not persist a workflow change")
	require.Equal(t, "step-home", task.WorkflowStepID, "a rejected move must not persist a step change")
}

// TestClassifyPluginMoveError_BindsToRealValidatorStrings pins AC-004.6: the
// binding error-mapping table in classifyPluginMoveError must be exercised
// against errors the real validateTaskMove/GetWorkflow/GetStep code paths
// actually produce, not hand-written stand-in strings — so a validator
// wording change that breaks the classifier's substring match fails this
// test, per Review round 1's must-fix finding (test-supervisor proved by
// mutation that TestClassifyPluginMoveError's stand-in literals do not
// notice such a change).
// SEC-002 (Review round 2): each subtest below also asserts the exact fixed
// message and the absence of the real, identity-bearing fragment this
// specific move actually produced internally (a workflow/step/task id, or
// the raw validator sentence) — status.Code alone would still pass if the
// classifier regressed to forwarding err.Error() verbatim, since the status
// code discrimination is unaffected by that regression.
func TestClassifyPluginMoveError_BindsToRealValidatorStrings(t *testing.T) {
	wf := "wf-target"
	const invalidArgMsg = "invalid move_task request: unknown or mismatched workflow, step, or workspace"

	t.Run("archived task, AC-001.7", func(t *testing.T) {
		h := newPluginMoveErrorBindingHarness(t)
		h.createTask(t, "task-archived", true)
		_, err := h.adapter.MoveTask(context.Background(), plugins.TaskMoveInput{
			TaskID: "task-archived", WorkflowStepID: "step-target", WorkflowID: &wf, Source: "plugin:acme",
		})
		st := status.Convert(err)
		require.Equal(t, codes.FailedPrecondition, st.Code())
		require.Equal(t, "task is archived and cannot be moved", st.Message())
		requireTaskUnmoved(t, h, "task-archived")
	})

	t.Run("active session, AC-001.8", func(t *testing.T) {
		h := newPluginMoveErrorBindingHarness(t)
		h.createTask(t, "task-live-session", false)
		require.NoError(t, h.repo.CreateTaskSession(context.Background(), &taskmodels.TaskSession{
			ID: "session-live", TaskID: "task-live-session", State: taskmodels.TaskSessionStateRunning, IsPrimary: true,
		}))
		_, err := h.adapter.MoveTask(context.Background(), plugins.TaskMoveInput{
			TaskID: "task-live-session", WorkflowStepID: "step-target", WorkflowID: &wf, Source: "plugin:acme",
		})
		st := status.Convert(err)
		require.Equal(t, codes.FailedPrecondition, st.Code())
		require.Equal(t, "task has an active session and cannot be moved", st.Message())
		require.NotContains(t, st.Message(), "RUNNING", "message must not leak the session's internal state value")
		requireTaskUnmoved(t, h, "task-live-session")
	})

	t.Run("unknown workflow, AC-001.6", func(t *testing.T) {
		h := newPluginMoveErrorBindingHarness(t)
		h.createTask(t, "task-unknown-wf", false)
		missing := "wf-does-not-exist"
		_, err := h.adapter.MoveTask(context.Background(), plugins.TaskMoveInput{
			TaskID: "task-unknown-wf", WorkflowStepID: "step-target", WorkflowID: &missing, Source: "plugin:acme",
		})
		st := status.Convert(err)
		require.Equal(t, codes.InvalidArgument, st.Code())
		require.Equal(t, invalidArgMsg, st.Message())
		require.NotContains(t, st.Message(), missing, "message must not leak the requested workflow id")
		requireTaskUnmoved(t, h, "task-unknown-wf")
	})

	t.Run("different workspace, AC-001.6", func(t *testing.T) {
		h := newPluginMoveErrorBindingHarness(t)
		h.createTask(t, "task-cross-workspace", false)
		other := "wf-other-workspace"
		_, err := h.adapter.MoveTask(context.Background(), plugins.TaskMoveInput{
			TaskID: "task-cross-workspace", WorkflowStepID: "step-target", WorkflowID: &other, Source: "plugin:acme",
		})
		st := status.Convert(err)
		require.Equal(t, codes.InvalidArgument, st.Code())
		require.Equal(t, invalidArgMsg, st.Message())
		require.NotContains(t, st.Message(), other, "message must not leak the target workflow id")
		requireTaskUnmoved(t, h, "task-cross-workspace")
	})

	t.Run("unknown step, AC-001.6", func(t *testing.T) {
		h := newPluginMoveErrorBindingHarness(t)
		h.createTask(t, "task-unknown-step", false)
		missingStep := "step-does-not-exist"
		_, err := h.adapter.MoveTask(context.Background(), plugins.TaskMoveInput{
			TaskID: "task-unknown-step", WorkflowStepID: missingStep, WorkflowID: &wf, Source: "plugin:acme",
		})
		st := status.Convert(err)
		require.Equal(t, codes.InvalidArgument, st.Code())
		require.Equal(t, invalidArgMsg, st.Message())
		require.NotContains(t, st.Message(), missingStep, "message must not leak the requested step id")
		requireTaskUnmoved(t, h, "task-unknown-step")
	})

	t.Run("step not in workflow, AC-001.6", func(t *testing.T) {
		h := newPluginMoveErrorBindingHarness(t)
		h.createTask(t, "task-step-wrong-workflow", false)
		// step-home is a real step, but it belongs to wf-home, not wf-target.
		_, err := h.adapter.MoveTask(context.Background(), plugins.TaskMoveInput{
			TaskID: "task-step-wrong-workflow", WorkflowStepID: "step-home", WorkflowID: &wf, Source: "plugin:acme",
		})
		st := status.Convert(err)
		require.Equal(t, codes.InvalidArgument, st.Code())
		require.Equal(t, invalidArgMsg, st.Message())
		require.NotContains(t, st.Message(), "step-home", "message must not leak the requested step id")
		require.NotContains(t, st.Message(), "wf-home", "message must not leak the task's actual workflow id")
		requireTaskUnmoved(t, h, "task-step-wrong-workflow")
	})

	t.Run("task not found, AC-005.6", func(t *testing.T) {
		h := newPluginMoveErrorBindingHarness(t)
		_, err := h.adapter.MoveTask(context.Background(), plugins.TaskMoveInput{
			TaskID: "task-missing", WorkflowStepID: "step-target", WorkflowID: &wf, Source: "plugin:acme",
		})
		st := status.Convert(err)
		require.Equal(t, codes.NotFound, st.Code())
		require.Equal(t, "task not found", st.Message())
		require.NotContains(t, st.Message(), "task-missing", "message must not leak the requested task id")
	})
}

// TestResolvePluginMoveWorkflowID_StaleReadFailsSafeNotSilentlyWrong pins
// TEST-003 (Review round 1, LOW/non-blocking, converges with Spec Review
// round 5's Q-001): an omitted WorkflowID is resolved by a separate GetTask
// pre-read (resolvePluginMoveWorkflowID) rather than atomically with the move
// itself. If the task's workflow changes between that pre-read and the move
// commit — the accepted race — the stale workflow id must never let the move
// silently land somewhere it shouldn't; validateTaskMove's
// targetStep.WorkflowID != workflowID check must reject it as
// InvalidArgument instead.
func TestResolvePluginMoveWorkflowID_StaleReadFailsSafeNotSilentlyWrong(t *testing.T) {
	h := newPluginMoveErrorBindingHarness(t)
	h.createTask(t, "task-race", false)

	staleWorkflowID, err := h.adapter.resolvePluginMoveWorkflowID(context.Background(), plugins.TaskMoveInput{
		TaskID: "task-race",
	})
	require.NoError(t, err)
	require.Equal(t, "wf-home", staleWorkflowID, "pre-read should observe the task's workflow at that moment")

	// Simulate the race: another caller moves the task to a different
	// workflow before the plugin's own move (using the now-stale pre-read)
	// commits.
	_, err = h.adapter.svc.MoveTaskWithOptions(context.Background(), "task-race", "wf-target", "step-target", 0, taskservice.MoveTaskOptions{})
	require.NoError(t, err)

	_, err = h.adapter.svc.MoveTaskWithOptions(context.Background(), "task-race", staleWorkflowID, "step-target", 0, taskservice.MoveTaskOptions{})
	require.Equal(t, codes.InvalidArgument, status.Code(classifyPluginMoveError(err)), "a stale workflow id must fail safe, never silently move the task")

	final, getErr := h.repo.GetTask(context.Background(), "task-race")
	require.NoError(t, getErr)
	require.Equal(t, "step-target", final.WorkflowStepID, "the failed stale move must not have changed the step set by the interceding real move")
	require.Equal(t, "wf-target", final.WorkflowID)
}

// TestConcurrentReassignmentSurvivesMatchingStepStaleMove pins the SEC-001 fix
// (Review round 2): the sibling gap in
// TestResolvePluginMoveWorkflowID_StaleReadFailsSafeNotSilentlyWrong, which
// only covers a stale workflow id whose requested step does NOT belong to it
// (caught for free by validateTaskMove's own targetStep.WorkflowID check).
// Here the requested step DOES belong to the stale, pre-read workflow — so
// without a write-time CAS check, the stale move would pass validateTaskMove
// outright and silently revert the concurrent, legitimate reassignment with
// no error at all. MoveTaskOptions.ExpectedWorkflowID must catch this.
func TestConcurrentReassignmentSurvivesMatchingStepStaleMove(t *testing.T) {
	h := newPluginMoveErrorBindingHarness(t)
	h.createTask(t, "task-race-2", false)

	staleWorkflowID, err := h.adapter.resolvePluginMoveWorkflowID(context.Background(), plugins.TaskMoveInput{
		TaskID: "task-race-2",
	})
	require.NoError(t, err)
	require.Equal(t, "wf-home", staleWorkflowID)

	// Simulate the race: another caller reassigns the task to a different
	// workflow before the plugin's own move (using the now-stale pre-read)
	// commits.
	_, err = h.adapter.svc.MoveTaskWithOptions(context.Background(), "task-race-2", "wf-target", "step-target", 0, taskservice.MoveTaskOptions{})
	require.NoError(t, err)

	// The plugin's stale move now targets step-home, which DOES belong to the
	// stale wf-home — the exploit case. Without the CAS guard this would pass
	// validateTaskMove and silently move the task back to wf-home/step-home.
	_, err = h.adapter.svc.MoveTaskWithOptions(context.Background(), "task-race-2", staleWorkflowID, "step-home", 0, taskservice.MoveTaskOptions{
		ExpectedWorkflowID: &staleWorkflowID,
	})
	require.ErrorIs(t, err, taskservice.ErrWorkflowResolutionConflict)
	require.Equal(t, codes.Aborted, status.Code(classifyPluginMoveError(err)))

	final, getErr := h.repo.GetTask(context.Background(), "task-race-2")
	require.NoError(t, getErr)
	require.Equal(t, "wf-target", final.WorkflowID, "the concurrent reassignment must survive the rejected stale move")
	require.Equal(t, "step-target", final.WorkflowStepID)
}

// TestPluginMoveWithLiveNonBlockingSessionRecordsPluginMoveTrigger pins the
// AC-003.2/AC-003.5 test-rigor gap (Review round 2): every existing plugin
// move test either has no session at all, or a Starting/Running session that
// validateMoveSessions rejects outright (isSessionMoveBlocked) before
// MoveTaskWithOptions ever reaches recordManualStepTransition. None of them
// prove what happens for a session that is live (resolvePrimaryOrActiveSession
// picks it up via isSessionActive, so it becomes the ADR 0015 audit row's
// session_id) but not move-blocking — WaitingForInput is exactly that gap.
// Without the plugin adapter's opts.StepHistoryTrigger reaching the recorder,
// recordManualStepTransition's zero-value fallback would silently default the
// row to StepTransitionTriggerManual instead of plugin_move.
func TestPluginMoveWithLiveNonBlockingSessionRecordsPluginMoveTrigger(t *testing.T) {
	h := newPluginMoveErrorBindingHarness(t)
	h.createTask(t, "task-live-waiting", false)
	h.createSession(t, "session-waiting", "task-live-waiting", taskmodels.TaskSessionStateWaitingForInput)
	wf := "wf-target"

	_, err := h.adapter.MoveTask(context.Background(), plugins.TaskMoveInput{
		TaskID: "task-live-waiting", WorkflowStepID: "step-target", WorkflowID: &wf, Source: "plugin:acme",
	})
	require.NoError(t, err, "a WaitingForInput session is live but not move-blocking (isSessionMoveBlocked covers only Starting/Running)")

	trigger := h.lastSessionStepHistoryTrigger(t, "session-waiting")
	require.Equal(t, string(wfmodels.StepTransitionTriggerPluginMove), trigger,
		"a plugin move against a task with a live session must record plugin_move, not fall back to manual")
}

// TestPluginMovePublishesTaskMovedEvent closes the test-rigor gap flagged
// alongside the round-5 CAS fix: the whole point of routing plugin moves
// through MoveTaskWithOptions (rather than the old UpdateTask WorkflowStepID
// path) is that a move publishes task.moved so the orchestrator runs
// on_enter/on_exit step actions. No existing test subscribes to the event
// bus to prove a plugin-initiated move actually publishes it — every other
// test in this file only asserts the DB row or the audit trigger. This
// subscribes before the move and asserts the handler observed the event
// synchronously (MemoryEventBus.Publish dispatches subscribers inline) with
// the expected task/step identifiers.
func TestPluginMovePublishesTaskMovedEvent(t *testing.T) {
	h := newPluginMoveErrorBindingHarness(t)
	h.createTask(t, "task-publishes-move", false)
	wf := "wf-target"

	var received *bus.Event
	sub, err := h.eventBus.Subscribe(events.TaskMoved, func(_ context.Context, event *bus.Event) error {
		received = event
		return nil
	})
	require.NoError(t, err)
	defer func() { _ = sub.Unsubscribe() }()

	_, err = h.adapter.MoveTask(context.Background(), plugins.TaskMoveInput{
		TaskID: "task-publishes-move", WorkflowStepID: "step-target", WorkflowID: &wf, Source: "plugin:acme",
	})
	require.NoError(t, err)

	require.NotNil(t, received, "a plugin move must publish task.moved so on_enter/on_exit step actions fire")
	require.Equal(t, events.TaskMoved, received.Type)
	data, ok := received.Data.(map[string]interface{})
	require.True(t, ok, "task.moved event data must be a map")
	require.Equal(t, "task-publishes-move", data["task_id"])
	require.Equal(t, "wf-home", data["from_workflow_id"])
	require.Equal(t, "wf-target", data["to_workflow_id"])
	require.Equal(t, "step-home", data["from_step_id"])
	require.Equal(t, "step-target", data["to_step_id"])
}
