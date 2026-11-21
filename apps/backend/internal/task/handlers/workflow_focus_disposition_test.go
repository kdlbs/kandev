package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestMoveTaskWorkflowFocusDisposition(t *testing.T) {
	for _, transport := range []string{"http", "ws"} {
		for _, scenario := range []string{"ended", "waiting", "auto-start", "skip", "skip-instructions", "skip-waiting"} {
			t.Run(transport+"/"+scenario, func(t *testing.T) {
				h, repo := newReorderHandlerTest(t, "source", "task-focus")
				ctx := context.Background()
				now := time.Date(2026, time.November, 20, 12, 0, 0, 0, time.UTC)
				_, err := repo.DB().Exec(`INSERT INTO workflow_steps
					(id, workflow_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
					"target", "wf-reorder-http", "Target", 1, now, now)
				require.NoError(t, err)
				target := &wfmodels.WorkflowStep{ID: "target", WorkflowID: "wf-reorder-http", Name: "Target", Position: 1}
				if scenario == "auto-start" || strings.HasPrefix(scenario, "skip") {
					target.Events.OnEnter = []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}
				}
				h.service.SetWorkflowStepGetter(&reorderHandlerStepGetter{steps: map[string]*wfmodels.WorkflowStep{
					"source": {ID: "source", WorkflowID: "wf-reorder-http", Name: "Source"}, "target": target,
				}})
				state := models.TaskSessionStateCompleted
				if scenario == "waiting" || scenario == "skip-waiting" {
					state = models.TaskSessionStateWaitingForInput
				}
				require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
					ID: "stored-session", TaskID: "task-focus", State: state, IsPrimary: true,
				}))
				request := `{"id":"task-focus","workflow_id":"wf-reorder-http","workflow_step_id":"target"}`
				if strings.HasPrefix(scenario, "skip") {
					instructions := ""
					if scenario == "skip-instructions" {
						instructions = "Run the requested verification"
					}
					body, err := json.Marshal(map[string]any{
						"id": "task-focus", "workflow_id": "wf-reorder-http", "workflow_step_id": "target",
						"entry_options": map[string]any{"skip_step_prompt": true, "instructions": instructions},
					})
					require.NoError(t, err)
					request = string(body)
				}
				var payload []byte
				if transport == "http" {
					gin.SetMode(gin.TestMode)
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Params = gin.Params{{Key: "id", Value: "task-focus"}}
					c.Request = httptest.NewRequest(http.MethodPost, "/tasks/task-focus/move", strings.NewReader(request))
					c.Request.Header.Set("Content-Type", "application/json")
					h.httpMoveTask(c)
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					payload = rec.Body.Bytes()
				} else {
					reply, err := h.wsMoveTask(ctx, &ws.Message{ID: "move", Action: "task.move", Payload: json.RawMessage(request)})
					require.NoError(t, err)
					payload = reply.Payload
				}
				var response map[string]any
				require.NoError(t, json.Unmarshal(payload, &response))
				require.NotEmpty(t, response["workflow_entry_identity"])
				if scenario == "ended" || scenario == "skip" {
					require.Equal(t, "none", response["workflow_focus_disposition"])
				} else {
					require.NotContains(t, response, "workflow_focus_disposition")
				}
			})
		}
	}
}
