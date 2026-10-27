package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestWSUpdateRepositoryIgnoresRunGitHooks(t *testing.T) {
	_, repo, svc := newRepositoryHTTPTestRouterWithService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-hooks", WorkspaceID: "ws-1", Name: "api",
	}))
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	h := NewRepositoryHandlers(svc, log)

	msg := &ws.Message{ID: "1", Action: ws.ActionRepositoryUpdate,
		Payload: json.RawMessage(`{"id":"repo-hooks","name":"renamed","run_git_hooks":true}`)}
	resp, err := h.wsUpdateRepository(ctx, msg)
	require.NoError(t, err)
	require.NotEqual(t, ws.MessageTypeError, resp.Type)

	got, err := repo.GetRepository(ctx, "repo-hooks")
	require.NoError(t, err)
	require.Equal(t, "renamed", got.Name)
	require.False(t, got.RunGitHooks)
}
