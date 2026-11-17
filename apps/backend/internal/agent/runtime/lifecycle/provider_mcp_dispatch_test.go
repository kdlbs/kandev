package lifecycle

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinGenerationBackendRejectsStaleScope(t *testing.T) {
	profile := builtinDispatchProfile(p.PackageCore)
	sealed, err := p.MarshalBuiltinBinding(profile)
	require.NoError(t, err)
	digest, err := p.BuiltinBindingDigest(profile)
	require.NoError(t, err)
	scope := p.ProviderMCPScope{BindingID: "current", ExecutionID: "execution", HelperID: "helper", ProcessGeneration: 2, HarnessGeneration: 2, ProfileDigest: digest}
	binding := p.ProviderMCPBinding{Scope: scope, Profile: sealed}
	e := &AgentExecution{ID: "execution", TaskID: profile.BuiltinPackages.TaskID, SessionID: "session", WorkspaceID: profile.BuiltinPackages.WorkspaceID}
	e.setMetadataValue(p.BuiltinBindingMetadataKey, sealed)
	e.DeliveryHarnessGeneration = 2
	e.setMetadataValue(p.ProviderMCPBindingMetadataKey, binding)
	for _, condition := range []string{"current", "missing", "old process", "old binding", "foreign execution", "invalid", "conversation generation advanced", "forged journal provenance"} {
		t.Run(condition, func(t *testing.T) {
			e.DeliveryHarnessGeneration = 2
			if condition == "conversation generation advanced" {
				e.DeliveryHarnessGeneration = 3
			}
			inner := &recordingMCPHandler{}
			sm := newMCPStreamManager(t, inner, nil)
			sm.mcpPrincipalScoper = builtinTestPrincipal
			sm.builtinActionResolver = func(p.Context) ([]string, error) { return []string{"mcp.list_tasks"}, nil }
			msg := mcpRequest(t, nil)
			sent := scope
			switch condition {
			case "forged journal provenance":
				sent.HarnessGeneration++
			case "old process":
				sent.ProcessGeneration = 1
			case "old binding":
				sent.BindingID = "old"
			case "foreign execution":
				sent.ExecutionID = "foreign"
			}
			raw, err := json.Marshal(sent)
			require.NoError(t, err)
			if condition != "missing" {
				msg.EnsureMetadata()[p.ProviderMCPScopeMetadataKey] = string(raw)
			}
			if condition == "invalid" {
				msg.Metadata[p.ProviderMCPScopeMetadataKey] = strings.Repeat("x", 30)
			}
			_, err = sm.mcpHandlerFor(e).Dispatch(context.Background(), msg)
			require.NoError(t, err)
			if condition == "current" || condition == "conversation generation advanced" {
				require.Equal(t, 1, inner.calls)
				actual, ok := streams.ProviderMCPScopeFromContext(inner.gotCtx)
				require.True(t, ok)
				require.Equal(t, scope, actual)
				executionContext, ok := streams.MCPExecutionContextFromContext(inner.gotCtx)
				require.True(t, ok)
				require.Equal(t, &scope, executionContext.ProviderScope)
			} else {
				require.Zero(t, inner.calls)
			}
		})
	}
}

func TestBuiltinGenerationBackendRetirementWaitsForEffects(t *testing.T) {
	e := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session"}
	msg := mcpRequest(t, nil)
	release, _, err := e.beginProviderMCPEffect(msg)
	require.NoError(t, err)
	require.Error(t, e.closeProviderMCPEffects())
	release()
	require.NoError(t, e.closeProviderMCPEffects())
	_, _, err = e.beginProviderMCPEffect(msg)
	require.Error(t, err)
}

func TestBuiltinGenerationBackendCorruptBarrierFailsClosed(t *testing.T) {
	e := &AgentExecution{}
	e.setMetadataValue(p.ProviderMCPEffectsClosedMetadataKey, map[string]any{"invalid": true})
	require.NotPanics(t, func() { _, _, err := e.beginProviderMCPEffect(mcpRequest(t, nil)); require.Error(t, err) })
}

func TestBuiltinGenerationManifestSnapshotUsesAcceptedBinding(t *testing.T) {
	initial, err := p.MarshalBuiltinBinding(builtinDispatchProfile(p.PackageCore))
	require.NoError(t, err)
	current := builtinDispatchProfile(p.PackageCore, p.PackageConfiguration)
	current.BuiltinPackages.TaskRevision = 3
	sealed, err := p.MarshalBuiltinBinding(current)
	require.NoError(t, err)
	digest, err := p.BuiltinBindingDigest(current)
	require.NoError(t, err)
	e := &AgentExecution{ID: "execution", TaskID: "task-a", SessionID: "session", WorkspaceID: "workspace"}
	e.setMetadataValue(p.BuiltinBindingMetadataKey, initial)
	e.setMetadataValue(p.ProviderMCPBindingMetadataKey, p.ProviderMCPBinding{Scope: p.ProviderMCPScope{BindingID: "accepted", ExecutionID: "execution", HelperID: "helper", ProcessGeneration: 2, HarnessGeneration: 2, ProfileDigest: digest}, Profile: sealed})
	actual := e.builtinMCPManifestSnapshot()
	require.NotNil(t, actual)
	require.Equal(t, int64(3), actual.TaskRevision)
	require.ElementsMatch(t, current.BuiltinPackages.Packages, actual.Packages)
	require.Equal(t, current.BuiltinPackages.Origins, actual.Origins)
	original, _ := e.metadataValue(p.BuiltinBindingMetadataKey)
	require.Equal(t, initial, original, "the initial execution binding stays immutable")
}
