package backendapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

func TestE2ECreateTaskSessionPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newDynamicRoutingCancelRaceRepo(t)
	seedDynamicRoutingCancelRaceSession(t, repo, "existing-session", models.TaskSessionStateWaitingForInput)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}

	tests := []struct {
		name            string
		body            string
		wantPassthrough bool
	}{
		{
			name:            "omitted mode defaults to structured",
			body:            `{"task_id":"task-1","state":"WAITING_FOR_INPUT"}`,
			wantPassthrough: false,
		},
		{
			name:            "true mode persists",
			body:            `{"task_id":"task-1","state":"WAITING_FOR_INPUT","is_passthrough":true}`,
			wantPassthrough: true,
		},
		{
			name:            "false mode overrides the default",
			body:            `{"task_id":"task-1","state":"WAITING_FOR_INPUT","is_passthrough":false}`,
			wantPassthrough: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/e2e/task-sessions", strings.NewReader(tt.body))
			ctx.Request.Header.Set("Content-Type", "application/json")

			handleE2ECreateTaskSession(repo, log)(ctx)

			if response.Code != http.StatusCreated {
				t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
			}
			var created struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			stored, err := repo.GetTaskSession(ctx.Request.Context(), created.ID)
			if err != nil {
				t.Fatalf("read stored session: %v", err)
			}
			if stored.IsPassthrough != tt.wantPassthrough {
				t.Errorf("stored is_passthrough = %t, want %t", stored.IsPassthrough, tt.wantPassthrough)
			}
			if !stored.IsPrimary {
				t.Error("new session should remain primary when is_primary is omitted")
			}
		})
	}
}
