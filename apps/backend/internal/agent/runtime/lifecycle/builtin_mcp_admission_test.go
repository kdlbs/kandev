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

func TestBuiltinStartupVerifiesBindingBeforeProcessAdmission(t *testing.T) {
	for _, kind := range []string{"corrupt", "foreign", "different", "missing-helper"} {
		t.Run(kind, func(t *testing.T) {
			helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if kind == "missing-helper" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				profile := builtinDispatchProfile(p.PackageCore)
				profile.BuiltinPackages.TaskID = "different"
				_ = json.NewEncoder(w).Encode(profile)
			}))
			t.Cleanup(helper.Close)
			_, execution := workspaceSourceTestManager(t, helper.URL, nil)
			mgr := newTestManager(t)
			mgr.SetBuiltinActionResolver(builtinTestActions)
			mgr.streamManager.mcpPrincipalScoper = builtinTestPrincipal
			execution.TaskID, execution.WorkspaceID = "task-a", "workspace"
			profile := builtinDispatchProfile(p.PackageCore)
			if kind == "foreign" {
				profile.BuiltinPackages.TaskID = "other"
			}
			encoded, err := p.MarshalBuiltinBinding(profile)
			require.NoError(t, err)
			if kind == "corrupt" {
				encoded = "{}"
			}
			execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
			require.NoError(t, mgr.executionStore.Add(execution))
			err = mgr.StartAgentProcess(context.Background(), execution.ID)
			require.ErrorContains(t, err, "built-in MCP")
		})
	}
}

func TestBuiltinLaunchRejectsInvalidOrUnenforceableBinding(t *testing.T) {
	for _, kind := range []string{"foreign-task", "foreign-workspace", "invalid", "unwired"} {
		t.Run(kind, func(t *testing.T) {
			mgr := newTestManager(t)
			profile := builtinDispatchProfile(p.PackageCore)
			req := &LaunchRequest{TaskID: "task-a", SessionID: "session-a", WorkspaceID: "workspace", McpProfile: &profile}
			switch kind {
			case "foreign-task":
				req.TaskID = "other"
			case "foreign-workspace":
				req.WorkspaceID = "other"
			case "invalid":
				profile.BuiltinPackages.Version = 99
			}
			_, err := mgr.Launch(context.Background(), req)
			require.ErrorContains(t, err, "built-in MCP")
		})
	}
}

func TestBuiltinExecutionVerificationAcceptsExactBindingAndLeavesLegacyUnknown(t *testing.T) {
	calls := 0
	profile := builtinDispatchProfile(p.PackageCore)
	helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, http.MethodGet, r.Method)
		_ = json.NewEncoder(w).Encode(profile)
	}))
	t.Cleanup(helper.Close)
	_, execution := workspaceSourceTestManager(t, helper.URL, nil)
	mgr := newTestManager(t)
	mgr.SetBuiltinActionResolver(builtinTestActions)
	mgr.streamManager.mcpPrincipalScoper = builtinTestPrincipal
	execution.TaskID, execution.WorkspaceID = "task-a", "workspace"
	require.NoError(t, mgr.verifyBuiltinExecution(context.Background(), execution))
	require.Zero(t, calls)
	encoded, err := p.MarshalBuiltinBinding(profile)
	require.NoError(t, err)
	execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
	require.NoError(t, mgr.verifyBuiltinExecution(context.Background(), execution))
	require.Equal(t, 1, calls)
}
