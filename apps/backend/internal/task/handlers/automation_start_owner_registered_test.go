package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestRegisteredTaskCreateCannotInjectAutomationStartOwner(t *testing.T) {
	for _, transport := range []string{"REST", "WS"} {
		t.Run(transport, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			svc, h, repo := newQueuedTaskDTOBuilder(t)
			ctx := context.Background()
			require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-owner", WorkspaceID: "ws-1", Name: "Owner"}))
			router := gin.New()
			dispatcher := ws.NewDispatcher()
			RegisterTaskRoutes(router, dispatcher, svc, nil, repo, nil, h.logger)
			body := `{"workspace_id":"ws-1","workflow_id":"wf-owner","title":"Caller task","automation_start_owned":true,"AutomationStartOwned":true,"automation_start_owner":"forged","AutomationStartOwner":"forged","metadata":{"automation_start_owned":"forged","ordinary":"kept"}}`
			if transport == "REST" {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(body)))
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			} else {
				var payload map[string]interface{}
				require.NoError(t, json.Unmarshal([]byte(body), &payload))
				request, err := ws.NewRequest("create-owner", ws.ActionTaskCreate, payload)
				require.NoError(t, err)
				response, err := dispatcher.Dispatch(ctx, request)
				require.NoError(t, err)
				require.NotEqual(t, ws.MessageTypeError, response.Type, response)
			}
			var id string
			require.NoError(t, repo.DB().QueryRowContext(ctx, `SELECT id FROM tasks WHERE title=?`, "Caller task").Scan(&id))
			task, err := repo.GetTask(ctx, id)
			require.NoError(t, err)
			require.False(t, models.AutomationStartOwned(task.Metadata))
			require.Equal(t, "kept", task.Metadata["ordinary"])
		})
	}
}
