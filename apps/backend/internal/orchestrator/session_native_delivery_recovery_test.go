package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
func TestRecoverSessionUnknownPromptKeepsNativeHistory(t *testing.T) {
	for _, scenario := range []struct {
		name                            string
		reason                          string
		failLaunch, failAcknowledgement bool
	}{
		{"resume", "unknown_prompt_outcome", false, false},
		{"launch failure", "unknown_prompt_outcome", true, false},
		{"acknowledgement failure", "unknown_prompt_outcome", false, true},
		{"retained journal recovery", durableDeliveryUnresolvedReason, false, false},
		{"retained journal failure", durableDeliveryUnresolvedReason, false, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			service, manager, block, submission, launched, started := nativeResumeFixture(t, scenario.reason, scenario.failLaunch, scenario.failAcknowledgement)
			sessionID, taskID := submission.SessionID, "task1"
			repo := service.repo.(nativeResumeTestRepository)
			response, err := service.RecoverSession(ctx, taskID, sessionID, "resume")
			if scenario.failLaunch || scenario.failAcknowledgement {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.True(t, response.Success)
				require.True(t, *started)
			}
			require.NotNil(t, *launched)
			require.Equal(t, "native-conversation", (*launched).ACPSessionID)
			require.Empty(t, manager.capturedPromptCalls, "resume must not replay the interrupted prompt")
			stored, err := repo.GetAgentDeliverySubmission(ctx, submission.ID)
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
			require.Equal(t, submission.Payload, stored.Payload)
			recovery, err := repo.GetSessionRecoveryBlock(ctx, block.ID)
			require.NoError(t, err)
			if scenario.failLaunch || scenario.failAcknowledgement {
				require.Equal(t, models.RecoveryBlockOpen, recovery.State)
			} else {
				require.Equal(t, models.RecoveryBlockResolved, recovery.State)
				require.Equal(t, "resume", recovery.AuthorizedAction)
			}
		})
	}
}

// The optional capability is present on the production lifecycle adapter.
type nativeResumeTestManager struct {
	*sessionUpdatingAgentManager
	acknowledge func(context.Context, string) error
}

func (m *nativeResumeTestManager) AcknowledgeNativeResumeDelivery(ctx context.Context, sessionID string) error {
	return m.acknowledge(ctx, sessionID)
}

type nativeResumeTestRepository interface {
	GetAgentDeliverySubmission(context.Context, string) (*models.AgentDeliverySubmission, error)
	GetSessionRecoveryBlock(context.Context, string) (*models.SessionRecoveryBlock, error)
	PrepareAgentDeliverySubmission(context.Context, *models.AgentDeliverySubmission) (bool, error)
}

func nativeResumeFixture(t *testing.T, reason string, failLaunch, failAcknowledgement bool) (*Service, *nativeResumeTestManager, *models.SessionRecoveryBlock, *models.AgentDeliverySubmission, **executor.LaunchAgentRequest, *bool) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)
	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	session.AgentProfileID = "profile1"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	now := time.Now().UTC()
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running1", SessionID: session.ID, TaskID: session.TaskID,
		AgentExecutionID: "old-execution", ResumeToken: "native-conversation", Resumable: true,
		CreatedAt: now, UpdatedAt: now,
	}))
	submission := &models.AgentDeliverySubmission{
		ID: "prompt:interrupted", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		PayloadHash: "original-hash", Payload: []byte("original prompt"), State: models.DeliverySubmissionInterruptedUnknown,
		Outcome: "prompt_dispatch_failed",
	}
	_, err = repo.PrepareAgentDeliverySubmission(ctx, submission)
	require.NoError(t, err)
	block := &models.SessionRecoveryBlock{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		Reason: reason, State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: submission.ID,
	}
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, block))
	var launched *executor.LaunchAgentRequest
	started := false
	manager := &sessionUpdatingAgentManager{
		mockAgentManager: &mockAgentManager{
			repoForExecutionLookup: repo,
			launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				launched = req
				if failLaunch {
					return nil, errors.New("runtime unavailable")
				}
				return &executor.LaunchAgentResponse{AgentExecutionID: "new-execution", Status: v1.AgentStatusStarting}, nil
			},
		},
		repo: repo, sessionID: session.ID, taskID: session.TaskID, onStartCalled: &started,
	}
	resumer := &nativeResumeTestManager{sessionUpdatingAgentManager: manager, acknowledge: func(ctx context.Context, sessionID string) error {
		open, err := repo.GetSessionRecoveryBlock(ctx, block.ID)
		require.NoError(t, err)
		require.Equal(t, models.RecoveryBlockOpen, open.State, "journal must be acknowledged before SQL resolution")
		if failAcknowledgement {
			return errors.New("journal unavailable")
		}
		return nil
	}}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), resumer)
	service.executor = executor.NewExecutor(resumer, repo, testLogger(), executor.ExecutorConfig{})
	return service, resumer, block, submission, &launched, &started
}

func TestNativeResumeRefusesUnresolvedLiveBackendWork(t *testing.T) {
	for _, state := range []models.DeliverySubmissionState{models.DeliverySubmissionPrepared, models.DeliverySubmissionAccepted, models.DeliverySubmissionDispatching} {
		t.Run(string(state), func(t *testing.T) {
			service, _, block, submission, launched, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
			repo := service.repo.(nativeResumeTestRepository)
			live := *submission
			live.ID, live.State = "prompt:live", state
			_, err := repo.PrepareAgentDeliverySubmission(context.Background(), &live)
			require.NoError(t, err)
			_, err = service.RecoverSession(context.Background(), "task1", submission.SessionID, "resume")
			var blocked *sessionRecoveryRequiredError
			require.ErrorAs(t, err, &blocked)
			require.Nil(t, *launched)
			stored, err := repo.GetSessionRecoveryBlock(context.Background(), block.ID)
			require.NoError(t, err)
			require.Equal(t, models.RecoveryBlockOpen, stored.State)
		})
	}
}

func TestNativeResumeKeepsOfficeDurableRecoveryWithScheduler(t *testing.T) {
	ctx := context.Background()
	service, _, block, submission, launched, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
	task, err := service.repo.GetTask(ctx, "task1")
	require.NoError(t, err)
	task.ProjectID = "office-project"
	require.NoError(t, service.repo.UpdateTask(ctx, task))
	_, err = service.RecoverSession(ctx, task.ID, submission.SessionID, "resume")
	var blocked *sessionRecoveryRequiredError
	require.ErrorAs(t, err, &blocked)
	require.Nil(t, *launched)
	stored, err := service.repo.(nativeResumeTestRepository).GetSessionRecoveryBlock(ctx, block.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockOpen, stored.State)
}
