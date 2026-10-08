package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestStartCreatedSessionPassesPersistedProjectSessionToLaunchResolver(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "project-task", "project-session", models.TaskSessionStateCreated)

	task, err := repo.GetTask(ctx, "project-task")
	require.NoError(t, err)
	_, err = repo.DB().ExecContext(ctx, `
		INSERT INTO agent_projects (id, workspace_id, name)
		VALUES (?, ?, ?)
	`, "project-1", "ws1", "Project")
	require.NoError(t, err)
	task.AgentProjectID = "project-1"
	task.AgentProjectTier = models.AgentProjectTierCoordinator
	task.AgentProjectProfileID = "profile-a"
	require.NoError(t, repo.UpdateTask(ctx, task))

	session, err := repo.GetTaskSession(ctx, "project-session")
	require.NoError(t, err)
	session.AgentProfileID = "profile-b"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	svc.SetAgentProjectsEnabled(true)
	resolverErr := errors.New("stop after resolver admission")
	var resolvedSession *models.TaskSession
	var requestedProfile string
	svc.SetAgentProjectLaunchResolver(func(
		_ context.Context,
		_ *models.Task,
		existingSession *models.TaskSession,
		profileID, _, _ string,
	) (string, string, string, error) {
		resolvedSession = existingSession
		requestedProfile = profileID
		return "", "", "", resolverErr
	})

	_, err = svc.StartCreatedSession(
		ctx, task.ID, session.ID, "caller-profile", "continue", true, false, false, nil, nil,
	)
	require.ErrorIs(t, err, resolverErr)
	require.NotNil(t, resolvedSession)
	require.Equal(t, session.ID, resolvedSession.ID)
	require.Equal(t, task.ID, resolvedSession.TaskID)
	require.Equal(t, "profile-b", resolvedSession.AgentProfileID)
	require.Equal(t, "caller-profile", requestedProfile)
}
