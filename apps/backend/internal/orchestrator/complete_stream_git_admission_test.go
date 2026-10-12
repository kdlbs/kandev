package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestCompleteStreamGitSnapshotDoesNotHoldPromptAdmission(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSharedGitSnapshotEnvironment(t, repo, "task-complete-git", "env-complete-git", "session-complete-git")
	session, err := repo.GetTaskSession(ctx, "session-complete-git")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	agent := &mockAgentManager{getGitStatusFreshFunc: func(context.Context, string) (*client.GitStatusResult, error) {
		close(entered)
		<-release
		return &client.GitStatusResult{Success: true, Branch: "main", BranchAdditions: 1}, nil
	}}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	go func() {
		defer close(finished)
		svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
			TaskID: "task-complete-git", SessionID: "session-complete-git",
			Data: &lifecycle.AgentStreamEventData{Type: agentEventComplete},
		})
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("completion did not reach Git snapshot")
	}
	admitted := make(chan struct{})
	go func() {
		guard := svc.lockCancelInFlightGuard("session-complete-git")
		defer guard.release()
		close(admitted)
	}()
	select {
	case <-admitted:
	case <-time.After(time.Second):
		unblock()
		<-finished
		<-admitted
		t.Fatal("Git snapshot blocked prompt admission after completion")
	}
	select {
	case <-finished:
		t.Fatal("completion returned before its snapshot was persisted")
	default:
	}
	session.State = models.TaskSessionStateRunning
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	unblock()
	<-finished
	session, err = repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	snapshots, err := repo.GetGitSnapshotsBySession(ctx, session.ID, 0)
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
}

func TestCompleteStreamGitSnapshotPrecedesOfficeRuntimeStop(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSharedGitSnapshotEnvironment(t, repo, "task-office-git", "env-office-git", "session-office-git")
	task, err := repo.GetTask(ctx, "task-office-git")
	require.NoError(t, err)
	task.ProjectID = "office-project"
	require.NoError(t, repo.UpdateTask(ctx, task))
	task, err = repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.True(t, task.IsFromOffice)
	session, err := repo.GetTaskSession(ctx, "session-office-git")
	require.NoError(t, err)
	session.AgentProfileID = "office-profile"
	session.AgentExecutionID = "office-execution"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: session.ID, SessionID: session.ID, TaskID: task.ID,
		AgentExecutionID: session.AgentExecutionID, Status: "ready",
	}))
	stopped := false
	agent := &mockAgentManager{
		getGitStatusFreshFunc: func(context.Context, string) (*client.GitStatusResult, error) {
			return &client.GitStatusResult{Success: true, Branch: "main", BranchAdditions: 1}, nil
		},
		stopAgentFunc: func(ctx context.Context, executionID string, _ bool) error {
			require.Equal(t, "office-execution", executionID)
			snapshots, err := repo.GetGitSnapshotsBySession(ctx, session.ID, 0)
			require.NoError(t, err)
			require.Len(t, snapshots, 1, "snapshot must survive runtime teardown")
			stopped = true
			return nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: task.ID, SessionID: session.ID,
		Data: &lifecycle.AgentStreamEventData{Type: agentEventComplete},
	})
	require.True(t, stopped)
}
