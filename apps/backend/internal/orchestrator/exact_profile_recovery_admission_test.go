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

func TestEnsureSessionRunningColdResumeCarriesExactProfile(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	revision := exactProfileRecoveryAssignment(t, repo)
	seedExecutorRunning(t, repo, "session1", "task1", "exec-before")

	var launchRequest *executor.LaunchAgentRequest
	agentManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		resolveProfileInfo:     exactProfileRecoveryInfo(revision),
		launchAgentFunc: func(_ context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchRequest = request
			return nil, errExactProfileRecoveryStop
		},
	}
	svc := exactProfileRecoveryService(repo, agentManager)
	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)

	err = svc.ensureSessionRunning(ctx, session.ID, session, launchOriginManual)
	require.Error(t, err)
	require.NotNil(t, launchRequest)
	require.True(t, launchRequest.ExactProfile)
	require.Equal(t, "gpt-exact", launchRequest.ExactProfileModel)
	require.Equal(t, revision.UnixNano(), launchRequest.ExactProfileRevision)
	exactProfileRecoveryBinding(t, repo, revision)
}

func TestEnsureSessionRunningColdResumeRejectsStaleExactProfileBeforeExecutor(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	revision := exactProfileRecoveryAssignment(t, repo)
	seedExecutorRunning(t, repo, "session1", "task1", "exec-before")

	launched := false
	agentManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		resolveProfileInfo: &executor.AgentProfileInfo{
			ProfileID: "profile-exact", WorkspaceID: "ws1", Enabled: true,
			Revision: revision.Add(time.Nanosecond), Model: "gpt-exact",
		},
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launched = true
			return nil, nil
		},
	}
	svc := exactProfileRecoveryService(repo, agentManager)
	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)

	err = svc.ensureSessionRunning(ctx, session.ID, session, launchOriginManual)
	require.ErrorIs(t, err, ErrExactProfileAssignmentInvalid)
	require.False(t, launched)
}

func TestEnsureSessionRunningPreparedWorkspaceBindsExactProfileBeforeStart(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCreated)
	revision := exactProfileRecoveryAssignment(t, repo)
	seedExecutorRunning(t, repo, "session1", "task1", "exec-prepared")

	started := false
	agentManager := &sessionUpdatingAgentManager{
		mockAgentManager: &mockAgentManager{resolveProfileInfo: exactProfileRecoveryInfo(revision)},
		repo:             repo, sessionID: "session1", taskID: "task1", onStartCalled: &started,
	}
	svc := exactProfileRecoveryService(repo, agentManager)
	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)

	require.NoError(t, svc.ensureSessionRunning(ctx, session.ID, session, launchOriginManual))
	require.True(t, started)
	exactProfileRecoveryBinding(t, repo, revision)
}

func TestEnsureSessionRunningPreparedWorkspaceRejectsStaleExactProfileBeforeStart(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCreated)
	revision := exactProfileRecoveryAssignment(t, repo)
	seedExecutorRunning(t, repo, "session1", "task1", "exec-prepared")

	started := false
	agentManager := &sessionUpdatingAgentManager{
		mockAgentManager: &mockAgentManager{resolveProfileInfo: &executor.AgentProfileInfo{
			ProfileID: "profile-exact", WorkspaceID: "ws1", Enabled: true,
			Revision: revision.Add(time.Nanosecond), Model: "gpt-exact",
		}},
		repo: repo, sessionID: "session1", taskID: "task1", onStartCalled: &started,
	}
	svc := exactProfileRecoveryService(repo, agentManager)
	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)

	err = svc.ensureSessionRunning(ctx, session.ID, session, launchOriginManual)
	require.ErrorIs(t, err, ErrExactProfileAssignmentInvalid)
	require.False(t, started)
}

func exactProfileRecoveryAssignment(t *testing.T, repo *sqliterepo.Repository) time.Time {
	t.Helper()
	task, err := repo.GetTask(context.Background(), "task1")
	require.NoError(t, err)
	task.WorkspaceID = "ws1"
	require.NoError(t, repo.UpdateTask(context.Background(), task))
	revision := time.Unix(1_726_500_000, 0).UTC()
	_, err = repo.AssignExactProfileAssignment(context.Background(), &models.ExactProfileAssignment{
		TaskID: "task1", WorkspaceID: "ws1", AgentProfileID: "profile-exact",
		ProfileRevision: revision, Generation: 1,
	})
	require.NoError(t, err)
	return revision
}

func exactProfileRecoveryInfo(revision time.Time) *executor.AgentProfileInfo {
	return &executor.AgentProfileInfo{
		ProfileID: "profile-exact", WorkspaceID: "ws1", Enabled: true,
		Revision: revision, Model: "gpt-exact",
	}
}

func exactProfileRecoveryService(repo *sqliterepo.Repository, agentManager executor.AgentManagerClient) *Service {
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{
		ID: "task1", WorkspaceID: "ws1", Title: "Test Task", State: v1.TaskStateInProgress,
	}
	return createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
}

func exactProfileRecoveryBinding(t *testing.T, repo *sqliterepo.Repository, revision time.Time) {
	t.Helper()
	session, err := repo.GetTaskSession(context.Background(), "session1")
	require.NoError(t, err)
	require.Equal(t, "profile-exact", session.AgentProfileID)
	require.Equal(t, int64(1), session.ExactProfileGeneration)
	require.Equal(t, revision.UnixNano(), session.ExactProfileRevision)
}

var errExactProfileRecoveryStop = errors.New("stop after exact launch request")
