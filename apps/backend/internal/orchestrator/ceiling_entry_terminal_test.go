package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-AGENTS-SESSION-CEILING-001.12
func TestEvaluateCeilingDropReasons_LegacySessionlessStartIsTerminal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskWithoutSession(t, repo, "drop-legacy-start", "step-auto")
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "drop-legacy-start", models.MetaKeyWorkflowSessionRoute, models.WorkflowSessionRoute{
		OperationID:       "route-legacy",
		DestinationStepID: "step-auto",
		EntryIdentity:     "entry:00000000000000000001",
		TargetKind:        "new_session",
		Phase:             "committed",
	}))
	task, err := repo.GetTask(ctx, "drop-legacy-start")
	require.NoError(t, err)

	stepGetter := newMockStepGetter()
	stepGetter.steps["step-auto"] = &wfmodels.WorkflowStep{ID: "step-auto", WorkflowID: "wf1",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}}}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())

	deferral := models.CeilingDeferral{Kind: models.CeilingLaunchStart, Payload: map[string]interface{}{}}
	reasonCode, detail, drop := svc.evaluateCeilingDropReasons(ctx, task, deferral)
	require.True(t, drop, "a legacy sessionless start can never become valid, so it must terminate")
	require.Equal(t, ceilingReasonSuperseded, reasonCode)
	require.Contains(t, detail, "legacy sessionless")
}

// @covers AC-AGENTS-SESSION-CEILING-001.12
func TestEvaluateCeilingDropReasons_AmbiguousBindingIsTerminal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "drop-ambiguous", "amb-session", models.TaskSessionStateCreated)
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "drop-ambiguous", models.MetaKeyWorkflowSessionRoute, models.WorkflowSessionRoute{
		OperationID:       "route-ambiguous",
		DestinationStepID: "step-x",
		EntryIdentity:     "entry:00000000000000000001",
		TargetKind:        "new_session",
		Phase:             "committed",
	}))
	task, err := repo.GetTask(ctx, "drop-ambiguous")
	require.NoError(t, err)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	deferral := models.CeilingDeferral{Kind: models.CeilingLaunchStartCreated, Payload: map[string]interface{}{
		metaKeySessionID:      "amb-session",
		metaKeyWorkflowStepID: "step-x",
	}}
	reasonCode, detail, drop := svc.evaluateCeilingDropReasons(ctx, task, deferral)
	require.True(t, drop, "an unresolvable workflow binding can never become valid, so it must terminate")
	require.Equal(t, ceilingReasonSuperseded, reasonCode)
	require.Contains(t, detail, "ambiguous")
}

// @covers AC-AGENTS-SESSION-CEILING-001.12
func TestCeilingSweepDropsIncompleteStoredEntryBindings(t *testing.T) {
	for _, missing := range []string{"workflow_id", "destination_step_id", "route_operation_id", "entry_identity"} {
		t.Run(missing, func(t *testing.T) {
			ctx := context.Background()
			f := newLaunchFailureReplayFixture(t, "incomplete-binding-"+missing, errors.New("launch must not run"))
			raw, prior, err := f.repo.GetTaskDeferredLaunch(ctx, f.taskID)
			require.NoError(t, err)
			deferral, err := models.ReadCeilingDeferral(raw)
			require.NoError(t, err)
			binding := map[string]interface{}{
				"workflow_id": "wf1", "destination_step_id": "step-auto",
				"route_operation_id": "stored-route", "entry_identity": "stored-entry",
			}
			delete(binding, missing)
			deferral.Payload[models.CeilingLaunchEntryBindingKey] = binding
			stored, _, err := f.repo.SetTaskDeferredLaunchIfUnchanged(ctx, f.taskID, prior, models.CeilingRecordKeys(deferral))
			require.NoError(t, err)
			require.True(t, stored)
			f.svc.drainDeferredCeilingLaunches(ctx)
			require.False(t, f.ceilingDeferred(t), "incomplete durable identity must terminate instead of retrying")
			require.Zero(t, f.launchCount(), "a malformed stored binding must never reach dispatch")
			drops := f.logs.FilterMessage("dropping a ceiling-deferred launch").All()
			require.NotEmpty(t, drops)
			require.Contains(t, drops[0].ContextMap()["detail"], "binding is incomplete")
		})
	}
}

func TestEvaluateCeilingDropReasons_StepReadErrorIsRetried(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSessionWithStep(t, repo, "keep-unreadable", "keep-unreadable-session", "step-unreadable")
	task, err := repo.GetTask(ctx, "keep-unreadable")
	require.NoError(t, err)

	stepGetter := newMockStepGetter()
	stepGetter.steps["step-unreadable"] = &wfmodels.WorkflowStep{ID: "step-unreadable", WorkflowID: "wf1"}
	stepGetter.getStepFunc = func(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
		if stepID == "step-unreadable" {
			return nil, errors.New("workflow store unavailable")
		}
		return stepGetter.steps[stepID], nil
	}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())
	entryIdentity := svc.workflowEntryIdentity(ctx, task.ID)
	binding := models.CeilingWorkflowEntryBinding{
		WorkflowID:        "wf1",
		DestinationStepID: "step-unreadable",
		RouteOperationID:  workflowSessionRouteID(task.ID, "step-unreadable", entryIdentity, nil, svc.resolveStepProfileSessionStartPolicy(stepGetter.steps["step-unreadable"])),
		EntryIdentity:     entryIdentity,
	}
	deferral := models.CeilingDeferral{
		Kind: models.CeilingLaunchStartCreated,
		Payload: map[string]interface{}{
			metaKeySessionID:                    "keep-unreadable-session",
			metaKeyWorkflowStepID:               "step-unreadable",
			models.CeilingLaunchEntryBindingKey: ceilingEntryBindingValue(binding),
		},
	}

	_, detail, drop := svc.evaluateCeilingDropReasons(ctx, task, deferral)
	require.False(t, drop, "a step read failure is uncertainty, so the record must be kept for a later sweep")
	require.Empty(t, detail)
}

func TestEvaluateCeilingDropReasons_BindingDerivationStepReadErrorIsRetried(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "keep-derive-unreadable", "derive-session", models.TaskSessionStateCreated)
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "keep-derive-unreadable", models.MetaKeyWorkflowSessionRoute, models.WorkflowSessionRoute{
		OperationID:       "route-derive",
		DestinationStepID: "step-derive",
		EntryIdentity:     "entry:00000000000000000001",
		TargetKind:        "new_session",
		DestinationID:     "derive-session",
		Phase:             "committed",
	}))
	task, err := repo.GetTask(ctx, "keep-derive-unreadable")
	require.NoError(t, err)

	stepGetter := newMockStepGetter()
	stepGetter.getStepFunc = func(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
		return nil, errors.New("workflow store unavailable")
	}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())

	deferral := models.CeilingDeferral{Kind: models.CeilingLaunchStartCreated, Payload: map[string]interface{}{
		metaKeySessionID:      "derive-session",
		metaKeyWorkflowStepID: "step-derive",
	}}
	_, detail, drop := svc.evaluateCeilingDropReasons(ctx, task, deferral)
	require.False(t, drop, "a step read failure while deriving the binding is uncertainty, so the record must be kept")
	require.Empty(t, detail)
}

func TestEvaluateCeilingDropReasons_IdentityReadErrorIsRetried(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSessionWithStep(t, repo, "keep-identity-unreadable", "identity-session", "step-identity")
	task, err := repo.GetTask(ctx, "keep-identity-unreadable")
	require.NoError(t, err)

	stepGetter := newMockStepGetter()
	stepGetter.steps["step-identity"] = &wfmodels.WorkflowStep{ID: "step-identity", WorkflowID: "wf1"}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())
	entryIdentity := svc.workflowEntryIdentity(ctx, task.ID)
	binding := models.CeilingWorkflowEntryBinding{
		WorkflowID:        "wf1",
		DestinationStepID: "step-identity",
		RouteOperationID:  workflowSessionRouteID(task.ID, "step-identity", entryIdentity, nil, svc.resolveStepProfileSessionStartPolicy(stepGetter.steps["step-identity"])),
		EntryIdentity:     entryIdentity,
	}
	deferral := models.CeilingDeferral{
		Kind: models.CeilingLaunchStartCreated,
		Payload: map[string]interface{}{
			metaKeySessionID:                    "identity-session",
			metaKeyWorkflowStepID:               "step-identity",
			models.CeilingLaunchEntryBindingKey: ceilingEntryBindingValue(binding),
		},
	}
	failing := &workflowStartPromptTransitionReadFailureRepository{Repository: repo, err: errors.New("transition ledger unavailable")}
	svc.repo = failing

	_, detail, drop := svc.evaluateCeilingDropReasons(ctx, task, deferral)
	require.False(t, drop, "an entry identity read failure is uncertainty, so the record must be kept")
	require.Empty(t, detail)
}

// @covers AC-AGENTS-SESSION-CEILING-001.12
func TestCeilingSweepDropsALegacySessionlessStart(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskWithoutSession(t, repo, "sweep-legacy-start", "step-auto")
	// The committed route names another step, so the sweep cannot derive a
	// binding for this legacy record and validation sees it sessionless.
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "sweep-legacy-start", models.MetaKeyWorkflowSessionRoute, models.WorkflowSessionRoute{
		OperationID:       "route-legacy",
		DestinationStepID: "step-routed",
		EntryIdentity:     "entry:00000000000000000001",
		TargetKind:        "new_session",
		Phase:             "committed",
	}))
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-auto"] = &wfmodels.WorkflowStep{ID: "step-auto", WorkflowID: "wf1",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}}}
	agent := &mockAgentManager{repoForExecutionLookup: repo}
	tasks := newMockTaskRepo()
	seedMockTaskState(tasks, "sweep-legacy-start", v1.TaskStateScheduling)
	svc := createTestServiceWithScheduler(repo, stepGetter, tasks, agent)
	svc.sessionCeiling = newSessionCeilingController(unlimitedSessionCeiling, nil, nil)
	record := models.CeilingRecordKeys(models.CeilingDeferral{
		Kind:       models.CeilingLaunchStart,
		Payload:    map[string]interface{}{metaKeyPrompt: "start"},
		Origin:     string(launchOriginAutomatic),
		ReasonCode: ceilingReasonRefused,
		QueuedAt:   time.Now().UTC(),
	})
	_, _, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "sweep-legacy-start", tasksqlite.AbsentDeferredLaunch(), record)
	require.NoError(t, err)

	svc.drainDeferredCeilingLaunches(ctx)

	require.False(t, models.HasCeilingDeferredIntent(&models.Task{Metadata: map[string]interface{}{
		models.MetaKeyDeferredLaunch: deferredLaunchOf(t, svc, "sweep-legacy-start"),
	}}), "a start that can never become valid must be dropped instead of kept for every later sweep")
	agent.mu.Lock()
	defer agent.mu.Unlock()
	require.Empty(t, agent.setExecutionDescriptionCalls, "a dropped start must not launch")
}

// @covers AC-AGENTS-SESSION-CEILING-001.12
func TestEvaluateCeilingDropReasons_DeletedStepDuringBindingDerivationIsTerminal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "drop-derive-deleted", "derive-deleted-session", models.TaskSessionStateCreated)
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "drop-derive-deleted", models.MetaKeyWorkflowSessionRoute, models.WorkflowSessionRoute{
		OperationID:       "route-derive-deleted",
		DestinationStepID: "step-derive-deleted",
		EntryIdentity:     "entry:00000000000000000001",
		TargetKind:        "new_session",
		DestinationID:     "derive-deleted-session",
		Phase:             "committed",
	}))
	task, err := repo.GetTask(ctx, "drop-derive-deleted")
	require.NoError(t, err)

	stepGetter := newMockStepGetter()
	stepGetter.getStepFunc = func(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
		return nil, fmt.Errorf("%w: %s", wfmodels.ErrWorkflowStepNotFound, stepID)
	}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())

	deferral := models.CeilingDeferral{Kind: models.CeilingLaunchStartCreated, Payload: map[string]interface{}{
		metaKeySessionID:      "derive-deleted-session",
		metaKeyWorkflowStepID: "step-derive-deleted",
	}}
	reasonCode, detail, drop := svc.evaluateCeilingDropReasons(ctx, task, deferral)
	require.True(t, drop, "a deleted destination step can never become valid, so it must terminate")
	require.Equal(t, ceilingReasonSuperseded, reasonCode)
	require.Contains(t, detail, "ambiguous")
}

// @covers AC-AGENTS-SESSION-CEILING-001.12
func TestEvaluateCeilingDropReasons_DeletedUnroutedDestinationStepIsTerminal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSessionWithStep(t, repo, "drop-unrouted-deleted", "unrouted-deleted-session", "step-unrouted-deleted")
	task, err := repo.GetTask(ctx, "drop-unrouted-deleted")
	require.NoError(t, err)

	stepGetter := newMockStepGetter()
	stepGetter.getStepFunc = func(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
		return nil, fmt.Errorf("%w: %s", wfmodels.ErrWorkflowStepNotFound, stepID)
	}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())
	entryIdentity := svc.workflowEntryIdentity(ctx, task.ID)
	step := &wfmodels.WorkflowStep{ID: "step-unrouted-deleted", WorkflowID: "wf1"}
	binding := models.CeilingWorkflowEntryBinding{
		WorkflowID:        "wf1",
		DestinationStepID: "step-unrouted-deleted",
		RouteOperationID:  workflowSessionRouteID(task.ID, "step-unrouted-deleted", entryIdentity, nil, svc.resolveStepProfileSessionStartPolicy(step)),
		EntryIdentity:     entryIdentity,
	}
	deferral := models.CeilingDeferral{
		Kind: models.CeilingLaunchStartCreated,
		Payload: map[string]interface{}{
			metaKeySessionID:                    "unrouted-deleted-session",
			metaKeyWorkflowStepID:               "step-unrouted-deleted",
			models.CeilingLaunchEntryBindingKey: ceilingEntryBindingValue(binding),
		},
	}
	_, detail, drop := svc.evaluateCeilingDropReasons(ctx, task, deferral)
	require.True(t, drop, "a deleted unrouted destination step can never become valid, so it must terminate")
	require.Contains(t, detail, "no longer exists")
}

// @covers AC-AGENTS-SESSION-CEILING-001.12
func TestEvaluateCeilingDropReasons_DeletedRoutedDestinationStepIsTerminal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSessionWithStep(t, repo, "drop-routed-deleted", "routed-deleted-session", "step-routed-deleted")

	stepGetter := newMockStepGetter()
	stepGetter.getStepFunc = func(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
		return nil, fmt.Errorf("%w: %s", wfmodels.ErrWorkflowStepNotFound, stepID)
	}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())
	task, err := repo.GetTask(ctx, "drop-routed-deleted")
	require.NoError(t, err)
	entryIdentity := svc.workflowEntryIdentity(ctx, task.ID)
	require.NotEqual(t, legacyWorkflowEntryIdentity, entryIdentity, "the task row must supply a usable identity")
	operationID := "route-routed-deleted"
	require.NoError(t, repo.SetTaskMetadataKey(ctx, "drop-routed-deleted", models.MetaKeyWorkflowSessionRoute, models.WorkflowSessionRoute{
		OperationID:       operationID,
		DestinationStepID: "step-routed-deleted",
		EntryIdentity:     entryIdentity,
		TargetKind:        "new_session",
		DestinationID:     "routed-deleted-session",
		Phase:             "committed",
	}))
	task, err = repo.GetTask(ctx, "drop-routed-deleted")
	require.NoError(t, err)
	binding := models.CeilingWorkflowEntryBinding{
		WorkflowID:           "wf1",
		DestinationStepID:    "step-routed-deleted",
		RouteOperationID:     operationID,
		EntryIdentity:        entryIdentity,
		DestinationSessionID: "routed-deleted-session",
	}
	deferral := models.CeilingDeferral{
		Kind: models.CeilingLaunchStartCreated,
		Payload: map[string]interface{}{
			metaKeySessionID:                    "routed-deleted-session",
			metaKeyWorkflowStepID:               "step-routed-deleted",
			models.CeilingLaunchEntryBindingKey: ceilingEntryBindingValue(binding),
		},
	}
	_, detail, drop := svc.evaluateCeilingDropReasons(ctx, task, deferral)
	require.True(t, drop, "a deleted routed destination step can never become valid, so it must terminate")
	require.Contains(t, detail, "workflow changed")
}
