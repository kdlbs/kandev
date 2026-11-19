package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestIsTransientLaunchFailure(t *testing.T) {
	for _, message := range []string{
		"agentctl not ready: context deadline exceeded",
		"failed to launch workspace for prepared session",
		"workspace execution is not ready: timed out",
	} {
		if !isTransientLaunchFailure(errors.New(message)) {
			t.Fatalf("%q was not classified as transient", message)
		}
	}
	for _, err := range []error{
		nil,
		errors.New("fatal: couldn't find remote ref feature/deleted-pr"),
		errors.New("repository has no clone URL"),
		context.Canceled,
		fmt.Errorf("agentctl not ready: %w", context.Canceled),
		errors.New("agentctl not ready: context canceled"),
	} {
		if isTransientLaunchFailure(err) {
			t.Fatalf("%v was classified as transient", err)
		}
	}
	if !isTransientLaunchFailure(context.DeadlineExceeded) {
		t.Fatal("context.DeadlineExceeded was not classified as transient")
	}
}

// The self-healing path retries once and stops: a second transient failure for
// the same session keeps the task FAILED.
func TestScheduleTransientLaunchRetry_RetriesOnce(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)
	require.NoError(t, repo.UpdateTaskState(ctx, "task1", v1.TaskStateFailed))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.launchRetryDelay = time.Millisecond

	var mu sync.Mutex
	attempts := 0
	svc.launchRetry = func(context.Context, string, string) error {
		mu.Lock()
		defer mu.Unlock()
		attempts++
		return errors.New("still unavailable")
	}

	transient := errors.New("agentctl not ready: context deadline exceeded")
	svc.scheduleTransientLaunchRetry(ctx, "task1", "session1", transient)
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return attempts == 1
	}, time.Second, 5*time.Millisecond)

	svc.scheduleTransientLaunchRetry(ctx, "task1", "session1", transient)
	require.Never(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return attempts > 1
	}, 100*time.Millisecond, 5*time.Millisecond, "the automatic retry must run at most once per session")
}

func TestTransientLaunchRetry_RecordsRealAgentLaunchFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-real-launch", "session-real-launch", models.TaskSessionStateCreated)
	launchErr := errors.New("agentctl not ready: context deadline exceeded")
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return nil, launchErr
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-real-launch"] = &v1.Task{
		ID: "task-real-launch", WorkspaceID: "ws1", WorkflowID: "wf1", State: v1.TaskStateInProgress,
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor.SetOnSessionStateChange(func(callbackCtx context.Context, taskID, sessionID string, state models.TaskSessionState, errorMessage string) error {
		svc.updateTaskSessionState(callbackCtx, taskID, sessionID, state, errorMessage, true)
		return nil
	})
	svc.executor.SetOnTaskStateChange(func(callbackCtx context.Context, taskID string, state v1.TaskState) error {
		_, _, err := repo.UpdateTaskStateIfNotArchived(callbackCtx, taskID, state)
		return err
	})
	svc.executor.SetOnLaunchFailureRecorded(svc.handleRecordedLaunchFailure)
	svc.launchRetryDelay = time.Millisecond
	started := make(chan struct{}, 1)
	svc.launchRetry = func(context.Context, string, string) error {
		started <- struct{}{}
		return nil
	}

	_, err := svc.StartCreatedSession(ctx, "task-real-launch", "session-real-launch", "profile1", "start", true, false, false, nil, nil)
	require.ErrorIs(t, err, launchErr)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("a real executor launch failure did not schedule the automatic retry")
	}
	svc.launchRetries.Wait()
}

func TestTransientLaunchRetry_DoesNotRetryReplacementFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-retry-replacement", "session-retry-replacement", models.TaskSessionStateFailed)
	require.NoError(t, repo.UpdateTaskState(ctx, "task-retry-replacement", v1.TaskStateFailed))
	session, err := repo.GetTaskSession(ctx, "session-retry-replacement")
	require.NoError(t, err)
	session.Metadata[models.SessionMetaKeyAgentStartAttemptID] = "attempt-a"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.launchRetryDelay = 300 * time.Millisecond
	started := make(chan struct{}, 1)
	svc.launchRetry = func(context.Context, string, string) error {
		started <- struct{}{}
		return nil
	}
	svc.scheduleTransientLaunchRetry(ctx, "task-retry-replacement", "session-retry-replacement", errors.New("agentctl not ready"))

	// A user starts attempt B and it fails before attempt A's timer expires.
	session, err = repo.GetTaskSession(ctx, "session-retry-replacement")
	require.NoError(t, err)
	session.Metadata[models.SessionMetaKeyAgentStartAttemptID] = "attempt-b"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	svc.launchRetries.Wait()
	select {
	case <-started:
		t.Fatal("the retry for attempt A acted on failed attempt B")
	default:
	}
}

func TestTransientLaunchRetryContextCancelsResumeAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ctx = withTransientLaunchRetryContext(ctx, transientLaunchFailureIdentity{updatedAt: time.Now().UTC()})
	attempt, started := newResumeAttemptRegistry().begin(ctx, "task", "session")
	require.True(t, started)
	cancel()
	select {
	case <-attempt.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("the retry deadline did not cancel the owned resume attempt")
	}
}

func TestResetTransientLaunchRetriesStartsNewWorkerGeneration(t *testing.T) {
	svc := &Service{}
	svc.stopTransientLaunchRetries()

	svc.resetTransientLaunchRetries()
	_, workers, started := svc.beginTransientLaunchRetryWorker()
	require.True(t, started, "a restarted service accepts retry workers")
	workers.Done()
	svc.stopTransientLaunchRetries()
}

// A transient timeout while resuming a session with history must retry
// through the real RecoverSession as a native resume: the stored resume token
// reaches the relaunch, is never cleared, and the session leaves FAILED.
func TestTransientLaunchRetry_RealRecoverSessionPreservesResumeToken(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	const taskID, sessionID = "task-retry", "session-retry"
	const resumeToken = "example-native-conversation"

	launched := make(chan *executor.LaunchAgentRequest, 2)
	started := false
	agentMgr := &sessionUpdatingAgentManager{
		mockAgentManager: &mockAgentManager{
			launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				launched <- req
				return &executor.LaunchAgentResponse{AgentExecutionID: "exec-retry", Status: v1.AgentStatusStarting}, nil
			},
		},
		repo: repo, sessionID: sessionID, taskID: taskID, onStartCalled: &started,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = withTestCapabilities(executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{}))
	svc.launchRetryDelay = time.Millisecond

	seedRetryableResumeSession(t, repo, taskID, sessionID, resumeToken)
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, WorkspaceID: "ws1", WorkflowID: "wf1", State: v1.TaskStateInProgress}
	// Production writes task state through the same database the service reads.
	taskRepo.updateIfSessionState = func(ctx context.Context, id, _ string, _ models.TaskSessionState, state v1.TaskState) (bool, error) {
		taskRepo.mu.Lock()
		taskRepo.tasks[id].State = state
		taskRepo.mu.Unlock()
		return true, repo.UpdateTaskState(ctx, id, state)
	}

	err := svc.handleSessionLaunchFailure(ctx, taskID, sessionID,
		fmt.Errorf("failed to resume session: agentctl not ready: %w", context.DeadlineExceeded))
	require.Error(t, err)

	select {
	case req := <-launched:
		require.Equal(t, sessionID, req.SessionID)
		require.Equal(t, resumeToken, req.ACPSessionID,
			"the automatic retry resumes the native conversation instead of starting fresh")
	case <-time.After(5 * time.Second):
		t.Fatal("the automatic retry did not relaunch the session")
	}
	require.Eventually(t, func() bool {
		session, getErr := repo.GetTaskSession(ctx, sessionID)
		return getErr == nil && session.State != models.TaskSessionStateFailed
	}, 5*time.Second, 10*time.Millisecond, "the retried session leaves FAILED")

	running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, resumeToken, running.ResumeToken, "the retry never clears the resume token")
	svc.launchRetries.Wait()
	_, marked := svc.transientLaunchRetries.Load(sessionID)
	require.True(t, marked, "the retry marker outlives the retried launch")
	require.False(t, svc.claimTransientLaunchRetry(sessionID),
		"a retried launch that fails again is not retried a second time")
}

func TestTransientLaunchRetryWaitsForFreshAgentStart(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	const taskID, sessionID = "task-retry-fresh", "session-retry-fresh"
	seedRetryableResumeSession(t, repo, taskID, sessionID, "")
	require.NoError(t, repo.UpdateTaskState(ctx, taskID, v1.TaskStateFailed))
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.State = models.TaskSessionStateFailed
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	taskRepo.tasks[taskID] = &v1.Task{
		ID: taskID, WorkspaceID: "ws1", WorkflowID: "wf1", State: v1.TaskStateFailed,
	}
	taskRepo.updateIfSessionState = func(updateCtx context.Context, id, _ string, _ models.TaskSessionState, state v1.TaskState) (bool, error) {
		taskRepo.mu.Lock()
		taskRepo.tasks[id].State = state
		taskRepo.mu.Unlock()
		return true, repo.UpdateTaskState(updateCtx, id, state)
	}

	startEntered := make(chan context.Context, 1)
	startResult := make(chan error, 1)
	releaseStart := make(chan struct{})
	var releaseStartOnce sync.Once
	defer releaseStartOnce.Do(func() { close(releaseStart) })
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-fresh-retry", Status: v1.AgentStatusStarting}, nil
		},
		startAgentProcessFunc: func(startCtx context.Context, _ string) error {
			session, getErr := repo.GetTaskSession(context.Background(), sessionID)
			if getErr != nil {
				startResult <- getErr
				return getErr
			}
			session.State = models.TaskSessionStateWaitingForInput
			session.UpdatedAt = time.Now().UTC()
			if updateErr := repo.UpdateTaskSession(context.Background(), session); updateErr != nil {
				startResult <- updateErr
				return updateErr
			}
			// Readiness can be visible before the asynchronous start call returns.
			startEntered <- startCtx
			select {
			case <-releaseStart:
				startResult <- nil
				return nil
			case <-startCtx.Done():
				startResult <- startCtx.Err()
				return startCtx.Err()
			}
		},
		isAgentReadyFn: func(context.Context, string) bool { return true },
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = withTestCapabilities(executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{}))
	svc.launchRetryDelay = time.Millisecond
	svc.scheduleTransientLaunchRetry(ctx, taskID, sessionID, errors.New("agentctl not ready"))

	var startCtx context.Context
	select {
	case startCtx = <-startEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("automatic fresh-start retry did not start the agent process")
	}
	require.Never(t, func() bool { return startCtx.Err() != nil }, 100*time.Millisecond, 5*time.Millisecond,
		"the retry worker must keep startup context alive until StartAgentProcess returns")
	releaseStartOnce.Do(func() { close(releaseStart) })
	select {
	case startErr := <-startResult:
		require.NoError(t, startErr)
	case <-time.After(5 * time.Second):
		t.Fatal("agent startup did not settle")
	}
	svc.launchRetries.Wait()
}

func seedRetryableResumeSession(t *testing.T, repo *sqliterepo.Repository, taskID, sessionID, resumeToken string) {
	t.Helper()
	ctx := context.Background()
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateStarting)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateExecutor(ctx, &models.Executor{
		ID: "executor-retry", Name: "Worktree", Type: models.ExecutorTypeWorktree,
		Status: models.ExecutorStatusActive, Resumable: true,
	}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-retry", WorkspaceID: "ws1", Name: "backend", SourceType: "local",
		LocalPath: t.TempDir(), DefaultBranch: "main", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-retry", TaskID: taskID, RepositoryID: "repo-retry",
		BaseBranch: "main", CreatedAt: now, UpdatedAt: now,
	}))
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileID = "profile-retry"
	session.ExecutorID = "executor-retry"
	session.RepositoryID = "repo-retry"
	session.BaseBranch = "main"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running-retry", SessionID: sessionID, TaskID: taskID,
		AgentExecutionID: "exec-before-retry", ResumeToken: resumeToken, Resumable: true,
		CreatedAt: now, UpdatedAt: now,
	}))
}

// The retry marker lives for its TTL and expired markers are swept, so the
// registry neither grows without bound nor blocks self-healing forever.
func TestClaimTransientLaunchRetry_TTL(t *testing.T) {
	svc := &Service{}
	require.True(t, svc.claimTransientLaunchRetry("session1"))
	require.False(t, svc.claimTransientLaunchRetry("session1"),
		"a second transient failure inside the TTL stays FAILED")

	svc.transientLaunchRetries.Store("session1", time.Now().Add(-2*transientLaunchRetryTTL))
	require.True(t, svc.claimTransientLaunchRetry("session1"),
		"an expired marker no longer blocks the automatic retry")

	svc.transientLaunchRetries.Store("session-old", time.Now().Add(-2*transientLaunchRetryTTL))
	require.True(t, svc.claimTransientLaunchRetry("session2"))
	_, kept := svc.transientLaunchRetries.Load("session-old")
	require.False(t, kept, "expired markers of other sessions are swept")
}

func TestScheduleTransientLaunchRetry_SkipsNonTransientFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)
	require.NoError(t, repo.UpdateTaskState(ctx, "task1", v1.TaskStateFailed))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.launchRetryDelay = time.Millisecond

	called := atomic.Bool{}
	svc.launchRetry = func(context.Context, string, string) error {
		called.Store(true)
		return nil
	}

	svc.scheduleTransientLaunchRetry(ctx, "task1", "session1", errors.New("fatal: couldn't find remote ref feature/deleted-pr"))
	time.Sleep(20 * time.Millisecond)
	if called.Load() {
		t.Fatal("a non-transient launch failure must not schedule a retry")
	}
}

// Stop cancels a retry that is still waiting out its delay, and no retry is
// scheduled once shutdown has begun.
func TestStopTransientLaunchRetriesCancelsPendingRetry(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)
	seedTaskAndSession(t, repo, "task2", "session2", models.TaskSessionStateFailed)
	require.NoError(t, repo.UpdateTaskState(ctx, "task1", v1.TaskStateFailed))
	require.NoError(t, repo.UpdateTaskState(ctx, "task2", v1.TaskStateFailed))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.launchRetryDelay = time.Hour

	called := atomic.Bool{}
	svc.launchRetry = func(context.Context, string, string) error {
		called.Store(true)
		return nil
	}

	transient := errors.New("agentctl not ready")
	svc.scheduleTransientLaunchRetry(ctx, "task1", "session1", transient)
	svc.stopTransientLaunchRetries()
	svc.launchRetries.Wait()
	svc.scheduleTransientLaunchRetry(ctx, "task2", "session2", transient)
	svc.launchRetries.Wait()

	require.False(t, called.Load(), "no relaunch may run once the service is stopping")
	_, claimed := svc.transientLaunchRetries.Load("session2")
	require.False(t, claimed, "a refused retry does not consume the session's attempt")
}

// A user stop while agentctl is still starting surfaces as a cancelled launch
// that the executor records FAILED; it is never retried.
func TestTransientLaunchRetry_CancelledLaunchIsNotRetried(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-cancelled-launch", "session-cancelled-launch", models.TaskSessionStateCreated)
	launchErr := fmt.Errorf("agentctl not ready: %w", context.Canceled)
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return nil, launchErr
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-cancelled-launch"] = &v1.Task{
		ID: "task-cancelled-launch", WorkspaceID: "ws1", WorkflowID: "wf1", State: v1.TaskStateInProgress,
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor.SetOnSessionStateChange(func(callbackCtx context.Context, taskID, sessionID string, state models.TaskSessionState, errorMessage string) error {
		svc.updateTaskSessionState(callbackCtx, taskID, sessionID, state, errorMessage, true)
		return nil
	})
	svc.executor.SetOnTaskStateChange(func(callbackCtx context.Context, taskID string, state v1.TaskState) error {
		_, _, err := repo.UpdateTaskStateIfNotArchived(callbackCtx, taskID, state)
		return err
	})
	svc.executor.SetOnLaunchFailureRecorded(svc.handleRecordedLaunchFailure)
	svc.launchRetryDelay = time.Millisecond
	called := atomic.Bool{}
	svc.launchRetry = func(context.Context, string, string) error {
		called.Store(true)
		return nil
	}

	_, err := svc.StartCreatedSession(ctx, "task-cancelled-launch", "session-cancelled-launch", "profile1", "start", true, false, false, nil, nil)
	require.ErrorIs(t, err, context.Canceled)
	svc.stopTransientLaunchRetries()
	require.False(t, called.Load(), "a cancelled launch is not retried")
	_, claimed := svc.transientLaunchRetries.Load("session-cancelled-launch")
	require.False(t, claimed)
}

// Stop closes the retries under the service mutex and cancels the closed
// generation afterwards; a Start that runs in between keeps its new generation.
func TestTransientLaunchRetriesSurviveStartBetweenCloseAndAwait(t *testing.T) {
	svc := &Service{logger: testLogger()}
	svc.resetTransientLaunchRetries()
	_, workers, started := svc.beginTransientLaunchRetryWorker()
	require.True(t, started)
	workers.Done()

	cancelClosed, closedWorkers := svc.closeTransientLaunchRetries()
	svc.resetTransientLaunchRetries()
	svc.awaitTransientLaunchRetries(cancelClosed, closedWorkers)

	workerCtx, workers, started := svc.beginTransientLaunchRetryWorker()
	require.True(t, started, "the restarted service accepts automatic retries")
	workers.Done()
	require.NoError(t, workerCtx.Err(), "the late cancel did not reach the new generation")
	svc.stopTransientLaunchRetries()
	require.Error(t, workerCtx.Err())
}

// A real Stop then Start keeps automatic retries available.
func TestServiceRestartReopensTransientLaunchRetries(t *testing.T) {
	log := testLogger()
	repo := setupTestRepo(t)
	svc := NewService(DefaultServiceConfig(), bus.NewMemoryEventBus(log), &mockAgentManager{repoForExecutionLookup: repo},
		newMockTaskRepo(), repo, nil, nil, nil, log)
	svc.SetWorkflowStepGetter(newMockStepGetter())
	svc.SetBackgroundProbe(&spyBackgroundProbe{})
	svc.SetTurnService(taskservice.NewService(taskservice.Repos{
		Tasks: repo, TaskRepos: repo, Workspaces: repo, Workflows: repo, Messages: repo,
		Turns: repo, Sessions: repo, Executors: repo, TaskEnvironments: repo,
	}, bus.NewMemoryEventBus(log), log, taskservice.RepositoryDiscoveryConfig{}))
	ctx := context.Background()

	require.NoError(t, svc.Start(ctx))
	require.NoError(t, svc.Stop())
	_, _, open := svc.beginTransientLaunchRetryWorker()
	require.False(t, open, "a stopped service accepts no automatic retry")

	require.NoError(t, svc.Start(ctx))
	t.Cleanup(func() { _ = svc.Stop() })
	workerCtx, workers, open := svc.beginTransientLaunchRetryWorker()
	require.True(t, open, "a restarted service accepts automatic retries")
	workers.Done()
	require.NoError(t, workerCtx.Err(), "the restarted worker context is live")
}
