package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

const (
	suspendedFocusTaskID    = "task-focus-suspended-admission"
	suspendedFocusSessionID = "session-focus-suspended-admission"
)

// gatedRestoreAgent holds the restore of an idle-suspended session until the
// test releases it, as a slow executor startup does.
type gatedRestoreAgent struct {
	*mockAgentManager
	launchEntered chan struct{}
	releaseLaunch chan struct{}
	enterOnce     sync.Once
	releaseOnce   sync.Once
}

func (a *gatedRestoreAgent) release() { a.releaseOnce.Do(func() { close(a.releaseLaunch) }) }

func newSuspendedFocusService(t *testing.T) (*Service, *gatedRestoreAgent) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, suspendedFocusTaskID, suspendedFocusSessionID, models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, suspendedFocusSessionID)
	require.NoError(t, err)
	session.AgentProfileID = "claude-acp"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	now := time.Now().UTC()
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: suspendedFocusSessionID, SessionID: suspendedFocusSessionID, TaskID: suspendedFocusTaskID,
		AgentExecutionID: "execution-focus-suspended", Status: models.ExecutorRunningStatusStopped,
		IdleSuspensionState: models.ExecutorIdleSuspensionSuspended,
		ExecutorID:          "executor-local", Resumable: true, ResumeToken: "suspended-conversation-token",
		CreatedAt: now, UpdatedAt: now,
	}))

	agent := &gatedRestoreAgent{launchEntered: make(chan struct{}), releaseLaunch: make(chan struct{})}
	agent.mockAgentManager = &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentReadyFn:         func(context.Context, string) bool { return true },
		launchAgentFunc: func(launchCtx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			agent.enterOnce.Do(func() { close(agent.launchEntered) })
			<-agent.releaseLaunch
			restored, getErr := repo.GetTaskSession(launchCtx, req.SessionID)
			if getErr != nil {
				return nil, getErr
			}
			restored.State = models.TaskSessionStateWaitingForInput
			if updateErr := repo.UpdateTaskSession(launchCtx, restored); updateErr != nil {
				return nil, updateErr
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: "execution-focus-restored"}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[suspendedFocusTaskID] = &v1.Task{ID: suspendedFocusTaskID, State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agent.mockAgentManager)
	svc.executor = withTestCapabilities(executor.NewExecutor(agent.mockAgentManager, repo, testLogger(), executor.ExecutorConfig{}))
	svc.turnService = &inactiveTurnService{}
	return svc, agent
}

// @covers AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.9
func TestFocusResumeOfSuspendedSessionKeepsAdmissionFree(t *testing.T) {
	ctx := context.Background()
	svc, agent := newSuspendedFocusService(t)

	type focusOutcome struct {
		resumed bool
		err     error
	}
	outcome := make(chan focusOutcome, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, _, resumed, err := svc.focusTaskSession(ctx, suspendedFocusTaskID, suspendedFocusSessionID)
		outcome <- focusOutcome{resumed: resumed, err: err}
	}()
	t.Cleanup(func() {
		agent.release()
		<-finished
	})

	waitForSignal(t, agent.launchEntered, "the suspended session restore")
	requireSessionAdmissionFree(t, svc, suspendedFocusSessionID)

	agent.release()
	select {
	case got := <-outcome:
		require.NoError(t, got.err)
		require.True(t, got.resumed, "focus resumes the idle-suspended session")
	case <-time.After(3 * time.Second):
		t.Fatal("focus resume did not finish after the restore was released")
	}
}
