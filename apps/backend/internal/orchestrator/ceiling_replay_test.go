package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// --- pure unit tests -------------------------------------------------------

func TestStripCeilingRecordKeys_RemovesOnlyCeilingPrefixedKeys(t *testing.T) {
	existing := map[string]interface{}{
		models.CeilingDeferredKey:                  true,
		models.CeilingLaunchKindKey:                string(models.CeilingLaunchStart),
		models.DeferredLaunchStartWhenUnblockedKey: true,
		"wip_overflow_marker":                      true,
	}
	stripped := stripCeilingRecordKeys(existing)
	require.NotContains(t, stripped, models.CeilingDeferredKey)
	require.NotContains(t, stripped, models.CeilingLaunchKindKey)
	require.Equal(t, true, stripped[models.DeferredLaunchStartWhenUnblockedKey])
	require.Equal(t, true, stripped["wip_overflow_marker"])
}

func TestStripCeilingRecordKeys_NonObjectValueYieldsEmptyMap(t *testing.T) {
	stripped := stripCeilingRecordKeys("not-an-object")
	require.NotNil(t, stripped)
	require.Empty(t, stripped)
	stripped = stripCeilingRecordKeys(nil)
	require.NotNil(t, stripped)
	require.Empty(t, stripped)
}

func TestCeilingReplayOutcomeFromExecution(t *testing.T) {
	require.Equal(t, ceilingReplaySucceeded, ceilingReplayOutcomeFromExecution(&executor.TaskExecution{}, nil))
	require.Equal(t, ceilingReplayStillDeferred, ceilingReplayOutcomeFromExecution(nil, nil))
	require.Equal(t, ceilingReplayFailed, ceilingReplayOutcomeFromExecution(nil, context.Canceled))
}

func TestSessionIDFromCeilingPayload(t *testing.T) {
	require.Empty(t, sessionIDFromCeilingPayload(models.CeilingDeferral{
		Kind:    models.CeilingLaunchStart,
		Payload: map[string]interface{}{metaKeySessionID: "should-be-ignored"},
	}))
	require.Equal(t, "session-x", sessionIDFromCeilingPayload(models.CeilingDeferral{
		Kind:    models.CeilingLaunchResume,
		Payload: map[string]interface{}{metaKeySessionID: "session-x"},
	}))
}

// --- clearCeilingDeferredRecord --------------------------------------------

func TestClearCeilingDeferredRecord_PreservesCoexistingWIPIntent(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "clear-task-a", Title: "t", State: v1.TaskStateInProgress,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}))

	record := models.CeilingRecordKeys(models.CeilingDeferral{
		Kind:       models.CeilingLaunchResume,
		Payload:    map[string]interface{}{metaKeySessionID: "session-a"},
		Origin:     string(launchOriginAutomatic),
		ReasonCode: ceilingReasonRefused,
		QueuedAt:   time.Now(),
	})
	record[models.DeferredLaunchStartWhenUnblockedKey] = true
	_, _, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "clear-task-a", tasksqlite.AbsentDeferredLaunch(), record)
	require.NoError(t, err)

	svc.clearCeilingDeferredRecord(ctx, "clear-task-a")

	after := deferredLaunchOf(t, svc, "clear-task-a")
	require.False(t, models.HasCeilingDeferredIntent(&models.Task{Metadata: after}))
	require.Equal(t, true, after[models.DeferredLaunchStartWhenUnblockedKey], "clearing the ceiling half must not disturb the WIP-overflow half")
}

// --- ListTasksWithCeilingDeferred -------------------------------------------

func TestListTasksWithCeilingDeferred_IncludesArchivedAndOrdersByID(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()

	mkTask := func(id string) {
		require.NoError(t, repo.CreateTask(ctx, &models.Task{
			ID: id, Title: "t", State: v1.TaskStateInProgress,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}))
		record := models.CeilingRecordKeys(models.CeilingDeferral{
			Kind:       models.CeilingLaunchResume,
			Payload:    map[string]interface{}{metaKeySessionID: id + "-session"},
			Origin:     string(launchOriginAutomatic),
			ReasonCode: ceilingReasonRefused,
			QueuedAt:   time.Now(),
		})
		_, _, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, id, tasksqlite.AbsentDeferredLaunch(), record)
		require.NoError(t, err)
	}
	mkTask("list-task-b")
	mkTask("list-task-a")
	mkTask("list-task-c")
	require.NoError(t, repo.ArchiveTask(ctx, "list-task-c"))

	// A task with no deferral at all must never appear.
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "list-task-no-deferral", Title: "t", State: v1.TaskStateInProgress,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}))

	lister, ok := svc.repo.(ceilingDeferredTaskLister)
	require.True(t, ok, "the production sqlite repository must satisfy ceilingDeferredTaskLister")
	tasks, err := lister.ListTasksWithCeilingDeferred(ctx)
	require.NoError(t, err)

	var ids []string
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	require.Equal(t, []string{"list-task-a", "list-task-b", "list-task-c"}, ids,
		"archived tasks must still be returned, in id-ascending order")
}

// --- evaluateCeilingDropReasons ---------------------------------------------

func TestEvaluateCeilingDropReasons_ArchivedTask(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	task := &models.Task{ID: "drop-archived", Title: "t", State: v1.TaskStateInProgress, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	require.NoError(t, repo.CreateTask(ctx, task))
	require.NoError(t, repo.ArchiveTask(ctx, "drop-archived"))
	task, err := repo.GetTask(ctx, "drop-archived")
	require.NoError(t, err)

	reasonCode, _, drop := svc.evaluateCeilingDropReasons(ctx, task, models.CeilingDeferral{Kind: models.CeilingLaunchResume})
	require.True(t, drop)
	require.Equal(t, ceilingReasonDroppedTaskIneligible, reasonCode)
}

func TestEvaluateCeilingDropReasons_CancelledTask(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	task := &models.Task{ID: "drop-cancelled", Title: "t", State: v1.TaskStateInProgress, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	require.NoError(t, repo.CreateTask(ctx, task))
	require.NoError(t, repo.UpdateTaskState(ctx, "drop-cancelled", v1.TaskStateCancelled))
	task, err := repo.GetTask(ctx, "drop-cancelled")
	require.NoError(t, err)

	reasonCode, _, drop := svc.evaluateCeilingDropReasons(ctx, task, models.CeilingDeferral{Kind: models.CeilingLaunchResume})
	require.True(t, drop)
	require.Equal(t, ceilingReasonDroppedTaskIneligible, reasonCode)
}

func TestEvaluateCeilingDropReasons_SessionNoLongerExists(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	task := &models.Task{ID: "drop-ghost-session", Title: "t", State: v1.TaskStateInProgress, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	require.NoError(t, repo.CreateTask(ctx, task))

	deferral := models.CeilingDeferral{
		Kind:    models.CeilingLaunchResume,
		Payload: map[string]interface{}{metaKeySessionID: "session-that-was-deleted"},
	}
	reasonCode, detail, drop := svc.evaluateCeilingDropReasons(ctx, task, deferral)
	require.True(t, drop)
	require.Equal(t, ceilingReasonDroppedTaskIneligible, reasonCode)
	require.Contains(t, detail, "session no longer exists")
}

func TestEvaluateCeilingDropReasons_StartKindStepNoLongerAutoStarts(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	stepGetter := newMockStepGetter()
	svc.workflowStepGetter = stepGetter
	ctx := context.Background()

	stepGetter.steps["step-no-auto-start"] = &wfmodels.WorkflowStep{ID: "step-no-auto-start"}
	task := &models.Task{
		ID: "drop-step-no-auto-start", Title: "t", State: v1.TaskStateInProgress,
		WorkflowStepID: "step-no-auto-start", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, repo.CreateTask(ctx, task))

	reasonCode, detail, drop := svc.evaluateCeilingDropReasons(ctx, task, models.CeilingDeferral{Kind: models.CeilingLaunchStart, Payload: map[string]interface{}{}})
	require.True(t, drop)
	require.Equal(t, ceilingReasonDroppedTaskIneligible, reasonCode)
	require.Contains(t, detail, "no longer auto-starts")
}

func TestEvaluateCeilingDropReasons_NonStartKindIgnoresStepAutoStart(t *testing.T) {
	// A resume/prompt_ensure/etc. record must not be dropped just because the
	// task's current step no longer auto-starts (AC-17b(d) is scoped to "start").
	svc, repo := newServiceWithRealRepo(t)
	stepGetter := newMockStepGetter()
	svc.workflowStepGetter = stepGetter
	ctx := context.Background()

	stepGetter.steps["step-no-auto-start"] = &wfmodels.WorkflowStep{ID: "step-no-auto-start"}
	task := &models.Task{
		ID: "keep-resume-despite-step", Title: "t", State: v1.TaskStateInProgress,
		WorkflowStepID: "step-no-auto-start", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, repo.CreateTask(ctx, task))

	_, _, drop := svc.evaluateCeilingDropReasons(ctx, task, models.CeilingDeferral{Kind: models.CeilingLaunchResume, Payload: map[string]interface{}{}})
	require.False(t, drop)
}

// --- end-to-end drain behaviour, through the real seam 2 gate ---------------

func TestDrainDeferredCeilingLaunches_StillDeferredThenSucceedsOnceSlotFrees(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "drain-task-a", "drain-session-a", models.TaskSessionStateCreated)
	seedExecutorRunning(t, repo, "drain-session-a", "drain-task-a", "exec-a")
	seedTaskAndSession(t, repo, "drain-task-b", "drain-session-b", models.TaskSessionStateCreated)
	seedExecutorRunning(t, repo, "drain-session-b", "drain-task-b", "exec-b")

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "drain-task-a", v1.TaskStateInProgress)
	seedMockTaskState(taskRepo, "drain-task-b", v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	events := &capturingTaskEvents{}
	svc.SetTaskEventPublisher(events)

	// Occupy the only slot directly against the controller (session-a "holds"
	// the ceiling's one unit for this test, without needing a real launch).
	decision := svc.sessionCeiling.admit(ctx, admissionRequest{
		taskID: "drain-task-a", sessionID: "drain-session-a", origin: launchOriginAutomatic, seam: "test-setup",
	})
	require.True(t, decision.admitted)

	// Task b's automatic launch is refused and deferred by the real seam 2 gate.
	exec, err := svc.StartCreatedSession(ctx, "drain-task-b", "drain-session-b", "profile-1", "go", false, false, true, nil, nil)
	require.NoError(t, err)
	require.Nil(t, exec)
	require.NotNil(t, deferredLaunchOf(t, svc, "drain-task-b"))

	// A drain pass while the slot is still occupied leaves the record in place.
	svc.drainDeferredCeilingLaunches(ctx)
	record := deferredLaunchOf(t, svc, "drain-task-b")
	require.NotNil(t, record, "a still-refused replay must not clear the record")
	require.Equal(t, true, record[models.CeilingDeferredKey])

	agentMgr.mu.Lock()
	callsWhileFull := len(agentMgr.setExecutionDescriptionCalls)
	agentMgr.mu.Unlock()
	require.Zero(t, callsWhileFull, "the still-deferred replay must not have dispatched an agent")

	// Free the slot and drain again: the replay must now succeed and clear the record.
	svc.sessionCeiling.release("drain-session-a")
	svc.drainDeferredCeilingLaunches(ctx)

	require.False(t, models.HasCeilingDeferredIntent(&models.Task{Metadata: map[string]interface{}{
		models.MetaKeyDeferredLaunch: deferredLaunchOf(t, svc, "drain-task-b"),
	}}), "a successful replay must clear the ceiling record")

	published := events.last()
	require.NotNil(t, published, "a successful replay must publish task.updated so the WS-driven UI clears the queued state")
	require.Equal(t, "drain-task-b", published.ID)
	require.False(t, models.HasCeilingDeferredIntent(published),
		"the published task after a successful replay must reflect the cleared record, not a stale queued snapshot")
	agentMgr.mu.Lock()
	callsAfterFree := len(agentMgr.setExecutionDescriptionCalls)
	agentMgr.mu.Unlock()
	require.Equal(t, 1, callsAfterFree, "the now-admitted replay must have dispatched exactly one agent")
}

func TestDrainDeferredCeilingLaunches_DropsArchivedTaskAndWritesCardNote(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "drain-archived-task", "drain-archived-session", models.TaskSessionStateCreated)

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "drain-archived-task", v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{repoForExecutionLookup: repo})
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	msgCreator := &mockMessageCreator{}
	svc.messageCreator = msgCreator

	record := models.CeilingRecordKeys(models.CeilingDeferral{
		Kind:       models.CeilingLaunchResume,
		Payload:    map[string]interface{}{metaKeySessionID: "drain-archived-session"},
		Origin:     string(launchOriginAutomatic),
		ReasonCode: ceilingReasonRefused,
		QueuedAt:   time.Now(),
	})
	_, _, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "drain-archived-task", tasksqlite.AbsentDeferredLaunch(), record)
	require.NoError(t, err)
	require.NoError(t, repo.ArchiveTask(ctx, "drain-archived-task"))

	svc.drainDeferredCeilingLaunches(ctx)

	require.False(t, models.HasCeilingDeferredIntent(&models.Task{Metadata: map[string]interface{}{
		models.MetaKeyDeferredLaunch: deferredLaunchOf(t, svc, "drain-archived-task"),
	}}), "a dropped record must be cleared")
	msgCreator.mu.Lock()
	attempts := msgCreator.sessionMessageAttempts
	msgCreator.mu.Unlock()
	require.Equal(t, 1, attempts, "dropping a record for a session that still exists must write one card note")
}
