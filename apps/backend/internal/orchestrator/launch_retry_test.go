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

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
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
}
