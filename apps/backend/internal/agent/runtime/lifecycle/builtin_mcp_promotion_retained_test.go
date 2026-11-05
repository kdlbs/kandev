package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestNewWorkspaceHelperDoesNotInheritPreviousProcessBinding(t *testing.T) {
	manager := newTestManager(t)
	prior, err := p.MarshalBuiltinBinding(builtinDispatchProfile(p.PackageCore))
	require.NoError(t, err)
	info := &WorkspaceInfo{TaskID: "task-a", SessionID: "session-a", WorkspaceID: "workspace", AgentID: "auggie", ExecutorType: "local", WorkspacePath: t.TempDir(), Metadata: map[string]interface{}{p.BuiltinBindingMetadataKey: prior}}
	prepared, err := manager.prepareExecutionCreateRequest(context.Background(), info.TaskID, info, "new-execution")
	require.NoError(t, err)
	require.NotContains(t, prepared.request.Metadata, p.BuiltinBindingMetadataKey)
	require.Equal(t, prior, info.Metadata[p.BuiltinBindingMetadataKey])
}
