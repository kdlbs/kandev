package process

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/mcp/profile"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	"github.com/stretchr/testify/require"
)

const policyTestCredential = "own-instance-credential"

func credentialedKandevServers(headers map[string]string) []config.McpServerConfig {
	return []config.McpServerConfig{
		{Name: "kandev", Type: "http", URL: "http://localhost:43210/mcp", Headers: headers},
		{Name: "kandev", Type: "sse", URL: "http://localhost:43210/sse", Headers: headers},
	}
}

func approveKandevTool(m *Manager, name string) bool {
	_, ok := m.autoApproveInjectedKandevPermission(&adapter.PermissionRequest{ToolName: &name,
		Options: []adapter.PermissionOption{{OptionID: "allow", Kind: streams.PermissionOptionKindAllowOnce}}})
	return ok
}

func TestInjectedKandevPermissionPolicyRecognizesExactCredentialHeader(t *testing.T) {
	own := config.BuiltinMCPHeaders(policyTestCredential)
	for _, tc := range []struct {
		name    string
		headers map[string]string
		want    bool
	}{
		{name: "own credential", headers: own, want: true},
		{name: "missing header", headers: nil, want: false},
		{name: "foreign credential", headers: config.BuiltinMCPHeaders("other-instance"), want: false},
		{name: "extra header", headers: map[string]string{"Authorization": own["Authorization"], "X-Extra": "1"}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := injectedKandevPermissionManager(t, credentialedKandevServers(tc.headers))
			m.cfg.BuiltinMCPCredential = policyTestCredential
			require.Equal(t, tc.want, approveKandevTool(m, "mcp__kandev__update_task_plan_kandev"))
		})
	}
}

func TestBuiltinPermissionRecognizesCredentialedPackageEntries(t *testing.T) {
	ctx := profile.New(profile.SurfaceKanbanTask, nil, nil)
	ctx.BuiltinPackages = &profile.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: ctx.Surface,
		Packages: []profile.PackageID{profile.PackageCore, profile.PackageConfiguration},
		Origins:  map[profile.PackageID]string{profile.PackageCore: "required", profile.PackageConfiguration: "task"}}
	cfg, err := (&config.Config{}).NewInstanceConfig(43210, &config.InstanceOverrides{TaskID: "task", McpProfile: &ctx, BuiltinMCPCredential: policyTestCredential})
	require.NoError(t, err)
	log := newTestLogger(t)
	server := mcpserver.NewWithProfile(nil, "session", "task", cfg.Port, log, "", true, ctx)
	t.Cleanup(func() { require.NoError(t, server.Close(context.Background())) })
	m := &Manager{cfg: cfg, logger: log}
	m.SetBuiltinToolCatalog(server.BuiltinCatalog)
	require.True(t, approveKandevTool(m, "mcp__kandev-configuration__create_workflow_kandev"))
	require.True(t, approveKandevTool(m, "mcp__kandev__step_complete_kandev"))

	for i := range cfg.McpServers {
		if cfg.McpServers[i].Name == "kandev-configuration" {
			cfg.McpServers[i].Headers = config.BuiltinMCPHeaders("other-instance")
		}
	}
	require.False(t, approveKandevTool(m, "mcp__kandev-configuration__create_workflow_kandev"), "a foreign header makes the entry third-party")
	require.True(t, approveKandevTool(m, "mcp__kandev__step_complete_kandev"))
}

func TestManagedToolPolicyKeepsCredentialHeader(t *testing.T) {
	policy := profile.ManagedToolPolicy{
		PluginID: "coordinator.v1", InstallationID: "installation-1", WorkspaceID: "workspace-1",
		InstanceKey: "lead", ConversationRevision: 3, ApprovalRevision: 2,
		ManifestDigest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		AgentToolNames: []string{"read_task"},
	}
	managed := profile.New(profile.SurfaceManagedConversation, nil, nil).WithManagedToolPolicy(policy)
	own := config.BuiltinMCPHeaders(policyTestCredential)
	m := &Manager{cfg: &config.InstanceConfig{
		Port: 43210, InjectedKandevMCP: true, McpProfile: &managed, BuiltinMCPCredential: policyTestCredential,
		McpServers: credentialedKandevServers(own),
	}, logger: newTestLogger(t)}

	servers, err := m.MCPServersForSession([]types.McpServer{{Name: "foreign", Type: "http", URL: "https://foreign.example.test/mcp"}})
	require.NoError(t, err)
	require.Len(t, servers, 2)
	for _, server := range servers {
		require.Equal(t, own, server.Headers)
	}
	adapterServers, err := m.adapterMCPServers()
	require.NoError(t, err)
	for _, server := range adapterServers {
		require.Equal(t, own, server.Headers)
	}

	m.cfg.McpServers = credentialedKandevServers(config.BuiltinMCPHeaders("other-instance"))
	_, err = m.MCPServersForSession(nil)
	require.Error(t, err, "a foreign header is not the injected broker")
}
