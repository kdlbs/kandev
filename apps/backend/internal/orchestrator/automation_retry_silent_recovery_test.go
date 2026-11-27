package orchestrator

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type retryOwnedLoadedRecoveryManager struct {
	*mockAgentManager
	calls int
}

func (m *retryOwnedLoadedRecoveryManager) ReconcileLoadedNativeDelivery(context.Context, string) (*models.NativeResumeDeliveryEvidence, error) {
	m.calls++
	return nil, nil
}

func TestOrdinaryOriginRetryOwnerFencesSilentRecoveryBeforeCleanupQueueAndCheckpoint(t *testing.T) {
	fixture := newSilentRestoreCheckpointFixtureWithLaunching(t, false)
	ctx := fixture.ctx
	task, err := fixture.repo.GetTask(ctx, fixture.session.TaskID)
	require.NoError(t, err)
	require.False(t, models.IsAutomationTaskOrigin(task.Origin))
	candidate := models.SilentRestoreCandidate{TaskID: task.ID, SessionID: fixture.session.ID, WorkspaceID: task.WorkspaceID}
	_, eligible, err := fixture.service.loadSilentRestoreOwner(ctx, candidate)
	require.NoError(t, err)
	require.True(t, eligible, "ordinary silent restoration remains eligible without a retry owner")
	fixture.service.messageQueue = newAuthoritativeMemoryQueue(fixture.repo, testLogger())
	_, err = fixture.service.messageQueue.QueueMessage(ctx, fixture.session.ID, task.ID, "retained queued prompt", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	queueBefore := fixture.service.messageQueue.GetStatus(ctx, fixture.session.ID)
	require.True(t, queueBefore.AutoRun)
	raw := sqlx.NewDb(fixture.repo.DB(), "sqlite3")
	store, err := automation.NewStore(raw, raw)
	require.NoError(t, err)
	a := &automation.Automation{WorkspaceID: task.WorkspaceID, Name: "ordinary retry continuation", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	run := &automation.AutomationRun{ID: "ordinary-retry-owner", AutomationID: a.ID, TaskID: task.ID, Status: automation.RunStatusTriggered, RetryGroupID: "ordinary-retry-group", RetryGroupGeneration: 1, RetryState: automation.RetryStateTriggered}
	require.NoError(t, store.CreateRetryAdmission(ctx, run, &automation.RetryGroup{ID: run.RetryGroupID, AutomationID: a.ID, Generation: 1, State: automation.RetryGroupLive}))
	_, err = store.ReserveRetryLaunch(ctx, run.ID, task.ID, fixture.session.ID, automation.ThreadActionResumed, "existing task")
	require.NoError(t, err)
	_, err = store.ClaimRetryLaunch(ctx, run.ID)
	require.NoError(t, err)
	fixture.service.SetAutomationService(automation.NewService(store, nil, testLogger()))
	before, err := fixture.repo.GetExecutorRunningBySessionID(ctx, fixture.session.ID)
	require.NoError(t, err)
	checkpointBefore, err := fixture.store.GetRestoreAttempt(ctx, fixture.attemptID)
	require.NoError(t, err)
	loaded := &retryOwnedLoadedRecoveryManager{mockAgentManager: fixture.manager}
	fixture.service.agentManager = loaded
	blockBefore, err := fixture.repo.GetSessionRecoveryBlock(ctx, fixture.inputs.block.ID)
	require.NoError(t, err)
	resolved, err := fixture.service.finishLoadedNativeDeliveryBlock(ctx, blockBefore)
	require.NoError(t, err)
	require.False(t, resolved)
	require.Zero(t, loaded.calls, "generic recovery must not retire retry-owned delivery history")
	_, err = fixture.service.repairRestartRecoveryPresentation(ctx, fixture.session)
	require.NoError(t, err)
	require.Zero(t, loaded.calls, "presentation repair cannot bypass retry ownership")
	blockAfter, err := fixture.repo.GetSessionRecoveryBlock(ctx, fixture.inputs.block.ID)
	require.NoError(t, err)
	require.Equal(t, blockBefore, blockAfter)
	_, eligible, err = fixture.service.loadSilentRestoreOwner(ctx, candidate)
	require.NoError(t, err)
	require.False(t, eligible, "task origin cannot authorize recovery of retry-owned work")
	_, err = fixture.service.revalidateSilentRestoreInputs(ctx, fixture.inputs, fixture.checkpoint)
	require.ErrorIs(t, err, automation.ErrRetryLaunchUncertain)
	err = fixture.service.startSilentRestoreCandidate(ctx, fixture.store, fixture.attemptID, fixture.checkpoint, fixture.inputs, before)
	require.ErrorIs(t, err, automation.ErrRetryLaunchUncertain)
	unallocated := fixture.checkpoint
	unallocated.CandidateExecutionID = ""
	err = fixture.service.ensureSilentRestoreCandidate(ctx, fixture.store, fixture.attemptID, unallocated, fixture.inputs)
	require.ErrorIs(t, err, automation.ErrRetryLaunchUncertain)
	fixture.service.runSilentRestoreRecoveryPass(context.Background(), fixture.store)
	after, err := fixture.repo.GetExecutorRunningBySessionID(ctx, fixture.session.ID)
	require.NoError(t, err)
	require.Equal(t, before, after, "silent recovery cannot remove the retry-owned source")
	checkpointAfter, err := fixture.store.GetRestoreAttempt(ctx, fixture.attemptID)
	require.NoError(t, err)
	require.Equal(t, checkpointBefore, checkpointAfter, "candidate checkpoints must remain unchanged")
	require.Equal(t, queueBefore, fixture.service.messageQueue.GetStatus(ctx, fixture.session.ID), "retry ownership must not pause or rewrite the queue")
}
