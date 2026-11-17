package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
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

func TestBuiltinVerificationUsesAcceptedProviderBinding(t *testing.T) {
	for _, scenario := range []string{"current", "stale-helper", "closed", "current-before-journal", "after-context-reset"} {
		t.Run(scenario, func(t *testing.T) {
			current := builtinDispatchProfile(p.PackageCore, p.PackageConfiguration)
			sealed, err := p.MarshalBuiltinBinding(current)
			require.NoError(t, err)
			digest, err := p.BuiltinBindingDigest(current)
			require.NoError(t, err)
			var binding p.ProviderMCPBinding
			var sessionID string
			helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/agent/provider-replacement" {
					scope := binding.Scope
					if scenario == "after-context-reset" {
						scope.HarnessGeneration = 2
					}
					if scenario == "stale-helper" {
						scope.ProcessGeneration++
					}
					_ = json.NewEncoder(w).Encode(types.ProviderReplacementStatus{Phase: "idle", Source: types.ProviderReplacementSource{ExecutionID: scope.ExecutionID, SessionID: sessionID, TaskID: "task-a", HelperID: scope.HelperID, BindingID: scope.BindingID, ProfileDigest: scope.ProfileDigest, ProcessGeneration: scope.ProcessGeneration, HarnessGeneration: scope.HarnessGeneration, IncarnationID: "incarnation", NativeSessionID: "native"}})
					return
				}
				_ = json.NewEncoder(w).Encode(current)
			}))
			defer helper.Close()
			_, execution := workspaceSourceTestManager(t, helper.URL, nil)
			sessionID = execution.SessionID
			mgr := newTestManager(t)
			mgr.SetBuiltinActionResolver(builtinTestActions)
			mgr.streamManager.mcpPrincipalScoper = builtinTestPrincipal
			execution.TaskID = "task-a"
			execution.WorkspaceID = "workspace"
			execution.DeliveryHarnessGeneration = 1
			if scenario == "after-context-reset" {
				execution.DeliveryHarnessGeneration = 2
			}
			if scenario == "current-before-journal" {
				execution.DeliveryHarnessGeneration = 0
				mgr.SetAgentDeliveryRepository(&adoptionDeliveryRepository{generation: &models.HarnessSessionGeneration{SessionID: sessionID, IncarnationID: "incarnation", Generation: 1, NativeSessionID: "native"}})
			}
			initial, err := p.MarshalBuiltinBinding(builtinDispatchProfile(p.PackageCore))
			require.NoError(t, err)
			execution.setMetadataValue(p.BuiltinBindingMetadataKey, initial)
			binding = p.ProviderMCPBinding{Scope: p.ProviderMCPScope{BindingID: "current", ExecutionID: execution.ID, HelperID: "helper", ProcessGeneration: 2, HarnessGeneration: 1, ProfileDigest: digest}, Profile: sealed}
			execution.setMetadataValue(p.ProviderMCPBindingMetadataKey, binding)
			if scenario == "closed" {
				execution.setMetadataValue(p.ProviderMCPEffectsClosedMetadataKey, true)
			}
			err = mgr.verifyBuiltinExecution(context.Background(), execution)
			if scenario == "current" || scenario == "current-before-journal" || scenario == "after-context-reset" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestBuiltinRecoveryRetainsClosedProviderAndWorkspace(t *testing.T) {
	calls := 0
	helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(http.StatusConflict) }))
	defer helper.Close()
	_, execution := workspaceSourceTestManager(t, helper.URL, nil)
	mgr := newTestManager(t)
	mgr.SetBuiltinActionResolver(builtinTestActions)
	mgr.streamManager.mcpPrincipalScoper = builtinTestPrincipal
	execution.TaskID = "task-a"
	execution.WorkspaceID = "workspace"
	sealed, err := p.MarshalBuiltinBinding(builtinDispatchProfile(p.PackageCore))
	require.NoError(t, err)
	execution.setMetadataValue(p.BuiltinBindingMetadataKey, sealed)
	execution.setMetadataValue(p.ProviderMCPEffectsClosedMetadataKey, true)
	require.NoError(t, mgr.restoreBuiltinExecution(context.Background(), execution), "a closed provider must retain its authenticated workspace while the coordinator reconciles the operation")
	require.Zero(t, calls)
	require.True(t, execution.providerMCPEffectsClosed())
	require.Error(t, mgr.verifyBuiltinExecution(context.Background(), execution), "recovery must not admit a prompt")
}
