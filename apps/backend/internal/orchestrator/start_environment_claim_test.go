package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func seedStartClaimTask(t *testing.T) (*sqliterepo.Repository, *Service) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Workspace", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Workflow", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task1", WorkspaceID: "ws1", WorkflowID: "wf1", Title: "Task", State: v1.TaskStateCreated, CreatedAt: now, UpdatedAt: now}))
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", WorkspaceID: "ws1", Title: "Task", State: v1.TaskStateCreated}
	manager := &mockAgentManager{launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		return nil, errors.New("runtime launch was not expected")
	}}
	return repo, createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, manager)
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.1
func TestStartReleasesEnvironmentClaimOfItsSessionOnEarlyReturn(t *testing.T) {
	ctx := context.Background()
	repo, svc := seedStartClaimTask(t)
	svc.repo = &quickChatLaunchReloadFailureStore{sessionExecutorStore: svc.repo, failAt: 2}

	_, err := svc.LaunchSession(ctx, &LaunchSessionRequest{TaskID: "task1", Intent: IntentStart, AgentProfileID: "profile1", ProfileExplicit: true})
	require.ErrorContains(t, err, "failed to reload launch session")

	sessions, err := repo.ListTaskSessions(ctx, "task1")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, models.TaskSessionStateCreated, sessions[0].State, "release must not change the Kanban session state")
	environment, err := repo.GetTaskEnvironmentByTaskID(ctx, "task1")
	require.NoError(t, err)
	require.NotNil(t, environment)
	require.Equal(t, sessions[0].TaskEnvironmentID, environment.ID)
	require.Equal(t, models.TaskEnvironmentStatusFailed, environment.Status)
	require.Empty(t, environment.MaterializationSessionID)
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.1
func TestReleaseStartEnvironmentClaimKeepsDeferredAndReusedClaims(t *testing.T) {
	ctx := context.Background()
	repo, svc := seedStartClaimTask(t)
	owner := &models.TaskSession{ID: "session-owner", TaskID: "task1"}
	require.NoError(t, repo.CreateTaskSessionWithWorkspaceBinding(ctx, owner, &models.TaskEnvironment{
		TaskID: "task1", ExecutorType: string(models.ExecutorTypeLocal),
	}))
	assertClaim := func(status models.TaskEnvironmentStatus, ownerID string) {
		t.Helper()
		environment, err := repo.GetTaskEnvironment(ctx, owner.TaskEnvironmentID)
		require.NoError(t, err)
		require.Equal(t, status, environment.Status)
		require.Equal(t, ownerID, environment.MaterializationSessionID)
	}

	svc.releaseStartEnvironmentClaim(ctx, "task1", owner.ID, true, ErrCeilingLaunchDeferred)
	assertClaim(models.TaskEnvironmentStatusCreating, owner.ID)
	svc.releaseStartEnvironmentClaim(ctx, "task1", owner.ID, false, errors.New("reused session failed"))
	assertClaim(models.TaskEnvironmentStatusCreating, owner.ID)
	svc.releaseStartEnvironmentClaim(ctx, "task1", owner.ID, true, errors.New("dynamic resolution failed"))
	assertClaim(models.TaskEnvironmentStatusFailed, "")
}
