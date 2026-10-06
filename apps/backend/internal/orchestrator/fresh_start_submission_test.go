package orchestrator

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.1
// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.2
// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.3
func TestFreshStartSubmissionReplay(t *testing.T) {
	assertFreshStartSubmissionReplay(t, freshStartSubmissionScenario{content: "original submitted request"})
	t.Run("files only", func(t *testing.T) {
		assertFreshStartSubmissionReplay(t, freshStartSubmissionScenario{})
	})
}

// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.1
// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.2
// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.3
func TestFreshStartSubmissionLegacyPreviewAdmission(t *testing.T) {
	assertFreshStartSubmissionReplay(t, freshStartSubmissionScenario{
		content: "original submitted request", legacyPreview: true,
	})
}

// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.1
// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.6
func TestFreshStartReplayUsesCurrentFirstLaunchInstructions(t *testing.T) {
	assertFreshStartSubmissionReplay(t, freshStartSubmissionScenario{
		content: "original submitted request", currentFirstLaunchPolicy: true,
	})
}

// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.5
func TestFreshStartReplayRuntimeDisappearsAfterReady(t *testing.T) {
	assertFreshStartSubmissionReplay(t, freshStartSubmissionScenario{
		content: "original submitted request", runtimeDisappearsAfterReady: true,
	})
}

// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.5
func TestFreshStartReplayHoldBlocksManualAndEnqueueQueueDrains(t *testing.T) {
	assertFreshStartSubmissionReplay(t, freshStartSubmissionScenario{
		content: "original submitted request", pauseBeforeReplayAdmission: true,
	})
}

// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.3
// @covers AC-TASKS-PROMPT-ATTACHMENTS-002.7
func TestFreshStartPendingReceiptBlockedByLaterPromptAcrossRestart(t *testing.T) {
	for _, resumePrompt := range []bool{false, true} {
		name := "interactive prompt"
		if resumePrompt {
			name = "ordinary resume prompt"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			const taskID = "fresh-start-receipt-provenance-task"
			const sessionID = "fresh-start-receipt-provenance-session"
			databasePath := filepath.Join(t.TempDir(), "fresh-start-receipt-provenance.db")
			connection, err := db.OpenSQLite(databasePath)
			require.NoError(t, err)
			writer := sqlx.NewDb(connection, "sqlite3")
			t.Cleanup(func() { _ = writer.Close() })
			repo, err := sqliterepo.NewWithDB(writer, writer, nil)
			require.NoError(t, err)
			seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateFailed)
			storedSession, err := repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
			storedSession.AgentProfileID = "fresh-start-receipt-test-profile"
			require.NoError(t, repo.UpdateTaskSession(ctx, storedSession))
			pending, err := models.NewInitialPromptSubmission("original request", false, nil)
			require.NoError(t, err)
			require.NoError(t, repo.SetSessionMetadataKey(
				ctx, sessionID, models.SessionMetaKeyInitialPromptSubmission, pending,
			))
			now := time.Now().UTC()
			require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
				ID: "fresh-start-original-turn", TaskSessionID: sessionID, TaskID: taskID,
				StartedAt: now, CreatedAt: now,
			}))
			require.NoError(t, repo.CreateMessage(ctx, &models.Message{
				TaskSessionID: sessionID, TaskID: taskID, TurnID: "fresh-start-original-turn", AuthorType: models.MessageAuthorUser,
				Content: "original request",
			}))
			require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, models.LastAgentError{
				Message: "initial bootstrap failed", Phase: models.LaunchErrorPhaseBootstrap,
			}))
			storedSession, err = repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
			checkService := &Service{repo: repo}
			stillPending, err := checkService.initialSubmissionForFreshStart(ctx, taskID, storedSession)
			require.NoError(t, err)
			require.Equal(t, models.InitialPromptSubmissionPending, stillPending.State,
				"the visible initial user row does not prove provider acceptance")

			var agentRunning atomic.Bool
			if !resumePrompt {
				require.NoError(t, repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""))
				agentRunning.Store(true)
				seedExecutorRunning(t, repo, sessionID, taskID, "fresh-start-existing-execution")
			}
			manager := &mockAgentManager{
				repoForExecutionLookup: repo,
				isAgentRunningFn:       func(context.Context, string) bool { return agentRunning.Load() },
				isAgentReadyFn:         func(context.Context, string) bool { return agentRunning.Load() },
			}
			if resumePrompt {
				manager.launchAgentFunc = func(_ context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
					agentRunning.Store(true)
					if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
						ID: sessionID, SessionID: sessionID, TaskID: taskID,
						AgentExecutionID: "fresh-start-resume-execution", Status: "ready",
					}); err != nil {
						return nil, err
					}
					if err := repo.UpdateTaskSessionState(ctx, request.SessionID, models.TaskSessionStateWaitingForInput, ""); err != nil {
						return nil, err
					}
					return &executor.LaunchAgentResponse{AgentExecutionID: "fresh-start-resume-execution"}, nil
				}
			}
			taskRepo := newMockTaskRepo()
			taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
			svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, manager)
			svc.messageCreator = &mockMessageCreator{}
			svc.turnService = &repoTurnService{repo: repo}
			if resumePrompt {
				_, err = svc.ResumeTaskSessionAndPrompt(ctx, taskID, sessionID, "accepted resume prompt", "", false, nil)
			} else {
				_, err = svc.PromptTask(ctx, taskID, sessionID, "accepted interactive prompt", "", false, nil, false)
			}
			require.NoError(t, err)
			storedSession, err = repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
			manager.mu.Lock()
			capturedPrompts := append([]string(nil), manager.capturedPrompts...)
			manager.mu.Unlock()
			require.Len(t, capturedPrompts, 1,
				"the later ordinary prompt must cross provider dispatch before the bootstrap failure")
			blocked, found, err := models.LoadInitialPromptSubmission(storedSession.Metadata)
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, "replay_blocked", blocked.State,
				"a later prompt reaching provider admission must durably retire the old replay authority")

			storedSession.State = models.TaskSessionStateFailed
			require.NoError(t, repo.UpdateTaskSession(ctx, storedSession))
			require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, models.LastAgentError{
				Message: "later bootstrap failed", Phase: models.LaunchErrorPhaseBootstrap,
				StampValue: "later-bootstrap-failure",
			}))
			require.NoError(t, writer.Close())

			reopenedConnection, openErr := db.OpenSQLite(databasePath)
			require.NoError(t, openErr)
			reopenedWriter := sqlx.NewDb(reopenedConnection, "sqlite3")
			t.Cleanup(func() { _ = reopenedWriter.Close() })
			reopenedRepo := sqliterepo.NewWithInitializedDB(reopenedWriter, reopenedWriter, nil)
			var freshLaunches atomic.Int32
			recoveryManager := &mockAgentManager{
				repoForExecutionLookup: reopenedRepo,
				launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
					freshLaunches.Add(1)
					return &executor.LaunchAgentResponse{AgentExecutionID: "unexpected-fresh-replay"}, nil
				},
			}
			recoveryTaskRepo := newMockTaskRepo()
			recoveryTaskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
			recoverySvc := createTestServiceWithScheduler(reopenedRepo, newMockStepGetter(), recoveryTaskRepo, recoveryManager)
			_, err = recoverySvc.RecoverSessionWithOptions(ctx, taskID, sessionID, "fresh_start", RecoverSessionOptions{})
			require.ErrorIs(t, err, ErrInitialSubmissionAcceptanceUncertain)
			require.Zero(t, freshLaunches.Load(), "restart recovery must not launch a replay for later conversation work")
			recoveryManager.mu.Lock()
			require.Empty(t, recoveryManager.capturedPrompts)
			recoveryManager.mu.Unlock()
		})
	}
}

func TestFreshStartSubmissionConcurrentRetry(t *testing.T) {
	ctx := context.Background()
	const taskID = "fresh-start-submission-concurrent-task"
	const sessionID = "fresh-start-submission-concurrent-session"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateFailed)
	pending, err := models.NewInitialPromptSubmission("original submitted request", true, nil)
	require.NoError(t, err)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyInitialPromptSubmission, pending))
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})

	start := make(chan struct{})
	results := make(chan struct {
		attempt string
		err     error
	}, 2)
	for _, identity := range []struct{ execution, attempt string }{
		{execution: "execution-a", attempt: "attempt-a"},
		{execution: "execution-b", attempt: "attempt-b"},
	} {
		identity := identity
		go func() {
			<-start
			results <- struct {
				attempt string
				err     error
			}{attempt: identity.attempt, err: svc.transitionInitialSubmissionToDispatching(
				ctx, sessionID, identity.execution, identity.attempt,
			)}
		}()
	}
	close(start)
	first, second := <-results, <-results
	winner, loser := first, second
	if winner.err != nil {
		winner, loser = second, first
	}
	require.NoError(t, winner.err)
	require.ErrorIs(t, loser.err, ErrInitialSubmissionAcceptanceUncertain)

	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	stored, found, err := models.LoadInitialPromptSubmission(session.Metadata)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, models.InitialPromptSubmissionDispatching, stored.State)
	require.Equal(t, winner.attempt, stored.AttemptID)
	winnerExecutionID := "execution-b"
	loserExecutionID := "execution-a"
	if winner.attempt == "attempt-a" {
		winnerExecutionID, loserExecutionID = loserExecutionID, winnerExecutionID
	}
	require.ErrorIs(t, svc.transitionInitialSubmissionToAccepted(ctx, sessionID, loserExecutionID, loser.attempt), ErrInitialSubmissionAcceptanceUncertain)
	session, err = repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	stored, found, err = models.LoadInitialPromptSubmission(session.Metadata)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, winner.attempt, stored.AttemptID)
	require.NoError(t, svc.transitionInitialSubmissionToAccepted(ctx, sessionID, winnerExecutionID, winner.attempt))
	session, err = repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	stored, found, err = models.LoadInitialPromptSubmission(session.Metadata)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, models.InitialPromptSubmissionAccepted, stored.State)
	require.Equal(t, winnerExecutionID, stored.ExecutionID)
}

func TestInitialTaskSubmissionReceiptSurvivesRequestCancellation(t *testing.T) {
	ctx := context.Background()
	requestCtx, cancelRequest := context.WithCancel(ctx)
	defer cancelRequest()
	const taskID = "initial-submission-request-context-task"
	const sessionID = "initial-submission-request-context-session"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCreated)
	pending, err := models.NewInitialPromptSubmission("initial submitted request", false, nil)
	require.NoError(t, err)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyInitialPromptSubmission, pending))
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})

	beforeAdmission, accepted, err := svc.initialSubmissionDispatchCallbacks(
		requestCtx, taskID, sessionID, "initial submitted request", false, nil,
	)
	require.NoError(t, err)
	require.NotNil(t, beforeAdmission)
	require.NotNil(t, accepted)
	cancelRequest()
	require.NoError(t, beforeAdmission("initial-submission-execution"))
	accepted("initial-submission-execution")

	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	stored, found, err := models.LoadInitialPromptSubmission(session.Metadata)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, models.InitialPromptSubmissionAccepted, stored.State)
	require.Equal(t, "initial-submission-execution", stored.ExecutionID)
}

func TestFreshStartSubmissionCancelledAttempt(t *testing.T) {
	assertFreshStartSubmissionReplay(t, freshStartSubmissionScenario{
		content: "original submitted request", cancelBeforeAdmission: true,
	})
}

func TestFreshStartSubmissionRejectsInvalidClaim(t *testing.T) {
	assertFreshStartSubmissionReplay(t, freshStartSubmissionScenario{
		content: "original submitted request", invalidAttachmentDescriptor: true,
	})
}

func TestFreshStartRecoveryPreservesSuccessorFailure(t *testing.T) {
	assertFreshStartSubmissionReplay(t, freshStartSubmissionScenario{
		content: "original submitted request", successorErrorBeforeBootReady: true,
	})
}

func TestFreshStartRecoveryCancelledBoot(t *testing.T) {
	assertFreshStartSubmissionReplay(t, freshStartSubmissionScenario{
		content: "original submitted request", cancelBeforeBootReady: true,
	})
}

type freshStartSubmissionScenario struct {
	content                       string
	legacyPreview                 bool
	currentFirstLaunchPolicy      bool
	runtimeDisappearsAfterReady   bool
	pauseBeforeReplayAdmission    bool
	cancelBeforeAdmission         bool
	cancelBeforeBootReady         bool
	successorErrorBeforeBootReady bool
	invalidAttachmentDescriptor   bool
}

func assertFreshStartSubmissionReplay(t *testing.T, scenario freshStartSubmissionScenario) {
	ctx := context.Background()
	const taskID = "fresh-start-submission-task"
	const sessionID = "fresh-start-submission-session"
	imageBytes := []byte("original image bytes")
	resourceBytes := []byte("original resource bytes")

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateFailed)
	task, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	task.WorkspaceID = "ws1"
	task.Description = "current task description"
	stepGetter := newMockStepGetter()
	if scenario.currentFirstLaunchPolicy {
		task.WorkflowStepID = "fresh-start-completion-step"
		stepGetter.steps[task.WorkflowStepID] = &wfmodels.WorkflowStep{
			ID: task.WorkflowStepID, WorkflowID: task.WorkflowID, AutoAdvanceRequiresSignal: true,
		}
	}
	require.NoError(t, repo.UpdateTask(ctx, task))
	if scenario.currentFirstLaunchPolicy {
		require.NoError(t, repo.SetTaskMetadataKey(ctx, taskID, models.MetaKeyAgentTitlePending, true))
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileID = "fresh-start-profile"
	session.TaskEnvironmentID = "fresh-start-submission-environment"
	if scenario.currentFirstLaunchPolicy {
		session.Metadata["prompt_reference_context"] = "stale hidden prompt context"
	}
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: session.TaskEnvironmentID, TaskID: taskID,
		ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
	}))
	seedExecutorRunning(t, repo, sessionID, taskID, "fresh-start-previous-execution")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message: "agent startup failed", Phase: models.LaunchErrorPhaseBootstrap,
		StampValue: "fresh-start-submission-error",
	}))

	attachmentService, err := taskservice.NewAttachmentService(repo, t.TempDir(), nil, testLogger())
	require.NoError(t, err)
	image, err := attachmentService.Stage(ctx, "user-1", "ws1", "screen.png", "image/png", "image", "prompt", bytes.NewReader(imageBytes))
	require.NoError(t, err)
	resource, err := attachmentService.Stage(ctx, "user-1", "ws1", "report.zip", "application/zip", "resource", "path", bytes.NewReader(resourceBytes))
	require.NoError(t, err)
	require.NoError(t, attachmentService.Claim(ctx, "user-1", "ws1", taskID, "", []string{image.ID, resource.ID}))

	attachments := []v1.MessageAttachment{
		{AttachmentID: image.ID, Type: image.Kind, Name: image.Name, MimeType: image.MimeType, SizeBytes: image.SizeBytes, DeliveryMode: image.DeliveryMode},
		{AttachmentID: resource.ID, Type: resource.Kind, Name: resource.Name, MimeType: resource.MimeType, SizeBytes: resource.SizeBytes, DeliveryMode: resource.DeliveryMode},
	}
	if scenario.invalidAttachmentDescriptor {
		attachments[0].Name = "forged.png"
	}
	// Seed the durable shape independently of the production model so this test
	// exercises the recovery boundary against the current implementation.
	if scenario.legacyPreview {
		require.NoError(t, repo.SetSessionMetadataKey(
			ctx, sessionID, models.SessionMetaKeyInitialPromptPreview,
			models.NewInitialPromptPreview(scenario.content, attachments),
		))
	} else {
		submission := map[string]interface{}{
			"version": 1, "content": scenario.content, "plan_mode": true,
			"attachments": attachments, "state": "pending",
		}
		require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyInitialPromptSubmission, submission))
	}

	var promptAttachments []v1.MessageAttachment
	var launchDescription string
	launchCount := 0
	promptEntered := make(chan struct{})
	startEntered := make(chan struct{})
	processStarted := make(chan struct{})
	agentManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true,
		isAgentReadyFn:         func(context.Context, string) bool { return true },
		launchAgentFunc: func(_ context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCount++
			launchDescription = request.TaskDescription
			return &executor.LaunchAgentResponse{AgentExecutionID: "fresh-start-submission-execution"}, nil
		},
		startAgentProcessFunc: func(context.Context, string) error {
			close(startEntered)
			return nil
		},
		promptAgentFunc: func(_ context.Context, _ string, _ string, got []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
			select {
			case <-promptEntered:
			default:
				close(promptEntered)
			}
			promptAttachments = append([]v1.MessageAttachment(nil), got...)
			if scenario.runtimeDisappearsAfterReady {
				return nil, executor.ErrExecutionNotFound
			}
			return &executor.PromptResult{}, nil
		},
	}
	if scenario.cancelBeforeAdmission {
		agentManager.promptAdmissionEntered = make(chan struct{}, 1)
		agentManager.promptAdmissionRelease = make(chan struct{})
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{
		ID: taskID, WorkspaceID: "ws1", Title: "Fresh start task",
		Description: task.Description, State: v1.TaskStateInProgress,
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentManager)
	var queueAdmissionBarrier *freshStartListMessagesBarrierRepo
	if scenario.pauseBeforeReplayAdmission {
		queueAdmissionBarrier = &freshStartListMessagesBarrierRepo{
			Repository: repo, sessionID: sessionID,
			listMessagesEntered: make(chan struct{}), allowListMessages: make(chan struct{}),
		}
		svc.repo = queueAdmissionBarrier
		t.Cleanup(func() {
			select {
			case <-queueAdmissionBarrier.allowListMessages:
			default:
				close(queueAdmissionBarrier.allowListMessages)
			}
		})
	}
	svc.SetAttachmentReader(attachmentService)
	svc.messageCreator = &mockMessageCreator{}
	svc.turnService = &repoTurnService{repo: repo}
	svc.executor.SetOnAgentProcessStartFailed(svc.handleAgentProcessStartFailed)
	svc.executor.SetOnAgentProcessStarted(func(callbackCtx context.Context, callbackTaskID, callbackSessionID, callbackExecutionID string) {
		svc.handleAgentProcessStarted(callbackCtx, callbackTaskID, callbackSessionID, callbackExecutionID)
		select {
		case <-processStarted:
		default:
			close(processStarted)
		}
	})
	if scenario.invalidAttachmentDescriptor {
		_, recoverErr := svc.RecoverSessionWithOptions(ctx, taskID, sessionID, "fresh_start", RecoverSessionOptions{})
		require.ErrorIs(t, recoverErr, ErrInitialSubmissionUnavailable)
		unchanged, loadErr := repo.GetTaskSession(ctx, sessionID)
		require.NoError(t, loadErr)
		require.Equal(t, "fresh-start-previous-execution", unchanged.AgentExecutionID)
		lastError, found := models.LoadLastAgentError(unchanged.Metadata)
		require.True(t, found)
		require.Equal(t, "fresh-start-submission-error", lastError.Stamp())
		require.False(t, lastError.IsDismissed())
		require.Empty(t, svc.messageCreator.(*mockMessageCreator).userMessages)
		return
	}

	recoveryDone := make(chan error, 1)
	go func() {
		_, recoverErr := svc.RecoverSessionWithOptions(ctx, taskID, sessionID, "fresh_start", RecoverSessionOptions{})
		recoveryDone <- recoverErr
	}()
	select {
	case <-startEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for fresh recovery startup")
	}
	attempt, ok := svc.resumeAttemptStore().current(sessionID)
	require.True(t, ok)
	require.Eventually(t, func() bool { return attempt.execution() != "" }, 3*time.Second, 10*time.Millisecond)
	select {
	case <-processStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for provider process startup to settle")
	}
	if scenario.cancelBeforeBootReady {
		svc.invalidateResumeAttempt(sessionID)
	}
	if scenario.successorErrorBeforeBootReady {
		require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, models.LastAgentError{
			Message: "a newer prompt delivery failed", Phase: "prompt",
			StampValue: "fresh-start-successor-error",
		}))
	}
	svc.handleAgentBootReady(ctx, watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID,
		AgentExecutionID: "fresh-start-submission-execution", AttemptID: attempt.identity(),
	})
	if scenario.pauseBeforeReplayAdmission {
		select {
		case <-queueAdmissionBarrier.listMessagesEntered:
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for the ready recovery to pause before replay admission")
		}
		require.NoError(t, svc.QueueUserPrompt(
			ctx, taskID, sessionID, "later queued work", "", false, nil, nil, false,
		))
		drained, drainErr := svc.DrainQueuedMessage(ctx, sessionID)
		require.NoError(t, drainErr)
		require.False(t, drained, "manual drain must leave the later queue entry behind replay")
		require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, sessionID).Count,
			"enqueue-side and manual drains must retain the queued entry")
		close(queueAdmissionBarrier.allowListMessages)
	}
	if scenario.runtimeDisappearsAfterReady {
		select {
		case <-promptEntered:
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for replay dispatch to observe the disappeared runtime")
		}
		select {
		case err = <-recoveryDone:
			require.ErrorIs(t, err, executor.ErrExecutionNotFound)
			require.Equal(t, 1, launchCount, "a dispatch failure must not start an untracked successor")
		case <-time.After(time.Second):
			t.Fatal("replay did not settle after the runtime disappeared following readiness")
		}
		return
	}
	if scenario.cancelBeforeBootReady {
		select {
		case err = <-recoveryDone:
			require.ErrorIs(t, err, ErrResumeAttemptCancelled)
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for cancelled boot recovery")
		}
		storedSession, loadErr := repo.GetTaskSession(ctx, sessionID)
		require.NoError(t, loadErr)
		receipt, found, receiptErr := models.LoadInitialPromptSubmission(storedSession.Metadata)
		require.NoError(t, receiptErr)
		require.True(t, found)
		require.Equal(t, models.InitialPromptSubmissionPending, receipt.State)
		lastError, found := models.LoadLastAgentError(storedSession.Metadata)
		require.True(t, found)
		require.Equal(t, "fresh-start-submission-error", lastError.Stamp())
		require.False(t, lastError.IsDismissed())
		require.Empty(t, svc.messageCreator.(*mockMessageCreator).userMessages)
		return
	}
	if agentManager.promptAdmissionEntered != nil {
		select {
		case <-agentManager.promptAdmissionEntered:
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for replay prompt admission")
		}
		if scenario.cancelBeforeAdmission {
			svc.invalidateResumeAttempt(sessionID)
		}
		close(agentManager.promptAdmissionRelease)
	}
	select {
	case err = <-recoveryDone:
		if scenario.cancelBeforeAdmission {
			require.ErrorIs(t, err, ErrResumeAttemptCancelled)
			storedSession, loadErr := repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, loadErr)
			receipt, found, receiptErr := models.LoadInitialPromptSubmission(storedSession.Metadata)
			require.NoError(t, receiptErr)
			require.True(t, found)
			require.Equal(t, models.InitialPromptSubmissionPending, receipt.State)
			return
		}
		if err != nil {
			storedSession, loadErr := repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, loadErr)
			t.Fatalf("fresh recovery failed: %v; session=%#v", err, storedSession)
		}
	case <-time.After(3 * time.Second):
		storedSession, _ := repo.GetTaskSession(ctx, sessionID)
		agentManager.mu.Lock()
		capturedPrompts := append([]string(nil), agentManager.capturedPrompts...)
		agentManager.mu.Unlock()
		t.Fatalf("timed out waiting for fresh recovery: session=%#v prompts=%#v attempt=%q", storedSession, capturedPrompts, attempt.execution())
	}
	require.Empty(t, launchDescription, "fresh recovery must start without the automatic task-description prompt")
	require.Len(t, promptAttachments, 2, "the captured submission must be replayed once after readiness")
	capturedPrompt := agentManager.capturedPrompts[len(agentManager.capturedPrompts)-1]
	require.NotContains(t, capturedPrompt, "current task description")
	require.NotContains(t, capturedPrompt, "stale hidden prompt context")
	if scenario.content != "" {
		require.Contains(t, capturedPrompt, scenario.content)
		if !scenario.currentFirstLaunchPolicy {
			require.True(t, strings.HasSuffix(capturedPrompt, scenario.content))
		}
	}
	if scenario.currentFirstLaunchPolicy {
		require.Contains(t, capturedPrompt, taskID)
		require.Contains(t, capturedPrompt, sessionID)
		require.Contains(t, capturedPrompt, "step_complete_kandev")
		require.Contains(t, capturedPrompt, "ask_user_question_kandev")
		require.Contains(t, capturedPrompt, "set_task_title_kandev")
	}
	require.Equal(t, []string{image.ID, resource.ID}, []string{promptAttachments[0].AttachmentID, promptAttachments[1].AttachmentID})
	require.Equal(t, []string{"prompt", "path"}, []string{promptAttachments[0].DeliveryMode, promptAttachments[1].DeliveryMode})

	for index, expected := range [][]byte{imageBytes, resourceBytes} {
		reader, _, _, _, openErr := attachmentService.OpenClaimed(ctx, promptAttachments[index].AttachmentID, taskID, sessionID)
		require.NoError(t, openErr)
		actual, readErr := io.ReadAll(reader)
		require.NoError(t, readErr)
		require.NoError(t, reader.Close())
		require.Equal(t, expected, actual)
	}
	require.Len(t, svc.messageCreator.(*mockMessageCreator).userMessages, 1)
	require.Equal(t, scenario.content, svc.messageCreator.(*mockMessageCreator).userMessages[0].content)
	if !scenario.legacyPreview {
		require.Equal(t, true, svc.messageCreator.(*mockMessageCreator).userMessages[0].metadata["plan_mode"])
	} else {
		require.NotEqual(t, true, svc.messageCreator.(*mockMessageCreator).userMessages[0].metadata["plan_mode"])
	}
	storedSession, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	replayed, found, err := models.LoadInitialPromptSubmission(storedSession.Metadata)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, models.InitialPromptSubmissionAccepted, replayed.State)
	require.Equal(t, "fresh-start-submission-execution", replayed.ExecutionID)
	require.Equal(t, attempt.identity(), replayed.AttemptID)
	if scenario.pauseBeforeReplayAdmission {
		agentManager.mu.Lock()
		capturedPrompts := append([]string(nil), agentManager.capturedPrompts...)
		agentManager.mu.Unlock()
		require.Len(t, capturedPrompts, 1)
		require.Contains(t, capturedPrompts[0], scenario.content,
			"the original submission must be the first admitted prompt")
		require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, sessionID).Count,
			"the later queued entry must remain intact after replay acceptance")
	}
	lastError, found := models.LoadLastAgentError(storedSession.Metadata)
	require.True(t, found)
	if scenario.successorErrorBeforeBootReady {
		require.Equal(t, "fresh-start-successor-error", lastError.Stamp())
		require.Equal(t, "a newer prompt delivery failed", lastError.Message)
		require.False(t, lastError.IsDismissed())
	} else {
		require.Equal(t, "fresh-start-submission-error", lastError.Stamp())
		require.Equal(t, "agent startup failed", lastError.Message)
		require.True(t, lastError.IsDismissed())
	}
}

type freshStartListMessagesBarrierRepo struct {
	*sqliterepo.Repository
	sessionID           string
	listMessagesEntered chan struct{}
	allowListMessages   chan struct{}
	listMessagesOnce    sync.Once
}

func (r *freshStartListMessagesBarrierRepo) ListMessages(
	ctx context.Context,
	sessionID string,
) ([]*models.Message, error) {
	if sessionID == r.sessionID {
		r.listMessagesOnce.Do(func() {
			close(r.listMessagesEntered)
			<-r.allowListMessages
		})
	}
	return r.Repository.ListMessages(ctx, sessionID)
}
