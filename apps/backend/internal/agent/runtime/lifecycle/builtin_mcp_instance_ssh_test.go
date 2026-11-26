package lifecycle

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentruntime"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

// @covers AC-AGENTS-MCP-INSTANCE-004.3
func TestBuiltinMCPInstanceSSHPreparationUsesSnapshotClient(t *testing.T) {
	newCalls := 0
	newServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		newCalls++
		http.NotFound(w, request)
	}))
	t.Cleanup(newServer.Close)
	newHost, newPort := testHTTPHostPort(t, newServer)
	newClient := agentctl.NewClient(newHost, newPort, newTestLogger(),
		agentctl.WithExecutionID("replacement-helper"), agentctl.WithBuiltinMCPScopedRoutes(false))
	t.Cleanup(newClient.Close)

	var execution *AgentExecution
	var oldPrepare types.PassthroughPrepareRequest
	oldPrepareCalls := 0
	oldServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/passthrough":
			execution.replaceAgentctlClient(newClient)
			_ = json.NewEncoder(w).Encode(types.PassthroughSnapshot{
				Version: types.PassthroughProtocolVersion, InstanceID: "ssh-helper", SessionID: "session-1",
				PassthroughTarget: types.PassthroughTarget{Generation: 4},
				WorkspacePath:     "/remote/workspace", ConfigPath: "/remote/.kandev/mcp.json",
				MCPURL: "http://127.0.0.1:43210/i/ssh-helper/mcp",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/passthrough/prepare":
			oldPrepareCalls++
			require.NoError(t, json.NewDecoder(r.Body).Decode(&oldPrepare))
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(oldServer.Close)
	oldHost, oldPort := testHTTPHostPort(t, oldServer)
	oldClient := agentctl.NewClient(oldHost, oldPort, newTestLogger(),
		agentctl.WithExecutionID("ssh-helper"), agentctl.WithBuiltinMCPScopedRoutes(true))
	t.Cleanup(oldClient.Close)

	profile := builtinDispatchProfile(p.PackageCore, p.PackageConfiguration)
	encoded, err := p.MarshalBuiltinBinding(profile)
	require.NoError(t, err)
	execution = &AgentExecution{
		ID: "ssh-helper", TaskID: "task-a", SessionID: "session-1", WorkspaceID: "workspace",
		RuntimeName: agentruntime.RuntimeSSH, ExecutorType: "ssh", metadata: map[string]interface{}{}, agentctl: oldClient,
	}
	execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
	manager := newTestManager(t)
	operation := &preparedMCPOperation{}

	prepared, err := manager.applySSHPassthroughMCP(
		context.Background(), execution, agents.PassthroughConfig{MCPStrategy: mcpconfig.ClaudeStrategy{}}, nil, nil, operation,
	)

	require.NoError(t, err)
	require.Equal(t, 1, oldPrepareCalls)
	require.Zero(t, newCalls, "preparation must remain on the client whose snapshot was read")
	require.Equal(t, uint64(5), oldPrepare.Generation)
	require.Len(t, oldPrepare.Files, 1)
	requireArtifactURL(t, oldPrepare.Files[0].Content, "mcpServers", "kandev", "http://127.0.0.1:43210/i/ssh-helper/mcp")
	requireArtifactURL(t, oldPrepare.Files[0].Content, "mcpServers", "kandev-configuration", "http://127.0.0.1:43210/i/ssh-helper/mcp/packages/configuration")
	require.NotEmpty(t, prepared.args)
	current, release := execution.AcquireAgentCtlClient()
	require.Same(t, newClient, current, "the test replaces the published client while the old lease remains in use")
	release()
}

func testHTTPHostPort(t *testing.T, server *httptest.Server) (string, int) {
	t.Helper()
	host, rawPort, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(rawPort)
	require.NoError(t, err)
	return host, port
}
