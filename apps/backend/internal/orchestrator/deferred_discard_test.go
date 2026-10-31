package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type discardUnreadableSessionsRepository struct {
	*sqliterepo.Repository
}

func (r *discardUnreadableSessionsRepository) ListTaskSessions(context.Context, string) ([]*models.TaskSession, error) {
	return nil, errors.New("session evidence unavailable")
}

type discardRepositoryWithoutAtomicOwnershipClear struct {
	sessionExecutorStore
}

type discardFailOnceAtomicClearRepository struct {
	*sqliterepo.Repository
	failNext bool
}

type discardSuccessorCASRepository struct {
	*sqliterepo.Repository
	replaced bool
}

type discardForeignClaimCASRepository struct {
	*sqliterepo.Repository
	claimed bool
}

type discardFailingSettlementService struct {
	*automation.Service
}

func (r *discardFailOnceAtomicClearRepository) ClearTaskDeferredLaunchAndReleaseOwnershipIfUnchanged(
	ctx context.Context, taskID string, prior interface{}, value map[string]interface{}, expectedOwner string,
) (bool, bool, bool, error) {
	if r.failNext {
		r.failNext = false
		return false, false, false, errors.New("temporary metadata write failure")
	}
	return r.Repository.ClearTaskDeferredLaunchAndReleaseOwnershipIfUnchanged(ctx, taskID, prior, value, expectedOwner)
}

func (r *discardSuccessorCASRepository) SetTaskDeferredLaunchIfUnchanged(
	ctx context.Context, taskID string, prior interface{}, value map[string]interface{},
) (bool, bool, error) {
	if !r.replaced {
		r.replaced = true
		current, currentPrior, err := r.GetTaskDeferredLaunch(ctx, taskID)
		if err != nil {
			return false, false, err
		}
		deferral, err := models.ReadCeilingDeferral(current)
		if err != nil {
			return false, false, err
		}
		deferral.QueuedAt = deferral.QueuedAt.Add(time.Second)
		deferral.Payload[metaKeyPrompt] = "successor launch"
		successor := models.CeilingRecordKeys(deferral)
		stored, lost, err := r.Repository.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, currentPrior, successor)
		if err != nil || !stored || lost {
			return false, false, errors.Join(err, errors.New("could not store successor deferred launch"))
		}
	}
	return r.Repository.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, prior, value)
}

func (r *discardForeignClaimCASRepository) SetTaskDeferredLaunchIfUnchanged(
	ctx context.Context, taskID string, prior interface{}, value map[string]interface{},
) (bool, bool, error) {
	if !r.claimed {
		r.claimed = true
		current, currentPrior, err := r.GetTaskDeferredLaunch(ctx, taskID)
		if err != nil {
			return false, false, err
		}
		current[models.CeilingLaunchClaimKey] = map[string]interface{}{
			"id": "late-send-now-claim", "owner": ceilingClaimOwnerSendNow,
			models.CeilingLaunchClaimExpiresAtKey: time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano),
		}
		stored, lost, err := r.Repository.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, currentPrior, current)
		if err != nil || !stored || lost {
			return false, false, errors.Join(err, errors.New("could not store competing claim"))
		}
	}
	return r.Repository.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, prior, value)
}

func (s *discardFailingSettlementService) DispatchRun(
	context.Context, string, automation.ThreadAction, string, func() (automation.RunDispatch, error),
) error {
	return errors.New("automation settlement unavailable")
}

func TestDiscardDeferredCeilingLaunchKeepsRunWhenOwnershipInventoryIsUnreadable(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "discard-unreadable-inventory")
	f.svc.repo = &discardUnreadableSessionsRepository{Repository: f.repo}

	result, err := f.svc.DiscardDeferredCeilingLaunch(ctx, f.taskID)

	require.NoError(t, err)
	require.Equal(t, models.DeferredLaunchDiscardRetained, result.Outcome)
	require.Equal(t, models.DeferredLaunchDiscardReasonOwnership, result.Reason)
	require.Equal(t, automation.RunStatusTriggered, f.run(t).Status,
		"unreadable ownership evidence must be detected before settling the run")
	require.True(t, f.ceilingDeferred(t), "the unreadable record remains available for a later discard")
}

func TestDiscardDeferredCeilingLaunchKeepsRunWhenAtomicClearIsUnavailable(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "discard-no-atomic-clear")
	f.svc.repo = &discardRepositoryWithoutAtomicOwnershipClear{sessionExecutorStore: f.repo}

	result, err := f.svc.DiscardDeferredCeilingLaunch(ctx, f.taskID)

	require.NoError(t, err)
	require.Equal(t, models.DeferredLaunchDiscardRetained, result.Outcome)
	require.Equal(t, models.DeferredLaunchDiscardReasonClear, result.Reason)
	require.Equal(t, automation.RunStatusTriggered, f.run(t).Status,
		"the repository capability must be checked before settling the run")
	require.True(t, f.ceilingDeferred(t), "the launch remains available when atomic ownership clear is unavailable")
}

func TestDiscardDeferredCeilingLaunchRetriesCleanupAfterFinalClearFailure(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "discard-clear-retry")
	f.svc.repo = &discardFailOnceAtomicClearRepository{Repository: f.repo, failNext: true}

	first, err := f.svc.DiscardDeferredCeilingLaunch(ctx, f.taskID)

	require.NoError(t, err)
	require.Equal(t, models.DeferredLaunchDiscardRetained, first.Outcome)
	require.Equal(t, models.DeferredLaunchDiscardReasonClear, first.Reason)
	require.Equal(t, automation.RunStatusFailed, f.run(t).Status,
		"the explicit discard settles the run before its final metadata transaction")
	require.True(t, f.ceilingDeferred(t), "the durable record remains available for cleanup retry")

	second, err := f.svc.DiscardDeferredCeilingLaunch(ctx, f.taskID)

	require.NoError(t, err)
	require.Equal(t, models.DeferredLaunchDiscardDiscarded, second.Outcome)
	require.False(t, f.ceilingDeferred(t), "a later discard completes the retained cleanup")
	task, err := f.repo.GetTask(ctx, f.taskID)
	require.NoError(t, err)
	require.False(t, models.AutomationStartOwned(task.Metadata))
}

func TestDiscardDeferredCeilingLaunchAlreadyClearDoesNotChangeAutomationOwnership(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "discard-already-clear")
	removed, err := f.repo.RemoveTaskMetadataKey(ctx, f.taskID, models.MetaKeyDeferredLaunch)
	require.NoError(t, err)
	require.True(t, removed)

	result, err := f.svc.DiscardDeferredCeilingLaunch(ctx, f.taskID)

	require.NoError(t, err)
	require.Equal(t, models.DeferredLaunchDiscardAlreadyClear, result.Outcome)
	require.Equal(t, automation.RunStatusTriggered, f.run(t).Status)
	task, err := f.repo.GetTask(ctx, f.taskID)
	require.NoError(t, err)
	require.Equal(t, f.runID, task.Metadata[models.MetaKeyAutomationStartOwned],
		"an already-clear response must not release an unrelated pending run owner")
}

func TestDiscardDeferredCeilingLaunchRetainsLiveForeignClaim(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "discard-foreign-claim")
	record, prior, err := f.repo.GetTaskDeferredLaunch(ctx, f.taskID)
	require.NoError(t, err)
	record[models.CeilingLaunchClaimKey] = map[string]interface{}{
		"id": "send-now-claim", "owner": ceilingClaimOwnerSendNow,
		models.CeilingLaunchClaimExpiresAtKey: time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano),
	}
	stored, lost, err := f.repo.SetTaskDeferredLaunchIfUnchanged(ctx, f.taskID, prior, record)
	require.NoError(t, err)
	require.True(t, stored)
	require.False(t, lost)

	result, err := f.svc.DiscardDeferredCeilingLaunch(ctx, f.taskID)

	require.NoError(t, err)
	require.Equal(t, models.DeferredLaunchDiscardRetained, result.Outcome)
	require.Equal(t, models.DeferredLaunchDiscardReasonClaimInFlight, result.Reason)
	require.Equal(t, automation.RunStatusTriggered, f.run(t).Status)
	current, _, err := f.repo.GetTaskDeferredLaunch(ctx, f.taskID)
	require.NoError(t, err)
	claim, ok := models.ReadCeilingLaunchClaimDetails(current)
	require.True(t, ok)
	require.Equal(t, "send-now-claim", claim.ID)
}

func TestDiscardDeferredCeilingLaunchDoesNotSettleAfterCASLosesToSuccessor(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "discard-cas-successor")
	repo := &discardSuccessorCASRepository{Repository: f.repo}
	f.svc.repo = repo

	result, err := f.svc.DiscardDeferredCeilingLaunch(ctx, f.taskID)

	require.NoError(t, err)
	require.True(t, repo.replaced, "the competing successor write must win the first compare")
	require.Equal(t, models.DeferredLaunchDiscardRetained, result.Outcome)
	require.Equal(t, models.DeferredLaunchDiscardReasonRecordSuperseded, result.Reason)
	require.Equal(t, automation.RunStatusTriggered, f.run(t).Status,
		"a stale discard must not settle the run belonging to a successor record")
	current, _, err := f.repo.GetTaskDeferredLaunch(ctx, f.taskID)
	require.NoError(t, err)
	deferral, err := models.ReadCeilingDeferral(current)
	require.NoError(t, err)
	require.Equal(t, "successor launch", deferral.Payload[metaKeyPrompt])
}

func TestDiscardDeferredCeilingLaunchDoesNotSettleAfterCASLosesToClaim(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "discard-cas-claim")
	repo := &discardForeignClaimCASRepository{Repository: f.repo}
	f.svc.repo = repo

	result, err := f.svc.DiscardDeferredCeilingLaunch(ctx, f.taskID)

	require.NoError(t, err)
	require.True(t, repo.claimed)
	require.Equal(t, models.DeferredLaunchDiscardRetained, result.Outcome)
	require.Equal(t, models.DeferredLaunchDiscardReasonClaimInFlight, result.Reason)
	require.Equal(t, automation.RunStatusTriggered, f.run(t).Status,
		"a claim that wins the compare must be observed before settling its run")
	current, _, err := f.repo.GetTaskDeferredLaunch(ctx, f.taskID)
	require.NoError(t, err)
	claim, ok := models.ReadCeilingLaunchClaimDetails(current)
	require.True(t, ok)
	require.Equal(t, "late-send-now-claim", claim.ID)
}

func TestDiscardDeferredCeilingLaunchRetainsUnreadableRecord(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "discard-unreadable-record")
	require.NoError(t, f.repo.SetTaskMetadataKey(ctx, f.taskID, models.MetaKeyDeferredLaunch,
		map[string]interface{}{models.CeilingDeferredKey: true}))

	result, err := f.svc.DiscardDeferredCeilingLaunch(ctx, f.taskID)

	require.NoError(t, err)
	require.Equal(t, models.DeferredLaunchDiscardRetained, result.Outcome)
	require.Equal(t, models.DeferredLaunchDiscardReasonRecordUnreadable, result.Reason)
	require.Equal(t, automation.RunStatusTriggered, f.run(t).Status)
}

func TestDiscardDeferredCeilingLaunchRetainsRecordWhenSettlementFails(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "discard-settlement-failure")
	f.svc.SetAutomationService(&discardFailingSettlementService{Service: f.autoSvc})

	result, err := f.svc.DiscardDeferredCeilingLaunch(ctx, f.taskID)

	require.NoError(t, err)
	require.Equal(t, models.DeferredLaunchDiscardRetained, result.Outcome)
	require.Equal(t, models.DeferredLaunchDiscardReasonSettlement, result.Reason)
	require.Equal(t, automation.RunStatusTriggered, f.run(t).Status)
	require.True(t, f.ceilingDeferred(t))
}

func TestDiscardDeferredCeilingLaunchPreservesCoexistingDeferredIntent(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "discard-coexisting-intent")
	record, prior, err := f.repo.GetTaskDeferredLaunch(ctx, f.taskID)
	require.NoError(t, err)
	record["wip_deferred"] = map[string]interface{}{"prompt": "keep this intent"}
	stored, lost, err := f.repo.SetTaskDeferredLaunchIfUnchanged(ctx, f.taskID, prior, record)
	require.NoError(t, err)
	require.True(t, stored)
	require.False(t, lost)

	result, err := f.svc.DiscardDeferredCeilingLaunch(ctx, f.taskID)

	require.NoError(t, err)
	require.Equal(t, models.DeferredLaunchDiscardDiscarded, result.Outcome)
	remaining, _, err := f.repo.GetTaskDeferredLaunch(ctx, f.taskID)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"prompt": "keep this intent"}, remaining["wip_deferred"])
	require.NotContains(t, remaining, models.CeilingDeferredKey)
	require.NotContains(t, remaining, models.CeilingLaunchKindKey)
}

func TestDiscardDeferredCeilingLaunchKeepsSuccessorAutomationOwner(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "discard-successor-owner")
	require.NoError(t, f.repo.SetTaskMetadataKey(ctx, f.taskID, models.MetaKeyAutomationStartOwned, "successor-run"))

	result, err := f.svc.DiscardDeferredCeilingLaunch(ctx, f.taskID)

	require.NoError(t, err)
	require.Equal(t, models.DeferredLaunchDiscardDiscarded, result.Outcome)
	task, err := f.repo.GetTask(ctx, f.taskID)
	require.NoError(t, err)
	require.Equal(t, "successor-run", task.Metadata[models.MetaKeyAutomationStartOwned])
}

func TestDiscardDeferredCeilingLaunchLeavesExistingSessionUntouched(t *testing.T) {
	ctx := context.Background()
	f := deferAutomationStartAtCeiling(t, "discard-existing-session")
	session := &models.TaskSession{ID: "existing-session", TaskID: f.taskID, State: models.TaskSessionStateRunning}
	require.NoError(t, f.repo.CreateTaskSession(ctx, session))

	result, err := f.svc.DiscardDeferredCeilingLaunch(ctx, f.taskID)

	require.NoError(t, err)
	require.Equal(t, models.DeferredLaunchDiscardDiscarded, result.Outcome)
	stored, err := f.repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, stored.State)
	task, err := f.repo.GetTask(ctx, f.taskID)
	require.NoError(t, err)
	require.Equal(t, f.runID, task.Metadata[models.MetaKeyAutomationStartOwned],
		"the discard must not release a start marker while a session remains")
}

func TestDiscardDeferredCeilingLaunchReleasesStoppedRetryOwner(t *testing.T) {
	ctx := context.Background()
	base := setupAutomationRetentionFixture(t)
	taskID, runID, groupID := "discard-retry-owner", "discard-retry-run", "discard-retry-group"
	seedAutomationTask(t, base.repo, taskID, models.TaskOriginAutomationRun, false)
	a := &automation.Automation{ID: "discard-retry-auto", WorkspaceID: "ws-" + taskID, Name: "discard", Enabled: true, MaxConcurrentRuns: 1}
	require.NoError(t, base.autoStore.CreateAutomation(ctx, a))
	run := &automation.AutomationRun{
		ID: runID, AutomationID: a.ID, TaskID: taskID, Status: automation.RunStatusTriggered,
		RetryGroupID: groupID, RetryGroupGeneration: 1, RetryState: automation.RetryStateTriggered,
		RetryLaunchConfigVersion: automation.RetryLaunchConfigVersion,
	}
	run.RetryLaunchConfigSnapshot = retrySnapshotForTest(run, a)
	require.NoError(t, base.autoStore.CreateRetryAdmission(ctx, run, &automation.RetryGroup{
		ID: groupID, AutomationID: a.ID, Generation: 1, State: automation.RetryGroupLive,
	}))
	launch, err := base.autoStore.ReserveRetryLaunch(ctx, runID, taskID, "", automation.ThreadActionCreated, "discard test")
	require.NoError(t, err)
	require.NoError(t, base.repo.SetTaskMetadataKey(ctx, taskID, models.MetaKeyAutomationStartOwned, runID))
	payload := map[string]interface{}{
		ceilingPayloadAutomationRunKey: automationRunForCeilingPayload(&automationRunLaunch{
			RunID: runID, ThreadAction: automation.ThreadActionCreated, ThreadReason: "discard test", Launch: launch,
		}),
	}
	record := models.CeilingRecordKeys(models.CeilingDeferral{
		Kind: models.CeilingLaunchStart, Origin: string(launchOriginAutomatic), ReasonCode: ceilingReasonRefused,
		QueuedAt: time.Now().UTC(), Payload: payload,
	})
	_, _, err = base.repo.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, sqliterepo.AbsentDeferredLaunch(), record)
	require.NoError(t, err)

	result, err := base.svc.DiscardDeferredCeilingLaunch(ctx, taskID)

	require.NoError(t, err)
	require.Equal(t, models.DeferredLaunchDiscardDiscarded, result.Outcome)
	task, err := base.repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	require.False(t, models.AutomationStartOwned(task.Metadata),
		"the exact stopped retry ledger must release its matching initial-start marker")
	stored, _, err := base.repo.GetTaskDeferredLaunch(ctx, taskID)
	require.NoError(t, err)
	require.False(t, models.HasCeilingDeferredIntent(&models.Task{Metadata: map[string]interface{}{models.MetaKeyDeferredLaunch: stored}}))
	stoppedRun, err := base.autoStore.GetRun(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, automation.RunStatusFailed, stoppedRun.Status)
	stoppedLaunch, err := base.autoStore.GetRetryLaunch(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, automation.RetryLaunchStopped, stoppedLaunch.State)
}
