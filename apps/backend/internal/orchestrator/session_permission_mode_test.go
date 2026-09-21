package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestTaskSessionPermissionModeUsesNativeRuntimeAndPreservesGates(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task", "session", "")
	manager := &mockAgentManager{isAgentRunning: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	ctx := context.Background()
	require.NoError(t, svc.SetTaskSessionPermissionMode(ctx, "task", "session", "default"))
	require.Len(t, manager.setSessionModeCalls, 1)
	session, err := repo.GetTaskSession(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, "default", session.Metadata[models.SessionMetaKeySessionMode])
	for _, mode := range []string{"bypassPermissions", "dontAsk", "", "unknown"} {
		require.Error(t, svc.SetTaskSessionPermissionMode(ctx, "task", "session", mode))
	}
	require.Error(t, svc.SetTaskSessionPermissionMode(ctx, "other-task", "session", "default"))
	require.Len(t, manager.setSessionModeCalls, 1)
	manager.setSessionModeErr = errors.New("provider unavailable")
	require.Error(t, svc.SetTaskSessionPermissionMode(ctx, "task", "session", "acceptEdits"))
	session, err = repo.GetTaskSession(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, "default", session.Metadata[models.SessionMetaKeySessionMode])
	manager.isAgentRunning = false
	require.NoError(t, svc.SetTaskSessionPermissionMode(ctx, "task", "session", "acceptEdits"))
	session, err = repo.GetTaskSession(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, "acceptEdits", session.Metadata[models.SessionMetaKeySessionMode])
}
