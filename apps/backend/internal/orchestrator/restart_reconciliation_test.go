package orchestrator

import (
	"context"
	"github.com/kandev/kandev/internal/common/processidentity"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type terminatedRestartJournalManager struct {
	*retainedJournalRecoveryManager
}

func (m *terminatedRestartJournalManager) InspectDeliveryRecordEvidence(ctx context.Context, request lifecycle.DeliveryRecordEvidenceRequest) (*lifecycle.DeliveryRecordEvidence, error) {
	evidence, err := m.retainedJournalRecoveryManager.InspectDeliveryRecordEvidence(ctx, request)
	if evidence != nil {
		evidence.ProcessTerminated = &m.terminated
	}
	return evidence, err
}

// @covers AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3
func TestRestartRecoveryRepairsDispatchStateDisagreement(t *testing.T) {
	testRestartDispatchReconciliation(t, "matching")
}

func TestRestartRecoveryRejectsConflictingDispatchEvidence(t *testing.T) {
	for _, condition := range []string{"hash", "stream", "extra SQL", "live owner"} {
		t.Run(condition, func(t *testing.T) { testRestartDispatchReconciliation(t, condition) })
	}
}

func testRestartDispatchReconciliation(t *testing.T, condition string) {
	service, repo, base, session, _ := newSilentRestoreEligibilityFixture(t, "native", "restart")
	ctx := context.Background()
	root := t.TempDir()
	path := journal.CheckStorage(root, session.ID).Path
	store, err := journal.Open(journal.Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	payload := []byte(`{"text":"original prompt"}`)
	peerStream := "restart-stream"
	if condition == "stream" {
		peerStream = "foreign-stream"
	}
	_, err = store.PutSubmission(ctx, journal.Submission{ID: "restart-submission", SessionID: session.ID,
		IncarnationID: session.QueueIncarnationID, StreamID: peerStream, HarnessGeneration: 1,
		Hash: journal.SubmissionHash(payload), Payload: payload, State: journal.SubmissionDispatching})
	require.NoError(t, err)
	require.NoError(t, store.Close())
	_, err = repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET state = ?, payload_hash = ?, payload = ? WHERE id = ?`,
		models.DeliverySubmissionDispatching, journal.SubmissionHash(payload), payload, "restart-submission")
	require.NoError(t, err)
	if condition == "hash" {
		_, err = repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET payload_hash = ? WHERE id = ?`, "foreign", "restart-submission")
		require.NoError(t, err)
	}
	if condition == "extra SQL" {
		_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: "unassociated", SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "hash", Payload: []byte("unassociated"), State: models.DeliverySubmissionInterruptedUnknown})
		require.NoError(t, err)
	}
	manager := &terminatedRestartJournalManager{&retainedJournalRecoveryManager{mockAgentManager: base, root: root, terminated: condition != "live owner"}}
	service.agentManager = manager
	inputs, err := service.loadSilentRestoreInputs(ctx, models.SilentRestoreCandidate{TaskID: session.TaskID,
		SessionID: session.ID, WorkspaceID: "ws1"})
	require.NoError(t, err)
	require.NotNil(t, inputs)
	reconcileErr := service.reconcileSilentRestoreSource(ctx, inputs)
	want := models.DeliverySubmissionInterruptedUnknown
	if condition == "matching" {
		require.NoError(t, reconcileErr)
	} else {
		require.Error(t, reconcileErr)
		want = models.DeliverySubmissionDispatching
	}
	submission, err := repo.GetAgentDeliverySubmission(ctx, "restart-submission")
	require.NoError(t, err)
	require.Equal(t, want, submission.State)
	require.Empty(t, manager.capturedPromptCalls)
}

type alreadyResumedRestartManager struct {
	*terminatedRestartJournalManager
	retirementCalls int
}

func (m *alreadyResumedRestartManager) ReconcileAlreadyResumedDelivery(ctx context.Context, identity lifecycle.AgentDeliveryRecoveryIdentity, _ string) (bool, error) {
	m.retirementCalls++
	store, err := journal.Open(journal.Config{Path: journal.CheckStorage(m.root, identity.SessionID).Path})
	if err != nil {
		return false, err
	}
	defer func() { _ = store.Close() }()
	if err = store.ClassifyAbandonedDispatches(ctx, identity.SessionID, identity.IncarnationID, identity.StreamID, identity.HarnessGeneration); err != nil {
		return false, err
	}
	_, err = store.RetireSubmission(ctx, identity.SubmissionID, identity.HarnessGeneration)
	return err == nil, err
}

func TestRestartRecoveryUsesAlreadyLoadedNativeConversation(t *testing.T) {
	testAlreadyLoadedRestartRecovery(t, false, false, false)
}

func TestRestartRecoveryRepairsBothDispatchingWithoutMetadata(t *testing.T) {
	testAlreadyLoadedRestartRecovery(t, true, false, false)
}

func TestAlreadyLoadedRestartRecoveryPreservesIndependentBlock(t *testing.T) {
	testAlreadyLoadedRestartRecovery(t, false, true, false)
}

func TestRestartRecoveryRepairsIncompleteBlockWithKnownOwner(t *testing.T) {
	testAlreadyLoadedRestartRecovery(t, false, false, true)
}

func testAlreadyLoadedRestartRecovery(t *testing.T, missingMetadata, independentBlock, partialBlock bool) {
	service, repo, base, session, store := newSilentRestoreEligibilityFixture(t, "native", "loaded")
	ctx := context.Background()
	root := t.TempDir()
	j, err := journal.Open(journal.Config{Path: journal.CheckStorage(root, session.ID).Path})
	require.NoError(t, err)
	payload := []byte(`{"text":"original prompt"}`)
	submissionID := "loaded-submission"
	state := journal.SubmissionInterruptedUnknown
	if missingMetadata {
		submissionID, state = "prompt:crash-message", journal.SubmissionDispatching
		require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "crash-turn", TaskID: session.TaskID, TaskSessionID: session.ID, StartedAt: time.Now().UTC()}))
		require.NoError(t, repo.CreateMessage(ctx, &models.Message{ID: "crash-message", TaskID: session.TaskID, TaskSessionID: session.ID,
			TurnID: "crash-turn", AuthorType: models.MessageAuthorUser, Content: "original prompt"}))
		_, err = repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET id = ?, dispatch_attempt_id = 'crash-message', state = 'dispatching' WHERE id = ?`, submissionID, "loaded-submission")
		require.NoError(t, err)
		_, err = repo.DB().ExecContext(ctx, `UPDATE task_sessions SET metadata = '{}' WHERE id = ?`, session.ID)
		require.NoError(t, err)
		_, err = repo.DB().ExecContext(ctx, `UPDATE session_recovery_blocks SET delivery_submission_id = '', delivery_stream_id = '', reason = 'unresolved_durable_work' WHERE session_id = ?`, session.ID)
		require.NoError(t, err)
	}
	_, err = j.PutSubmission(ctx, journal.Submission{ID: submissionID, SessionID: session.ID,
		IncarnationID: session.QueueIncarnationID, StreamID: "loaded-stream", HarnessGeneration: 1,
		Hash: journal.SubmissionHash(payload), Payload: payload, State: state})
	require.NoError(t, err)
	_, err = j.Append(ctx, journal.Event{SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, StreamID: "loaded-stream", SubmissionID: submissionID, Type: "message_chunk", Payload: []byte(`{"text":"retained output"}`)})
	require.NoError(t, err)
	require.NoError(t, j.Close())
	_, err = repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET payload_hash = ?, payload = ? WHERE id = ?`, journal.SubmissionHash(payload), payload, submissionID)
	require.NoError(t, err)
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "replacement", TaskID: session.TaskID,
		SessionID: session.ID, AgentExecutionID: "replacement", Status: models.ExecutorRunningStatusRunning}))
	require.NoError(t, repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateWaitingForInput, ""))
	manager := &alreadyResumedRestartManager{terminatedRestartJournalManager: &terminatedRestartJournalManager{
		&retainedJournalRecoveryManager{mockAgentManager: base, root: root, terminated: true}}}
	manager.controlIdentity = &lifecycle.AgentDeliveryRecoveryIdentity{TaskID: session.TaskID, SessionID: session.ID,
		ExecutionID: "loaded-execution", IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1,
		SubmissionID: submissionID, StreamID: "loaded-stream", PromptGeneration: 3,
		OriginalRuntime: processidentity.Identity{PID: 42, BirthToken: "verified-historical-birth"}}
	service.agentManager = manager
	if partialBlock {
		_, err = repo.DB().ExecContext(ctx, `UPDATE session_recovery_blocks SET delivery_stream_id = '' WHERE session_id = ?`, session.ID)
		require.NoError(t, err)
	}
	if independentBlock {
		require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{ID: "independent", SessionID: session.ID, IncarnationID: session.QueueIncarnationID, ExpectedGeneration: 1, Reason: "independent", State: models.RecoveryBlockOpen}))
	}
	err = service.restoreSilentRestartCandidate(ctx, store, models.SilentRestoreCandidate{
		TaskID: session.TaskID, SessionID: session.ID, WorkspaceID: "ws1"})
	if independentBlock {
		require.Error(t, err)
		require.Zero(t, manager.retirementCalls)
		return
	}
	require.NoError(t, err)
	require.Equal(t, 1, manager.retirementCalls, "recovery must reuse the loaded conversation before allocating another execution")
	updated, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	recovery, exists := models.LoadAgentDeliveryRecovery(updated.Metadata)
	require.True(t, exists)
	require.Equal(t, models.AgentDeliveryRecoveryRestored, recovery.Phase)
	block, err := repo.ListOpenSessionRecoveryBlocks(ctx, session.ID, session.QueueIncarnationID, 1)
	require.NoError(t, err)
	require.Empty(t, block)
	require.Equal(t, "replacement", updated.AgentExecutionID)
	canonical, err := repo.GetAgentDeliverySubmission(ctx, submissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, canonical.State)
	require.Equal(t, payload, canonical.Payload)
	require.Empty(t, canonical.Outcome)
	retiredStore, err := journal.Open(journal.Config{Path: journal.CheckStorage(root, session.ID).Path})
	require.NoError(t, err)
	retired, err := retiredStore.GetSubmission(ctx, submissionID)
	require.NoError(t, err)
	require.True(t, retired.Retired)
	require.Equal(t, journal.SubmissionInterruptedUnknown, retired.State)
	require.Equal(t, payload, retired.Payload)
	require.NoError(t, retiredStore.Close())
	require.Empty(t, manager.capturedPromptCalls)
	err = service.restoreSilentRestartCandidate(ctx, store, models.SilentRestoreCandidate{
		TaskID: session.TaskID, SessionID: session.ID, WorkspaceID: "ws1"})
	if independentBlock {
		require.Error(t, err)
		require.Zero(t, manager.retirementCalls)
		return
	}
	require.NoError(t, err)
	require.Equal(t, 1, manager.retirementCalls, "another restart must not retire or replay the old prompt again")
}
