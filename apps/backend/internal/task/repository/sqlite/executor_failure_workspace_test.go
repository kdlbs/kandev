package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestExecutorFailureWorkspaceGateScopesEnvironmentAndGeneration(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedRecoveryClaimEnvironment(t, repo, "task-failure", "env-first")
	_, err := repo.db.Exec(`UPDATE task_environments SET container_id='first',status='ready' WHERE id='env-first'`)
	require.NoError(t, err)
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "legacy-session", TaskID: "task-failure", State: models.TaskSessionStateRunning}))
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "legacy-row", SessionID: "legacy-session", TaskID: "task-failure", AgentExecutionID: "legacy-execution", Runtime: "docker", ContainerID: "second", Status: "running"}))
	first := models.ExecutorObservationTarget{TaskID: "task-failure", EnvironmentID: "env-first", OwnershipGeneration: 1, ResourceKey: "first", Runtime: "docker"}
	targets, err := repo.ListExecutorObservationTargets(t.Context(), "", 100)
	require.NoError(t, err)
	require.Len(t, targets, 2)
	second := targets[1]
	require.Equal(t, "legacy-session", second.SessionID)
	now := time.Now().UTC()
	firstEpisode, _, err := repo.ObserveExecutorFailure(t.Context(), first, &models.ExecutorObservation{Outcome: "terminated", Runtime: "docker", ResourceKey: "first", ObservedAt: now, Workspace: "unknown"})
	require.NoError(t, err)
	secondEpisode, _, err := repo.ObserveExecutorFailure(t.Context(), second, &models.ExecutorObservation{Outcome: "terminated", Runtime: "docker", ResourceKey: "second", ObservedAt: now.Add(time.Second), Workspace: "unknown"})
	require.NoError(t, err)
	reader, ok := any(repo).(interface {
		GetActiveExecutorFailure(context.Context, models.ExecutorObservationTarget) (*models.ExecutorFailureEpisode, error)
	})
	require.True(t, ok, "workspace admission needs the exact active failure scope")
	episode, err := reader.GetActiveExecutorFailure(t.Context(), first)
	require.NoError(t, err)
	require.Equal(t, firstEpisode.ID, episode.ID)
	shared := first
	shared.SessionID = "attached-sibling"
	episode, err = reader.GetActiveExecutorFailure(t.Context(), shared)
	require.NoError(t, err)
	require.Equal(t, firstEpisode.ID, episode.ID)
	episode, err = reader.GetActiveExecutorFailure(t.Context(), second)
	require.NoError(t, err)
	require.Equal(t, secondEpisode.ID, episode.ID)
	foreign := second
	foreign.SessionID = "unaffected-session"
	episode, err = reader.GetActiveExecutorFailure(t.Context(), foreign)
	require.NoError(t, err)
	require.Nil(t, episode)
	replacement := first
	replacement.OwnershipGeneration = 2
	episode, err = reader.GetActiveExecutorFailure(t.Context(), replacement)
	require.NoError(t, err)
	require.Nil(t, episode)
	_, _, err = repo.ObserveExecutorFailure(t.Context(), first, &models.ExecutorObservation{Outcome: "healthy", Runtime: "docker", ResourceKey: "first", ObservedAt: now.Add(2 * time.Second), Workspace: "unknown"})
	require.NoError(t, err)
	episode, err = reader.GetActiveExecutorFailure(t.Context(), first)
	require.NoError(t, err)
	require.Nil(t, episode)
}
