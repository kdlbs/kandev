package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// persistACPSessionID must write the ACP session id into the session's "acp"
// metadata map, preserve keys session_info already stored, and be a no-op
// when the stored id is current.
func TestPersistACPSessionID(t *testing.T) {
	ctx := context.Background()

	t.Run("writes id into empty metadata", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
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
}
