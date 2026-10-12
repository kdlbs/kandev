package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type recordingInterruptedOwnerManager struct {
	*mockAgentManager
	recoveryMu sync.Mutex
	identities []lifecycle.AgentDeliveryRecoveryIdentity
}

func (m *recordingInterruptedOwnerManager) RecoverAgentPromptStreamWithIdentity(
	_ context.Context,
	identity lifecycle.AgentDeliveryRecoveryIdentity,
) lifecycle.DeliveryReconciliationResult {
	m.recoveryMu.Lock()
	m.identities = append(m.identities, identity)
	m.recoveryMu.Unlock()
	return lifecycle.DeliveryReconciliationResult{
		Outcome:           lifecycle.DeliveryReconciliationUncertain,
		Reason:            "terminal_outcome_missing",
		ProcessTerminated: true,
	}
}

func TestRestartRecoveryRestoresNativeConversationWithoutPrompt(t *testing.T) {
	ctx := context.Background()
	base, _, _ := continuationFailureFixture(t)
	repo := base.repo.(*sqliterepo.Repository)
	stepGetter, ok := base.workflowStepGetter.(*mockStepGetter)
	require.True(t, ok)
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
	}
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateFailed
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	incarnationID := session.QueueIncarnationID
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: incarnationID, Generation: 1,
		NativeSessionID: "provider-session", CreationReason: "initial",
	}))
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: "restart-submission", SessionID: session.ID, IncarnationID: incarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "restart-payload-hash",
		Payload: []byte("original prompt"), State: models.DeliverySubmissionInterruptedUnknown,
	})
	require.NoError(t, err)
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "source-execution", TaskID: session.TaskID, SessionID: session.ID,
		AgentExecutionID: "old-execution", Status: "running",
	}))
	recovery := models.AgentDeliveryRecovery{
		SessionID: session.ID, AgentExecutionID: "old-execution", IncarnationID: incarnationID,
		HarnessGeneration: 1, SubmissionID: "restart-submission", StreamID: "old-stream",
		PromptGeneration: 7, Phase: models.AgentDeliveryRecoveryUncertain,
	}
	block := &models.SessionRecoveryBlock{
		SessionID: session.ID, IncarnationID: incarnationID, ExpectedGeneration: 1,
		Reason: "unknown_prompt_outcome", ConsumerReference: agentDeliveryConsumer,
		DeliverySubmissionID: recovery.SubmissionID, DeliveryStreamID: recovery.StreamID,
		State: models.RecoveryBlockOpen,
	}
	opened, err := repo.UpsertAgentDeliveryRecovery(ctx, &recovery, block)
	require.NoError(t, err)
	require.True(t, opened)
	queue := messagequeue.NewService(newAuthoritativeMemoryRepository(repo), messagequeue.DefaultMaxPerSession, testLogger())
	queued, err := queue.QueueMessage(ctx, session.ID, session.TaskID, "queued before restart", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	require.NotNil(t, queued)

	manager := &recordingInterruptedOwnerManager{mockAgentManager: &mockAgentManager{repoForExecutionLookup: repo}}
	manager.isAgentReadyFn = func(context.Context, string) bool {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		return manager.isAgentRunning
	}
	manager.getACPSessionIDForSessionFunc = func(string) (string, bool) {
		return "provider-session", true
	}
	var candidatesMu sync.Mutex
	var candidateExecutionIDs []string
	var launchedSessions []string
	var lastCandidateExecutionID string
	var lastCandidateAttemptID string
	manager.rowLivenessFn = func(running *models.ExecutorRunning) models.ProcessLiveness {
		if running.AgentExecutionID == "old-execution" {
			manager.mu.Lock()
			manager.isAgentRunning = false
			manager.mu.Unlock()
			return models.ProcessLivenessDead
		}
		candidatesMu.Lock()
		firstCandidateID := ""
		if len(candidateExecutionIDs) > 0 {
			firstCandidateID = candidateExecutionIDs[0]
		}
		candidatesMu.Unlock()
		if running.AgentExecutionID == firstCandidateID {
			manager.mu.Lock()
			manager.isAgentRunning = false
			manager.mu.Unlock()
			return models.ProcessLivenessDead
		}
		return models.ProcessLivenessAlive
	}
	var service *Service
	manager.launchAgentFunc = func(launchCtx context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		require.Equal(t, "provider-session", request.RequiredNativeConversationID)
		require.Equal(t, "provider-session", request.ACPSessionID)
		require.Equal(t, session.ID, request.SessionID)
		require.Empty(t, request.TaskDescription, "silent recovery must not create a replacement instruction")
		require.Empty(t, request.InitialDeliverySubmissionID)
		require.EqualValues(t, 2, request.DeliveryHarnessGeneration)
		require.NotEmpty(t, request.CandidateExecutionID)
		require.NotNil(t, request.OnExecutionAllocated)
		require.NoError(t, request.OnExecutionAllocated(launchCtx, request.CandidateExecutionID))
		candidatesMu.Lock()
		candidateExecutionIDs = append(candidateExecutionIDs, request.CandidateExecutionID)
		launchedSessions = append(launchedSessions, request.SessionID)
		candidatesMu.Unlock()
		require.NoError(t, repo.UpsertExecutorRunning(launchCtx, &models.ExecutorRunning{
			ID: request.CandidateExecutionID, TaskID: session.TaskID, SessionID: session.ID,
			AgentExecutionID: request.CandidateExecutionID, ResumeToken: "provider-session", Resumable: true, Status: "ready",
		}))
		return &executor.LaunchAgentResponse{AgentExecutionID: request.CandidateExecutionID}, nil
	}
	manager.startAgentProcessFunc = func(ctx context.Context, executionID string) error {
		attemptID := executor.ResumeAttemptIDFromContext(ctx)
		require.NotEmpty(t, attemptID, "recovery lifecycle events must retain the immutable launch attempt")
		manager.mu.Lock()
		manager.isAgentRunning = true
		manager.mu.Unlock()
		service.handleAgentBootReady(ctx, watcher.AgentEventData{
			TaskID: session.TaskID, SessionID: session.ID,
			AgentExecutionID: executionID, AttemptID: attemptID,
		})
		stored, readErr := repo.GetTaskSession(ctx, session.ID)
		if readErr != nil {
			return readErr
		}
		if stored.State != models.TaskSessionStateWaitingForInput {
			return errors.New("boot-ready handler did not move the restored session to waiting")
		}
		candidatesMu.Lock()
		lastCandidateExecutionID = executionID
		lastCandidateAttemptID = attemptID
		candidatesMu.Unlock()
		return nil
	}

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, session.TaskID, v1.TaskStateInProgress)
	service = NewService(DefaultServiceConfig(), bus.NewMemoryEventBus(testLogger()), manager,
		taskRepo, repo, nil, nil, nil, testLogger())
	service.workflowStepGetter = stepGetter
	service.messageQueue = queue
	service.SetTurnService(&repoTurnService{repo: repo})
	service.SetMessageCreator(&repositoryBackedMessageCreator{mockMessageCreator: &mockMessageCreator{}, repo: repo})
	service.SetStartupRecoveryOwnerContextResolver(func(ownerCtx context.Context, ownerID, orgID string) (context.Context, error) {
		if ownerID != "" || orgID != "" {
			return nil, errors.New("unexpected startup recovery owner")
		}
		return ownerCtx, nil
	})
	seedBlockedSilentRestoreCandidate(t, ctx, repo)
	require.NoError(t, service.Start(ctx))
	t.Cleanup(func() {
		if service.running {
			if stopErr := service.Stop(); stopErr != nil {
				t.Errorf("stop orchestrator: %v", stopErr)
			}
		}
	})

	waitCtx, cancelWait := context.WithTimeout(ctx, 20*time.Second)
	defer cancelWait()
	deadline := time.NewTicker(20 * time.Millisecond)
	defer deadline.Stop()
	var current *models.HarnessSessionGeneration
	for {
		current, err = repo.GetCurrentHarnessSessionGeneration(waitCtx, session.ID, incarnationID)
		if err == nil && current != nil && current.Generation == 2 {
			break
		}
		select {
		case <-waitCtx.Done():
			t.Fatalf("startup did not restore the interrupted native conversation: %v", waitCtx.Err())
		case <-deadline.C:
		}
	}
	require.Equal(t, "provider-session", current.NativeSessionID)
	stored, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, stored.State)
	messages, err := repo.ListMessages(ctx, session.ID)
	require.NoError(t, err)
	require.Empty(t, messages, "startup recovery must not write a user message")
	turns, err := repo.ListTurnsBySession(ctx, session.ID)
	require.NoError(t, err)
	require.Empty(t, turns, "startup recovery must not create a turn")
	submission, err := repo.GetAgentDeliverySubmission(ctx, recovery.SubmissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, submission.State)
	manager.mu.Lock()
	require.Empty(t, manager.capturedPrompts, "startup recovery must not dispatch a prompt")
	manager.mu.Unlock()
	manager.recoveryMu.Lock()
	require.Len(t, manager.identities, 1, "startup must reconcile the interrupted durable owner")
	manager.recoveryMu.Unlock()
	candidatesMu.Lock()
	require.Equal(t, []string{session.ID, session.ID}, launchedSessions,
		"the earlier foreign-owner candidate must not block either candidate attempt for the eligible session")
	require.Len(t, candidateExecutionIDs, 2, "a dead candidate must be rotated after its ready callback releases lifecycle locks")
	require.NotEqual(t, candidateExecutionIDs[0], candidateExecutionIDs[1], "each native process attempt needs a fresh execution identity")
	candidatesMu.Unlock()
	queueStatus := queue.GetStatus(ctx, session.ID)
	require.False(t, queueStatus.AutoRun, "recovery must visibly pause auto-run while preserving queued work")
	queuedCount := queue.GetStatus(ctx, session.ID).Count
	require.Equal(t, 1, queuedCount)

	_, err = service.PromptTask(ctx, session.TaskID, session.ID, "ordinary follow-up", "", false, nil, false)
	require.NoError(t, err, "the restored conversation must accept a later ordinary message")
	manager.mu.Lock()
	require.Equal(t, []string{"ordinary follow-up"}, manager.capturedPrompts)
	manager.mu.Unlock()
	candidatesMu.Lock()
	completedExecutionID := lastCandidateExecutionID
	completedAttemptID := lastCandidateAttemptID
	candidatesMu.Unlock()
	require.NotEmpty(t, completedExecutionID)
	require.NotEmpty(t, completedAttemptID)
	preReadySession, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	activeTurnID, turnErr := service.peekActiveTurnID(ctx, session.ID)
	require.NoError(t, turnErr)
	require.True(t, service.resumeAttemptAllowsExecution(session.ID, completedExecutionID, completedAttemptID),
		"the successful candidate's immutable attempt must own normal lifecycle callbacks")
	require.NotEmpty(t, activeTurnID, "ordinary follow-up must have an active turn to settle")
	service.handleAgentReady(ctx, watcher.AgentEventData{
		TaskID: session.TaskID, SessionID: session.ID, AgentExecutionID: completedExecutionID,
		AttemptID: completedAttemptID,
	})
	service.handleCompleteStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: session.TaskID, SessionID: session.ID, ExecutionID: completedExecutionID,
		AttemptID: completedAttemptID,
		Data:      &lifecycle.AgentStreamEventData{Type: agentEventComplete},
	})
	settled, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, preReadySession.State)
	require.Equal(t, models.TaskSessionStateWaitingForInput, settled.State,
		"the normal completion event must make the session available for queued work")
	queuedCount = queue.GetStatus(ctx, session.ID).Count
	require.Equal(t, 1, queuedCount, "ordinary chat must leave the preserved queue entry intact")
	autoRun, dispatched, err := service.SetQueueAutoRun(ctx, session.ID, true)
	require.NoError(t, err)
	require.True(t, autoRun)
	require.True(t, dispatched, "the user can re-enable Auto-run and dispatch the preserved item")
	queuedCount = queue.GetStatus(ctx, session.ID).Count
	require.Zero(t, queuedCount)
	require.Eventually(t, func() bool {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		return len(manager.capturedPrompts) == 2
	}, 2*time.Second, 10*time.Millisecond, "the re-enabled queue prompt should reach the restored native conversation")
	manager.mu.Lock()
	require.Equal(t, []string{"ordinary follow-up", "queued before restart"}, manager.capturedPrompts)
	manager.mu.Unlock()
}

func TestContinuedRecoveryDoesNotRetryPreviousOwner(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-continued-owner", "session-continued-owner", "step-1")
	session, err := repo.GetTaskSession(ctx, "session-continued-owner")
	require.NoError(t, err)

	incarnationID := session.QueueIncarnationID
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: incarnationID, Generation: 1,
		NativeSessionID: "native-continued-owner", CreationReason: "initial",
	}))
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: "submission-continued-owner", SessionID: session.ID, IncarnationID: incarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "payload-hash",
		Payload: []byte("original prompt"), State: models.DeliverySubmissionInterruptedUnknown,
	})
	require.NoError(t, err)
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "source-execution", TaskID: session.TaskID, SessionID: session.ID,
		AgentExecutionID: "old-execution", Status: "running",
	}))
	recovery := models.AgentDeliveryRecovery{
		SessionID: session.ID, AgentExecutionID: "old-execution", IncarnationID: incarnationID,
		HarnessGeneration: 1, SubmissionID: "submission-continued-owner", StreamID: "old-stream",
		PromptGeneration: 1, Phase: models.AgentDeliveryRecoveryUncertain,
	}
	block := &models.SessionRecoveryBlock{
		SessionID: session.ID, IncarnationID: incarnationID, ExpectedGeneration: 1,
		Reason: "unknown_prompt_outcome", ConsumerReference: "agent_delivery",
		DeliverySubmissionID: recovery.SubmissionID, DeliveryStreamID: recovery.StreamID,
		State: models.RecoveryBlockOpen,
	}
	opened, err := repo.UpsertAgentDeliveryRecovery(ctx, &recovery, block)
	require.NoError(t, err)
	require.True(t, opened)
	block, err = repo.GetOpenSessionRecoveryBlock(ctx, session.ID, incarnationID, 1)
	require.NoError(t, err)
	require.NotNil(t, block)
	require.NoError(t, repo.CreateRestoreAttempt(ctx, &models.RestoreAttempt{
		ID: "legacy-attempt", SessionID: session.ID, IncarnationID: incarnationID,
		ExpectedGeneration: 1, Action: "resume_interrupted", Authorized: true, TargetWorkspace: "ws1",
	}))
	require.NoError(t, repo.CreateContinuationSnapshot(ctx, &models.ContinuationSnapshot{
		ID: "legacy-snapshot", AttemptID: "legacy-attempt", SessionID: session.ID,
		TargetGeneration: 2, SubmissionID: "prompt:legacy-snapshot", Content: "legacy instruction",
		ContentHash: "legacy-hash", Status: models.ContinuitySnapshotPrepared,
	}))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "candidate-row", TaskID: session.TaskID, SessionID: session.ID,
		AgentExecutionID: "candidate-execution", Status: "running",
	}))
	changed, err := repo.CommitInterruptedContinuation(ctx, &models.InterruptedContinuationCommit{
		Recovery: recovery,
		Generation: models.HarnessSessionGeneration{
			SessionID: session.ID, IncarnationID: incarnationID,
			Generation: 2, PredecessorGeneration: 1,
			NativeSessionID: "native-continued-owner", CreationReason: "interrupted_continued",
			CreatedAt: time.Now().UTC(), CommittedAt: time.Now().UTC(),
		},
		CandidateExecutionID: "candidate-execution", BlockID: block.ID,
		SnapshotID: "legacy-snapshot", ContentHash: "legacy-hash",
	})
	require.NoError(t, err)
	require.True(t, changed, "legacy successor generation must be committed before testing stale-owner retry")

	manager := &recordingInterruptedOwnerManager{mockAgentManager: &mockAgentManager{}}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	response, err := service.RetrySessionDelivery(ctx, session.TaskID, session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryContinued, response.Outcome)
	require.Empty(t, manager.identities, "the predecessor execution must never be retried after the SQL generation transition")
}

func TestSilentRestoreAllocatedCandidateRejectsChangedOwnerAndGeneration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(context.Context, *sqliterepo.Repository, *models.TaskSession)
	}{
		{
			name: "workspace owner changed",
			mutate: func(ctx context.Context, repo *sqliterepo.Repository, _ *models.TaskSession) {
				require.NoError(t, repo.TransferWorkspaceOwnership(ctx, "ws1", "owner-test", "successor-owner"))
			},
		},
		{
			name: "native generation changed",
			mutate: func(ctx context.Context, repo *sqliterepo.Repository, session *models.TaskSession) {
				require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
					SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: 2,
					PredecessorGeneration: 1, NativeSessionID: "native-successor", CreationReason: "successor",
				}))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			service, _, _ := continuationFailureFixture(t)
			repo := service.repo.(*sqliterepo.Repository)
			session, err := repo.GetTaskSession(ctx, "s1")
			require.NoError(t, err)
			session.State = models.TaskSessionStateFailed
			require.NoError(t, repo.UpdateTaskSession(ctx, session))
			require.NoError(t, repo.ClaimUnownedWorkspaces(ctx, "owner-test"))
			_, err = repo.AssignWorkspacesWithoutOrg(ctx, "org-test")
			require.NoError(t, err)
			workspace, err := repo.GetWorkspace(ctx, "ws1")
			require.NoError(t, err)
			incarnationID := session.QueueIncarnationID
			require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
				SessionID: session.ID, IncarnationID: incarnationID, Generation: 1,
				NativeSessionID: "provider-session", CreationReason: "initial",
			}))
			_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
				ID: "source-submission", SessionID: session.ID, IncarnationID: incarnationID,
				HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "source-payload-hash",
				Payload: []byte("original prompt"), State: models.DeliverySubmissionInterruptedUnknown,
			})
			require.NoError(t, err)
			recovery := models.AgentDeliveryRecovery{
				SessionID: session.ID, AgentExecutionID: "source-execution", IncarnationID: incarnationID,
				HarnessGeneration: 1, SubmissionID: "source-submission", StreamID: "source-stream",
				PromptGeneration: 1, Phase: models.AgentDeliveryRecoveryUncertain,
			}
			require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID: "source-execution", TaskID: session.TaskID, SessionID: session.ID,
				AgentExecutionID: "source-execution", Status: "running",
			}))
			opened, err := repo.UpsertAgentDeliveryRecovery(ctx, &recovery, &models.SessionRecoveryBlock{
				SessionID: session.ID, IncarnationID: incarnationID, ExpectedGeneration: 1,
				Reason: "unknown_prompt_outcome", ConsumerReference: agentDeliveryConsumer,
				DeliverySubmissionID: recovery.SubmissionID, DeliveryStreamID: recovery.StreamID,
				State: models.RecoveryBlockOpen,
			})
			require.NoError(t, err)
			require.True(t, opened)
			service.SetStartupRecoveryOwnerContextResolver(func(ownerCtx context.Context, ownerID, orgID string) (context.Context, error) {
				if ownerID != "owner-test" || orgID != "org-test" {
					return nil, errors.New("unexpected startup recovery owner")
				}
				return ownerCtx, nil
			})
			candidate := models.SilentRestoreCandidate{TaskID: session.TaskID, SessionID: session.ID, WorkspaceID: workspace.ID}
			inputs, err := service.loadSilentRestoreInputs(ctx, candidate)
			require.NoError(t, err)
			require.NotNil(t, inputs)
			store := service.repo.(taskrepo.SilentRestoreRepository)
			attemptID := silentRestoreAttemptID(session.ID, incarnationID, 1)
			attempt, err := service.ensureSilentRestoreAttempt(ctx, store, attemptID, inputs)
			require.NoError(t, err)
			require.NotNil(t, attempt)
			require.NotNil(t, attempt.Checkpoint)
			allocated := *attempt.Checkpoint
			allocated.CandidateExecutionID = "candidate-execution"
			allocated.Stage = models.SilentRestoreStageCandidateAllocated
			changed, err := store.CompareAndSwapSilentRestoreCheckpoint(ctx, attemptID, *attempt.Checkpoint, allocated)
			require.NoError(t, err)
			require.True(t, changed)
			tc.mutate(ctx, repo, session)

			err = service.markSilentRestoreCandidateLaunching(ctx, store, attemptID, allocated, allocated.CandidateExecutionID, inputs)
			require.Error(t, err, "a stale owner or generation must be rejected before native loading")
			persisted, err := store.GetRestoreAttempt(ctx, attemptID)
			require.NoError(t, err)
			require.Equal(t, models.SilentRestoreStageCandidateAllocated, persisted.Checkpoint.Stage)
		})
	}
}

func TestSilentRestoreStopCancelsBlockedLaunchAndJoinsWorker(t *testing.T) {
	ctx := context.Background()
	base, _, _ := continuationFailureFixture(t)
	repo := base.repo.(*sqliterepo.Repository)
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateFailed
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	incarnationID := session.QueueIncarnationID
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: incarnationID, Generation: 1,
		NativeSessionID: "provider-session", CreationReason: "initial",
	}))
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: "stop-submission", SessionID: session.ID, IncarnationID: incarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "stop-payload-hash",
		Payload: []byte("original prompt"), State: models.DeliverySubmissionInterruptedUnknown,
	})
	require.NoError(t, err)
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "source-execution", TaskID: session.TaskID, SessionID: session.ID,
		AgentExecutionID: "old-execution", Status: "running",
	}))
	recovery := models.AgentDeliveryRecovery{
		SessionID: session.ID, AgentExecutionID: "old-execution", IncarnationID: incarnationID,
		HarnessGeneration: 1, SubmissionID: "stop-submission", StreamID: "old-stream",
		PromptGeneration: 1, Phase: models.AgentDeliveryRecoveryUncertain,
	}
	opened, err := repo.UpsertAgentDeliveryRecovery(ctx, &recovery, &models.SessionRecoveryBlock{
		SessionID: session.ID, IncarnationID: incarnationID, ExpectedGeneration: 1,
		Reason: "unknown_prompt_outcome", ConsumerReference: agentDeliveryConsumer,
		DeliverySubmissionID: recovery.SubmissionID, DeliveryStreamID: recovery.StreamID,
		State: models.RecoveryBlockOpen,
	})
	require.NoError(t, err)
	require.True(t, opened)

	manager := &recordingInterruptedOwnerManager{mockAgentManager: &mockAgentManager{repoForExecutionLookup: repo}}
	manager.rowLivenessFn = func(running *models.ExecutorRunning) models.ProcessLiveness {
		if running.AgentExecutionID == "old-execution" {
			return models.ProcessLivenessDead
		}
		return models.ProcessLivenessAlive
	}
	manager.getACPSessionIDForSessionFunc = func(string) (string, bool) { return "provider-session", true }
	launchEntered := make(chan struct{})
	launchCancelled := make(chan error, 1)
	manager.launchAgentFunc = func(launchCtx context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		require.NotNil(t, request.OnExecutionAllocated)
		require.NoError(t, request.OnExecutionAllocated(launchCtx, request.CandidateExecutionID))
		require.NoError(t, repo.UpsertExecutorRunning(launchCtx, &models.ExecutorRunning{
			ID: request.CandidateExecutionID, TaskID: session.TaskID, SessionID: session.ID,
			AgentExecutionID: request.CandidateExecutionID, Status: "starting",
		}))
		close(launchEntered)
		<-launchCtx.Done()
		launchCancelled <- launchCtx.Err()
		return nil, launchCtx.Err()
	}
	manager.startAgentProcessFunc = func(context.Context, string) error {
		t.Errorf("native process start must not run after Stop cancels the allocated launch")
		return nil
	}

	service := NewService(DefaultServiceConfig(), bus.NewMemoryEventBus(testLogger()), manager,
		newMockTaskRepo(), repo, nil, nil, nil, testLogger())
	service.SetTurnService(&repoTurnService{repo: repo})
	service.SetMessageCreator(&repositoryBackedMessageCreator{mockMessageCreator: &mockMessageCreator{}, repo: repo})
	service.SetStartupRecoveryOwnerContextResolver(func(ownerCtx context.Context, ownerID, orgID string) (context.Context, error) {
		if ownerID != "" || orgID != "" {
			return nil, errors.New("unexpected startup recovery owner")
		}
		return ownerCtx, nil
	})
	require.NoError(t, service.Start(ctx))
	select {
	case <-launchEntered:
	case <-time.After(10 * time.Second):
		t.Fatal("startup recovery did not enter the blocking launch")
	}

	stopped := make(chan error, 1)
	go func() { stopped <- service.Stop() }()
	select {
	case stopErr := <-stopped:
		require.NoError(t, stopErr)
	case <-time.After(10 * time.Second):
		t.Fatal("Stop did not cancel and join the blocked startup recovery worker")
	}
	select {
	case err := <-launchCancelled:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("resume attempt did not inherit the worker cancellation")
	}
	generation, err := repo.GetCurrentHarnessSessionGeneration(ctx, session.ID, incarnationID)
	require.NoError(t, err)
	require.EqualValues(t, 1, generation.Generation, "a cancelled launch must not commit a successor generation")
}
