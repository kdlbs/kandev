package mcp

import (
	"context"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-BUILTIN-MCP-002.5
func TestBuiltinConfigurationBuildersCannotReplaceCoreTools(t *testing.T) {
	builders := map[string]func(*Server){
		"workflows":   (*Server).registerConfigWorkflowTools,
		"agents":      (*Server).registerConfigAgentTools,
		"mcp":         (*Server).registerConfigMcpTools,
		"prompts":     (*Server).registerConfigPromptTools,
		"executors":   (*Server).registerConfigExecutorTools,
		"settings":    (*Server).registerConfigSettingsTools,
		"automations": (*Server).registerConfigAutomationTools,
	}
	for name, register := range builders {
		t.Run(name, func(t *testing.T) {
			s := &Server{mcpServer: server.NewMCPServer("configuration", "1.0.0")}
			register(s)
			require.NotEmpty(t, s.mcpServer.ListTools())
			for name := range s.mcpServer.ListTools() {
				require.Equal(t, mcpprofile.PackageConfiguration, mcpprofile.BuiltinToolPackage(name), name)
			}
		})
	}
}

// @covers AC-AGENTS-BUILTIN-MCP-002.5
func TestBuiltinConfigurationPackageIsIdenticalAcrossCallers(t *testing.T) {
	task := NewWithProfile(&testBackend{}, "session", "task", 10005, newTestLogger(t), "", false,
		packageTestProfile(mcpprofile.PackageCore, mcpprofile.PackageConfiguration))
	t.Cleanup(func() { require.NoError(t, task.Close(context.Background())) })
	ctx := packageTestProfile(mcpprofile.PackageCore, mcpprofile.PackageConfiguration)
	ctx.Surface = mcpprofile.SurfaceConfiguration
	ctx.BuiltinPackages.Surface = ctx.Surface
	ctx.BuiltinPackages.Origins[mcpprofile.PackageConfiguration] = "required"
	chat := NewWithProfile(&testBackend{}, "session", "task", 10005, newTestLogger(t), "", false, ctx)
	t.Cleanup(func() { require.NoError(t, chat.Close(context.Background())) })
	taskTools := task.builtinPackages[mcpprofile.PackageConfiguration].registry.ListTools()
	chatTools := chat.builtinPackages[mcpprofile.PackageConfiguration].registry.ListTools()
	require.Len(t, taskTools, len(chatTools))
	for name, tool := range chatTools {
		require.Contains(t, taskTools, name)
		require.Equal(t, tool.Tool, taskTools[name].Tool, name)
	}
	coreOnly := NewWithProfile(&testBackend{}, "session", "task", 10005, newTestLogger(t), "", false,
		packageTestProfile(mcpprofile.PackageCore))
	t.Cleanup(func() { require.NoError(t, coreOnly.Close(context.Background())) })
	require.Len(t, task.mcpServer.ListTools(), len(coreOnly.mcpServer.ListTools()))
	for name, tool := range coreOnly.mcpServer.ListTools() {
		require.Equal(t, tool.Tool, task.mcpServer.ListTools()[name].Tool, name)
	}
}

// @covers AC-AGENTS-BUILTIN-MCP-002.4
func TestBuiltinAssemblyOmitsUnselectedPackages(t *testing.T) {
	for _, configuration := range []bool{false, true} {
		ids := []mcpprofile.PackageID{mcpprofile.PackageCore}
		if configuration {
			ids = append(ids, mcpprofile.PackageConfiguration)
		}
		s := NewWithProfile(&testBackend{}, "session", "task", 10005, newTestLogger(t), "", false, packageTestProfile(ids...))
		t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
		foundConfiguration := false
		for _, tool := range s.assembleTools() {
			owner := mcpprofile.BuiltinToolPackage(tool.Tool.Name)
			require.Contains(t, ids, owner, tool.Tool.Name)
			foundConfiguration = foundConfiguration || owner == mcpprofile.PackageConfiguration
		}
		require.Equal(t, configuration, foundConfiguration)
	}
}
