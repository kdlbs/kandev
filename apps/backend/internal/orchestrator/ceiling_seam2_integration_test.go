package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// TestStartCreatedSession_SecondAutomaticLaunchOverCeilingIsDeferred pins
// AC-4d/AC-11 at the real seam: StartCreatedSession itself, gated before
// claimDeferredLaunchForStart, must refuse the second automatic launch once
// the ceiling's single slot is already held, and must not dispatch an agent
// for the refused session.
func TestStartCreatedSession_SecondAutomaticLaunchOverCeilingIsDeferred(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "seam2-it-task-a", "seam2-it-session-a", models.TaskSessionStateCreated)
	seedExecutorRunning(t, repo, "seam2-it-session-a", "seam2-it-task-a", "exec-a")
	seedTaskAndSession(t, repo, "seam2-it-task-b", "seam2-it-session-b", models.TaskSessionStateCreated)
	seedExecutorRunning(t, repo, "seam2-it-session-b", "seam2-it-task-b", "exec-b")

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "seam2-it-task-a", v1.TaskStateInProgress)
	seedMockTaskState(taskRepo, "seam2-it-task-b", v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)

	exec1, err := svc.StartCreatedSession(ctx, "seam2-it-task-a", "seam2-it-session-a", "profile-1", "go", false, false, true, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, exec1)

	agentMgr.mu.Lock()
	firstCalls := len(agentMgr.setExecutionDescriptionCalls)
	agentMgr.mu.Unlock()
	require.Equal(t, 1, firstCalls)

	exec2, err := svc.StartCreatedSession(ctx, "seam2-it-task-b", "seam2-it-session-b", "profile-1", "go", false, false, true, nil, nil)
	require.NoError(t, err)
	require.Nil(t, exec2, "a refused automatic launch must not produce a launched execution")

	agentMgr.mu.Lock()
	secondCalls := len(agentMgr.setExecutionDescriptionCalls)
	agentMgr.mu.Unlock()
	require.Equal(t, 1, secondCalls, "the refused launch must not reach the agent manager")

	record := deferredLaunchOf(t, svc, "seam2-it-task-b")
	require.NotNil(t, record, "a ceiling refusal must persist a deferred_launch record")
	require.Equal(t, true, record[models.CeilingDeferredKey])
	require.Equal(t, string(models.CeilingLaunchStartCreated), record[models.CeilingLaunchKindKey])
}

// TestStartCreatedSession_ManualLaunchOverCeilingIsAdmitted pins AC-14 through
// the real StartCreatedSession entry point.
func TestStartCreatedSession_ManualLaunchOverCeilingIsAdmitted(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "seam2-it-manual-a", "seam2-it-manual-session-a", models.TaskSessionStateCreated)
	seedExecutorRunning(t, repo, "seam2-it-manual-session-a", "seam2-it-manual-a", "exec-a")
	seedTaskAndSession(t, repo, "seam2-it-manual-b", "seam2-it-manual-session-b", models.TaskSessionStateCreated)
	seedExecutorRunning(t, repo, "seam2-it-manual-session-b", "seam2-it-manual-b", "exec-b")

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "seam2-it-manual-a", v1.TaskStateInProgress)
	seedMockTaskState(taskRepo, "seam2-it-manual-b", v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)

	_, err := svc.StartCreatedSession(ctx, "seam2-it-manual-a", "seam2-it-manual-session-a", "profile-1", "go", false, false, true, nil, nil)
	require.NoError(t, err)

	// autoStart=false: a direct, manual start. AC-14 requires this be admitted
	// even though the ceiling's only slot is already held.
	exec2, err := svc.StartCreatedSession(ctx, "seam2-it-manual-b", "seam2-it-manual-session-b", "profile-1", "go", false, false, false, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, exec2, "a manual launch must be admitted even over the ceiling")

	record := deferredLaunchOf(t, svc, "seam2-it-manual-b")
	require.Nil(t, record, "a manual override must never write a ceiling_deferred record")
}

// TestStartCreatedSession_ManualLaunchOverCeilingIsAuditedEvenWhenTheLaunchFails
// pins AC-14/AC-53's unconditional audit contract: the record must be written
// once the reservation is admitted onto a real session, not only once the
// subsequent launch attempt has also succeeded. A launch failure between
// admission and agent start must not erase the only evidence that a ceiling
// override happened.
func TestStartCreatedSession_ManualLaunchOverCeilingIsAuditedEvenWhenTheLaunchFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "seam2-it-fail-a", "seam2-it-fail-session-a", models.TaskSessionStateCreated)
	seedTaskAndSession(t, repo, "seam2-it-fail-b", "seam2-it-fail-session-b", models.TaskSessionStateCreated)

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "seam2-it-fail-a", v1.TaskStateInProgress)
	seedMockTaskState(taskRepo, "seam2-it-fail-b", v1.TaskStateInProgress)
	launchErr := errors.New("workspace launch failed")
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			if req.SessionID == "seam2-it-fail-session-b" {
				return nil, launchErr
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: "mock-launch-" + req.SessionID}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	_, err := svc.StartCreatedSession(ctx, "seam2-it-fail-a", "seam2-it-fail-session-a", "profile-1", "go", false, false, true, nil, nil)
	require.NoError(t, err)

	// autoStart=false: a manual start admitted over the ceiling whose launch
	// then fails.
	_, err = svc.StartCreatedSession(ctx, "seam2-it-fail-b", "seam2-it-fail-session-b", "profile-1", "go", false, false, false, nil, nil)
	require.Error(t, err, "the forced launch failure must still surface")

	session, err := repo.GetTaskSession(ctx, "seam2-it-fail-session-b")
	require.NoError(t, err)
	require.NotNil(t, session.Metadata[ceilingManualOverrideMetadataKey],
		"a manual override must be audited even when its launch subsequently fails")
}
