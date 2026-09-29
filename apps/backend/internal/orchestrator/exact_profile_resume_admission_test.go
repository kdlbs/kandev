package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestEnsureSessionRunningColdResumeAdmitsExactAttemptBeforeProcessStart(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	revision := exactProfileRecoveryAssignment(t, repo)
	seedExecutorRunning(t, repo, "session1", "task1", "exec-before")

	started := false
	admittedBeforeStart := false
	agentManager := &sessionUpdatingAgentManager{
		mockAgentManager: &mockAgentManager{resolveProfileInfo: exactProfileRecoveryInfo(revision)},
		repo:             repo,
		sessionID:        "session1",
		taskID:           "task1",
		onStartCalled:    &started,
	}
	agentManager.beforeStart = func() {
		agentManager.mu.Lock()
		admittedBeforeStart = len(agentManager.exactProfileAttemptBindings) == 1
		agentManager.mu.Unlock()
	}
	svc := exactProfileRecoveryService(repo, agentManager)
	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)

	require.NoError(t, svc.ensureSessionRunning(ctx, session.ID, session, launchOriginManual))
	require.True(t, started)
	require.True(t, admittedBeforeStart)
	requireExactProfileAttemptAttached(t, agentManager.mockAgentManager, "mock-launch-session1", revision)
}

func TestEnsureSessionRunningColdResumeRefusedExactAdmissionLeavesNoEvidence(t *testing.T) {
	testResumeAdmissionRefusalLeavesNoEvidence(t, models.TaskSessionStateWaitingForInput, "exec-cold-refused")
}

func TestEnsureSessionRunningPreparedResumeRefusedExactAdmissionLeavesNoEvidence(t *testing.T) {
	testResumeAdmissionRefusalLeavesNoEvidence(t, models.TaskSessionStateCreated, "exec-prepared-refused")
}

func testResumeAdmissionRefusalLeavesNoEvidence(t *testing.T, state models.TaskSessionState, executionID string) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", state)
	revision := exactProfileRecoveryAssignment(t, repo)
	seedExecutorRunning(t, repo, "session1", "task1", "exec-before")

	started := false
	agentManager := &sessionUpdatingAgentManager{
		mockAgentManager: &mockAgentManager{
			resolveProfileInfo: exactProfileRecoveryInfo(revision),
			launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				_, err := repo.AssignExactProfileAssignment(ctx, &models.ExactProfileAssignment{
					TaskID: "task1", WorkspaceID: "ws1", AgentProfileID: "profile-exact",
					ProfileRevision: revision.Add(time.Second), Generation: 2,
				})
				require.NoError(t, err)
				return &executor.LaunchAgentResponse{AgentExecutionID: executionID}, nil
			},
		},
		repo: repo, sessionID: "session1", taskID: "task1", onStartCalled: &started,
	}
	svc := exactProfileRecoveryService(repo, agentManager)
	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)

	require.Error(t, svc.ensureSessionRunning(ctx, session.ID, session, launchOriginManual))
	require.False(t, started)
	agentManager.mu.Lock()
	attached := len(agentManager.exactProfileAttemptBindings)
	agentManager.mu.Unlock()
	require.Zero(t, attached)
	receipt, err := repo.GetExactProfileLaunchReceipt(ctx, "task1", "session1")
	require.NoError(t, err)
	require.Nil(t, receipt)
}

func TestEnsureSessionRunningPreparedWorkspaceAdmitsExactAttemptBeforeProcessStart(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCreated)
	revision := exactProfileRecoveryAssignment(t, repo)
	seedExecutorRunning(t, repo, "session1", "task1", "exec-prepared")

	started := false
	admittedBeforeStart := false
	agentManager := &sessionUpdatingAgentManager{
		mockAgentManager: &mockAgentManager{resolveProfileInfo: exactProfileRecoveryInfo(revision)},
		repo:             repo,
		sessionID:        "session1",
		taskID:           "task1",
		onStartCalled:    &started,
	}
	agentManager.beforeStart = func() {
		agentManager.mu.Lock()
		admittedBeforeStart = len(agentManager.exactProfileAttemptBindings) == 1
		agentManager.mu.Unlock()
	}
	svc := exactProfileRecoveryService(repo, agentManager)
	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)

	require.NoError(t, svc.ensureSessionRunning(ctx, session.ID, session, launchOriginManual))
	require.True(t, started)
	require.True(t, admittedBeforeStart)
	requireExactProfileAttemptAttached(t, agentManager.mockAgentManager, "mock-launch-session1", revision)
}

func requireExactProfileAttemptAttached(t *testing.T, manager *mockAgentManager, executionID string, revision interface{ UnixNano() int64 }) {
	t.Helper()
	manager.mu.Lock()
	bindings := append([]*models.ExactProfileLaunchAttemptBinding(nil), manager.exactProfileAttemptBindings...)
	manager.mu.Unlock()
	require.Len(t, bindings, 1)
	require.Equal(t, "task1", bindings[0].TaskID)
	require.Equal(t, "session1", bindings[0].SessionID)
	require.Equal(t, executionID, bindings[0].ExecutionID)
	require.Equal(t, executionID, bindings[0].AttemptID)
	require.Equal(t, "profile-exact", bindings[0].AgentProfileID)
	require.Equal(t, "gpt-exact", bindings[0].Model)
	require.Equal(t, revision.UnixNano(), bindings[0].ProfileRevision.UnixNano())
	require.Equal(t, int64(1), bindings[0].Generation)
	require.NotEmpty(t, bindings[0].SessionIncarnationID)
}
