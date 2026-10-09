package process

import (
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/mcp/profile"
)

const managedMCPHTTPTransport = "http"

// MCPServersForSession constrains managed conversations to the local Kandev
// broker. Caller-supplied MCP servers are never forwarded for these turns.
func (m *Manager) MCPServersForSession(requested []types.McpServer) ([]types.McpServer, error) {
	if m == nil || m.cfg == nil || m.cfg.McpProfile == nil {
		return append([]types.McpServer(nil), requested...), nil
	}
	if !m.RequiresManagedToolPolicy() {
		return append([]types.McpServer(nil), requested...), nil
	}
	if err := m.validateManagedMCPProfile(); err != nil {
		return nil, err
	}
	return m.canonicalManagedMCPServers(), nil
}

func (m *Manager) validateManagedMCPProfile() error {
	if m.cfg.McpProfile.Surface != profile.SurfaceManagedConversation || m.cfg.McpProfile.ManagedToolPolicy == nil {
		return fmt.Errorf("managed conversation MCP profile is incomplete")
	}
	if err := m.cfg.McpProfile.ManagedToolPolicy.Validate(); err != nil {
		return fmt.Errorf("managed conversation MCP profile is invalid: %w", err)
	}
	if !m.cfg.InjectedKandevMCP || m.cfg.Port <= 0 || !injectedKandevMCPConfigured(m.cfg) {
		return fmt.Errorf("managed conversation requires the injected Kandev MCP broker")
	}
	return nil
}

func (m *Manager) canonicalManagedMCPServers() []types.McpServer {
	servers := make([]types.McpServer, 0, 2)
	for _, server := range m.cfg.McpServers {
		if server.Name != injectedKandevMCPServerName || server.Command != "" || len(server.Args) != 0 || len(server.Env) != 0 || len(server.Headers) != 0 {
			continue
		}
		wantURL := ""
		switch server.Type {
		case managedMCPHTTPTransport:
			wantURL = config.MCPServerURL(m.cfg.MCPHost, m.cfg.Port, "/mcp")
		case "sse":
			wantURL = config.MCPServerURL(m.cfg.MCPHost, m.cfg.Port, "/sse")
		default:
			continue
		}
		if server.URL != wantURL {
			continue
		}
		servers = append(servers, types.McpServer{Name: injectedKandevMCPServerName, Type: server.Type, URL: wantURL})
	}
	return servers
}

func (m *Manager) adapterMCPServers() ([]adapter.McpServerConfig, error) {
	if !m.RequiresManagedToolPolicy() {
		servers := make([]adapter.McpServerConfig, len(m.cfg.McpServers))
		for i, server := range m.cfg.McpServers {
			servers[i] = adapter.McpServerConfig{
				Name: server.Name, URL: server.URL, Type: server.Type, Command: server.Command,
				Args: server.Args, Env: server.Env, Headers: server.Headers,
			}
		}
		return servers, nil
	}
	servers, err := m.MCPServersForSession(nil)
	if err != nil {
		return nil, err
	}
	out := make([]adapter.McpServerConfig, len(servers))
	for i, server := range servers {
		out[i] = adapter.McpServerConfig{Name: server.Name, URL: server.URL, Type: server.Type}
	}
	return out, nil
}

func (m *Manager) adapterAutoApprove() bool {
	return m.cfg.AutoApprovePermissions && !m.RequiresManagedToolPolicy()
}

func (m *Manager) RequiresManagedToolPolicy() bool {
	return m != nil && m.cfg != nil && m.cfg.McpProfile != nil &&
		(m.cfg.McpProfile.Surface == profile.SurfaceManagedConversation || m.cfg.McpProfile.ManagedToolPolicy != nil)
}
