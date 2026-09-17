package orchestrator

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestWorkflowAsyncStartFailure_PreservesPromptThroughRecovery(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "workflow-async-task"
		sessionID   = "workflow-async-session"
		stepID      = "workflow-async-step"
		profileID   = "workflow-async-profile"
		executionID = "workflow-async-execution"
	)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCreated)
	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	task.WorkflowStepID = stepID
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("update task workflow step: %v", err)
	}
	// The mock message creator does not persist rows in SQLite. Keep the task
	// description empty so recovery's unrelated initial-message backfill does
	// not fabricate a second row while this test counts the queue marker's
	// exactly-once transcript behavior.
	task.Description = ""
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("clear task description for transcript assertion: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.AgentProfileID = profileID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session profile: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	step := &wfmodels.WorkflowStep{ID: stepID, WorkflowID: "wf1", Name: "Async work"}
	stepGetter := newMockStepGetter()
	stepGetter.steps[stepID] = step
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{
		ID:          taskID,
		WorkflowID:  "wf1",
		Title:       "Async workflow task",
		Description: "task description",
		State:       v1.TaskStateInProgress,
	}

	startEntered := make(chan struct{})
	releaseStart := make(chan struct{})
	startReturned := make(chan struct{})
	secondStartEntered := make(chan struct{})
	startupErr := &lifecycle.BootstrapFailure{
		Code:   models.AgentErrorCauseCodeTransportUnavailable,
		Detail: "The agent could not start.",
		Cause:  errors.New("provider startup failed"),
	}
	var svc *Service
	var startCalls atomic.Int32
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true,
		promptDone:             make(chan struct{}),
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return &executor.LaunchAgentResponse{AgentExecutionID: executionID, WorktreePath: req.RepositoryPath}, nil
		},
		startAgentProcessFunc: func(_ context.Context, _ string) error {
			if startCalls.Add(1) == 1 {
				close(startEntered)
				<-releaseStart
				close(startReturned)
				return startupErr
			}
			close(secondStartEntered)
			return nil
		},
	}

	svc = createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)
	queueRepo := newWorkflowStartPromptSQLiteRepository(t, repo)
	svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	svc.messageCreator = &mockMessageCreator{}
	svc.turnService = &repoTurnService{repo: repo}
	svc.executor.SetOnAgentStartFailed(svc.handleAgentStartFailed)
	svc.executor.SetOnAgentProcessStarted(svc.handleAgentProcessStarted)
	svc.executor.SetOnAgentProcessStartFailed(svc.handleAgentProcessStartFailed)

	if err := svc.autoStartStepPrompt(ctx, taskID, session, step, "Run the async workflow step", false, true, nil); err != nil {
		t.Fatalf("autoStartStepPrompt returned launch error before asynchronous failure: %v", err)
	}
	select {
	case <-startEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for asynchronous startup")
	}
	if got := svc.messageQueue.GetStatus(ctx, sessionID).Count; got != 0 {
		t.Fatalf("queue populated before startup failure: %d", got)
	}

	close(releaseStart)
	select {
	case <-startReturned:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for startup failure callback")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		failedSession, getErr := repo.GetTaskSession(ctx, sessionID)
		if getErr == nil && failedSession != nil && failedSession.State == models.TaskSessionStateFailed {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	failedSession, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload failed session: %v", err)
	}
	if failedSession.State != models.TaskSessionStateFailed {
		t.Fatalf("session state = %s, want FAILED after asynchronous startup failure", failedSession.State)
	}
	if got := svc.messageQueue.GetStatus(ctx, sessionID).Count; got != 1 {
		t.Fatalf("expected one preserved workflow prompt after asynchronous failure, queue count = %d", got)
	}

	recoveryDone := make(chan error, 1)
	go func() {
		_, recoverErr := svc.RecoverSession(ctx, taskID, sessionID, "fresh_start")
		recoveryDone <- recoverErr
	}()
	select {
	case <-secondStartEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for explicit recovery startup")
	}
	resumeAttempt, ok := svc.resumeAttemptStore().current(sessionID)
	if !ok || resumeAttempt == nil {
		t.Fatal("explicit recovery did not retain a live resume attempt")
	}
	svc.handleAgentBootReady(ctx, watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: executionID,
		AttemptID:        resumeAttempt.identity(),
	})
	select {
	case recoverErr := <-recoveryDone:
		if recoverErr != nil {
			t.Fatalf("explicit recovery failed: %v", recoverErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for explicit recovery")
	}
	select {
	case <-agentMgr.promptDone:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for preserved prompt delivery")
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && svc.messageQueue.GetStatus(ctx, sessionID).Count != 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if got := svc.messageQueue.GetStatus(ctx, sessionID).Count; got != 0 {
		t.Fatalf("queue count after recovery delivery = %d, want 0", got)
	}
	if len(agentMgr.capturedPrompts) != 1 {
		t.Fatalf("provider prompt count = %d, want one preserved prompt", len(agentMgr.capturedPrompts))
	}
	if agentMgr.capturedPrompts[0] != "Run the async workflow step" {
		t.Fatalf("provider prompt = %q, want preserved workflow input", agentMgr.capturedPrompts[0])
	}
	messageCreator, ok := svc.messageCreator.(*mockMessageCreator)
	if !ok {
		t.Fatal("workflow service message creator has unexpected type")
	}
	if len(messageCreator.userMessages) != 1 {
		t.Fatalf("user transcript rows = %d, want one", len(messageCreator.userMessages))
	}
}

func TestWorkflowAsyncStartFailure_QueueFailureKeepsLaunchError(t *testing.T) {
	fixture := newWorkflowAsyncStartFailureFixture(t, &lifecycle.BootstrapFailure{
		Code:   models.AgentErrorCauseCodeTransportUnavailable,
		Detail: "The agent could not start.",
		Cause:  errors.New("provider startup failed"),
	})
	queueErr := errors.New("queue unavailable")
	fixture.svc.messageQueue = messagequeue.NewService(&workflowStartPromptQueueFailureRepository{
		Repository: newAuthoritativeMemoryRepository(fixture.repo),
		err:        queueErr,
	}, messagequeue.DefaultMaxPerSession, testLogger())

	if err := fixture.svc.autoStartStepPrompt(
		context.Background(), fixture.taskID, fixture.session, fixture.step,
		fixture.prompt, false, true, nil,
	); err != nil {
		t.Fatalf("autoStartStepPrompt returned launch error before asynchronous failure: %v", err)
	}
	select {
	case <-fixture.startEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for asynchronous startup")
	}
	close(fixture.releaseStart)
	select {
	case <-fixture.startReturned:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for startup failure callback")
	}
	waitForSessionState(t, fixture.repo, fixture.sessionID, models.TaskSessionStateFailed)

	if got := fixture.svc.messageQueue.GetStatus(context.Background(), fixture.sessionID).Count; got != 0 {
		t.Fatalf("queue count after failed persistence = %d, want 0", got)
	}
	failedSession, err := fixture.repo.GetTaskSession(context.Background(), fixture.sessionID)
	if err != nil {
		t.Fatalf("reload failed session: %v", err)
	}
	lastError, ok := models.LoadLastAgentError(failedSession.Metadata)
	if !ok {
		t.Fatal("typed launch error was lost when queue persistence failed")
	}
	if lastError.Code != models.LaunchErrorCategoryGenericLaunchFailure {
		t.Fatalf("last launch error code = %q, want generic launch failure", lastError.Code)
	}
	if lastError.Message != "The agent could not start." {
		t.Fatalf("last launch error message = %q, want safe bootstrap detail", lastError.Message)
	}
	if strings.Contains(lastError.Details, "provider startup failed") {
		t.Fatalf("last launch error details expose provider cause: %q", lastError.Details)
	}
}

func TestWorkflowAsyncStartFailure_QueueFullKeepsLaunchError(t *testing.T) {
	fixture := newWorkflowAsyncStartFailureFixture(t, errors.New("provider startup failed"))
	ctx := context.Background()
	fixture.svc.messageQueue.SetMaxPerSession(1)
	fixture.svc.messageQueue.SetAutoMergeEnabled(false)
	if err := fixture.svc.messageQueue.SetAutoRun(ctx, fixture.sessionID, false); err != nil {
		t.Fatalf("pause queue auto-run: %v", err)
	}
	if _, err := fixture.svc.messageQueue.QueueMessage(
		ctx, fixture.sessionID, fixture.taskID, "existing queued prompt", "",
		messagequeue.QueuedByUser, false, nil,
	); err != nil {
		t.Fatalf("seed full queue: %v", err)
	}

	if err := fixture.svc.autoStartStepPrompt(
		ctx, fixture.taskID, fixture.session, fixture.step,
		fixture.prompt, false, true, nil,
	); err != nil {
		t.Fatalf("autoStartStepPrompt returned launch error before asynchronous failure: %v", err)
	}
	select {
	case <-fixture.startEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for asynchronous startup")
	}
	close(fixture.releaseStart)
	select {
	case <-fixture.startReturned:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for startup failure callback")
	}
	waitForSessionState(t, fixture.repo, fixture.sessionID, models.TaskSessionStateFailed)

	status := fixture.svc.messageQueue.GetStatus(ctx, fixture.sessionID)
	if status.Count != 1 || status.Entries[0].Content != "existing queued prompt" {
		t.Fatalf("queue after full admission = %#v, want original single entry", status)
	}
	failedSession, err := fixture.repo.GetTaskSession(ctx, fixture.sessionID)
	if err != nil {
		t.Fatalf("reload failed session: %v", err)
	}
	lastError, ok := models.LoadLastAgentError(failedSession.Metadata)
	if !ok || lastError.Code != models.LaunchErrorCategoryGenericLaunchFailure {
		t.Fatalf("last launch error = %#v, want generic launch failure", lastError)
	}
}

func TestWorkflowAsyncStartFailure_PreservesPromptWhenTranscriptWriteFails(t *testing.T) {
	fixture := newWorkflowAsyncStartFailureFixture(t, errors.New("provider startup failed"))
	fixture.svc.messageCreator = &mockMessageCreator{userMessageErr: errors.New("transcript unavailable")}
	ctx := context.Background()

	if err := fixture.svc.autoStartStepPrompt(
		ctx, fixture.taskID, fixture.session, fixture.step,
		fixture.prompt, false, true, nil,
	); err != nil {
		t.Fatalf("autoStartStepPrompt returned launch error before asynchronous failure: %v", err)
	}
	select {
	case <-fixture.startEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for asynchronous startup")
	}
	close(fixture.releaseStart)
	select {
	case <-fixture.startReturned:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for startup failure callback")
	}
	waitForSessionState(t, fixture.repo, fixture.sessionID, models.TaskSessionStateFailed)

	status := fixture.svc.messageQueue.GetStatus(ctx, fixture.sessionID)
	if status.Count != 1 {
		t.Fatalf("queue count after transcript failure = %d, want 1", status.Count)
	}
	if recorded, _ := status.Entries[0].Metadata[metaKeyUserMessageRecorded].(bool); recorded {
		t.Fatal("queue incorrectly marked a failed transcript write as recorded")
	}
}

func TestWorkflowAsyncStartFailure_PreservesClassifiedBootstrapErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		handled bool
	}{
		{
			name: "generic bootstrap",
			err: &lifecycle.BootstrapFailure{
				Code:   models.AgentErrorCauseCodeTransportUnavailable,
				Detail: "The agent could not start.",
				Cause:  errors.New("transport unavailable"),
			},
		},
		{
			name: "authentication bootstrap",
			err: &lifecycle.BootstrapFailure{
				Code:   models.AgentErrorCauseCodeAuthenticationRequired,
				Detail: "Authentication is required.",
				Cause:  errors.New("auth required"),
			},
			handled: true,
		},
		{
			name: "managed runtime bootstrap",
			err: &routingerr.ManagedRuntimeStartupError{
				Code:    routingerr.CodeManagedRuntimeNpmResolution,
				Details: "managed runtime could not be resolved",
				Cause:   errors.New("runtime resolution failed"),
			},
			handled: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, ctx, attemptCtx, _ := newWorkflowStartPromptAttemptFixture(
				t, models.TaskSessionStateStarting, "workflow-classified-execution",
			)
			handled := svc.handleAgentStartFailed(
				attemptCtx, "workflow-fence-task", "workflow-fence-session",
				"workflow-classified-execution", tc.err, false,
			)
			if handled != tc.handled {
				t.Fatalf("handleAgentStartFailed = %v, want %v", handled, tc.handled)
			}
			if got := svc.messageQueue.GetStatus(ctx, "workflow-fence-session").Count; got != 1 {
				t.Fatalf("queue count = %d, want one preserved entry", got)
			}
		})
	}
}

func TestWorkflowAsyncStartFailure_DynamicFallbackOwnership(t *testing.T) {
	ctx := context.Background()
	svc, repo, _, attemptCtx, _ := newWorkflowStartPromptAttemptFixture(
		t, models.TaskSessionStateStarting, "workflow-fence-execution",
	)
	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, engine, true))
	decision := seedClaimedDynamicRoute(
		t, ctx, repo, engine, "workflow-fence-session", "workflow-fence-execution",
	)

	// A classified synchronous candidate failure is advanced by the conductor;
	// the successful candidate's process-start callback owns retirement of the
	// logical workflow attempt. A late failure for that candidate must not
	// replay the prompt after the fallback has succeeded.
	svc.handleAgentProcessStarted(
		attemptCtx, "workflow-fence-task", "workflow-fence-session", "workflow-fence-execution",
	)
	state, err := repo.LoadRouteState(ctx, "workflow-fence-session")
	if err != nil {
		t.Fatalf("load dynamic route state: %v", err)
	}
	if state == nil || state.Generation != decision.Generation || state.Status != dynamicRouteStatusActive {
		t.Fatalf("dynamic route state = %#v, want active generation %d", state, decision.Generation)
	}

	if handled := svc.handleAgentStartFailed(
		attemptCtx, "workflow-fence-task", "workflow-fence-session", "workflow-fence-execution",
		errors.New("final candidate startup failed after success callback"), false,
	); handled {
		t.Fatal("generic final candidate failure should remain owned by executor projection")
	}
	if got := svc.messageQueue.GetStatus(ctx, "workflow-fence-session").Count; got != 0 {
		t.Fatalf("late dynamic candidate failure queued %d entries", got)
	}
}

func TestWorkflowAsyncStartFailure_PersistedQueueSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	svc, _, _, attemptCtx, _ := newWorkflowStartPromptAttemptFixture(
		t, models.TaskSessionStateStarting, "workflow-restart-execution",
	)
	dbPath := filepath.Join(t.TempDir(), "workflow-queue.db")
	queue, queueDB := newWorkflowTransferQueue(t, dbPath)
	svc.messageQueue = queue
	// This fixture intentionally uses a queue-only database to verify durable
	// restart persistence. Entry-fenced admission requires the production
	// shared task/queue database, so exercise the ordinary queue path here.
	workflowStartPromptAttemptFromContext(attemptCtx).workflowEntryCaptured = false
	workflowStartPromptAttemptFromContext(attemptCtx).workflowEntryRequired = false

	if err := queue.SetAutoRun(ctx, "workflow-fence-session", false); err != nil {
		t.Fatalf("pause queue auto-run: %v", err)
	}
	svc.preserveWorkflowStartPromptAfterFailure(
		attemptCtx, "workflow-fence-task", "workflow-fence-session", "workflow-restart-execution",
	)
	status := queue.GetStatus(ctx, "workflow-fence-session")
	if status.Count != 1 || status.AutoRun {
		t.Fatalf("queue before restart = %#v, want one paused entry", status)
	}
	if err := queueDB.Close(); err != nil {
		t.Fatalf("close queue database: %v", err)
	}

	restartedQueue, restartedDB := newWorkflowTransferQueue(t, dbPath)
	defer func() { _ = restartedDB.Close() }()
	restarted := restartedQueue.GetStatus(ctx, "workflow-fence-session")
	if restarted.Count != 1 || restarted.AutoRun {
		t.Fatalf("queue after restart = %#v, want one paused entry", restarted)
	}
	if restarted.Entries[0].Content != "fenced prompt" {
		t.Fatalf("restarted queue content = %q, want preserved prompt", restarted.Entries[0].Content)
	}
}

func TestWorkflowAsyncStartFailure_SuccessAndSynchronousRejection(t *testing.T) {
	t.Run("successful startup retires attempt", func(t *testing.T) {
		fixture := newWorkflowAsyncStartFailureFixture(t, nil)
		if err := fixture.svc.autoStartStepPrompt(
			context.Background(), fixture.taskID, fixture.session, fixture.step,
			fixture.prompt, false, true, nil,
		); err != nil {
			t.Fatalf("autoStartStepPrompt returned launch error: %v", err)
		}
		select {
		case <-fixture.startEntered:
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for successful asynchronous startup")
		}
		close(fixture.releaseStart)
		select {
		case <-fixture.startReturned:
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for successful startup callback")
		}
		select {
		case <-fixture.processStarted:
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for successful process-start callback")
		}
		if got := fixture.svc.messageQueue.GetStatus(context.Background(), fixture.sessionID).Count; got != 0 {
			t.Fatalf("successful startup created %d recovery entries", got)
		}
	})

	t.Run("synchronous permanent rejection does not queue", func(t *testing.T) {
		ctx := context.Background()
		const (
			taskID    = "workflow-rejected-task"
			sessionID = "workflow-rejected-session"
			stepID    = "workflow-rejected-step"
		)
		repo := setupTestRepo(t)
		seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCreated)
		dbTask, err := repo.GetTask(ctx, taskID)
		if err != nil {
			t.Fatalf("get rejected task: %v", err)
		}
		dbTask.ProjectID = "office-project"
		dbTask.WorkflowStepID = stepID
		if err := repo.UpdateTask(ctx, dbTask); err != nil {
			t.Fatalf("mark rejected task as Office-owned: %v", err)
		}
		seedExecutorRunning(t, repo, sessionID, taskID, "workflow-rejected-execution")

		taskRepo := newMockTaskRepo()
		taskRepo.tasks[taskID] = &v1.Task{ID: taskID, Title: "Rejected task", State: v1.TaskStateInProgress}
		step := &wfmodels.WorkflowStep{ID: stepID, WorkflowID: "wf1", Name: "Rejected step"}
		stepGetter := newMockStepGetter()
		stepGetter.steps[stepID] = step
		svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, &mockAgentManager{
			repoForExecutionLookup: repo,
		})
		svc.messageCreator = &mockMessageCreator{}
		session, err := repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			t.Fatalf("get rejected session: %v", err)
		}
		session.AgentProfileID = "workflow-rejected-profile"
		if err := repo.UpdateTaskSession(ctx, session); err != nil {
			t.Fatalf("set rejected session profile: %v", err)
		}

		err = svc.autoStartStepPrompt(ctx, taskID, session, step, "Do not retry this", false, true, nil)
		if err == nil || !strings.Contains(err.Error(), "office tasks must be started through Office") {
			t.Fatalf("rejection error = %v, want Office scheduler rejection", err)
		}
		if got := svc.messageQueue.GetStatus(ctx, sessionID).Count; got != 0 {
			t.Fatalf("synchronous rejection created %d recovery entries", got)
		}
	})
}

type workflowStartPromptQueueFailureRepository struct {
	messagequeue.Repository
	err error
}

func (r *workflowStartPromptQueueFailureRepository) Insert(
	context.Context, *messagequeue.QueuedMessage, int,
) error {
	return r.err
}

func (r *workflowStartPromptQueueFailureRepository) InsertForSession(
	context.Context, messagequeue.QueueSessionIdentity, *messagequeue.QueuedMessage, int,
) error {
	return r.err
}

type workflowAsyncStartFailureFixture struct {
	svc       *Service
	repo      *sqliterepo.Repository
	agentMgr  *mockAgentManager
	session   *models.TaskSession
	step      *wfmodels.WorkflowStep
	taskID    string
	sessionID string
	prompt    string

	startEntered       chan struct{}
	releaseStart       chan struct{}
	startReturned      chan struct{}
	secondStartEntered chan struct{}
	processStarted     chan struct{}
}

func newWorkflowAsyncStartFailureFixture(
	t *testing.T,
	startupErr error,
) *workflowAsyncStartFailureFixture {
	t.Helper()
	ctx := context.Background()
	const (
		taskID      = "workflow-async-task"
		sessionID   = "workflow-async-session"
		stepID      = "workflow-async-step"
		profileID   = "workflow-async-profile"
		executionID = "workflow-async-execution"
	)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateCreated)
	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	task.WorkflowStepID = stepID
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("update task workflow step: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.AgentProfileID = profileID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session profile: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	step := &wfmodels.WorkflowStep{ID: stepID, WorkflowID: "wf1", Name: "Async work"}
	stepGetter := newMockStepGetter()
	stepGetter.steps[stepID] = step
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{
		ID:          taskID,
		WorkflowID:  "wf1",
		Title:       "Async workflow task",
		Description: "task description",
		State:       v1.TaskStateInProgress,
	}

	startEntered := make(chan struct{})
	releaseStart := make(chan struct{})
	startReturned := make(chan struct{})
	secondStartEntered := make(chan struct{})
	processStarted := make(chan struct{})
	var startCalls atomic.Int32
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true,
		promptDone:             make(chan struct{}),
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return &executor.LaunchAgentResponse{AgentExecutionID: executionID, WorktreePath: req.RepositoryPath}, nil
		},
		startAgentProcessFunc: func(_ context.Context, _ string) error {
			if startCalls.Add(1) == 1 {
				close(startEntered)
				<-releaseStart
				close(startReturned)
				return startupErr
			}
			close(secondStartEntered)
			return nil
		},
	}

	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)
	queueRepo := newWorkflowStartPromptSQLiteRepository(t, repo)
	svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	svc.messageCreator = &mockMessageCreator{}
	svc.turnService = &repoTurnService{repo: repo}
	svc.executor.SetOnAgentStartFailed(svc.handleAgentStartFailed)
	var processStartedOnce sync.Once
	svc.executor.SetOnAgentProcessStarted(func(
		callbackCtx context.Context, callbackTaskID, callbackSessionID, callbackExecutionID string,
	) {
		svc.handleAgentProcessStarted(callbackCtx, callbackTaskID, callbackSessionID, callbackExecutionID)
		processStartedOnce.Do(func() { close(processStarted) })
	})
	svc.executor.SetOnAgentProcessStartFailed(svc.handleAgentProcessStartFailed)

	return &workflowAsyncStartFailureFixture{
		svc:                svc,
		repo:               repo,
		agentMgr:           agentMgr,
		session:            session,
		step:               step,
		taskID:             taskID,
		sessionID:          sessionID,
		prompt:             "Run the async workflow step",
		startEntered:       startEntered,
		releaseStart:       releaseStart,
		startReturned:      startReturned,
		secondStartEntered: secondStartEntered,
		processStarted:     processStarted,
	}
}

func newWorkflowStartPromptAttemptFixture(
	t *testing.T,
	sessionState models.TaskSessionState,
	executionID string,
) (*Service, *sqliterepo.Repository, context.Context, context.Context, string) {
	t.Helper()
	ctx := context.Background()
	const (
		taskID    = "workflow-fence-task"
		sessionID = "workflow-fence-session"
		stepID    = "workflow-fence-step"
	)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, sessionState)
	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	task.WorkflowStepID = stepID
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("update task workflow step: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	stepGetter := newMockStepGetter()
	stepGetter.steps[stepID] = &wfmodels.WorkflowStep{ID: stepID, WorkflowID: "wf1", Name: "Fence step"}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{
		ID:          taskID,
		WorkflowID:  "wf1",
		Title:       "Fence task",
		Description: "fenced task",
		State:       v1.TaskStateInProgress,
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)
	queueRepo := newWorkflowStartPromptSQLiteRepository(t, repo)
	svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	svc.turnService = &repoTurnService{repo: repo}
	turnID := svc.startTurnForSession(ctx, sessionID)
	if turnID == "" {
		t.Fatal("expected active turn for preservation fixture")
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get preservation session: %v", err)
	}
	queueIdentity, workflowEntry, workflowEntryCaptured := svc.captureWorkflowStartPromptAdmission(ctx, taskID, session)
	attempt := newWorkflowStartPromptAttemptWithAdmission(
		taskID, sessionID,
		workflowMessageOrigin{StepID: stepID, StepName: "Fence step"},
		"fenced prompt", false, nil, nil, "", true, true,
		queueIdentity, workflowEntry, workflowEntryCaptured,
	)
	return svc, repo, ctx, bindWorkflowStartPromptAttemptTurn(
		withWorkflowStartPromptAttempt(ctx, attempt), turnID,
	), turnID
}
