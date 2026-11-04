package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/common/sandboxmcp"
	"github.com/stretchr/testify/require"
)

func TestSandboxMCPControlPreservesPackageRegistryAndOwner(t *testing.T) {
	server, _ := newBuiltinSessionServer(t, "control-secret")
	launch := apiSandboxFixture(t)
	launch.MCPURL = "http://127.0.0.1:31234/" + strings.Repeat("a", 64) + "/mcp"
	launch.MCPPaths = []string{"/mcp", "/mcp/packages/configuration", "/mcp/packages/rich-output"}
	server.cfg.Sandbox = launch
	bindSandboxMCPTestOwner(t, server, launch)
	for _, test := range []struct {
		path, registry string
		status         int
	}{
		{"", "kandev-mcp", http.StatusOK},
		{"/packages/configuration", "kandev-configuration", http.StatusOK},
		{"/packages/rich-output", "", http.StatusNotFound},
		{"/packages/diagnostics", "", http.StatusForbidden},
		{"/packages/configuration?other=1", "", http.StatusForbidden},
		{"/packages/%63onfiguration", "", http.StatusForbidden},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/agent/sandbox/mcp"+test.path,
			strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`))
		request.Header.Set("Authorization", "Bearer control-secret")
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		request.Header.Set("X-Kandev-Sandbox-Attempt", launch.AttemptID)
		request.Header.Set("X-Kandev-Sandbox-Generation", strconv.FormatUint(launch.Generation, 10))
		request.Header.Set("X-Kandev-Sandbox-Policy", launch.PolicyDigest)
		request.Header.Set(sandboxmcp.MCPPathHeader, "/mcp/packages/diagnostics")
		response := httptest.NewRecorder()
		server.Router().ServeHTTP(response, request)
		require.Equal(t, test.status, response.Code, test.path)
		if test.registry != "" {
			var body struct {
				Result struct {
					ServerInfo struct{ Name string } `json:"serverInfo"`
				} `json:"result"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			require.Equal(t, test.registry, body.Result.ServerInfo.Name)
		}
	}
}

func bindSandboxMCPTestOwner(t *testing.T, server *Server, launch *sandbox.Launch) {
	t.Helper()
	command := exec.CommandContext(context.Background(), "/bin/sh", "-c", "read ignored")
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	input, err := command.StdinPipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = input.Close() })
	require.NoError(t, command.Start())
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	owner, err := sandbox.CaptureProcessOwner(launch, command.Process.Pid)
	require.NoError(t, err)
	require.NoError(t, server.procMgr.SetSandboxOwner(owner))
}
