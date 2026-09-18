package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type ceilingDeferredLaunchReadErrorRepo struct {
	sessionExecutorStore
	err error
}

func (r ceilingDeferredLaunchReadErrorRepo) GetTaskDeferredLaunch(
	context.Context, string,
) (map[string]interface{}, interface{}, error) {
	return nil, nil, r.err
}

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

func TestClearCeilingDeferredRecord_DoesNotClearNewerSuccessor(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "clear-task-successor", Title: "t", State: v1.TaskStateInProgress,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}))

	original := models.CeilingDeferral{
		Kind:       models.CeilingLaunchResume,
		Payload:    map[string]interface{}{metaKeySessionID: "session-original"},
		Origin:     string(launchOriginAutomatic),
		ReasonCode: ceilingReasonRefused,
		QueuedAt:   time.Now(),
	}
	successor := models.CeilingDeferral{
		Kind:       models.CeilingLaunchStartCreated,
		Payload:    map[string]interface{}{metaKeySessionID: "session-successor"},
		Origin:     string(launchOriginAutomatic),
		ReasonCode: ceilingReasonRefused,
		QueuedAt:   original.QueuedAt.Add(time.Second),
	}
	_, _, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "clear-task-successor", tasksqlite.AbsentDeferredLaunch(), models.CeilingRecordKeys(successor))
	require.NoError(t, err)

	svc.clearCeilingDeferredRecord(ctx, "clear-task-successor", original)

	after := deferredLaunchOf(t, svc, "clear-task-successor")
	got, err := models.ReadCeilingDeferral(after)
	require.NoError(t, err)
	require.Equal(t, successor.Kind, got.Kind)
	require.Equal(t, "session-successor", sessionIDFromCeilingPayload(got))
}

func TestClaimCeilingDeferredLaunchSerializesSendNowAndReplay(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "claim-task", Title: "t", State: v1.TaskStateScheduling,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}))
	queuedAt := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	deferral := models.CeilingDeferral{
		Kind:    models.CeilingLaunchStartCreated,
		Payload: map[string]interface{}{metaKeySessionID: "claim-session"},
		Origin:  string(launchOriginAutomatic), ReasonCode: ceilingReasonRefused,
		QueuedAt: queuedAt, Ceiling: 5, Population: 5, PopulationKnown: true,
	}
	record := models.CeilingRecordKeys(deferral)
	record[models.DeferredLaunchStartWhenUnblockedKey] = true
	_, _, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "claim-task", tasksqlite.AbsentDeferredLaunch(), record)
	require.NoError(t, err)

	first, found, err := svc.claimCeilingDeferredLaunch(ctx, "claim-task", "claim-session", ceilingClaimOwnerSendNow)
	require.NoError(t, err)
	require.True(t, found)
	require.NotNil(t, first)
	second, found, err := svc.claimCeilingDeferredLaunch(ctx, "claim-task", "claim-session", ceilingClaimOwnerReplay)
	require.NoError(t, err)
	require.True(t, found)
	require.Nil(t, second, "a replay must not dispatch while Send Now owns the record")

	first.settle(ctx)
	after := deferredLaunchOf(t, svc, "claim-task")
	require.False(t, models.HasCeilingDeferredIntent(&models.Task{Metadata: after}))
	require.Equal(t, true, after[models.DeferredLaunchStartWhenUnblockedKey])
}

func TestClaimCeilingDeferredLaunchReleasesAfterCapacityBookkeepingChanges(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "claim-bookkeeping", Title: "t", State: v1.TaskStateScheduling,
		CreatedAt: now, UpdatedAt: now,
	}))
	deferral := models.CeilingDeferral{
		Kind:    models.CeilingLaunchStartCreated,
		Payload: map[string]interface{}{metaKeySessionID: "claim-bookkeeping-session"},
		Origin:  string(launchOriginAutomatic), ReasonCode: ceilingReasonRefused,
		QueuedAt: now, Ceiling: 5, Population: 6, PopulationKnown: true,
	}
	record := models.CeilingRecordKeys(deferral)
	_, _, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "claim-bookkeeping", tasksqlite.AbsentDeferredLaunch(), record)
	require.NoError(t, err)

	claim, found, err := svc.claimCeilingDeferredLaunch(
		ctx, "claim-bookkeeping", "claim-bookkeeping-session", ceilingClaimOwnerReplay,
	)
	require.NoError(t, err)
	require.True(t, found)
	require.NotNil(t, claim)

	// Capacity observations are mutable queue bookkeeping, not a successor
	// launch. A claim release must still remove its own marker after that write.
	updatedRecord := deferredLaunchOf(t, svc, "claim-bookkeeping")
	updatedRecord[models.CeilingReasonCodeKey] = "capacity_changed"
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "claim-bookkeeping", models.MetaKeyDeferredLaunch, updatedRecord))
	claim.releaseIfHeld(ctx)

	after := deferredLaunchOf(t, svc, "claim-bookkeeping")
	_, _, claimed := models.ReadCeilingLaunchClaim(after)
	require.False(t, claimed, "mutable capacity bookkeeping must not strand the in-flight claim")
	decoded, err := models.ReadCeilingDeferral(after)
	require.NoError(t, err)
	require.Equal(t, "capacity_changed", decoded.ReasonCode)
}

func TestClaimCeilingDeferredLaunchReclaimsExpiredClaim(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "claim-expired", Title: "t", State: v1.TaskStateScheduling,
		CreatedAt: now, UpdatedAt: now,
	}))
	deferral := models.CeilingDeferral{
		Kind:    models.CeilingLaunchStartCreated,
		Payload: map[string]interface{}{metaKeySessionID: "claim-expired-session"},
		Origin:  string(launchOriginAutomatic), ReasonCode: ceilingReasonRefused,
		QueuedAt: now, Ceiling: 5, Population: 5, PopulationKnown: true,
	}
	record := models.CeilingRecordKeys(deferral)
	record[models.CeilingLaunchClaimKey] = map[string]interface{}{
		"id": "abandoned-claim", "owner": ceilingClaimOwnerReplay,
		"expires_at": now.Add(-time.Minute).Format(time.RFC3339Nano),
	}
	_, _, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "claim-expired", tasksqlite.AbsentDeferredLaunch(), record)
	require.NoError(t, err)

	claim, found, err := svc.claimCeilingDeferredLaunch(ctx, "claim-expired", "claim-expired-session", ceilingClaimOwnerSendNow)
	require.NoError(t, err)
	require.True(t, found)
	require.NotNil(t, claim)
	claimID, owner, ok := models.ReadCeilingLaunchClaim(deferredLaunchOf(t, svc, "claim-expired"))
	require.True(t, ok)
	require.NotEqual(t, "abandoned-claim", claimID)
	require.Equal(t, ceilingClaimOwnerSendNow, owner)
}

func TestCeilingClaimReleaseUsesDetachedContext(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "claim-cancelled", Title: "t", State: v1.TaskStateScheduling,
		CreatedAt: now, UpdatedAt: now,
	}))
	deferral := models.CeilingDeferral{
		Kind:    models.CeilingLaunchStartCreated,
		Payload: map[string]interface{}{metaKeySessionID: "claim-cancelled-session"},
		Origin:  string(launchOriginAutomatic), ReasonCode: ceilingReasonRefused, QueuedAt: now,
	}
	_, _, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "claim-cancelled", tasksqlite.AbsentDeferredLaunch(), models.CeilingRecordKeys(deferral))
	require.NoError(t, err)
	claim, found, err := svc.claimCeilingDeferredLaunch(ctx, "claim-cancelled", "claim-cancelled-session", ceilingClaimOwnerReplay)
	require.NoError(t, err)
	require.True(t, found)
	require.NotNil(t, claim)
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	claim.releaseIfHeld(cancelledCtx)
	_, _, claimed := models.ReadCeilingLaunchClaim(deferredLaunchOf(t, svc, "claim-cancelled"))
	require.False(t, claimed, "a cancelled caller must not strand its durable claim")
}

func TestReplayCeilingDeferralRejectsStaleWorkflowEntryBeforeDispatch(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	task := &models.Task{
		ID:             "replay-stale-entry",
		WorkflowID:     "workflow-1",
		WorkflowStepID: "step-new",
		Title:          "stale replay",
		State:          v1.TaskStateScheduling,
		CreatedAt:      now,
		UpdatedAt:      now,
		Metadata: map[string]interface{}{models.MetaKeyWorkflowSessionRoute: models.WorkflowSessionRoute{
			OperationID:       "route-new",
			DestinationStepID: "step-new",
			EntryIdentity:     "entry:new",
			TargetKind:        "new_session",
			DestinationID:     "session-new",
			Phase:             "committed",
		}},
	}
	require.NoError(t, repo.CreateTask(ctx, task))

	old := models.CeilingDeferral{
		Kind: models.CeilingLaunchStart,
		Payload: map[string]interface{}{
			metaKeyPrompt: "old workflow prompt",
			models.CeilingLaunchEntryBindingKey: map[string]interface{}{
				"workflow_id":            "workflow-1",
				"destination_step_id":    "step-old",
				"route_operation_id":     "route-old",
				"entry_identity":         "entry:old",
				"destination_session_id": "session-old",
			},
		},
		Origin:     string(launchOriginAutomatic),
		ReasonCode: ceilingReasonRefused,
		QueuedAt:   now,
	}

	// The replay caller may hold an old task snapshot. The method must reload
	// and reject before any start/LaunchAgent side effect can use that snapshot.
	outcome := svc.replayCeilingDeferral(ctx, &models.Task{ID: task.ID}, old)
	require.Equal(t, ceilingReplaySuperseded, outcome)
	reloaded, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	route, ok := models.LoadWorkflowSessionRoute(reloaded.Metadata)
	require.True(t, ok)
	require.Equal(t, "route-new", route.OperationID)
}

func TestReplayCeilingDeferralRejectsRouteChangedAfterInitialValidation(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSessionWithStep(t, repo, "replay-interleaving", "replay-interleaving-session", "step-old")
	now := time.Now().UTC()
	task, err := repo.GetTask(ctx, "replay-interleaving")
	require.NoError(t, err)
	entryIdentity := (&Service{repo: repo}).workflowEntryIdentity(ctx, task.ID)
	oldRoute := models.WorkflowSessionRoute{
		OperationID:       "route-old",
		DestinationStepID: "step-old",
		EntryIdentity:     entryIdentity,
		TargetKind:        "new_session",
		DestinationID:     "replay-interleaving-session",
		Phase:             "committed",
	}
	newRoute := oldRoute
	newRoute.OperationID = "route-new"
	newRoute.DestinationStepID = "step-new"
	newRoute.EntryIdentity = "entry:new"
	require.NoError(t, repo.SetTaskMetadataKey(ctx, task.ID, models.MetaKeyWorkflowSessionRoute, oldRoute))

	deferral := models.CeilingDeferral{
		Kind: models.CeilingLaunchStartCreated,
		Payload: map[string]interface{}{
			metaKeySessionID: "replay-interleaving-session",
			metaKeyPrompt:    "old workflow prompt",
			models.CeilingLaunchEntryBindingKey: map[string]interface{}{
				"workflow_id":            "wf1",
				"destination_step_id":    "step-old",
				"route_operation_id":     "route-old",
				"entry_identity":         entryIdentity,
				"destination_session_id": "replay-interleaving-session",
			},
		},
		Origin: string(launchOriginAutomatic), ReasonCode: ceilingReasonRefused,
		QueuedAt: now,
	}
	require.NoError(t, repo.SetTaskMetadataKey(ctx, task.ID, models.MetaKeyDeferredLaunch, models.CeilingRecordKeys(deferral)))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step-old"] = &wfmodels.WorkflowStep{ID: "step-old", WorkflowID: "wf1"}
	var switchOnce sync.Once
	stepGetter.getStepFunc = func(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
		if stepID == "step-old" {
			switchOnce.Do(func() {
				latest, getErr := repo.GetTask(ctx, task.ID)
				require.NoError(t, getErr)
				latest.Metadata[models.MetaKeyWorkflowSessionRoute] = newRoute
				require.NoError(t, repo.UpdateTask(ctx, latest))
			})
		}
		return stepGetter.steps[stepID], nil
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, task.ID, v1.TaskStateScheduling)
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)

	// The old snapshot passes the first route comparison. The step read then
	// commits a successor before the concrete start-created seam is admitted;
	// that seam must re-read the route and reject before LaunchAgent.
	outcome := svc.replayCeilingDeferral(ctx, &models.Task{ID: task.ID}, deferral)
	require.Equal(t, ceilingReplaySuperseded, outcome)
	agentMgr.mu.Lock()
	launchCalls := len(agentMgr.setExecutionDescriptionCalls)
	agentMgr.mu.Unlock()
	require.Zero(t, launchCalls, "a stale replay must not dispatch the old workflow entry")

	reloaded, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	route, ok := models.LoadWorkflowSessionRoute(reloaded.Metadata)
	require.True(t, ok)
	require.Equal(t, newRoute.OperationID, route.OperationID)
}

func TestWorkflowEntryDispatchRejectsStaleCommittedRoute(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSessionWithStep(t, repo, "workflow-dispatch-stale", "workflow-dispatch-session", "step-current")
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "workflow-dispatch-stale", models.MetaKeyWorkflowSessionRoute,
		models.WorkflowSessionRoute{
			OperationID:       "route-current",
			DestinationStepID: "step-current",
			EntryIdentity:     "entry:00000000000000000042",
			TargetKind:        "new_session",
			DestinationID:     "workflow-dispatch-session",
			Phase:             "committed",
		}))

	step := &wfmodels.WorkflowStep{ID: "step-current", WorkflowID: "wf1", Name: "Current"}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	require.False(t, svc.workflowEntryDispatchIsCurrent(ctx, "workflow-dispatch-stale", step, int64(41)),
		"a late callback for an older entry must not dispatch against the committed successor route")
	require.True(t, svc.workflowEntryDispatchIsCurrent(ctx, "workflow-dispatch-stale", step, int64(42)))
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "workflow-dispatch-stale", models.MetaKeyWorkflowSessionRoute,
		models.WorkflowSessionRoute{
			OperationID:       "route-current",
			DestinationStepID: "step-current",
			EntryIdentity:     "entry:00000000000000000042",
			TargetKind:        "new_session",
			DestinationID:     "successor-session",
			Phase:             "committed",
		}))
	require.False(t, svc.workflowEntryDispatchIsCurrentForSession(
		ctx, "workflow-dispatch-stale", "workflow-dispatch-session", step, int64(42),
	), "a callback for a replaced destination must not dispatch the old session")
}

func TestValidateCeilingEntryAllowsDirectProfileWorkflowEntryWithoutRoute(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	taskID := "workflow-direct-profile"
	sessionID := "workflow-direct-profile-session"
	stepID := "workflow-direct-profile-step"
	seedTaskAndSessionWithStep(t, repo, taskID, sessionID, stepID)

	stepGetter := newMockStepGetter()
	stepGetter.steps[stepID] = &wfmodels.WorkflowStep{
		ID:             stepID,
		WorkflowID:     "wf1",
		AgentProfileID: "profile-direct",
	}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())
	task, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	entryIdentity := svc.workflowEntryIdentity(ctx, taskID)
	binding := models.CeilingWorkflowEntryBinding{
		WorkflowID:           "wf1",
		DestinationStepID:    stepID,
		RouteOperationID:     workflowSessionRouteID(taskID, stepID, entryIdentity, nil, svc.resolveStepProfileSessionStartPolicy(stepGetter.steps[stepID])),
		EntryIdentity:        entryIdentity,
		DestinationSessionID: sessionID,
	}

	deferral := models.CeilingDeferral{
		Kind: models.CeilingLaunchStartCreated,
		Payload: map[string]interface{}{
			metaKeySessionID:                    sessionID,
			metaKeyWorkflowStepID:               stepID,
			models.CeilingLaunchEntryBindingKey: ceilingEntryBindingValue(binding),
		},
		Origin:     string(launchOriginAutomatic),
		ReasonCode: ceilingReasonRefused,
		QueuedAt:   time.Now().UTC(),
	}

	disposition, detail, validationErr := svc.validateCeilingEntry(ctx, task, deferral)
	require.NoError(t, validationErr)
	require.Equal(t, ceilingEntryValid, disposition, detail)

	// A successor transition changes the latest ledger identity even when the
	// old destination has no materialized workflow_session_route. The old
	// record must become terminal instead of being retargeted by task fields.
	task.WorkflowStepID = "workflow-direct-profile-successor"
	task.UpdatedAt = time.Now().UTC()
	require.NoError(t, repo.UpdateTaskPreservingDeferredLaunch(ctx, task))
	latest, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	disposition, detail, validationErr = svc.validateCeilingEntry(ctx, latest, deferral)
	require.NoError(t, validationErr)
	require.Equal(t, ceilingEntrySuperseded, disposition, detail)
}

func TestValidateCeilingEntryAllowsPendingWorkflowStepEnsureOnlyForCurrentEntry(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSessionWithStep(t, repo, "workflow-step-pending", "workflow-step-session", "step-source")
	task, err := repo.GetTask(ctx, "workflow-step-pending")
	require.NoError(t, err)
	entryIdentity := (&Service{repo: repo}).workflowEntryIdentity(ctx, task.ID)
	require.NoError(t, repo.SetTaskMetadataKey(ctx, task.ID, models.MetaKeyWorkflowSessionRoute, models.WorkflowSessionRoute{
		OperationID:       "route-target",
		DestinationStepID: "step-target",
		EntryIdentity:     entryIdentity,
		TargetKind:        "new_session",
		DestinationID:     "workflow-step-session",
		Phase:             "committed",
	}))
	task, err = repo.GetTask(ctx, task.ID)
	require.NoError(t, err)

	stepGetter := newMockStepGetter()
	stepGetter.steps["step-target"] = &wfmodels.WorkflowStep{ID: "step-target", WorkflowID: "wf1"}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())
	binding, bound := svc.workflowEntryBindingForStep(ctx, task.ID, stepGetter.steps["step-target"], "workflow-step-session")
	require.True(t, bound)

	deferral := models.CeilingDeferral{
		Kind: models.CeilingLaunchWorkflowStepEnsure,
		Payload: map[string]interface{}{
			metaKeySessionID:                    "workflow-step-session",
			metaKeyWorkflowStepID:               "step-target",
			models.CeilingLaunchEntryBindingKey: ceilingEntryBindingValue(*binding),
		},
		Origin:   string(launchOriginAutomatic),
		QueuedAt: time.Now().UTC(),
	}

	disposition, detail, validationErr := svc.validateCeilingEntry(ctx, task, deferral)
	require.NoError(t, validationErr)
	require.Equal(t, ceilingEntryValid, disposition, detail)

	// Admission of a newer entry changes both the task step and the ledger
	// identity. The old queued transition must become terminal rather than
	// being retargeted to the new step.
	task.WorkflowStepID = "step-successor"
	task.UpdatedAt = time.Now().UTC()
	task.Metadata[models.MetaKeyWorkflowSessionRoute] = models.WorkflowSessionRoute{
		OperationID:       "route-successor",
		DestinationStepID: "step-successor",
		EntryIdentity:     "entry:successor",
		TargetKind:        "new_session",
		DestinationID:     "workflow-step-session",
		Phase:             "committed",
	}
	require.NoError(t, repo.UpdateTaskPreservingDeferredLaunch(ctx, task))

	latest, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	disposition, detail, validationErr = svc.validateCeilingEntry(ctx, latest, deferral)
	require.NoError(t, validationErr)
	require.Equal(t, ceilingEntrySuperseded, disposition, detail)
}

func TestWriteTaskReviewStateFailsClosedWhenCeilingQueueReadFails(t *testing.T) {
	baseService, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	task := &models.Task{
		ID: "review-read-error", Title: "read error", State: v1.TaskStateReview,
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreateTask(ctx, task))
	baseTaskRepo := newMockTaskRepo()
	seedMockTaskState(baseTaskRepo, task.ID, v1.TaskStateReview)
	readErr := errors.New("deferred launch storage unavailable")
	baseService.repo = ceilingDeferredLaunchReadErrorRepo{
		sessionExecutorStore: repo,
		err:                  readErr,
	}
	baseService.taskRepo = baseTaskRepo

	baseService.writeTaskReviewState(ctx, task.ID, "")

	baseTaskRepo.mu.Lock()
	writes := baseTaskRepo.stateWrites[task.ID]
	baseTaskRepo.mu.Unlock()
	require.Zero(t, writes, "queue uncertainty must not publish a REVIEW or Scheduling state")
}

func TestWriteTaskReviewStateRepairsLegacyReviewQueuedCreatedSession(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	seedTaskAndSessionWithStep(t, repo, "review-legacy-queued", "review-legacy-session", "review-step")
	require.NoError(t, repo.UpdateTaskState(ctx, "review-legacy-queued", v1.TaskStateReview))
	deferral := models.CeilingDeferral{
		Kind: models.CeilingLaunchStartCreated,
		Payload: map[string]interface{}{
			metaKeySessionID: "review-legacy-session",
		},
		Origin:          string(launchOriginAutomatic),
		ReasonCode:      ceilingReasonRefused,
		QueuedAt:        time.Now().UTC(),
		Ceiling:         5,
		Population:      6,
		PopulationKnown: true,
	}
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "review-legacy-queued", models.MetaKeyDeferredLaunch, models.CeilingRecordKeys(deferral)))
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "review-legacy-queued", v1.TaskStateReview)
	svc.taskRepo = taskRepo

	task, err := repo.GetTask(ctx, "review-legacy-queued")
	require.NoError(t, err)
	svc.writeTaskReviewState(ctx, task.ID, "review-legacy-session")

	taskRepo.mu.Lock()
	state := taskRepo.updatedStates[task.ID]
	writes := taskRepo.stateWrites[task.ID]
	taskRepo.mu.Unlock()
	require.Equal(t, v1.TaskStateScheduling, state)
	require.Equal(t, 1, writes)
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

func TestEvaluateCeilingDropReasons_OfficeStartIgnoresWorkflowAutoStartEligibility(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	stepGetter := newMockStepGetter()
	svc.workflowStepGetter = stepGetter
	ctx := context.Background()

	stepGetter.steps["office-step-no-auto-start"] = &wfmodels.WorkflowStep{ID: "office-step-no-auto-start"}
	task := &models.Task{
		ID: "keep-office-start", Title: "Office task", State: v1.TaskStateInProgress,
		WorkflowStepID: "office-step-no-auto-start", IsFromOffice: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, repo.CreateTask(ctx, task))

	_, detail, drop := svc.evaluateCeilingDropReasons(ctx, task, models.CeilingDeferral{
		Kind: models.CeilingLaunchStart, Payload: map[string]interface{}{},
	})
	require.False(t, drop, "an Office automatic start is not governed by workflow auto-start eligibility")
	require.Empty(t, detail)
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
