package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// TestStartTask_SecondAutomaticLaunchOverCeilingIsDeferred pins AC-4a/AC-11 at
// the real seam: StartTask itself, gated before claimDeferredLaunchForStart,
// must refuse the second automatic launch once the ceiling's single slot is
// already held by the first, and must not create a session or dispatch an
// agent for the refused task.
func TestStartTask_SecondAutomaticLaunchOverCeilingIsDeferred(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "seam1-it-first", "seam1-it-first-session", models.TaskSessionStateCompleted)
	seedTaskAndSession(t, repo, "seam1-it-second", "seam1-it-second-session", models.TaskSessionStateCompleted)

	var launches int
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "seam1-it-first", v1.TaskStateInProgress)
	seedMockTaskState(taskRepo, "seam1-it-second", v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launches++
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)

	exec1, err := svc.StartTask(ctx, "seam1-it-first", "profile-1", "", "", "", "go", "", false, true, nil)
	require.NoError(t, err)
	require.NotNil(t, exec1)
	require.Equal(t, 1, launches)

	exec2, err := svc.StartTask(ctx, "seam1-it-second", "profile-1", "", "", "", "go", "", false, true, nil)
	require.NoError(t, err)
	require.Nil(t, exec2, "a refused automatic launch must not produce a launched execution")
	require.Equal(t, 1, launches, "the refused launch must not reach the agent manager")

	record := deferredLaunchOf(t, svc, "seam1-it-second")
	require.NotNil(t, record, "a ceiling refusal must persist a deferred_launch record")
	require.Equal(t, true, record[models.CeilingDeferredKey])
}

// TestStartTask_ManualLaunchOverCeilingIsAdmitted pins AC-14: a manual launch
// at the ceiling is always admitted, never deferred, through the real
// StartTask entry point.
func TestStartTask_ManualLaunchOverCeilingIsAdmitted(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "seam1-it-manual-first", "seam1-it-manual-first-session", models.TaskSessionStateCompleted)
	seedTaskAndSession(t, repo, "seam1-it-manual-second", "seam1-it-manual-second-session", models.TaskSessionStateCompleted)

	var launches int
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "seam1-it-manual-first", v1.TaskStateInProgress)
	seedMockTaskState(taskRepo, "seam1-it-manual-second", v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launches++
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)

	_, err := svc.StartTask(ctx, "seam1-it-manual-first", "profile-1", "", "", "", "go", "", false, true, nil)
	require.NoError(t, err)
	require.Equal(t, 1, launches)

	// autoStart=false: a direct, manual start. AC-14 requires this be admitted
	// even though the ceiling's only slot is already held.
	exec2, err := svc.StartTask(ctx, "seam1-it-manual-second", "profile-1", "", "", "", "go", "", false, false, nil)
	require.NoError(t, err)
	require.NotNil(t, exec2, "a manual launch must be admitted even over the ceiling")
	require.Equal(t, 2, launches)

	record := deferredLaunchOf(t, svc, "seam1-it-manual-second")
	require.Nil(t, record, "a manual override must never write a ceiling_deferred record")
}
