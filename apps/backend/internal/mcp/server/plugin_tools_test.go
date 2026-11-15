package mcp

import (
	"testing"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func instanceReadDefinition(pluginID, localName, installationID, policyVersion string, surfaces ...string) plugintools.Definition {
	return plugintools.Definition{
		PluginID: pluginID, LocalName: localName,
		ExposedName:     plugintools.ExposedName(pluginID, localName),
		Description:     localName,
		Surfaces:        surfaces,
		InvocationScope: plugintools.InvocationScopeInstance,
		InstallationID:  installationID,
		PolicyVersion:   policyVersion,
		InputSchema:     []byte(`{"type":"object"}`),
		ReadOnlyHint:    true,
	}
}

func workspaceReadDefinition(pluginID, localName, installationID, policyVersion string, surfaces ...string) plugintools.Definition {
	definition := instanceReadDefinition(pluginID, localName, installationID, policyVersion, surfaces...)
	definition.InvocationScope = plugintools.InvocationScopeWorkspace
	definition.InputSchema = []byte(`{"type":"object","properties":{"key":{"type":"string"}}}`)
	return definition
}

func TestPluginToolsWorkspaceScopeEligibility(t *testing.T) {
	valid := workspaceReadDefinition("kandev-workspace-reader", "read_workspace_data", "install-1", "policy-1",
		plugintools.SurfaceKanban, plugintools.SurfaceExternal)
	require.NoError(t, validatePluginToolSnapshot(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{valid}}))

	coordinator := valid
	coordinator.Surfaces = []string{plugintools.SurfaceCoordinator, plugintools.SurfaceExternal}
	require.ErrorContains(t, validatePluginToolSnapshot(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{coordinator}}),
		"instance scope and a true read-only hint")

	notReadOnly := valid
	notReadOnly.ReadOnlyHint = false
	require.ErrorContains(t, validatePluginToolSnapshot(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{notReadOnly}}),
		"instance or workspace scope and a true read-only hint")

	taskScoped := valid
	taskScoped.InvocationScope = plugintools.InvocationScopeTask
	require.ErrorContains(t, validatePluginToolSnapshot(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{taskScoped}}),
		"instance or workspace scope and a true read-only hint")

	unknown := valid
	unknown.InvocationScope = "global"
	require.ErrorContains(t, validatePluginToolSnapshot(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{unknown}}),
		"unsupported invocation scope")
}

func TestPluginToolsExternalSurfaceRegistersWorkspaceRead(t *testing.T) {
	s := NewWithProfile(&testBackend{}, "", "", 10005, newTestLogger(t), "", false, mcpprofile.Legacy(ModeExternal, true, nil))
	tool := workspaceReadDefinition("kandev-workspace-reader", "read_workspace_data", "install-1", "policy-1",
		plugintools.SurfaceKanban, plugintools.SurfaceExternal)

	require.NoError(t, s.SetFilteredPluginTools(plugintools.Snapshot{Generation: "external", Revision: 1, Tools: []plugintools.Definition{tool}}))
	require.Contains(t, s.mcpServer.ListTools(), tool.ExposedName)

	instanceTool := instanceReadDefinition("kandev-provider-usage", "get_provider_usage", "install-1", "policy-1",
		plugintools.SurfaceKanban, plugintools.SurfaceExternal)
	require.NoError(t, s.SetFilteredPluginTools(plugintools.Snapshot{Generation: "external", Revision: 2,
		Tools: []plugintools.Definition{instanceTool}}))
	require.Contains(t, s.mcpServer.ListTools(), instanceTool.ExposedName)
}

func TestPluginToolsContextEligibility(t *testing.T) {
	valid := instanceReadDefinition("kandev-provider-usage", "get_provider_usage", "install-1", "policy-1",
		plugintools.SurfaceKanban, plugintools.SurfaceCoordinator, plugintools.SurfaceExternal)
	require.NoError(t, validatePluginToolSnapshot(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{valid}}))

	taskScoped := valid
	taskScoped.InvocationScope = plugintools.InvocationScopeTask
	require.ErrorContains(t, validatePluginToolSnapshot(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{taskScoped}}),
		"instance scope and a true read-only hint")

	notReadOnly := valid
	notReadOnly.ReadOnlyHint = false
	require.ErrorContains(t, validatePluginToolSnapshot(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{notReadOnly}}),
		"instance scope and a true read-only hint")

	unknownScope := valid
	unknownScope.InvocationScope = "global"
	require.ErrorContains(t, validatePluginToolSnapshot(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{unknownScope}}),
		"unsupported invocation scope")

	configuration := instanceReadDefinition("kandev-provider-usage", "get_provider_usage", "install-1", "policy-1", "configuration")
	require.ErrorContains(t, validatePluginToolSnapshot(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{configuration}}),
		"unsupported surface")
}

func TestPluginToolsExternalSurfaceRegistration(t *testing.T) {
	s := NewWithProfile(&testBackend{}, "", "", 10005, newTestLogger(t), "", false, mcpprofile.Legacy(ModeExternal, true, nil))
	tool := instanceReadDefinition("kandev-provider-usage", "get_provider_usage", "install-1", "policy-1",
		plugintools.SurfaceKanban, plugintools.SurfaceExternal)

	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "global", Revision: 1, Tools: []plugintools.Definition{tool}}))
	require.NotContains(t, s.mcpServer.ListTools(), tool.ExposedName, "a global push must not install external tools")

	require.NoError(t, s.SetFilteredPluginTools(plugintools.Snapshot{Generation: "external", Revision: 1, Tools: []plugintools.Definition{tool}}))
	require.Contains(t, s.mcpServer.ListTools(), tool.ExposedName)

	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "global", Revision: 2}))
	require.NotContains(t, s.mcpServer.ListTools(), tool.ExposedName, "global invalidation clears the filtered catalog")
}

func TestPluginToolsCoordinatorRegistrationUsesExactBinding(t *testing.T) {
	profileContext := mcpprofile.NewCoordinator()
	profileContext.CoordinatorToolPolicy = &mcpprofile.CoordinatorToolPolicy{
		Version: mcpprofile.CoordinatorToolPolicyVersionV2, CoordinatorID: "coordinator-1",
		WorkspaceID: "ws-1", ConversationTaskID: "conversation-1", PolicyRevision: 1,
		ToolNames: []string{"list_tasks_kandev"},
		PluginTools: []mcpprofile.CoordinatorPluginTool{{
			PluginID: "kandev-provider-usage", LocalName: "get_provider_usage",
			InstallationID: "install-1", PolicyVersion: "policy-1",
		}},
	}
	s := NewWithProfile(&testBackend{}, "conversation-session", "conversation-1", 10005, newTestLogger(t), "", false, profileContext)
	bound := instanceReadDefinition("kandev-provider-usage", "get_provider_usage", "install-1", "policy-1", plugintools.SurfaceCoordinator)
	stale := instanceReadDefinition("kandev-provider-usage", "stale_usage", "install-2", "policy-1", plugintools.SurfaceCoordinator)
	unbound := instanceReadDefinition("kandev-provider-usage", "unbound_usage", "install-1", "policy-1", plugintools.SurfaceCoordinator)

	require.NoError(t, s.SetFilteredPluginTools(plugintools.Snapshot{Generation: "coordinator", Revision: 1, Tools: []plugintools.Definition{bound, stale, unbound}}))
	require.Contains(t, s.mcpServer.ListTools(), bound.ExposedName)
	require.NotContains(t, s.mcpServer.ListTools(), stale.ExposedName, "a changed installation must not be registered")
	require.NotContains(t, s.mcpServer.ListTools(), unbound.ExposedName, "an unbound tool must not be registered")

	for name, definition := range map[string]plugintools.Definition{
		"changed installation":   instanceReadDefinition("kandev-provider-usage", "get_provider_usage", "install-2", "policy-1", plugintools.SurfaceCoordinator),
		"changed policy version": instanceReadDefinition("kandev-provider-usage", "get_provider_usage", "install-1", "policy-2", plugintools.SurfaceCoordinator),
	} {
		t.Run(name, func(t *testing.T) {
			isolated := NewWithProfile(&testBackend{}, "conversation-session", "conversation-1", 10005, newTestLogger(t), "", false, profileContext)
			require.NoError(t, isolated.SetFilteredPluginTools(plugintools.Snapshot{
				Generation: "coordinator", Revision: 1, Tools: []plugintools.Definition{definition},
			}))
			require.NotContains(t, isolated.mcpServer.ListTools(), definition.ExposedName)
		})
	}
}

func TestPluginToolsCoordinatorLegacyBindingRegistersNothing(t *testing.T) {
	profileContext := mcpprofile.NewCoordinator()
	profileContext.CoordinatorToolPolicy = &mcpprofile.CoordinatorToolPolicy{
		Version: mcpprofile.CoordinatorToolPolicyVersionV1, CoordinatorID: "coordinator-1",
		WorkspaceID: "ws-1", ConversationTaskID: "conversation-1", PolicyRevision: 1,
		ToolNames: []string{"list_tasks_kandev"},
	}
	s := NewWithProfile(&testBackend{}, "conversation-session", "conversation-1", 10005, newTestLogger(t), "", false, profileContext)
	tool := instanceReadDefinition("kandev-provider-usage", "get_provider_usage", "install-1", "policy-1", plugintools.SurfaceCoordinator)
	require.NoError(t, s.SetFilteredPluginTools(plugintools.Snapshot{Generation: "coordinator", Revision: 1, Tools: []plugintools.Definition{tool}}))
	require.NotContains(t, s.mcpServer.ListTools(), tool.ExposedName)
}

func TestPluginToolsCoordinatorGlobalPushInstallsProjection(t *testing.T) {
	profileContext := mcpprofile.NewCoordinator()
	profileContext.CoordinatorToolPolicy = &mcpprofile.CoordinatorToolPolicy{
		Version: mcpprofile.CoordinatorToolPolicyVersionV2, CoordinatorID: "coordinator-1",
		WorkspaceID: "ws-1", ConversationTaskID: "conversation-1", PolicyRevision: 1,
		ToolNames: []string{"list_tasks_kandev"},
		PluginTools: []mcpprofile.CoordinatorPluginTool{{
			PluginID: "kandev-provider-usage", LocalName: "get_provider_usage",
			InstallationID: "install-1", PolicyVersion: "policy-1",
		}},
	}
	s := NewWithProfile(&testBackend{}, "conversation-session", "conversation-1", 10005, newTestLogger(t), "", false, profileContext)
	coordinatorTool := instanceReadDefinition("kandev-provider-usage", "get_provider_usage", "install-1", "policy-1",
		plugintools.SurfaceKanban, plugintools.SurfaceCoordinator, plugintools.SurfaceExternal)
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "global", Revision: 2, Tools: []plugintools.Definition{coordinatorTool}}))
	require.Contains(t, s.mcpServer.ListTools(), coordinatorTool.ExposedName,
		"a coordinator global push must install its coordinator projection")
}
