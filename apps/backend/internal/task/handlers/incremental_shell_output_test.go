package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

type incrementalShellDetailRepo struct{ mockRepository }

func (r *incrementalShellDetailRepo) GetMessage(context.Context, string) (*models.Message, error) {
	return &models.Message{ID: "message", TaskSessionID: "session", Metadata: map[string]any{"shell_output_storage": 1, "normalized": map[string]any{"shell_exec": map[string]any{"output": map[string]any{"has_output": true, "stdout_bytes": 7}}}}}, nil
}
func (r *incrementalShellDetailRepo) RehydrateMessagePayload(_ context.Context, message *models.Message) error {
	message.Metadata, _ = models.RehydrateShellOutput(message.Metadata, models.ShellExecOutputSnapshot{Stdout: "current"})
	return nil
}

func TestHTTPGetIncrementalShellOutput(t *testing.T) {
	log := newTestLogger(t)
	repo := &incrementalShellDetailRepo{}
	svc := service.NewService(service.Repos{Workspaces: repo, Tasks: repo, Messages: repo, Turns: repo, Sessions: repo}, nil, log, service.RepositoryDiscoveryConfig{})
	h := &MessageHandlers{service: svc, logger: log}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/task-sessions/session/messages/message/shell-output", nil)
	c.Params = gin.Params{{Key: "id", Value: "session"}, {Key: "message_id", Value: "message"}}
	h.httpGetShellOutput(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "current")
}
