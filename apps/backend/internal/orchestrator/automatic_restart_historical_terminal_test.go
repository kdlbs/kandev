package orchestrator

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type automaticHistoricalRecoveryManager struct {
	*alreadyResumedRestartManager
	repo        *sqliterepo.Repository
	proof       *models.NativeResumeDeliveryEvidence
	retirements atomic.Int32
}

func (m *automaticHistoricalRecoveryManager) GetNativeResumeDeliveryEvidence(ctx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
	proof := *m.proof
	proof.Submissions = append([]models.NativeResumeSubmissionEvidence(nil), m.proof.Submissions...)
	running, err := m.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	proof.ExecutionID = running.AgentExecutionID
	return &proof, nil
}

func (m *automaticHistoricalRecoveryManager) ReconcileAlreadyResumedDelivery(ctx context.Context, identity lifecycle.AgentDeliveryRecoveryIdentity, nativeSessionID string) (bool, error) {
	restored, err := m.alreadyResumedRestartManager.ReconcileAlreadyResumedDelivery(ctx, identity, nativeSessionID)
	if err == nil && restored {
		m.retirements.Add(1)
	}
	return restored, err
}

func (*automaticHistoricalRecoveryManager) AcknowledgeNativeResumeDelivery(context.Context, string) error {
	return nil
}

func TestAutomaticRestartAfterHistoricalTerminalRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	service, repo, manager, session, current, proofManager := seedAutomaticHistoricalTerminalRecovery(t, ctx)
	service.messageCreator = newServiceBackedMessageCreator(repo)
	service.agentManager = proofManager
	t.Cleanup(service.stopSilentRestoreRecovery)

	service.startSilentRestoreRecovery(ctx)
	require.Eventually(t, func() bool {
		updated, err := repo.GetTaskSession(ctx, session.ID)
		if err != nil {
			return false
		}
		messages, err := repo.ListMessages(ctx, session.ID)
		recovery, exists := models.LoadAgentDeliveryRecovery(updated.Metadata)
		return err == nil && len(messages) == 2 && exists && recovery.Phase == models.AgentDeliveryRecoveryRestored
	}, 5*time.Second, 20*time.Millisecond, "startup should restore the failed session and persist its notice without user focus")
	storedSession, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, storedSession.State)
	require.Equal(t, "upgrade-successor", storedSession.AgentExecutionID)
	require.Nil(t, storedSession.Metadata[models.SessionMetaKeyRecoveryNoticePending])

	stored, err := repo.GetAgentDeliverySubmission(ctx, current.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
	require.Equal(t, current.Payload, stored.Payload, "startup must preserve the unknown prompt without resending it")
	require.Empty(t, manager.capturedPromptCalls)
	require.EqualValues(t, 1, proofManager.retirements.Load())
	block, err := repo.GetSessionRecoveryBlock(ctx, current.BlockID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockResolved, block.State)

	service.startSilentRestoreRecovery(ctx)
	require.Never(t, func() bool {
		return proofManager.retirements.Load() > 1
	}, 250*time.Millisecond, 20*time.Millisecond, "a repeated startup trigger must not restore the same incident twice")
	messages, err := repo.ListMessages(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, messages, 2, "the original user prompt and one status notice should be retained")
	require.Equal(t, current.MessageID, messages[0].ID)
	require.Equal(t, sessionRecoveryNoticeContent, messages[1].Content)

	for _, scenario := range []string{"mixed_inventory", "cancelled", "changed_owner"} {
		t.Run(scenario, func(t *testing.T) {
			assertAutomaticHistoricalRecoveryRemainsBlocked(t, scenario)
		})
	}
}

func assertAutomaticHistoricalRecoveryRemainsBlocked(t *testing.T, scenario string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	service, repo, manager, session, current, proofManager := seedAutomaticHistoricalTerminalRecovery(t, ctx)
	service.messageCreator = newServiceBackedMessageCreator(repo)
	t.Cleanup(service.stopSilentRestoreRecovery)
	switch scenario {
	case "mixed_inventory":
		require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
			ID: "upgrade-independent-block", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
			ExpectedGeneration: 1, Reason: "native_state_missing", State: models.RecoveryBlockOpen,
			ConsumerReference: "native_resume",
		}))
	case "cancelled":
		require.NoError(t, repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateCancelled, ""))
	case "changed_owner":
		_, err := repo.DB().ExecContext(ctx, `UPDATE workspaces SET owner_id = ? WHERE id = ?`, "replacement-owner", "ws1")
		require.NoError(t, err)
		workspace, err := repo.GetWorkspace(ctx, "ws1")
		require.NoError(t, err)
		require.Equal(t, "replacement-owner", workspace.OwnerID)
		candidates, err := repo.ListSilentRestoreCandidates(ctx, "", 10)
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		_, eligible, err := service.loadSilentRestoreOwner(ctx, candidates[0])
		require.Error(t, err)
		require.False(t, eligible)
	default:
		t.Fatalf("unknown guard scenario %q", scenario)
	}

	service.startSilentRestoreRecovery(ctx)
	require.Never(t, func() bool { return proofManager.retirements.Load() > 0 }, 300*time.Millisecond, 20*time.Millisecond,
		"the startup worker must preserve recovery fences when the candidate is ineligible")
	require.Empty(t, manager.capturedPromptCalls)
	require.Zero(t, proofManager.retirements.Load())
	messages, err := repo.ListMessages(ctx, session.ID)
	require.NoError(t, err)
	for _, message := range messages {
		require.NotEqual(t, "session_restored_after_interruption", message.Metadata["kind"])
	}
	blocks, err := repo.ListOpenSessionRecoveryBlocks(ctx, session.ID, session.QueueIncarnationID, 1)
	require.NoError(t, err)
	wantBlocks := 1
	if scenario == "mixed_inventory" {
		wantBlocks = 2
	}
	require.Len(t, blocks, wantBlocks)
	for _, block := range blocks {
		require.Equal(t, models.RecoveryBlockOpen, block.State)
	}
	stored, err := repo.GetAgentDeliverySubmission(ctx, current.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
}

type automaticRecoveryCurrentSource struct {
	ID        string
	MessageID string
	Payload   []byte
	BlockID   string
}

func seedAutomaticHistoricalTerminalRecovery(
	t *testing.T,
	ctx context.Context,
) (*Service, *sqliterepo.Repository, *alreadyResumedRestartManager, *models.TaskSession, automaticRecoveryCurrentSource, *automaticHistoricalRecoveryManager) {
	t.Helper()
	service, repo, base, session, _ := newSilentRestoreEligibilityFixture(t, "provider-session", "upgrade")
	old := models.AgentDeliveryRecovery{
		SessionID: session.ID, AgentExecutionID: "upgrade-execution", IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, SubmissionID: "upgrade-submission", StreamID: "upgrade-stream",
		PromptGeneration: 1, Phase: models.AgentDeliveryRecoveryUncertain,
	}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyAgentDeliveryRecovery, old))
	oldBlock, err := repo.GetOpenSessionRecoveryBlock(ctx, session.ID, session.QueueIncarnationID, old.HarnessGeneration)
	require.NoError(t, err)
	_, err = repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET state = ? WHERE id = ?`, models.DeliverySubmissionDispatching, old.SubmissionID)
	require.NoError(t, err)
	terminalTurn := "upgrade-terminal-turn"
	terminalPayload, err := json.Marshal(map[string]string{"turn_id": terminalTurn})
	require.NoError(t, err)
	event := models.AgentDeliveryEvent{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: old.HarnessGeneration, StreamID: old.StreamID, Sequence: 1,
		SubmissionID: old.SubmissionID, EventType: "error", Payload: terminalPayload, Terminal: true,
		ReceivedAt: time.Now().UTC(),
	}
	inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &event, 1)
	require.NoError(t, err)
	require.True(t, inserted)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, &event, nil)
	require.NoError(t, err)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, event.StreamID, event.Sequence, models.DeliverySubmissionFailed, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, settled)
	completedAt := time.Now().UTC()
	workflowEffect := models.AgentDeliveryEffect{
		EffectKey: "workflow.on_turn_complete:" + terminalTurn, StreamID: event.StreamID,
		Sequence: event.Sequence, EffectType: "workflow.on_turn_complete", State: models.DeliveryEffectCompleted,
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: old.HarnessGeneration, SubmissionID: old.SubmissionID,
		TurnID: terminalTurn, Outcome: "not_required", CreatedAt: completedAt, CompletedAt: &completedAt,
	}
	_, err = repo.PutAgentDeliveryEffect(ctx, &workflowEffect)
	require.NoError(t, err)
	completed, err := repo.CompleteAgentDeliveryTerminalSettlement(ctx,
		"agent_delivery.terminal:"+event.StreamID+":1", completedAt)
	require.NoError(t, err)
	require.True(t, completed)
	old.Phase = models.AgentDeliveryRecoverySettled
	old.Revision++
	old.UpdatedAt = completedAt
	require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyAgentDeliveryRecovery, old))
	oldSubmission, err := repo.GetAgentDeliverySubmission(ctx, old.SubmissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionFailed, oldSubmission.State)
	oldResolved, err := repo.GetSessionRecoveryBlock(ctx, oldBlock.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockResolved, oldResolved.State)
	messageID := "upgrade-current-message"
	turnID := "upgrade-current-turn"
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: turnID, TaskID: session.TaskID, TaskSessionID: session.ID, StartedAt: time.Now().UTC()}))
	payload := []byte(`{"text":"continue in the saved conversation"}`)
	err = repo.CreateMessage(ctx, &models.Message{
		ID: messageID, TaskID: session.TaskID, TaskSessionID: session.ID, TurnID: turnID,
		AuthorType: models.MessageAuthorUser, Content: "continue in the saved conversation",
	})
	require.NoError(t, err)
	currentID := "prompt:" + messageID
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: currentID, SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: old.HarnessGeneration, OwnerGeneration: old.HarnessGeneration,
		DispatchAttemptID: messageID, PayloadHash: journal.SubmissionHash(payload), Payload: payload,
		State: models.DeliverySubmissionInterruptedUnknown, Outcome: unknownPromptOutcome,
	})
	require.NoError(t, err)
	currentBlock := &models.SessionRecoveryBlock{
		ID: "upgrade-current-block", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		ExpectedGeneration: old.HarnessGeneration, Reason: unknownPromptOutcome, State: models.RecoveryBlockOpen,
		ConsumerReference: agentDeliveryConsumer, DeliverySubmissionID: currentID, DeliveryStreamID: old.StreamID,
	}
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, currentBlock))
	historicalVerifier := interface {
		HasResolvedHistoricalTerminalRecovery(context.Context, models.AgentDeliveryRecovery, int64, string) (bool, error)
	}(repo)
	classified, err := historicalVerifier.HasResolvedHistoricalTerminalRecovery(ctx, old, 1, "provider-session")
	require.NoError(t, err)
	require.True(t, classified, "historical source should be fully proven: recovery=%+v block=%+v", old, oldResolved)
	_, err = repo.DB().ExecContext(ctx, `UPDATE executors_running SET agent_execution_id = ?, status = ? WHERE session_id = ?`,
		"upgrade-successor", models.ExecutorRunningStatusRunning, session.ID)
	require.NoError(t, err)

	root := t.TempDir()
	j, err := journal.Open(journal.Config{Path: journal.CheckStorage(root, session.ID).Path})
	require.NoError(t, err)
	_, err = j.PutSubmission(ctx, journal.Submission{
		ID: old.SubmissionID, SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: uint64(old.HarnessGeneration), StreamID: old.StreamID,
		Hash: oldSubmission.PayloadHash, Payload: oldSubmission.Payload, State: journal.SubmissionDispatching,
	})
	require.NoError(t, err)
	_, err = j.Append(ctx, journal.Event{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: uint64(old.HarnessGeneration), StreamID: old.StreamID,
		SubmissionID: old.SubmissionID, Type: "error", Payload: terminalPayload, Terminal: true,
	})
	require.NoError(t, err)
	_, err = j.PutSubmission(ctx, journal.Submission{
		ID: currentID, SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: uint64(old.HarnessGeneration), StreamID: old.StreamID,
		Hash: journal.SubmissionHash(payload), Payload: payload, State: journal.SubmissionInterruptedUnknown,
	})
	require.NoError(t, err)
	_, err = j.RetireSubmission(ctx, currentID, uint64(old.HarnessGeneration))
	require.NoError(t, err)
	require.NoError(t, j.Close())

	identity := processidentity.Identity{PID: 42, BirthToken: "upgrade-successor-owner"}
	baseManager := &retainedJournalRecoveryManager{mockAgentManager: base, root: root, terminated: true,
		controlIdentity: &lifecycle.AgentDeliveryRecoveryIdentity{
			TaskID: session.TaskID, SessionID: session.ID, ExecutionID: "upgrade-successor",
			IncarnationID: session.QueueIncarnationID, HarnessGeneration: uint64(old.HarnessGeneration),
			SubmissionID: currentID, StreamID: currentBlock.DeliveryStreamID, PromptGeneration: 2,
			OriginalRuntime: identity,
		},
	}
	journalManager := &alreadyResumedRestartManager{terminatedRestartJournalManager: &terminatedRestartJournalManager{baseManager}}
	proof := &models.NativeResumeDeliveryEvidence{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: old.HarnessGeneration,
		NativeSessionID: "provider-session", ExecutionID: "upgrade-successor", StreamID: old.StreamID,
		Submissions: []models.NativeResumeSubmissionEvidence{
			{ID: old.SubmissionID, HarnessGeneration: old.HarnessGeneration, PayloadHash: oldSubmission.PayloadHash,
				State: oldSubmission.State, Outcome: oldSubmission.Outcome, StreamID: oldSubmission.TerminalStreamID,
				TerminalStreamID: oldSubmission.TerminalStreamID, TerminalSequence: oldSubmission.TerminalSequence},
			{ID: currentID, HarnessGeneration: old.HarnessGeneration, PayloadHash: journal.SubmissionHash(payload),
				State: models.DeliverySubmissionInterruptedUnknown, StreamID: old.StreamID},
		},
	}
	proof.Cursor, err = repo.GetAgentDeliveryCursor(ctx, old.StreamID)
	require.NoError(t, err)
	proofManager := &automaticHistoricalRecoveryManager{alreadyResumedRestartManager: journalManager, repo: repo, proof: proof}
	service.agentManager = proofManager
	storedEvidence, err := proofManager.InspectDeliveryRecordEvidence(ctx, lifecycle.DeliveryRecordEvidenceRequest{
		TaskID: session.TaskID, SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: uint64(old.HarnessGeneration), StoredSubmissionID: currentID,
	})
	require.NoError(t, err)
	require.NotNil(t, storedEvidence)
	require.NotNil(t, storedEvidence.StoredSubmission)
	require.True(t, storedEvidence.StoredSubmission.Retired)
	require.True(t, journal.HasUnprovenOutcome(*storedEvidence.StoredSubmission))
	generation, err := repo.GetCurrentHarnessSessionGeneration(ctx, session.ID, session.QueueIncarnationID)
	require.NoError(t, err)
	hasLaterSubmission := service.hasNewRestartJournalSubmission(ctx, session, generation, old.SubmissionID)
	require.True(t, hasLaterSubmission, "retired current submission must survive in stored evidence")
	return service, repo, journalManager, session,
		automaticRecoveryCurrentSource{ID: currentID, MessageID: messageID, Payload: payload, BlockID: currentBlock.ID}, proofManager
}
