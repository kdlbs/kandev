package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// seedResumeToken puts the session's executors_running row into the state
// storeResumeToken leaves it in after a successful CAS: resume_token holds the
// agent's ACP session id. persistACPSessionID only mirrors ids that match this
// row, so every subtest that expects a write needs it.
func seedResumeToken(t *testing.T, repo *sqliterepo.Repository, sessionID, taskID, token string) {
	t.Helper()
	require.NoError(t, repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID:          sessionID,
		SessionID:   sessionID,
		TaskID:      taskID,
		ResumeToken: token,
		Status:      "ready",
	}))
}

// persistACPSessionID must write the ACP session id into the session's "acp"
// metadata map, preserve keys session_info already stored, be a no-op when the
// stored id is current, and refuse to mirror an id that no longer matches the
// executors_running resume token (stale event from a rotated execution).
func TestPersistACPSessionID(t *testing.T) {
	ctx := context.Background()

	t.Run("writes id into empty metadata", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		seedResumeToken(t, repo, "s1", "t1", "acp-123")
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		svc.persistACPSessionID(ctx, "s1", "acp-123")

		session, err := repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		acp, ok := session.Metadata["acp"].(map[string]interface{})
		require.True(t, ok, "acp metadata map missing: %+v", session.Metadata)
		require.Equal(t, "acp-123", acp["session_id"])
	})

	t.Run("merges with existing acp map and updates stale id", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		seedResumeToken(t, repo, "s1", "t1", "acp-new")
		require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", "acp",
			map[string]interface{}{"session_id": "acp-old", "title": "My session"}))
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		svc.persistACPSessionID(ctx, "s1", "acp-new")

		session, err := repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		acp := session.Metadata["acp"].(map[string]interface{})
		require.Equal(t, "acp-new", acp["session_id"])
		require.Equal(t, "My session", acp["title"], "session_info keys must survive")
	})

	t.Run("skips write when id is current", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		seedResumeToken(t, repo, "s1", "t1", "acp-123")
		require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", "acp",
			map[string]interface{}{"session_id": "acp-123"}))
		before, err := repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		svc.persistACPSessionID(ctx, "s1", "acp-123")

		after, err := repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		require.Equal(t, before.UpdatedAt, after.UpdatedAt, "no-op must not touch the row")
	})

	t.Run("skips write when execution rotated to a newer token", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		// The successor execution's CAS already stored its own token; a
		// delayed mirror from the previous execution must not land.
		seedResumeToken(t, repo, "s1", "t1", "acp-successor")
		require.NoError(t, repo.SetSessionMetadataKey(ctx, "s1", "acp",
			map[string]interface{}{"session_id": "acp-successor"}))
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		svc.persistACPSessionID(ctx, "s1", "acp-stale")

		session, err := repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		acp := session.Metadata["acp"].(map[string]interface{})
		require.Equal(t, "acp-successor", acp["session_id"], "stale id must not overwrite the live execution's id")
	})
}
