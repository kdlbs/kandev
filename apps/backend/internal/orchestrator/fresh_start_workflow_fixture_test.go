package orchestrator

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type freshStartWorkflowOptions struct {
	withReceipt           bool
	acceptedReceipt       bool
	receiptContent        string
	receiptPlanMode       bool
	description           string
	workflowPrompt        string
	stepPrompt            string
	stepEvents            wfmodels.StepEvents
	stepReadErr           error
	workflowReadErr       error
	passthrough           bool
	configMode            bool
	providerToken         string
	runningToken          string
	savedPromptDefinition string
	stalePromptContext    string
	routeWithoutIdentity  bool
	strictPromptErr       bool
	repositoryMessages    bool
	withAttachment        bool
	seedTranscript        bool
	failureStamp          string
}

type freshStartWorkflowObservation struct {
	launchCount       int
	launchDescription string
	capturedPrompts   []string
	recoveryErr       error
	session           *models.TaskSession
	receipt           *models.InitialPromptSubmission
	messages          []*models.Message
	lastError         *models.LastAgentError
}

type freshStartWorkflowFixture struct {
	ctx         context.Context
	repo        *sqliterepo.Repository
	svc         *Service
	manager     *mockAgentManager
	stepGetter  *mockStepGetter
	taskID      string
	sessionID   string
	stepID      string
	started     chan struct{}
	startOnce   sync.Once
	processed   chan struct{}
	processOnce sync.Once
	done        chan error
	mu          sync.Mutex
	launches    []string
}

func newFreshStartWorkflowFixture(t *testing.T, options freshStartWorkflowOptions) *freshStartWorkflowFixture {
	t.Helper()
	const (
		taskID    = "fresh-start-workflow-task"
		sessionID = "fresh-start-workflow-session"
		stepID    = "fresh-start-workflow-step"
	)
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateFailed)

	task, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	task.WorkflowStepID = stepID
	task.Description = options.description
	task.WorkspaceID = "ws1"
	if options.routeWithoutIdentity {
		if task.Metadata == nil {
			task.Metadata = map[string]interface{}{}
		}
		task.Metadata[models.MetaKeyWorkflowSessionRoute] = map[string]interface{}{
			"operation_id":        "legacy-route-operation",
			"destination_step_id": stepID,
			"target_kind":         "reuse",
			"phase":               "committed",
		}
	}
	require.NoError(t, repo.UpdateTask(ctx, task))

	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileID = "fresh-start-workflow-profile"
	session.TaskEnvironmentID = "fresh-start-workflow-environment"
	session.IsPassthrough = options.passthrough
	session.DownstreamACPSessionID = options.providerToken
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	sessionMetadata := map[string]interface{}{}
	if options.configMode {
		sessionMetadata["config_mode"] = true
	}
	if options.stalePromptContext != "" {
		sessionMetadata["prompt_reference_context"] = options.stalePromptContext
	}
	if len(sessionMetadata) > 0 {
		require.NoError(t, repo.UpdateSessionMetadata(ctx, sessionID, sessionMetadata))
	}
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: session.TaskEnvironmentID, TaskID: taskID,
		ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
	}))
	seedExecutorRunning(t, repo, sessionID, taskID, "fresh-start-workflow-previous-execution")
	if options.runningToken != "" {
		running, runningErr := repo.GetExecutorRunningBySessionID(ctx, sessionID)
		require.NoError(t, runningErr)
		running.ResumeToken = options.runningToken
		require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
	}
	failureStamp := options.failureStamp
	if failureStamp == "" {
		failureStamp = "fresh-start-workflow-error"
	}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message: "agent startup failed", Phase: models.LaunchErrorPhaseBootstrap,
		StampValue: failureStamp,
	}))

	var attachments []v1.MessageAttachment
	var attachmentService *taskservice.AttachmentService
	if options.withAttachment {
		var attachmentErr error
		attachmentService, attachmentErr = taskservice.NewAttachmentService(repo, t.TempDir(), nil, testLogger())
		require.NoError(t, attachmentErr)
		image, stageErr := attachmentService.Stage(
			ctx, "user-1", "ws1", "screen.png", "image/png", "image", "prompt",
			bytes.NewReader([]byte("original image bytes")),
		)
		require.NoError(t, stageErr)
		require.NoError(t, attachmentService.Claim(ctx, "user-1", "ws1", taskID, "", []string{image.ID}))
		attachments = []v1.MessageAttachment{{
			AttachmentID: image.ID, Type: image.Kind, Name: image.Name,
			MimeType: image.MimeType, SizeBytes: image.SizeBytes, DeliveryMode: image.DeliveryMode,
		}}
	}

	if options.withReceipt || options.acceptedReceipt {
		submission, submissionErr := models.NewInitialPromptSubmission(
			options.receiptContent, options.receiptPlanMode, attachments,
		)
		require.NoError(t, submissionErr)
		if options.acceptedReceipt {
			submission.State = models.InitialPromptSubmissionAccepted
			submission.ExecutionID = "fresh-start-workflow-accepted-execution"
			submission.AcceptedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		require.NoError(t, repo.SetSessionMetadataKey(
			ctx, sessionID, models.SessionMetaKeyInitialPromptSubmission, submission,
		))
	}

	if options.seedTranscript {
		now := time.Now().UTC()
		turnID := "fresh-start-workflow-original-turn"
		require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
			ID: turnID, TaskID: taskID, TaskSessionID: sessionID,
			StartedAt: now, CreatedAt: now,
		}))
		require.NoError(t, repo.CreateMessage(ctx, &models.Message{
			ID:            initialSubmissionUserMessageID(taskID, sessionID),
			TaskID:        taskID,
			TaskSessionID: sessionID,
			TurnID:        turnID,
			AuthorType:    models.MessageAuthorUser,
			Content:       options.receiptContent,
			Metadata:      NewUserMessageMeta().WithAttachments(attachments).ToMap(),
		}))
	}

	stepGetter := newMockStepGetter()
	if options.stepReadErr != nil {
		stepGetter.getStepFunc = func(context.Context, string) (*wfmodels.WorkflowStep, error) {
			return nil, options.stepReadErr
		}
	} else {
		stepGetter.steps[stepID] = &wfmodels.WorkflowStep{
			ID: stepID, WorkflowID: "wf1", Name: "Fresh start workflow",
			Prompt: options.stepPrompt, Events: options.stepEvents,
		}
	}
	if options.workflowPrompt != "" {
		stepGetter.workflowPrompts["wf1"] = options.workflowPrompt
	}
	if options.workflowReadErr != nil {
		stepGetter.workflowMetaErr = options.workflowReadErr
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{
		ID: taskID, WorkspaceID: "ws1", WorkflowID: "wf1", Title: "Fresh start workflow task",
		Description: options.description, State: v1.TaskStateInProgress,
	}

	started := make(chan struct{})
	processed := make(chan struct{})
	fixture := &freshStartWorkflowFixture{
		ctx: ctx, repo: repo,
		taskID: taskID, sessionID: sessionID, stepID: stepID,
		stepGetter: stepGetter,
		started:    started, processed: processed,
	}
	manager := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true,
		isAgentReadyFn:         func(context.Context, string) bool { return true },
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			fixture.mu.Lock()
			fixture.launches = append(fixture.launches, req.TaskDescription)
			fixture.mu.Unlock()
			return &executor.LaunchAgentResponse{AgentExecutionID: "fresh-start-workflow-execution"}, nil
		},
		promptAgentFunc: func(context.Context, string, string, []v1.MessageAttachment, bool) (*executor.PromptResult, error) {
			return &executor.PromptResult{}, nil
		},
	}
	fixture.manager = manager
	manager.startAgentProcessFunc = func(context.Context, string) error {
		fixture.startOnce.Do(func() { close(started) })
		return nil
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, manager)
	fixture.svc = svc
	if options.savedPromptDefinition != "" {
		promptService := newPromptServiceForLaunchFallbackTest(t)
		_, promptErr := promptService.CreatePrompt(ctx, "principles", options.savedPromptDefinition)
		require.NoError(t, promptErr)
		svc.promptExpander = promptService
	} else if options.strictPromptErr {
		svc.promptExpander = failingStrictPromptExpander{}
	}
	if options.repositoryMessages {
		svc.messageCreator = &repositoryBackedMessageCreator{
			mockMessageCreator: &mockMessageCreator{},
			repo:               repo,
		}
	} else {
		svc.messageCreator = &mockMessageCreator{}
	}
	if attachmentService != nil {
		svc.SetAttachmentReader(attachmentService)
	}
	svc.turnService = &repoTurnService{repo: repo}
	svc.executor.SetOnAgentProcessStarted(func(callbackCtx context.Context, callbackTaskID, callbackSessionID, callbackExecutionID string) {
		svc.handleAgentProcessStarted(callbackCtx, callbackTaskID, callbackSessionID, callbackExecutionID)
		fixture.processOnce.Do(func() { close(processed) })
	})
	svc.executor.SetOnAgentProcessStartFailed(svc.handleAgentProcessStartFailed)
	return fixture
}

func (f *freshStartWorkflowFixture) recover(t *testing.T, action string) freshStartWorkflowObservation {
	t.Helper()
	f.done = make(chan error, 1)
	go func() {
		_, err := f.svc.RecoverSessionWithOptions(f.ctx, f.taskID, f.sessionID, action, RecoverSessionOptions{})
		f.done <- err
	}()

	var recoveryErr error
	select {
	case recoveryErr = <-f.done:
	case <-f.started:
		attempt, ok := f.svc.resumeAttemptStore().current(f.sessionID)
		require.True(t, ok)
		require.Eventually(t, func() bool { return attempt.execution() != "" }, 3*time.Second, 10*time.Millisecond)
		if action != "resume" {
			select {
			case <-f.processed:
			case <-time.After(3 * time.Second):
				t.Fatal("timed out waiting for provider process startup")
			}
		}
		f.svc.handleAgentBootReady(f.ctx, watcher.AgentEventData{
			TaskID: f.taskID, SessionID: f.sessionID,
			AgentExecutionID: "fresh-start-workflow-execution", AttemptID: attempt.identity(),
		})
		select {
		case recoveryErr = <-f.done:
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for fresh recovery")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for fresh recovery launch")
	}

	observation := freshStartWorkflowObservation{recoveryErr: recoveryErr}
	f.mu.Lock()
	observation.launchCount = len(f.launches)
	if len(f.launches) > 0 {
		observation.launchDescription = f.launches[0]
	}
	f.mu.Unlock()
	f.manager.mu.Lock()
	observation.capturedPrompts = append([]string(nil), f.manager.capturedPrompts...)
	f.manager.mu.Unlock()

	stored, loadErr := f.repo.GetTaskSession(f.ctx, f.sessionID)
	require.NoError(t, loadErr)
	observation.session = stored
	if receipt, found, receiptErr := models.LoadInitialPromptSubmission(stored.Metadata); receiptErr == nil && found {
		observation.receipt = receipt
	}
	if messages, messageErr := f.repo.ListMessages(f.ctx, f.sessionID); messageErr == nil {
		observation.messages = messages
	}
	if lastError, found := models.LoadLastAgentError(stored.Metadata); found {
		observation.lastError = &lastError
	}
	return observation
}

type freshStartWorkflowResetBarrierRepo struct {
	*sqliterepo.Repository
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *freshStartWorkflowResetBarrierRepo) UpdateResumeToken(
	ctx context.Context, sessionID, expectedExecID, resumeToken, lastMessageUUID string,
) error {
	r.once.Do(func() {
		close(r.entered)
		<-r.release
	})
	return r.Repository.UpdateResumeToken(ctx, sessionID, expectedExecID, resumeToken, lastMessageUUID)
}

type freshStartWorkflowProviderReadErrorRepo struct {
	*sqliterepo.Repository
	err   error
	calls atomic.Int32
}

func (r *freshStartWorkflowProviderReadErrorRepo) GetExecutorRunningBySessionID(
	ctx context.Context, sessionID string,
) (*models.ExecutorRunning, error) {
	if r.calls.Add(1) == 1 {
		return nil, r.err
	}
	return r.Repository.GetExecutorRunningBySessionID(ctx, sessionID)
}

type freshStartWorkflowQueueReadErrorRepository struct {
	messagequeue.Repository
	err error
}

func (r *freshStartWorkflowQueueReadErrorRepository) ListBySession(
	context.Context, string,
) ([]messagequeue.QueuedMessage, error) {
	return nil, r.err
}

type failingStrictPromptExpander struct{}

func (failingStrictPromptExpander) AppendReferenceExpansionsWithContext(
	_ context.Context, prompt string, _ *zap.Logger,
) (string, string) {
	return prompt, ""
}

func (failingStrictPromptExpander) AppendReferenceExpansionsToTrustedContext(
	_ context.Context, _ string, trustedContext string, _ *zap.Logger,
) string {
	return trustedContext
}

func (failingStrictPromptExpander) AppendReferenceExpansionsWithContextStrict(
	_ context.Context, prompt string, _ *zap.Logger,
) (string, string, error) {
	return prompt, "", errors.New("saved prompt read failed")
}
