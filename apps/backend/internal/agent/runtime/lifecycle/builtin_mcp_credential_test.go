package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

const projectMCPTestCredential = "test-instance-credential"

// newBuiltinMCPCredentialTestClient returns an instance client whose agentctl
// serves credential on the built-in MCP credential route. An empty credential
// simulates an agentctl that predates the route.
func newBuiltinMCPCredentialTestClient(t *testing.T, credential string) *agentctl.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != types.BuiltinMCPCredentialPath || credential == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.BuiltinMCPCredentialResponse{Credential: credential, Enforced: true})
	}))
	t.Cleanup(srv.Close)
	host, port := splitHostPortForTest(t, srv.URL)
	client := agentctl.NewClient(host, port, newTestLogger(), agentctl.WithExecutionID("instance-1"))
	t.Cleanup(client.Close)
	return client
}

func splitHostPortForTest(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)
	return parsed.Hostname(), port
}

func credentialTestExecution(t *testing.T, credential string, packages ...p.PackageID) *AgentExecution {
	t.Helper()
	execution := &AgentExecution{
		ID: "execution", SessionID: "session", TaskID: "task-a", WorkspaceID: "workspace",
		WorkspacePath: t.TempDir(), standalonePort: 41234, metadata: map[string]interface{}{},
		agentctl: newBuiltinMCPCredentialTestClient(t, credential),
	}
	if len(packages) > 0 {
		encoded, err := p.MarshalBuiltinBinding(builtinDispatchProfile(packages...))
		require.NoError(t, err)
		execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
	}
	return execution
}

func TestPassthroughStrategiesCarryInstanceCredential(t *testing.T) {
	mgr := newTestManager(t)
	want := "Bearer " + projectMCPTestCredential
	for _, packages := range [][]p.PackageID{nil, {p.PackageCore, p.PackageConfiguration}} {
		execution := credentialTestExecution(t, projectMCPTestCredential, packages...)
		operation := &preparedMCPOperation{servers: []types.McpServer{{Name: "third-party", Type: "http", URL: "https://example.test/mcp", Headers: map[string]string{"X-Key": "user"}}}}
		servers, err := mgr.passthroughMCPServers(context.Background(), execution, nil, nil, "", mcpconfig.CodexStrategy{}, operation)
		require.NoError(t, err)
		for _, server := range servers {
			if server.Name == "third-party" {
				require.Equal(t, map[string]string{"X-Key": "user"}, server.Headers, "user headers are untouched")
				continue
			}
			require.Equal(t, map[string]string{"Authorization": want}, server.Headers, server.Name)
		}
		paths := mcpconfig.PassthroughPaths{TempConfigPath: filepath.Join(execution.WorkspacePath, "mcp.json"), WorkspaceDir: execution.WorkspacePath}
		for name, strategy := range map[string]mcpconfig.PassthroughMCPStrategy{
			"claude": mcpconfig.ClaudeStrategy{}, "codex": mcpconfig.CodexStrategy{}, "opencode": mcpconfig.OpenCodeStrategy{},
			"cursor": mcpconfig.CursorStrategy{}, "pi": mcpconfig.PiStrategy{},
		} {
			artifacts, err := strategy.BuildPassthroughMCP(servers, paths)
			require.NoError(t, err, name)
			rendered := strings.Join(artifacts.Args, " ")
			for _, file := range artifacts.Files {
				rendered += string(file.Content)
			}
			for _, value := range artifacts.Env {
				rendered += value
			}
			require.Contains(t, rendered, want, name)
		}
	}
}

func TestPassthroughWithoutCredentialRouteKeepsHeaderlessEntries(t *testing.T) {
	mgr := newTestManager(t)
	execution := credentialTestExecution(t, "")
	servers, err := mgr.passthroughMCPServers(context.Background(), execution, nil, nil, "", mcpconfig.CodexStrategy{}, &preparedMCPOperation{})
	require.NoError(t, err)
	require.NotEmpty(t, servers)
	for _, server := range servers {
		require.Empty(t, server.Headers)
	}
}

func TestPassthroughFailsWhenCredentialReadFails(t *testing.T) {
	mgr := newTestManager(t)
	execution := credentialTestExecution(t, projectMCPTestCredential)
	execution.agentctl = agentctl.NewClient("127.0.0.1", 1, newTestLogger())
	t.Cleanup(execution.agentctl.Close)
	_, err := mgr.passthroughMCPServers(context.Background(), execution, nil, nil, "", mcpconfig.CodexStrategy{}, &preparedMCPOperation{})
	require.Error(t, err, "an unreadable credential must not produce header-less entries")
}

func TestCursorProjectFileCarriesInstanceCredential(t *testing.T) {
	for _, packages := range [][]p.PackageID{nil, {p.PackageCore, p.PackageConfiguration}} {
		mgr := newTestManager(t)
		execution := credentialTestExecution(t, projectMCPTestCredential, packages...)
		workspace := execution.WorkspacePath
		key := cursorProjectMCPWorkspaceKey(workspace)
		generation := beginCursorProjectMCPPreparation(key)
		_, err := mgr.reconcileCursorProjectMCPFiles(context.Background(), workspace, key, generation, execution, nil, nil)
		finishCursorProjectMCPPreparation(key, generation)
		require.NoError(t, err)
		data, err := os.ReadFile(filepath.Join(workspace, ".cursor", "mcp.json"))
		require.NoError(t, err)
		var config struct {
			Servers map[string]struct {
				URL     string            `json:"url"`
				Headers map[string]string `json:"headers"`
			} `json:"mcpServers"`
		}
		require.NoError(t, json.Unmarshal(data, &config))
		require.Contains(t, config.Servers, "kandev")
		for name, server := range config.Servers {
			require.Equal(t, "Bearer "+projectMCPTestCredential, server.Headers["Authorization"], name)
		}
	}
}

func TestCursorBuiltinFingerprintChangesWithCredential(t *testing.T) {
	first, err := canonicalJSONFingerprint(cursorBuiltinMCPEntry("http://localhost:1/mcp", map[string]string{"Authorization": "Bearer a"}))
	require.NoError(t, err)
	second, err := canonicalJSONFingerprint(cursorBuiltinMCPEntry("http://localhost:1/mcp", map[string]string{"Authorization": "Bearer b"}))
	require.NoError(t, err)
	require.NotEqual(t, first, second)
}

func TestProjectMCPFileUsesReplacementInstanceCredential(t *testing.T) {
	mgr := newTestManager(t)
	agentConfig, ok := mgr.registry.Get("pi-acp")
	require.True(t, ok)
	execution := newProjectMCPDeliveryTestExecution(t)
	execution.agentctl = newBuiltinMCPCredentialTestClient(t, "retiring-credential")
	replacement := newBuiltinMCPCredentialTestClient(t, "replacement-credential")

	err := mgr.materializeRuntimeProjectMCPWithClient(context.Background(), execution, agentConfig, nil, "", replacement, nil)
	require.NoError(t, err)

	written, err := os.ReadFile(filepath.Join(execution.WorkspacePath, ".pi", "mcp.json"))
	require.NoError(t, err)
	require.Contains(t, string(written), "Bearer replacement-credential")
	require.NotContains(t, string(written), "retiring-credential")
}
