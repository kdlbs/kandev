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
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type reconstructedContinuationManager struct {
	*retainedJournalRecoveryManager
}

func (m *reconstructedContinuationManager) DurableDeliveryCapabilityForExecution(ctx context.Context, executionID string) (lifecycle.DurableDeliveryCapability, bool) {
	return (&interruptedRecoveryManager{mockAgentManager: m.mockAgentManager}).DurableDeliveryCapabilityForExecution(ctx, executionID)
}

func (m *reconstructedContinuationManager) PromptAgentWithAdmissionCallbackAndSubmissionID(ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool, beforeAdmission func() error, onDispatched func(), _ string) (*executor.PromptResult, error) {
	return m.PromptAgentWithAdmissionCallback(ctx, executionID, prompt, attachments, dispatchOnly, beforeAdmission, onDispatched)
}

func TestReconstructedDeliveryContinuesOnlyExplicitly(t *testing.T) {
	ctx := context.Background()
	const submissionID = "prompt:original-message"
	svc, request, launches := interruptedResumeFixtureWithSubmission(t, submissionID)
	repo := svc.repo.(*sqliterepo.Repository)
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	delete(session.Metadata, models.SessionMetaKeyAgentDeliveryRecovery)
	require.NoError(t, repo.UpdateTaskSessionWithMetadata(ctx, session, session.Metadata))
	_, err = repo.DB().ExecContext(ctx, `DELETE FROM agent_delivery_submissions WHERE id = ?`, submissionID)
	require.NoError(t, err)
	_, err = repo.DB().ExecContext(ctx, `UPDATE session_recovery_blocks SET delivery_submission_id = '', delivery_stream_id = '' WHERE session_id = ?`, "s1")
	require.NoError(t, err)
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "original-turn", TaskID: "t1", TaskSessionID: "s1", StartedAt: time.Now().UTC()}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{TurnID: "original-turn", ID: "original-message", TaskID: "t1", TaskSessionID: "s1", AuthorType: models.MessageAuthorUser, Content: "old instruction"}))
	root := t.TempDir()
	storage := journal.CheckStorage(root, "s1")
	retained, err := journal.Open(journal.Config{Path: storage.Path})
	require.NoError(t, err)
	payload := []byte(`{"text":"old instruction"}`)
	now := time.Now().UTC()
	_, err = retained.PutSubmission(ctx, journal.Submission{
		ID: submissionID, SessionID: "s1", IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1,
		StreamID: "stream", Hash: journal.SubmissionHash(payload), Payload: payload,
		State: journal.SubmissionDispatching, CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	_, err = retained.Append(ctx, journal.Event{
		SessionID: "s1", IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1,
		StreamID: "stream", SubmissionID: submissionID, Type: "message", Payload: []byte(`{"text":"retained output"}`),
	})
	require.NoError(t, err)
	require.NoError(t, retained.Close())
	manager := &reconstructedContinuationManager{&retainedJournalRecoveryManager{
		mockAgentManager: svc.agentManager.(*interruptedRecoveryManager).mockAgentManager, root: root, terminated: true,
		controlIdentity: &lifecycle.AgentDeliveryRecoveryIdentity{
			TaskID: "t1", SessionID: "s1", ExecutionID: "old-execution", IncarnationID: session.QueueIncarnationID,
			HarnessGeneration: 1, SubmissionID: submissionID, StreamID: "stream", PromptGeneration: 1,
			OriginalRuntime: processidentity.Identity{PID: 42, BirthToken: "verified-birth"},
		},
	}}
	svc.agentManager = manager
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	ready, err := svc.RetrySessionDelivery(ctx, "t1", "s1")
	require.NoError(t, err)
	require.Equal(t, []SessionDeliveryRecoveryAction{SessionDeliveryRecoveryActionContinueInterrupted}, ready.AllowedActions)
	require.Empty(t, manager.capturedPrompts)
	require.Zero(t, *launches)
	request.RecoveryRevision = ready.RecoveryRevision
	request.RecoveryIdentity = *ready.RecoveryIdentity
	for range 2 {
		result, resumeErr := svc.ResumeInterruptedSession(ctx, "t1", "s1", request)
		require.NoError(t, resumeErr)
		require.Equal(t, SessionDeliveryRecoveryContinued, result.Outcome)
	}
	require.Equal(t, 1, *launches)
	require.Equal(t, []string{request.Instruction}, manager.capturedPrompts)
	stored, err := repo.GetAgentDeliverySubmission(ctx, submissionID)
	require.NoError(t, err)
	require.Equal(t, payload, stored.Payload)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
}
