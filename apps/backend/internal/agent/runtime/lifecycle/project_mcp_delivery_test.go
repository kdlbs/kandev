package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types"
)

type projectDeliveryHandoffCapture struct {
	mu        sync.Mutex
	configure []map[string]any
	started   int
}

func (c *projectDeliveryHandoffCapture) configureBodies() []map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]map[string]any(nil), c.configure...)
}

func (c *projectDeliveryHandoffCapture) startCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.started
}

func newProjectDeliveryHandoffServer(t *testing.T, capable bool) (*httptest.Server, *projectDeliveryHandoffCapture) {
	t.Helper()
	capture := &projectDeliveryHandoffCapture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity":
			capabilities := []string{"agent-survival.v1"}
			if capable {
				capabilities = append(capabilities, types.ProjectMCPDeliveryCapability)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": capabilities})
		case "/api/v1/agent/configure":
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			capture.mu.Lock()
			capture.configure = append(capture.configure, body)
			capture.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		case "/api/v1/start":
			capture.mu.Lock()
			capture.started++
			capture.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "command": "pi-acp"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server, capture
}

func projectDeliveryHandoffExecution(t *testing.T, server *httptest.Server) *AgentExecution {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)
	execution := &AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "session-1", WorkspaceID: "workspace-1",
		WorkspacePath: t.TempDir(), AgentCommand: "pi-acp", metadata: map[string]interface{}{},
		agentctl: agentctl.NewClient(parsed.Hostname(), port, newTestLogger(), agentctl.WithExecutionID("instance-1")),
	}
	execution.setProjectMCPDelivery(&types.ProjectMCPDelivery{
		Version: types.ProjectMCPDeliveryVersion, Strategy: types.ProjectMCPDeliveryStrategyPi,
		Workspace: execution.WorkspacePath, TaskID: execution.TaskID, InstanceID: "instance-1",
		BindingDigest: strings.Repeat("a", 64), Packages: []string{"core"},
	})
	return execution
}

// @covers AC-AGENTS-BUILTIN-MCP-003.5, AC-AGENTS-BUILTIN-MCP-003.6
func TestProjectMCPDeliveryConfigureHandoff(t *testing.T) {
	server, capture := newProjectDeliveryHandoffServer(t, true)
	mgr := newTestManager(t)
	execution := projectDeliveryHandoffExecution(t, server)
	_, err := mgr.configureAndStartAgentWithPreparedArgs(context.Background(), execution, []string{"pi-acp"})
	require.NoError(t, err)

	configured := capture.configureBodies()
	require.Len(t, configured, 1)
	raw, present := configured[0]["project_mcp_delivery"]
	require.True(t, present, "configure must carry execution-bound project delivery")
	encoded, err := json.Marshal(raw)
	require.NoError(t, err)
	var delivery types.ProjectMCPDelivery
	require.NoError(t, json.Unmarshal(encoded, &delivery))
	require.Equal(t, "instance-1", delivery.InstanceID)
	require.Equal(t, []string{"core"}, delivery.Packages)
	require.Equal(t, 1, capture.startCount())
}

// @covers AC-AGENTS-BUILTIN-MCP-003.5
func TestProjectMCPDeliveryConfigureFailsBeforeOldHelper(t *testing.T) {
	server, capture := newProjectDeliveryHandoffServer(t, false)
	mgr := newTestManager(t)
	execution := projectDeliveryHandoffExecution(t, server)
	_, err := mgr.configureAndStartAgentWithPreparedArgs(context.Background(), execution, []string{"pi-acp"})
	require.ErrorContains(t, err, "does not support project-mcp-delivery-v1")
	require.Empty(t, capture.configureBodies(), "an old helper must fail before configuration")
	require.Zero(t, capture.startCount(), "an old helper must fail before process start")
}
