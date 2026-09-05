package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

func TestPublishWorkspacePreviewForSessionForwardsToAgentctl(t *testing.T) {
	var received agentctl.WorkspacePreviewRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workspace/html-previews" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode preview request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(agentctl.WorkspacePreviewResponse{
			Port: 43127, Path: "/site/index.html", Version: 2,
		})
	}))
	t.Cleanup(server.Close)

	client := newTestAgentctlClient(t, server.URL, newTestLogger())
	mgr, _ := newProcessRunnerManager(t, client)
	response, err := mgr.PublishWorkspacePreview(context.Background(), "session-1", agentctl.WorkspacePreviewRequest{
		Repo:    "frontend",
		Path:    "site/index.html",
		Content: "<body>current</body>",
	})

	require.NoError(t, err)
	require.Equal(t, agentctl.WorkspacePreviewResponse{Port: 43127, Path: "/site/index.html", Version: 2}, response)
	require.Equal(t, agentctl.WorkspacePreviewRequest{
		Repo:    "frontend",
		Path:    "site/index.html",
		Content: "<body>current</body>",
	}, received)
}
