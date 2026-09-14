package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func seedSharedGitSnapshotEnvironment(
	t *testing.T,
	repo *sqliterepo.Repository,
	taskID, environmentID string,
	sessionIDs ...string,
) {
	t.Helper()
	if len(sessionIDs) == 0 {
		t.Fatal("seedSharedGitSnapshotEnvironment requires at least one session")
	}

	seedSession(t, repo, taskID, sessionIDs[0], "")
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:            environmentID,
		TaskID:        taskID,
		ExecutorType:  "worktree",
		WorkspacePath: "/tmp/kandev-test",
		Status:        models.TaskEnvironmentStatusReady,
		CreatedAt:     now,
		UpdatedAt:     now,
	}))

	for index, sessionID := range sessionIDs {
		var session *models.TaskSession
		if index == 0 {
			var err error
			session, err = repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
		} else {
			session = &models.TaskSession{
				ID:        sessionID,
				TaskID:    taskID,
				State:     models.TaskSessionStateCompleted,
				StartedAt: now,
				UpdatedAt: now,
			}
			require.NoError(t, repo.CreateTaskSession(ctx, session))
		}
		session.TaskEnvironmentID = environmentID
		require.NoError(t, repo.UpdateTaskSession(ctx, session))
	}
}

func TestPersistGitStatusSnapshotScopesSiblingSessionsByEnvironmentAndRepository(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSharedGitSnapshotEnvironment(t, repo, "task-git-live", "env-git-live", "session-live-a", "session-live-b")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.gitSnapshotCache = newGitSnapshotCache()

	first := &lifecycle.GitStatusData{
		RepositoryName: "backend",
		Branch:         "feature/a",
		HeadCommit:     "commit-a",
		BaseCommit:     "base",
		Ahead:          2,
	}
	svc.persistGitStatusSnapshot(ctx, watcher.GitEventData{
		TaskEnvironmentID: "env-git-live",
		SessionID:         "session-live-a",
		Status:            first,
	})

	second := *first
	second.HeadCommit = "commit-b"
	svc.persistGitStatusSnapshot(ctx, watcher.GitEventData{
		// Recovered event payloads may omit the identity. The session row is
		// the compatibility fallback for this capture path.
		SessionID: "session-live-b",
		Status:    &second,
	})

	current, err := repo.GetLatestGitStatusSnapshotsByTaskEnvironmentIDs(ctx, []string{"env-git-live"})
	require.NoError(t, err)
	require.Len(t, current, 1)
	require.Equal(t, "env-git-live", current[0].TaskEnvironmentID)
	require.Equal(t, "session-live-b", current[0].SessionID)
	require.Equal(t, "backend", current[0].Metadata["repository_name"])
}
