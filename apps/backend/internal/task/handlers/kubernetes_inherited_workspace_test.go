package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type unsupportedWorkspaceOrchestrator struct{ recordingOrchestrator }

func (*unsupportedWorkspaceOrchestrator) EnsureSession(context.Context, string, ...orchestrator.EnsureSessionOptions) (*orchestrator.EnsureSessionResponse, error) {
	return nil, fmt.Errorf("prepare session: %w", models.ErrWorkspaceReuseUnsupported)
}

func TestKubernetesInheritedWorkspaceHTTPEnsureConflict(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	h := &TaskHandlers{orchestrator: &unsupportedWorkspaceOrchestrator{}, logger: log}
	router := gin.New()
	router.POST("/tasks/:id/sessions/ensure", h.httpEnsureTaskSession)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/tasks/child/sessions/ensure", nil))
	require.Equal(t, http.StatusConflict, response.Code)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.Equal(t, "workspace_reuse_unsupported", payload["reason"])
	require.Equal(t, true, payload["recoverable"])
	require.Equal(t, "create a task with workspace_mode=\"new_workspace\"", payload["retry"])
	require.Contains(t, payload["error"], "new_workspace")
}
