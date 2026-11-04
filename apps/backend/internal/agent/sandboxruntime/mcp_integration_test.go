//go:build linux

package sandboxruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/common/sandboxmcp"
	profile "github.com/kandev/kandev/internal/mcp/profile"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func runtimeMCPFixture(t *testing.T) (func(string, uint64, string) sandboxmcp.Forward, *atomic.Int32) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	require.NoError(t, err)
	calls := &atomic.Int32{}
	dispatcher := ws.NewDispatcher()
	dispatcher.RegisterFunc(ws.ActionMCPGetTaskPlan, func(_ context.Context, msg *ws.Message) (*ws.Message, error) {
		var payload struct {
			TaskID string `json:"task_id"`
		}
		require.NoError(t, json.Unmarshal(msg.Payload, &payload))
		require.Equal(t, "sandbox-task", payload.TaskID)
		calls.Add(1)
		return ws.NewResponse(msg.ID, msg.Action, map[string]any{"content": "sandbox-mcp-permitted", "version": "fixture"})
	})
	view := profile.New(profile.SurfaceKanbanTask, nil, nil)
	view.BuiltinPackages = &profile.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "sandbox-task", Surface: view.Surface, Packages: []profile.PackageID{profile.PackageCore, profile.PackageConfiguration}, Origins: map[profile.PackageID]string{profile.PackageCore: "required", profile.PackageConfiguration: "task"}}
	server := mcpserver.NewWithProfile(mcpserver.NewDispatcherBackendClient(dispatcher, log), "approval-session", "sandbox-task", 0, log, "", false, view)
	t.Cleanup(func() { require.NoError(t, server.Close(context.Background())) })
	return func(attemptID string, generation uint64, digest string) sandboxmcp.Forward {
		require.NotEmpty(t, attemptID)
		require.EqualValues(t, 1, generation)
		require.Len(t, digest, 64)
		return func(ctx context.Context, method string, headers http.Header, body []byte) (*http.Response, error) {
			path := headers.Get(sandboxmcp.MCPPathHeader)
			if path == "" {
				path = contract.MCPRootPath
			}
			request := httptest.NewRequest(method, "http://127.0.0.1"+path, bytes.NewReader(body)).WithContext(ctx)
			request.Header = headers
			response := httptest.NewRecorder()
			id := profile.PackageCore
			if path != "/mcp" {
				id = profile.PackageID(strings.TrimPrefix(path, contract.MCPPackagePathPrefix))
			}
			server.ServeBuiltinStreamableHTTP(id, response, request)
			return response.Result(), nil
		}
	}, calls
}
