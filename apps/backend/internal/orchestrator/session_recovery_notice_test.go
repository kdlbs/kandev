package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type failOnceRecoveryNoticeCreator struct {
	MessageCreator
	fail bool
}

func (m *failOnceRecoveryNoticeCreator) CreateSessionMessageIdempotent(
	ctx context.Context,
	messageID, taskID, content, sessionID, messageType, turnID string,
	metadata map[string]interface{},
	requestsInput bool,
) error {
	if m.fail {
		m.fail = false
		return errors.New("message store unavailable")
	}
	return m.MessageCreator.CreateSessionMessageIdempotent(
		ctx, messageID, taskID, content, sessionID, messageType, turnID, metadata, requestsInput,
	)
}

func TestRecoveryNoticePublicationIdempotent(t *testing.T) {
	ctx := context.Background()
	service, manager, block, submission, _, _ := nativeResumeFixture(t, "unknown_prompt_outcome", false, false)
	repo := service.repo.(*sqliterepo.Repository)
	task, err := repo.GetTask(ctx, "task1")
	require.NoError(t, err)
	task.WorkspaceID = "ws1"
	require.NoError(t, repo.UpdateTask(ctx, task))
	service.SetStartupRecoveryOwnerContextResolver(func(ownerCtx context.Context, _, _ string) (context.Context, error) {
		return ownerCtx, nil
	})
	realCreator := newServiceBackedMessageCreator(repo)
	service.messageCreator = &failOnceRecoveryNoticeCreator{MessageCreator: realCreator, fail: true}
	manager.acknowledge = func(ctx context.Context, _ string) error {
		current, err := repo.GetSessionRecoveryBlock(ctx, block.ID)
		if err != nil {
			return err
		}
		if current.State != models.RecoveryBlockOpen && current.State != models.RecoveryBlockResolved {
			t.Fatalf("unexpected recovery block state: %s", current.State)
		}
		return nil
	}
	service.agentManager = &nativeProofTestManager{nativeResumeTestManager: manager, proof: func(ctx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
		running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		return &models.NativeResumeDeliveryEvidence{
			SessionID: sessionID, IncarnationID: submission.IncarnationID,
			HarnessGeneration: submission.HarnessGeneration, NativeSessionID: "native-conversation",
			ExecutionID: running.AgentExecutionID, StreamID: block.DeliveryStreamID,
			Submissions: []models.NativeResumeSubmissionEvidence{{
				ID: submission.ID, HarnessGeneration: submission.HarnessGeneration,
				PayloadHash: submission.PayloadHash, StreamID: block.DeliveryStreamID,
			}},
		}, nil
	}}

	response, err := service.RecoverSession(ctx, "task1", submission.SessionID, "resume")
	require.ErrorContains(t, err, "message store unavailable")
	require.Nil(t, response)
	resolved, err := repo.GetSessionRecoveryBlock(ctx, block.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockResolved, resolved.State, "recovery commits before message publication")
	current, err := repo.GetTaskSession(ctx, submission.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryNoticeSourceIdentity(models.AgentDeliveryRecovery{
		SessionID: current.ID, IncarnationID: submission.IncarnationID,
		HarnessGeneration: submission.HarnessGeneration, SubmissionID: submission.ID,
	}), current.Metadata[models.SessionMetaKeyRecoveryNoticePending])
	_, err = repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET state = 'failed', outcome = 'failed' WHERE id = ?`, submission.ID)
	require.NoError(t, err, "the durable source may settle after restoration commits but before notice publication")

	service.messageCreator = realCreator
	candidates, err := repo.ListSilentRestoreCandidates(ctx, "", 100)
	require.NoError(t, err)
	require.Len(t, candidates, 1, "startup inventory must find a notice left pending after a crash")
	require.Equal(t, submission.SessionID, candidates[0].SessionID)
	require.Equal(t, "task1", candidates[0].TaskID)
	require.NotEmpty(t, candidates[0].WorkspaceID)
	require.NoError(t, service.restoreSilentRestartCandidate(ctx, repo, candidates[0]))
	require.NoError(t, service.restoreSilentRestartCandidate(ctx, repo, candidates[0]), "retrying the same incident is idempotent")

	messages, err := repo.ListMessages(ctx, submission.SessionID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "The session was interrupted. Your conversation is restored. You can continue.", messages[0].Content)
	require.Equal(t, models.MessageTypeStatus, messages[0].Type)
	require.Equal(t, "session_restored_after_interruption", messages[0].Metadata["kind"])
	require.Equal(t, submission.ID, messages[0].Metadata["source_submission_id"])
	require.Equal(t, float64(submission.HarnessGeneration), messages[0].Metadata["source_harness_generation"])
	require.False(t, messages[0].RequestsInput)
	require.Empty(t, manager.capturedPromptCalls)

	firstNoticeID := messages[0].ID
	secondPayload := []byte(`{"text":"a later independent instruction"}`)
	secondSubmission := &models.AgentDeliverySubmission{
		ID: "prompt:later-independent-instruction", SessionID: submission.SessionID,
		IncarnationID: submission.IncarnationID, HarnessGeneration: submission.HarnessGeneration,
		OwnerGeneration: submission.OwnerGeneration, DispatchAttemptID: "later-independent-instruction",
		PayloadHash: journal.SubmissionHash(secondPayload), Payload: secondPayload,
		State: models.DeliverySubmissionInterruptedUnknown, Outcome: unknownPromptOutcome,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	_, err = repo.PrepareAgentDeliverySubmission(ctx, secondSubmission)
	require.NoError(t, err)
	secondBlock := &models.SessionRecoveryBlock{
		SessionID: submission.SessionID, IncarnationID: submission.IncarnationID,
		Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: secondSubmission.ID,
		ExpectedGeneration: secondSubmission.HarnessGeneration, DeliveryStreamID: block.DeliveryStreamID,
	}
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, secondBlock))
	storedLaterSubmission, err := repo.GetAgentDeliverySubmission(ctx, secondSubmission.ID)
	require.NoError(t, err)
	running, err := repo.GetExecutorRunningBySessionID(ctx, submission.SessionID)
	require.NoError(t, err)
	proof := &models.NativeResumeDeliveryEvidence{
		SessionID: submission.SessionID, IncarnationID: submission.IncarnationID,
		HarnessGeneration: submission.HarnessGeneration, NativeSessionID: "native-conversation",
		ExecutionID: running.AgentExecutionID, StreamID: block.DeliveryStreamID,
		Submissions: []models.NativeResumeSubmissionEvidence{{
			ID: storedLaterSubmission.ID, State: storedLaterSubmission.State,
			Outcome: storedLaterSubmission.Outcome, HarnessGeneration: storedLaterSubmission.HarnessGeneration,
			PayloadHash: storedLaterSubmission.PayloadHash, StreamID: block.DeliveryStreamID,
		}},
	}
	manager.acknowledge = func(ctx context.Context, _ string) error {
		current, err := repo.GetSessionRecoveryBlock(ctx, secondBlock.ID)
		if err != nil {
			return err
		}
		if current.State != models.RecoveryBlockOpen && current.State != models.RecoveryBlockResolved {
			t.Fatalf("unexpected later recovery block state: %s", current.State)
		}
		return nil
	}
	service.agentManager = &nativeProofTestManager{
		nativeResumeTestManager: manager,
		proof: func(proofCtx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
			running, err := repo.GetExecutorRunningBySessionID(proofCtx, sessionID)
			if err != nil {
				return nil, err
			}
			proof.ExecutionID = running.AgentExecutionID
			return proof, nil
		},
	}
	service.messageCreator = &failOnceRecoveryNoticeCreator{MessageCreator: realCreator, fail: true}
	response, err = service.RecoverSession(ctx, "task1", submission.SessionID, "resume")
	require.ErrorContains(t, err, "message store unavailable")
	require.Nil(t, response)
	current, err = repo.GetTaskSession(ctx, submission.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryNoticeSourceIdentity(models.AgentDeliveryRecovery{
		SessionID: submission.SessionID, IncarnationID: submission.IncarnationID,
		HarnessGeneration: secondSubmission.HarnessGeneration, SubmissionID: secondSubmission.ID,
	}), current.Metadata[models.SessionMetaKeyRecoveryNoticePending], "Resume must persist the later incident's notice marker")
	currentRecovery, exists := models.LoadAgentDeliveryRecovery(current.Metadata)
	require.True(t, exists)
	require.Equal(t, submission.ID, currentRecovery.SubmissionID, "the earlier restored snapshot remains historical metadata")

	service.messageCreator = realCreator
	require.NoError(t, service.publishPendingRecoveryNotice(ctx, submission.SessionID))
	require.NoError(t, service.publishPendingRecoveryNotice(ctx, submission.SessionID), "publishing the later source twice is idempotent")
	messages, err = repo.ListMessages(ctx, submission.SessionID)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.NotEqual(t, firstNoticeID, messages[1].ID, "a distinct source submission must get a distinct notice ID")
	require.Equal(t, secondSubmission.ID, messages[1].Metadata["source_submission_id"])

	t.Run("ordinary interrupted restart without older terminal history", func(t *testing.T) {
		ordinary, ordinaryManager, block, source, _, _ := nativeResumeFixture(t, "unknown_prompt_outcome", false, false)
		ordinaryRepo := ordinary.repo.(*sqliterepo.Repository)
		ordinary.messageCreator = newServiceBackedMessageCreator(ordinaryRepo)
		ordinary.agentManager = &nativeProofTestManager{nativeResumeTestManager: ordinaryManager, proof: func(proofCtx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
			running, err := ordinaryRepo.GetExecutorRunningBySessionID(proofCtx, sessionID)
			if err != nil {
				return nil, err
			}
			return &models.NativeResumeDeliveryEvidence{
				SessionID: sessionID, IncarnationID: source.IncarnationID,
				HarnessGeneration: source.HarnessGeneration, NativeSessionID: "native-conversation",
				ExecutionID: running.AgentExecutionID, StreamID: block.DeliveryStreamID,
				Submissions: []models.NativeResumeSubmissionEvidence{{
					ID: source.ID, HarnessGeneration: source.HarnessGeneration,
					PayloadHash: source.PayloadHash, StreamID: block.DeliveryStreamID,
				}},
			}, nil
		}}
		response, err := ordinary.RecoverSession(ctx, "task1", source.SessionID, "resume")
		require.NoError(t, err)
		require.True(t, response.Success)
		messages, err := ordinaryRepo.ListMessages(ctx, source.SessionID)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		require.Equal(t, sessionRecoveryNoticeContent, messages[0].Content)
		require.Equal(t, source.ID, messages[0].Metadata["source_submission_id"])
		require.Empty(t, ordinaryManager.capturedPromptCalls)
	})
}

func TestRecoveryNoticePublicationAcceptsAuthenticatedUnprovenCompletion(t *testing.T) {
	ctx := context.Background()
	service, manager, block, submission, _, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
	repo := service.repo.(*sqliterepo.Repository)
	submission = markUnprovenCompletion(t, repo, submission.ID)
	cursor := seedUnprovenCursor(t, repo, submission, block.DeliveryStreamID)
	proof := unprovenProof(submission, cursor)
	service.agentManager = &nativeProofTestManager{
		nativeResumeTestManager: manager,
		proof: func(context.Context, string) (*models.NativeResumeDeliveryEvidence, error) {
			return proof, nil
		},
	}
	service.messageCreator = newServiceBackedMessageCreator(repo)

	response, err := service.RecoverSession(ctx, "task1", submission.SessionID, "resume")
	require.NoError(t, err, "the notice must publish after the authenticated recovery commits")
	require.True(t, response.Success)
	stored, err := repo.GetAgentDeliverySubmission(ctx, submission.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionCompleted, stored.State, "recovery preserves the historical completed label")
	messages, err := repo.ListMessages(ctx, submission.SessionID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, submission.ID, messages[0].Metadata["source_submission_id"])
	require.Nil(t, manager.capturedPromptCalls, "recovery must not replay the old prompt")
}
