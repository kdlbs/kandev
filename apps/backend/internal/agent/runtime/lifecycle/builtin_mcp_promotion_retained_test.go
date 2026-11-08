package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinPromotionVerifiesRetainedBindingWithoutReplacingIt(t *testing.T) {
	for _, condition := range []string{"retained", "mismatch", "corrupt"} {
		t.Run(condition, func(t *testing.T) {
			bound := builtinDispatchProfile(p.PackageCore)
			encoded, err := p.MarshalBuiltinBinding(bound)
			require.NoError(t, err)
			calls := 0
			helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "/api/v1/mcp/builtin-binding", r.URL.Path)
				confirmed := bound
				if condition == "mismatch" {
					confirmed = builtinDispatchProfile(p.PackageCore, p.PackageDiagnostics)
				}
				_ = json.NewEncoder(w).Encode(confirmed)
			}))
			t.Cleanup(helper.Close)
			_, execution := workspaceSourceTestManager(t, helper.URL, nil)
			mgr := newTestManager(t)
			mgr.SetBuiltinActionResolver(builtinTestActions)
			mgr.streamManager.mcpPrincipalScoper = builtinTestPrincipal
			execution.TaskID, execution.SessionID, execution.WorkspaceID = "task-a", "session-a", "workspace"
			if condition == "corrupt" {
				encoded = "{}"
			}
			require.NoError(t, mgr.executionStore.Add(execution))
			execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
			desired := builtinDispatchProfile(p.PackageCore, p.PackageConfiguration)
			err = mgr.prepareBuiltinWorkspaceExecution(context.Background(), execution, &LaunchRequest{McpProfile: &desired})
			if condition == "retained" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			actual, _ := execution.metadataValue(p.BuiltinBindingMetadataKey)
			require.Equal(t, encoded, actual, "promotion must not replace retained or invalid evidence")
			if condition == "corrupt" {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
			}
		})
	}
}

func TestAdoptedStandalonePeerRestoresPersistedBuiltinBinding(t *testing.T) {
	bound := builtinDispatchProfile(p.PackageCore)
	encoded, err := p.MarshalBuiltinBinding(bound)
	require.NoError(t, err)
	var calls []string
	helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		require.Equal(t, "/api/v1/mcp/builtin-binding", r.URL.Path)
		_ = json.NewEncoder(w).Encode(bound)
	}))
	t.Cleanup(helper.Close)
	parsed, err := url.Parse(helper.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)

	mgr := newTestManager(t)
	mgr.SetBuiltinActionResolver(builtinTestActions)
	mgr.streamManager.mcpPrincipalScoper = builtinTestPrincipal
	executor := NewStandaloneExecutor(
		agentctl.NewControlClient(parsed.Hostname(), port, newTestLogger()),
		parsed.Hostname(), port, newTestLogger(),
	)
	request := &ExecutorCreateRequest{
		InstanceID: "requested-execution", TaskID: "task-a", SessionID: "session-a",
		WorkspaceID: "workspace", WorkspacePath: t.TempDir(),
		RetainedBuiltinBinding: encoded,
		McpProfile:             &bound,
		Env:                    map[string]string{"REQUEST_ONLY": "value"},
		WorkspaceSourceRoots:   []string{"/replacement/root"},
	}
	info := &agentctl.InstanceInfo{
		ID: "execution", Port: port, TaskID: "task-a", SessionID: "session-a",
		WorkspacePath: request.WorkspacePath, WorkspaceSourceRoots: []string{}, Env: map[string]string{},
	}
	instance, err := executor.buildAdoptedStandaloneInstance(nil, request, info)
	require.NoError(t, err)
	execution := instance.ToAgentExecution(request)
	require.NoError(t, mgr.executionStore.Add(execution))

	require.NoError(t, mgr.prepareBuiltinWorkspaceExecution(context.Background(), execution, &LaunchRequest{McpProfile: &bound}))
	require.Equal(t, []string{"GET /api/v1/mcp/builtin-binding"}, calls,
		"retained adoption must verify the helper binding without preparing or rebinding it")
	actual, boundNow := execution.metadataValue(p.BuiltinBindingMetadataKey)
	require.True(t, boundNow)
	require.Equal(t, encoded, actual)
}

func TestAdoptedStandalonePeerWithoutBindingBlocksBuiltinPreparation(t *testing.T) {
	bound := builtinDispatchProfile(p.PackageCore)
	helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected helper request %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(helper.Close)
	parsed, err := url.Parse(helper.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)
	executor := NewStandaloneExecutor(
		agentctl.NewControlClient(parsed.Hostname(), port, newTestLogger()),
		parsed.Hostname(), port, newTestLogger(),
	)
	request := &ExecutorCreateRequest{
		InstanceID: "requested-execution", TaskID: "task-a", SessionID: "session-a",
		WorkspaceID: "workspace", WorkspacePath: t.TempDir(), McpProfile: &bound,
	}
	info := &agentctl.InstanceInfo{
		ID: "execution", Port: port, TaskID: "task-a", SessionID: "session-a",
		WorkspacePath: request.WorkspacePath, WorkspaceSourceRoots: []string{}, Env: map[string]string{},
	}
	_, err = executor.buildAdoptedStandaloneInstance(nil, request, info)
	require.ErrorIs(t, err, errStandaloneJournalOwnerBlocked)
	require.Contains(t, err.Error(), "no persisted built-in MCP binding")
}

func TestNewWorkspaceHelperDoesNotInheritPreviousProcessBinding(t *testing.T) {
	manager := newTestManager(t)
	prior, err := p.MarshalBuiltinBinding(builtinDispatchProfile(p.PackageCore))
	require.NoError(t, err)
	info := &WorkspaceInfo{TaskID: "task-a", SessionID: "session-a", WorkspaceID: "workspace", AgentID: "auggie", ExecutorType: "local", WorkspacePath: t.TempDir(), Metadata: map[string]interface{}{p.BuiltinBindingMetadataKey: prior}}
	prepared, err := manager.prepareExecutionCreateRequest(context.Background(), info.TaskID, info, "new-execution")
	require.NoError(t, err)
	require.NotContains(t, prepared.request.Metadata, p.BuiltinBindingMetadataKey)
	require.Equal(t, prior, prepared.request.RetainedBuiltinBinding)
	require.Equal(t, prior, info.Metadata[p.BuiltinBindingMetadataKey])
}
