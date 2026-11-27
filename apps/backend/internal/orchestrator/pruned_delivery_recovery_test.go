package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.3
// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.4
func TestPrunedDeliveryReconstructsDeadDispatchWithoutAdvancingCursor(t *testing.T) {
	ctx := context.Background()
	service, repo, base, session, _ := newSilentRestoreEligibilityFixture(t, "native", "gap")
	const submissionID = "prompt:gap-message"
	const streamID = "gap-stream"
	payload := []byte(`{"text":"original prompt"}`)
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "gap-turn", TaskID: session.TaskID, TaskSessionID: session.ID, StartedAt: time.Now().UTC()}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{ID: "gap-message", TaskID: session.TaskID, TaskSessionID: session.ID,
		TurnID: "gap-turn", AuthorType: models.MessageAuthorUser, Content: "original prompt"}))
	_, err := repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET id = ?, dispatch_attempt_id = 'gap-message', state = 'dispatching', payload_hash = ?, payload = ? WHERE id = 'gap-submission'`, submissionID, journal.SubmissionHash(payload), payload)
	require.NoError(t, err)
	_, err = repo.DB().ExecContext(ctx, `UPDATE task_sessions SET metadata = '{}', state = 'WAITING_FOR_INPUT' WHERE id = ?`, session.ID)
	require.NoError(t, err)
	_, err = repo.DB().ExecContext(ctx, `DELETE FROM session_recovery_blocks WHERE session_id = ?`, session.ID)
	require.NoError(t, err)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, &models.AgentDeliveryEvent{SessionID: session.ID,
		IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1, StreamID: streamID, Sequence: 1,
		EventType: "context_window", Payload: []byte(`{}`)}, 1)
	require.NoError(t, err)
	_, err = repo.DB().ExecContext(ctx, `UPDATE agent_delivery_cursors SET projected_sequence = 1 WHERE stream_id = ?`, streamID)
	require.NoError(t, err)
	before, err := repo.GetAgentDeliveryCursor(ctx, streamID)
	require.NoError(t, err)
	root := t.TempDir()
	j, err := journal.Open(journal.Config{Path: journal.CheckStorage(root, session.ID).Path})
	require.NoError(t, err)
	_, err = j.PutSubmission(ctx, journal.Submission{ID: submissionID, SessionID: session.ID,
		IncarnationID: session.QueueIncarnationID, StreamID: streamID, HarnessGeneration: 1,
		Hash: journal.SubmissionHash(payload), Payload: payload, State: journal.SubmissionInterruptedUnknown})
	require.NoError(t, err)
	for range 6 {
		_, err = j.Append(ctx, journal.Event{SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
			HarnessGeneration: 1, StreamID: streamID, SubmissionID: submissionID, Type: "context_window", Payload: []byte(`{}`)})
		require.NoError(t, err)
	}
	require.NoError(t, j.Acknowledge(ctx, streamID, 6))
	require.NoError(t, j.Close())
	manager := &terminatedRestartJournalManager{&retainedJournalRecoveryManager{mockAgentManager: base, root: root, terminated: true}}
	manager.controlIdentity = &lifecycle.AgentDeliveryRecoveryIdentity{TaskID: session.TaskID, SessionID: session.ID,
		ExecutionID: "gap-execution", IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1,
		SubmissionID: submissionID, StreamID: streamID, PromptGeneration: 3,
		OriginalRuntime: processidentity.Identity{PID: 42, BirthToken: "verified-historical-birth"}}
	service.agentManager = manager
	inputs, err := service.loadSilentRestoreInputs(ctx, models.SilentRestoreCandidate{TaskID: session.TaskID, SessionID: session.ID, WorkspaceID: "ws1"})
	require.NoError(t, err)
	require.NotNil(t, inputs, "a proven dead, fully acknowledged pruned stream needs a guarded successor, not manual database editing")
	after, err := repo.GetAgentDeliveryCursor(ctx, streamID)
	require.NoError(t, err)
	require.Equal(t, before, after, "unavailable events must never be fabricated as SQL receipts or projections")
	stored, err := repo.GetAgentDeliverySubmission(ctx, submissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
	require.Equal(t, payload, stored.Payload)
	require.Empty(t, manager.capturedPromptCalls)
	assertPrunedDeliveryResumeUsesGuardedSuccessor(t, service, manager, session)
}

func assertPrunedDeliveryResumeUsesGuardedSuccessor(t *testing.T, service *Service, manager *terminatedRestartJournalManager, session *models.TaskSession) {
	t.Helper()
	service.sessionCeiling = newSessionCeilingController(unlimitedSessionCeiling, nil, nil)
	manager.rowLivenessFn = func(*models.ExecutorRunning) models.ProcessLiveness { return models.ProcessLivenessDead }
	launches := 0
	manager.launchAgentFunc = func(_ context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launches++
		require.Equal(t, "native", request.RequiredNativeConversationID)
		require.Equal(t, "gap-stream", request.InterruptedStreamID)
		require.EqualValues(t, 2, request.DeliveryHarnessGeneration)
		require.Empty(t, request.TaskDescription)
		return nil, errors.New("native restore deliberately interrupted before publication")
	}
	_, err := service.RecoverSessionWithOptions(context.Background(), session.TaskID, session.ID, "resume", RecoverSessionOptions{})
	require.ErrorContains(t, err, "native restore deliberately interrupted")
	require.Equal(t, 1, launches)
	require.Empty(t, manager.capturedPromptCalls)
}
