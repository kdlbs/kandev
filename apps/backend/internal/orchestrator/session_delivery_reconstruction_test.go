package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type retainedJournalRecoveryManager struct {
	*mockAgentManager
	root                 string
	inspectCalls         int
	selectedPayloadReads int
	recoveryCalls        int
	controlIdentity      *lifecycle.AgentDeliveryRecoveryIdentity
	terminated           bool
}

func (m *retainedJournalRecoveryManager) InspectDeliveryRecordEvidence(
	ctx context.Context,
	request lifecycle.DeliveryRecordEvidenceRequest,
) (*lifecycle.DeliveryRecordEvidence, error) {
	m.inspectCalls++
	retained, err := journal.ReadRetainedReconstructionEvidence(ctx, journal.RetainedReconstructionEvidenceRequest{
		Root: m.root, SessionID: request.SessionID, IncarnationID: request.IncarnationID,
		HarnessGeneration: request.HarnessGeneration,
	})
	if err != nil {
		return nil, err
	}
	return &lifecycle.DeliveryRecordEvidence{Descriptor: retained.Descriptor, ControlIdentity: m.controlIdentity}, nil
}

func (m *retainedJournalRecoveryManager) ReadDeliveryRecordSubmission(
	ctx context.Context,
	request lifecycle.DeliveryRecordSubmissionRequest,
) (*lifecycle.DeliveryRecordSubmission, error) {
	m.selectedPayloadReads++
	return journal.ReadRetainedReconstructionSubmission(ctx, journal.RetainedReconstructionSubmissionRequest{
		Root: m.root, SessionID: request.SessionID, IncarnationID: request.IncarnationID,
		HarnessGeneration: request.HarnessGeneration, Candidate: request.Candidate,
	})
}

func (m *retainedJournalRecoveryManager) RecoverAgentPromptStream(context.Context, string) error {
	m.recoveryCalls++
	return nil
}

func (m *retainedJournalRecoveryManager) RecoverAgentPromptStreamWithIdentity(context.Context, lifecycle.AgentDeliveryRecoveryIdentity) lifecycle.DeliveryReconciliationResult {
	m.recoveryCalls++
	return lifecycle.DeliveryReconciliationResult{Outcome: lifecycle.DeliveryReconciliationUncertain, ProcessTerminated: m.terminated}
}

var _ executor.AgentManagerClient = (*retainedJournalRecoveryManager)(nil)

func TestRetrySessionDeliveryReconstructsMissingSubmission(t *testing.T) {
	ctx := context.Background()
	_, repo := newServiceWithRealRepo(t)
	seedSession(t, repo, "task-reconstruct-missing", "session-reconstruct-missing", "step-1")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-reconstruct-missing", models.TaskSessionStateWaitingForInput, ""))
	seedExecutorRunning(t, repo, "session-reconstruct-missing", "task-reconstruct-missing", "successor-execution")
	session, err := repo.GetTaskSession(ctx, "session-reconstruct-missing")
	require.NoError(t, err)
	require.Equal(t, "successor-execution", session.AgentExecutionID)
	const streamID = "stream-reconstruct-missing"
	const messageID = "message-reconstruct-missing"
	const nativeSessionID = "native-reconstruct-missing"
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: 1,
		NativeSessionID: nativeSessionID, CreationReason: "initial",
	}))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-reconstruct-missing", TaskID: session.TaskID, TaskSessionID: session.ID,
		StartedAt: time.Now().UTC(),
	}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{
		ID: messageID, TaskID: session.TaskID, TaskSessionID: session.ID,
		TurnID: "turn-reconstruct-missing", AuthorType: models.MessageAuthorUser,
		Content: "Inspect the retained changes and continue from them",
	}))
	independent := &models.SessionRecoveryBlock{
		ID: "block-independent-reconstruct", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		ExpectedGeneration: 1, Reason: "native_state_missing", State: models.RecoveryBlockOpen,
		ConsumerReference: "native_resume", CreatedAt: time.Now().UTC().Add(-time.Minute),
	}
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, independent))
	deliveryBlock := &models.SessionRecoveryBlock{
		ID: "block-delivery-reconstruct", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		ExpectedGeneration: 1, Reason: "unresolved_durable_work", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery",
	}
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, deliveryBlock))

	root := t.TempDir()
	capability := journal.CheckStorage(root, session.ID)
	journalDB, err := journal.Open(journal.Config{Path: capability.Path})
	require.NoError(t, err)
	payload := []byte(`{"text":"Inspect the retained changes and continue from them"}`)
	submissionID := "prompt:" + messageID
	now := time.Now().UTC()
	_, err = journalDB.PutSubmission(ctx, journal.Submission{
		ID: submissionID, StreamID: streamID, SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, Hash: journal.SubmissionHash(payload), Payload: payload,
		State: journal.SubmissionDispatching, CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	_, err = journalDB.PutSubmission(ctx, journal.Submission{
		ID: "prompt:completed-history", StreamID: streamID, SessionID: session.ID,
		IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1,
		Hash: journal.SubmissionHash([]byte(`{"text":"completed"}`)), Payload: []byte(`{"text":"completed"}`),
		State: journal.SubmissionCompleted, TerminalEventRetained: true, CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	_, err = journalDB.Append(ctx, journal.Event{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1,
		StreamID: streamID, SubmissionID: submissionID, Type: "message", Payload: []byte(`{"text":"acknowledged output"}`),
	})
	require.NoError(t, err)
	require.NoError(t, journalDB.Acknowledge(ctx, streamID, 1))
	require.NoError(t, journalDB.Close())

	_, err = repo.ReceiveAgentDeliveryEvent(ctx, &models.AgentDeliveryEvent{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1,
		StreamID: streamID, Sequence: 1, SubmissionID: submissionID,
		EventType: "message", Payload: []byte(`{"text":"acknowledged output"}`),
	}, 1)
	require.NoError(t, err)
	manager := &retainedJournalRecoveryManager{
		mockAgentManager: &mockAgentManager{repoForExecutionLookup: repo}, root: root,
	}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)

	response, err := service.RetrySessionDelivery(ctx, session.TaskID, session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryBlocked, response.Outcome)
	require.Equal(t, "recovery_identity_incomplete", response.Reason)
	require.EqualValues(t, 1, response.RecoveryRevision)
	require.Empty(t, response.AllowedActions)
	require.Equal(t, 1, manager.inspectCalls)
	require.Equal(t, 1, manager.selectedPayloadReads)
	require.Zero(t, manager.recoveryCalls, "incomplete historical identity must not enter stream reconciliation")
	require.Empty(t, manager.capturedPromptCalls, "Retry must not dispatch the saved instruction")
	require.NotNil(t, response.RecoveryIdentity)
	require.Equal(t, submissionID, response.RecoveryIdentity.SubmissionID)
	require.Zero(t, response.RecoveryIdentity.PromptGeneration)

	stored, err := repo.GetAgentDeliverySubmission(ctx, submissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
	require.Equal(t, payload, stored.Payload)
	require.Equal(t, journal.SubmissionHash(payload), stored.PayloadHash)
	canonicalSubmissions, err := repo.ListAgentDeliverySubmissions(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, canonicalSubmissions, 1)

	updatedSession, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	recovery, ok := models.LoadAgentDeliveryRecovery(updatedSession.Metadata)
	require.True(t, ok)
	require.Empty(t, recovery.AgentExecutionID, "the idle successor cannot stand in for the old prompt owner")
	require.Zero(t, recovery.PromptGeneration)
	require.Equal(t, "dispatching", recovery.Reconstruction.ObservedState)
	require.False(t, recovery.Reconstruction.ProcessIdentityKnown)
	blocks, err := repo.ListOpenSessionRecoveryBlocks(ctx, session.ID, session.QueueIncarnationID, 1)
	require.NoError(t, err)
	require.Len(t, blocks, 2, "reconstruction must preserve independent recovery causes")
	require.Equal(t, deliveryBlock.ID, blocks[1].ID)
	require.Equal(t, submissionID, blocks[1].DeliverySubmissionID)
	require.Equal(t, streamID, blocks[1].DeliveryStreamID)

	duplicate, err := service.RetrySessionDelivery(ctx, session.TaskID, session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryBlocked, duplicate.Outcome)
	require.Equal(t, response.RecoveryRevision, duplicate.RecoveryRevision)
	require.Equal(t, 2, manager.inspectCalls, "Retry must inspect again for newly available historical proof")
	require.Empty(t, manager.capturedPromptCalls)

	continued, err := service.ResumeInterruptedSession(ctx, session.TaskID, session.ID, InterruptedSessionResumeRequest{
		Acknowledge: true, RecoveryRevision: response.RecoveryRevision, RecoveryIdentity: *response.RecoveryIdentity,
		Instruction: "Continue after reviewing the uncertainty", IdempotencyKey: "continue-once",
	})
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryBlocked, continued.Outcome)
	require.Empty(t, continued.AllowedActions)
	require.Empty(t, manager.capturedPromptCalls, "only a recovery identity with verified termination may continue")
	_, err = repo.ResolveSessionRecoveryBlock(ctx, independent.ID, "retry", time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, repo.DeleteExecutorRunningBySessionID(ctx, session.ID))
	manager.controlIdentity = &lifecycle.AgentDeliveryRecoveryIdentity{
		TaskID: session.TaskID, SessionID: session.ID, ExecutionID: "original-execution",
		IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1,
		SubmissionID: submissionID, StreamID: streamID, PromptGeneration: 9,
		OriginalRuntime: processidentity.Identity{PID: 42, BirthToken: "verified-birth"},
	}
	enriched, err := service.RetrySessionDelivery(ctx, session.TaskID, session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryUncertain, enriched.Outcome)
	require.EqualValues(t, 2, enriched.RecoveryRevision)
	require.EqualValues(t, 9, enriched.RecoveryIdentity.PromptGeneration)
	require.Empty(t, enriched.AllowedActions, "proof of ownership alone does not prove termination")
	manager.terminated = true
	ready, err := service.RetrySessionDelivery(ctx, session.TaskID, session.ID)
	require.NoError(t, err)
	require.Equal(t, []SessionDeliveryRecoveryAction{SessionDeliveryRecoveryActionContinueInterrupted}, ready.AllowedActions)
	require.EqualValues(t, 2, ready.RecoveryRevision)
	require.Empty(t, manager.capturedPromptCalls, "Retry must never dispatch")

}
