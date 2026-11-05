package mcp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-BUILTIN-MCP-002.2
func TestBuiltinCatalogRequiresExplicitMembership(t *testing.T) {
	require.Empty(t, builtinToolPackage("unreviewed_tool_kandev"))
	for _, surface := range []mcpprofile.Surface{mcpprofile.SurfaceKanbanTask, mcpprofile.SurfaceOfficeTask, mcpprofile.SurfaceConfiguration} {
		t.Run(string(surface), func(t *testing.T) {
			log := newTestLogger(t)
			backend := NewChannelBackendClient(log)
			t.Cleanup(backend.Close)
			profile := mcpprofile.New(surface, []mcpprofile.Capability{mcpprofile.CapabilityUserQuestion, mcpprofile.CapabilityParentQuestion, mcpprofile.CapabilityTaskTitle, mcpprofile.CapabilityCanvas}, []string{"github", "gitlab"})
			s := NewWithProfile(backend, "session", "task", 10005, log, "", false, profile)
			t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
			for name := range s.mcpServer.ListTools() {
				require.NotEmpty(t, builtinToolPackage(name), name)
			}
		})
	}
}

func TestBuiltinRebuildRetainsGrantedPluginTools(t *testing.T) {
	s := NewWithProfile(&testBackend{}, "session", "task", 10005, newTestLogger(t), "", false, packageTestProfile(mcpprofile.PackageCore))
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	definition := plugintools.Definition{
		PluginID: "notifications", LocalName: "notify", ExposedName: plugintools.ExposedName("notifications", "notify"),
		Description: "Notify a user", InputSchema: []byte(`{"type":"object"}`),
		Surfaces: []string{plugintools.SurfaceKanban},
	}
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "g", Revision: 1, Tools: []plugintools.Definition{definition}}))
	require.Contains(t, s.BuiltinCatalog()[mcpprofile.PackageCore], definition.ExposedName)
	require.NotContains(t, s.BuiltinCatalog()[mcpprofile.PackageCore], "get_diagnostic_bundle_kandev")
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "g", Revision: 2}))
	require.NotContains(t, s.BuiltinCatalog()[mcpprofile.PackageCore], definition.ExposedName)
}

func TestBuiltinProfileEqualityIncludesBoundManifest(t *testing.T) {
	left := packageTestProfile(mcpprofile.PackageCore)
	right := mcpprofile.Normalize(left)
	require.True(t, sameProfile(left, right))
	right.BuiltinPackages.TaskRevision++
	require.False(t, sameProfile(left, right))
}

func TestBuiltinOfficeOptionalStates(t *testing.T) {
	for _, rich := range []bool{false, true} {
		ctx := packageTestProfile(mcpprofile.PackageCore)
		ctx.Surface = mcpprofile.SurfaceOfficeTask
		ctx.BuiltinPackages.Surface = ctx.Surface
		if rich {
			ctx.BuiltinPackages.Packages = append(ctx.BuiltinPackages.Packages, mcpprofile.PackageRichOutput)
			ctx.BuiltinPackages.Origins[mcpprofile.PackageRichOutput] = "workspace"
		}
		s := NewWithProfile(&testBackend{}, "session", "task", 10005, newTestLogger(t), "", false, ctx)
		t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
		require.Contains(t, s.BuiltinCatalog()[mcpprofile.PackageCore], "step_complete_kandev")
		require.Contains(t, s.BuiltinCatalog()[mcpprofile.PackageCore], "transfer_task_kandev")
		_, hasRich := s.BuiltinCatalog()[mcpprofile.PackageRichOutput]
		require.Equal(t, rich, hasRich)
	}
}
