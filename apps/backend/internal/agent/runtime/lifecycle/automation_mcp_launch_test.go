package lifecycle

import (
	"context"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
	"testing"
)

type automationLaunchCatalog struct {
	mcpconfig.CatalogRepository
	definition *mcpconfig.MCPServerDefinition
}

func (c automationLaunchCatalog) ListMCPServerDefinitions(context.Context, string) ([]*mcpconfig.MCPServerDefinition, error) {
	return []*mcpconfig.MCPServerDefinition{c.definition}, nil
}
func (c automationLaunchCatalog) GetMCPServerDefinition(context.Context, string, string) (*mcpconfig.MCPServerDefinition, error) {
	return c.definition, nil
}

type automationLaunchSelections struct{ mcpconfig.SelectionRepository }

func (automationLaunchSelections) ListMCPSelections(context.Context, mcpconfig.SelectionScope, string, string) ([]string, error) {
	return nil, nil
}

func TestAutomationMCPLaunchConvertsCapturedManifestToResolverInput(t *testing.T) {
	manager, _ := createTestManagerWithTracking()
	definition := &mcpconfig.MCPServerDefinition{ID: "server", WorkspaceID: "workspace", DisplayName: "Reports", RuntimeName: "reports", NormalizedRuntimeName: "reports", Enabled: true, ExecutionMode: mcpconfig.ExecutionModeRemote, Transport: mcpconfig.ServerTypeHTTP, Configuration: mcpconfig.MCPServerConfiguration{URL: "https://reports.example.test/mcp"}}
	manager.mcpResolver = mcpconfig.NewResolver(automationLaunchCatalog{definition: definition}, automationLaunchSelections{})
	profile := p.NewAutomation()
	profile.BuiltinPackages = &p.BuiltinManifest{Version: 2, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: p.SurfaceAutomation, AutomationID: "automation", AutomationRevision: 1, AutomationDefinitionIDs: []string{"server"}, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
	encoded, err := p.MarshalBuiltinBinding(profile)
	require.NoError(t, err)
	result, err := manager.resolveEffectiveMCPResolution(t.Context(), mcpconfig.ResolutionContext{WorkspaceID: "workspace", TaskID: "task", SessionID: "session"}, "", map[string]interface{}{p.BuiltinBindingMetadataKey: encoded}, agents.NewMockAgent(), agentruntime.RuntimeStandalone)
	require.NoError(t, err)
	require.Len(t, result.Servers, 1)
	require.Equal(t, "https://reports.example.test/mcp", result.Servers[0].URL)
	require.Equal(t, []mcpconfig.SelectionOrigin{{Scope: mcpconfig.SelectionScope("automation"), WorkspaceID: "workspace", OwnerID: "automation"}}, result.Servers[0].Origins)
}
