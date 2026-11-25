package lifecycle

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinMCPInstanceEndpointValidation(t *testing.T) {
	for _, test := range []struct {
		name      string
		scoped    bool
		url       string
		wantError bool
		wantHost  string
		wantPort  int
	}{
		{name: "legacy canonical", url: "http://127.0.0.1:43120/mcp", wantHost: "127.0.0.1", wantPort: 43120},
		{name: "scoped canonical", scoped: true, url: "http://127.0.0.1:43120/i/helper-a/mcp", wantHost: "127.0.0.1", wantPort: 43120},
		{name: "legacy rejects scoped", url: "http://127.0.0.1:43120/i/helper-a/mcp", wantError: true},
		{name: "scoped rejects bare", scoped: true, url: "http://127.0.0.1:43120/mcp", wantError: true},
		{name: "scoped rejects another helper", scoped: true, url: "http://127.0.0.1:43120/i/helper-b/mcp", wantError: true},
		{name: "rejects package URL", scoped: true, url: "http://127.0.0.1:43120/i/helper-a/mcp/packages/core", wantError: true},
		{name: "rejects query", scoped: true, url: "http://127.0.0.1:43120/i/helper-a/mcp?x=1", wantError: true},
		{name: "rejects user info", scoped: true, url: "http://user@127.0.0.1:43120/i/helper-a/mcp", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := agentctl.NewClient("127.0.0.1", 43120, newTestLogger(),
				agentctl.WithExecutionID("helper-a"),
				agentctl.WithBuiltinMCPScopedRoutes(test.scoped),
			)
			host, port, err := builtinPassthroughEndpoint(test.url, client)
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.wantHost, host)
			require.Equal(t, test.wantPort, port)
		})
	}
}

func TestBuiltinMCPInstanceDelivery(t *testing.T) {
	for _, routeMode := range []struct {
		name     string
		scoped   bool
		helperID string
		wantCore string
		wantPkg  string
	}{
		{name: "legacy helper", helperID: "helper-legacy", wantCore: "http://127.0.0.1:43120/mcp", wantPkg: "http://127.0.0.1:43120/mcp/packages/configuration"},
		{name: "scoped helper", scoped: true, helperID: "helper-scoped", wantCore: "http://127.0.0.1:43120/i/helper-scoped/mcp", wantPkg: "http://127.0.0.1:43120/i/helper-scoped/mcp/packages/configuration"},
	} {
		for _, catalog := range []struct {
			name  string
			bound bool
		}{
			{name: "unbound catalog"},
			{name: "bound catalog", bound: true},
		} {
			t.Run(routeMode.name+"/"+catalog.name, func(t *testing.T) {
				execution := &AgentExecution{ID: "task-execution", TaskID: "task-a", WorkspaceID: "workspace", metadata: map[string]interface{}{}}
				if catalog.bound {
					encoded, err := p.MarshalBuiltinBinding(builtinDispatchProfile(p.PackageCore, p.PackageConfiguration))
					require.NoError(t, err)
					execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
				}
				client := agentctl.NewClient("127.0.0.1", 43120, newTestLogger(),
					agentctl.WithExecutionID(routeMode.helperID),
					agentctl.WithBuiltinMCPScopedRoutes(routeMode.scoped),
				)
				requested := []types.McpServer{{Name: "third-party", Type: "http", URL: "https://example.test/mcp", Headers: map[string]string{"X-Test": "preserved"}}}
				servers, err := builtinPassthroughMCPServers(execution, client, routeMode.wantCore, requested)
				require.NoError(t, err)
				wantCount := 2
				if catalog.bound {
					wantCount++
				}
				require.Len(t, servers, wantCount)
				require.Equal(t, "kandev", servers[0].Name)
				require.Equal(t, "http", servers[0].Type)
				require.Equal(t, routeMode.wantCore, servers[0].URL)
				if catalog.bound {
					require.Equal(t, "kandev-configuration", servers[1].Name)
					require.Equal(t, routeMode.wantPkg, servers[1].URL)
				}
				require.Equal(t, requested[0], servers[len(servers)-1])

				artifacts := []struct {
					name     string
					strategy mcpconfig.PassthroughMCPStrategy
					root     string
				}{
					{name: "Claude", strategy: mcpconfig.ClaudeStrategy{}, root: "mcpServers"},
					{name: "OpenCode", strategy: mcpconfig.OpenCodeStrategy{}, root: "mcp"},
					{name: "Cursor", strategy: mcpconfig.CursorStrategy{}, root: "mcpServers"},
					{name: "Pi", strategy: mcpconfig.PiStrategy{}, root: "mcpServers"},
					{name: "Codex", strategy: mcpconfig.CodexStrategy{}},
				}
				for _, artifactCase := range artifacts {
					t.Run(artifactCase.name, func(t *testing.T) {
						paths := mcpconfig.PassthroughPaths{TempConfigPath: "/tmp/kandev-mcp.json", WorkspaceDir: "/workspace"}
						artifact, err := artifactCase.strategy.BuildPassthroughMCP(servers, paths)
						require.NoError(t, err)
						if artifactCase.name == "Codex" {
							requireCodexMCPURL(t, artifact.Args, "kandev", routeMode.wantCore)
							requireCodexMCPURL(t, artifact.Args, "third-party", requested[0].URL)
							if catalog.bound {
								requireCodexMCPURL(t, artifact.Args, "kandev-configuration", routeMode.wantPkg)
							}
							return
						}
						require.Len(t, artifact.Files, 1)
						requireArtifactURL(t, artifact.Files[0].Content, artifactCase.root, "kandev", routeMode.wantCore)
						requireArtifactURL(t, artifact.Files[0].Content, artifactCase.root, "third-party", requested[0].URL)
						if catalog.bound {
							requireArtifactURL(t, artifact.Files[0].Content, artifactCase.root, "kandev-configuration", routeMode.wantPkg)
						}
					})
				}
			})
		}
	}
}

func TestBuiltinMCPInstanceCursorComposition(t *testing.T) {
	for _, routeMode := range []struct {
		name   string
		scoped bool
		helper string
		core   string
		pkg    string
	}{
		{name: "legacy", helper: "cursor-legacy", core: "http://127.0.0.1:43120/mcp", pkg: "http://127.0.0.1:43120/mcp/packages/configuration"},
		{name: "scoped", scoped: true, helper: "cursor-scoped", core: "http://127.0.0.1:43120/i/cursor-scoped/mcp", pkg: "http://127.0.0.1:43120/i/cursor-scoped/mcp/packages/configuration"},
	} {
		for _, bound := range []bool{false, true} {
			catalog := "unbound"
			if bound {
				catalog = "bound"
			}
			t.Run(routeMode.name+"/"+catalog, func(t *testing.T) {
				execution := &AgentExecution{TaskID: "task-a", WorkspaceID: "workspace", standalonePort: 43120, metadata: map[string]interface{}{}}
				if bound {
					encoded, err := p.MarshalBuiltinBinding(builtinDispatchProfile(p.PackageCore, p.PackageConfiguration))
					require.NoError(t, err)
					execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
				}
				client := agentctl.NewClient("127.0.0.1", 43120, newTestLogger(),
					agentctl.WithExecutionID(routeMode.helper),
					agentctl.WithBuiltinMCPScopedRoutes(routeMode.scoped))
				servers, ownership, err := composeEffectiveMCPServers(execution, client, nil, nil, nil, "127.0.0.1")
				require.NoError(t, err)
				core, ok := servers[kandevMCPServerName].(map[string]any)
				require.True(t, ok)
				require.Equal(t, routeMode.core, core["url"])
				if bound {
					pkg, ok := servers["kandev-configuration"].(map[string]any)
					require.True(t, ok)
					require.Equal(t, routeMode.pkg, pkg["url"])
					wantFingerprint, err := canonicalJSONFingerprint(map[string]any{"url": routeMode.pkg})
					require.NoError(t, err)
					require.Equal(t, wantFingerprint, ownership["kandev-configuration"])
				} else {
					require.NotContains(t, servers, "kandev-configuration")
				}
			})
		}
	}
}

func requireCodexMCPURL(t *testing.T, args []string, serverName, wantURL string) {
	t.Helper()
	want := fmt.Sprintf("mcp_servers.%s.url=%q", serverName, wantURL)
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "-c" && args[index+1] == want {
			return
		}
	}
	require.FailNow(t, "Codex argument pair did not contain the exact MCP URL", "wanted -c %q in %#v", want, args)
}

func requireArtifactURL(t *testing.T, content []byte, rootKey, serverName, wantURL string) {
	t.Helper()
	var document map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(content, &document))
	var entries map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(document[rootKey], &entries))
	var server map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(entries[serverName], &server))
	var gotURL string
	require.NoError(t, json.Unmarshal(server["url"], &gotURL))
	require.Equal(t, wantURL, gotURL, "%s.%s URL", rootKey, serverName)
}

func TestBuiltinMCPInstanceDefaultCreate(t *testing.T) {
	req := &ExecutorCreateRequest{InstanceID: "exec-1", WorkspacePath: "/workspace", SessionID: "session-1", TaskID: "task-1"}
	container := ContainerConfig{InstanceID: req.InstanceID}

	builders := map[string]func() bool{
		"standalone fresh": func() bool {
			return buildStandaloneCreateInstanceRequest(req, nil, "", false, false, false, false, nil).ScopedMCPRoutes
		},
		"docker fresh": func() bool {
			return buildContainerCreateInstanceRequest(container, "", false, false, false, false, nil).ScopedMCPRoutes
		},
		"docker and kubernetes recreation": func() bool {
			return buildReconnectCreateInstanceRequest(req, req.InstanceID).ScopedMCPRoutes
		},
		"ssh direct create": func() bool {
			return buildSSHCreateInstanceRequest(req, "/workspace", "/usr/bin/agentctl").ScopedMCPRoutes
		},
		"sprites and plugin remote": func() bool {
			return agentctlInstanceRequest(req, "/workspace").ScopedMCPRoutes
		},
	}
	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			require.True(t, build(), "managed create request must negotiate scoped routes")
		})
	}
}
