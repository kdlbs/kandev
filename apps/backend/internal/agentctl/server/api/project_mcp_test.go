package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/passthroughconfig"
)

func newProjectMCPTestServer(t *testing.T, authToken string) (*Server, *config.InstanceConfig) {
	t.Helper()
	cfg := &config.InstanceConfig{
		InstanceID: "instance-1", SessionID: "session-1", Port: 43210,
		WorkDir: t.TempDir(), AuthToken: authToken,
	}
	log := newTestLogger()
	server := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
	t.Cleanup(func() { _ = server.procMgr.ProjectMCPFiles().Cleanup(context.Background()) })
	return server, cfg
}

// @covers AC-AGENTS-MCP-PREP-005.1
func projectMCPAuthenticatedRequest(method, path string, body *bytes.Reader, token string) *http.Request {
	var request *http.Request
	if body == nil {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, body)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	request.Header.Set("Content-Type", "application/json")
	return request
}

func TestProjectMCPRoutesMaterializeOnHelper(t *testing.T) {
	server, cfg := newProjectMCPTestServer(t, "control-token")

	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, projectMCPAuthenticatedRequest(http.MethodGet, "/api/v1/mcp/project", nil, "control-token"))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var target types.ProjectMCPTarget
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &target))
	require.Equal(t, cfg.WorkDir, target.WorkspacePath)
	require.Equal(t, "http://127.0.0.1:43210/mcp", target.MCPURL)

	file := filepath.Join(cfg.WorkDir, ".pi", "mcp.json")
	payload, err := json.Marshal(types.ProjectMCPPrepareRequest{
		Version: types.ProjectMCPFilesVersion, Generation: 1,
		Files: []passthroughconfig.File{{Path: file, Content: []byte(`{"mcpServers":{}}`), MergeKey: "mcpServers"}},
	})
	require.NoError(t, err)
	request := projectMCPAuthenticatedRequest(http.MethodPost, "/api/v1/mcp/project/prepare", bytes.NewReader(payload), "control-token")
	response = httptest.NewRecorder()
	server.Router().ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	_, statErr := os.Stat(file)
	require.NoError(t, statErr)
}

func TestProjectMCPRoutesRejectInvalidPreparation(t *testing.T) {
	server, cfg := newProjectMCPTestServer(t, "control-token")
	for _, payload := range [][]byte{
		[]byte(`{"version":999,"generation":1,"files":[]}`),
		func() []byte {
			content := bytes.Repeat([]byte("x"), (1<<20)+1)
			encoded, err := json.Marshal(types.ProjectMCPPrepareRequest{
				Version: types.ProjectMCPFilesVersion, Generation: 1,
				Files: []passthroughconfig.File{{Path: filepath.Join(cfg.WorkDir, ".pi", "mcp.json"), Content: content}},
			})
			require.NoError(t, err)
			return encoded
		}(),
	} {
		request := projectMCPAuthenticatedRequest(http.MethodPost, "/api/v1/mcp/project/prepare", bytes.NewReader(payload), "control-token")
		response := httptest.NewRecorder()
		server.Router().ServeHTTP(response, request)
		require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	}
}

func TestProjectMCPRoutesRequireAuthentication(t *testing.T) {
	server, _ := newProjectMCPTestServer(t, "control-token")
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/mcp/project", nil))
	require.Equal(t, http.StatusUnauthorized, response.Code)

	// Auth disabled leaves no authenticated control identity; the handlers
	// still refuse project file access instead of trusting the local listener.
	openServer, _ := newProjectMCPTestServer(t, "")
	response = httptest.NewRecorder()
	openServer.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/mcp/project", nil))
	require.Equal(t, http.StatusForbidden, response.Code)
}
