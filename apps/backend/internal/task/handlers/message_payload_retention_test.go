package handlers

import (
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestShellOutputRemovedReturnsGone(t *testing.T) {
	repo := &shellOutputMessageRepo{messages: map[string]*models.Message{"removed": {ID: "removed", TaskSessionID: "session-1", Type: models.MessageTypeToolExecute, Metadata: map[string]any{"payload_retention": map[string]any{"version": 1, "removed_at": "2026-09-14T00:00:00Z"}}}}}
	router := shellOutputTestRouter(t, repo)
	for _, tc := range []struct {
		session string
		status  int
	}{{"session-1", http.StatusGone}, {"foreign", http.StatusNotFound}} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/task-sessions/"+tc.session+"/messages/removed/shell-output", nil))
		require.Equal(t, tc.status, response.Code)
		if tc.status == http.StatusGone {
			require.Contains(t, response.Body.String(), `"code":"tool_payload_removed"`)
			require.Contains(t, response.Body.String(), `"removed_at":"2026-09-14T00:00:00Z"`)
		}
	}
}
