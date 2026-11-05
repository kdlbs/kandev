package process

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/mcp/profile"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-BUILTIN-MCP-003.2
func TestBuiltinPermissionRequiresRegisteredToolAndExactEndpoint(t *testing.T) {
	ctx := profile.New(profile.SurfaceKanbanTask, nil, nil)
	ctx.BuiltinPackages = &profile.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: ctx.Surface,
		Packages: []profile.PackageID{profile.PackageCore, profile.PackageConfiguration},
		Origins:  map[profile.PackageID]string{profile.PackageCore: "required", profile.PackageConfiguration: "task"}}
	cfg, err := (&config.Config{}).NewInstanceConfig(43210, &config.InstanceOverrides{TaskID: "task", McpProfile: &ctx})
	require.NoError(t, err)
	log := newTestLogger(t)
	server := mcpserver.NewWithProfile(nil, "session", "task", cfg.Port, log, "", true, ctx)
	t.Cleanup(func() { require.NoError(t, server.Close(context.Background())) })
	m := &Manager{cfg: cfg, logger: log}
	m.SetBuiltinToolCatalog(server.BuiltinCatalog)
	check := func(name string, allowed bool) {
		t.Helper()
		_, ok := m.autoApproveInjectedKandevPermission(&adapter.PermissionRequest{ToolName: &name,
			Options: []adapter.PermissionOption{{OptionID: "allow", Kind: streams.PermissionOptionKindAllowOnce}}})
		require.Equal(t, allowed, ok, name)
	}
	check("mcp__kandev-configuration__create_workflow_kandev", true)
	check("mcp__kandev__step_complete_kandev", true)
	check("mcp__kandev__create_workflow_kandev", false)
	check("mcp__kandev-configuration-extra__create_workflow_kandev", false)
	check("mcp__kandev-diagnostics__get_diagnostic_bundle_kandev", false)
	check("mcp__kandev__set_task_title_kandev", false)
	server.SetMode(mcpserver.ModeTaskTitlePending)
	check("mcp__kandev__set_task_title_kandev", true)
	server.SetMode(mcpserver.ModeTask)
	check("mcp__kandev__set_task_title_kandev", false)
	for i := range cfg.McpServers {
		if cfg.McpServers[i].Name == "kandev-configuration" {
			cfg.McpServers[i].URL = "https://untrusted.example/mcp/packages/configuration"
		}
	}
	check("mcp__kandev-configuration__create_workflow_kandev", false)
	check("mcp__kandev__step_complete_kandev", true)
	m.SetBuiltinToolCatalog(nil)
	check("mcp__kandev__step_complete_kandev", false)
}
