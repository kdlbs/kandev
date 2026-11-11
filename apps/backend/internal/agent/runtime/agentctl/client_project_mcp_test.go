package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/passthroughconfig"
)

func TestProjectMCPClientRoundTrip(t *testing.T) {
	prepared := 0
	cleaned := 0
	requests := make(chan types.ProjectMCPPrepareRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/mcp/project" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(types.ProjectMCPTarget{
				Version: types.ProjectMCPFilesVersion, InstanceID: "instance-1", SessionID: "session-1",
				WorkspacePath: "/workspace", MCPURL: "http://127.0.0.1:43210/mcp",
			})
		case r.URL.Path == "/api/v1/mcp/project/prepare" && r.Method == http.MethodPost:
			var request types.ProjectMCPPrepareRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			requests <- request
			prepared++
			_ = json.NewEncoder(w).Encode(types.ProjectMCPPrepareResponse{
				Version: types.ProjectMCPFilesVersion, InstanceID: "instance-1", SessionID: "session-1",
				WorkspacePath: "/workspace", MCPURL: "http://127.0.0.1:43210/mcp", Generation: request.Generation,
				Files: []types.ProjectMCPPreparedFile{{Path: "/workspace/.pi/mcp.json", Fingerprint: "abc"}},
			})
		case r.URL.Path == "/api/v1/mcp/project/cleanup" && r.Method == http.MethodPost:
			cleaned++
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "cleaned"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := newHTTPOnlyClient(server.URL)

	target, err := client.GetProjectMCP(context.Background())
	require.NoError(t, err)
	require.Equal(t, "/workspace", target.WorkspacePath)
	require.Equal(t, "http://127.0.0.1:43210/mcp", target.MCPURL)

	response, err := client.PrepareProjectMCP(context.Background(), types.ProjectMCPPrepareRequest{
		Generation: 1,
		Files:      []passthroughconfig.File{{Path: "/workspace/.pi/mcp.json", Content: []byte(`{}`)}},
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), response.Generation)
	require.Len(t, response.Files, 1)
	require.Equal(t, 1, prepared)
	sent := <-requests
	require.Equal(t, types.ProjectMCPFilesVersion, sent.Version)
	require.Equal(t, uint64(1), sent.Generation)

	require.NoError(t, client.CleanupProjectMCP(context.Background()))
	require.Equal(t, 1, cleaned)
}

func TestProjectMCPClientReportsUnsupportedAndConflict(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{
		{status: http.StatusNotFound, want: ErrProjectMCPUnsupported},
		{status: http.StatusServiceUnavailable, want: ErrProjectMCPUnavailable},
		{status: http.StatusConflict, want: ErrProjectMCPConflict},
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(server.Close)
			_, err := newHTTPOnlyClient(server.URL).PrepareProjectMCP(context.Background(), types.ProjectMCPPrepareRequest{})
			require.ErrorIs(t, err, tc.want)
		})
	}
}
