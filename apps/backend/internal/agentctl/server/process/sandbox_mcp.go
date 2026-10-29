package process

import (
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/sandbox"
	"strconv"
)

// configureSandboxMCP replaces automatic local access with the captured grant.
// The caller holds the configuration lock and has validated the launch envelope.
func (m *Manager) configureSandboxMCP() {
	if m.cfg.Sandbox == nil {
		return
	}
	servers := make([]config.McpServerConfig, 0, len(m.cfg.McpServers))
	if m.cfg.Sandbox.MCPURL != "" {
		servers = append(servers, config.McpServerConfig{Name: injectedKandevMCPServerName, Type: "http", URL: m.cfg.Sandbox.MCPURL})
	}
	for _, server := range m.cfg.McpServers {
		if server.Name != injectedKandevMCPServerName {
			servers = append(servers, server)
		}
	}
	m.cfg.McpServers = servers
	m.cfg.InjectedKandevMCP = m.cfg.Sandbox.MCPURL != ""
}

func canonicalKandevMCPURL(cfg *config.InstanceConfig, transport string) string {
	if cfg.Sandbox != nil {
		if transport == "http" {
			return cfg.Sandbox.MCPURL
		}
		return ""
	}
	if transport == "http" {
		return config.MCPServerURL(cfg.MCPHost, cfg.Port, "/mcp")
	}
	if transport == "sse" {
		return config.MCPServerURL(cfg.MCPHost, cfg.Port, "/sse")
	}
	return ""
}

// AdmitSandboxMCP fences the control route to the retained, live launch owner.
func (m *Manager) AdmitSandboxMCP(attemptID, generation, digest string) bool {
	m.mu.RLock()
	launch := m.cfg.Sandbox.Clone()
	m.mu.RUnlock()
	return sandboxMCPMatches(launch, attemptID, generation, digest) && m.sandboxMCPLive(launch)
}

func sandboxMCPMatches(launch *sandbox.Launch, attemptID, generation, digest string) bool {
	return launch != nil && launch.MCPURL != "" && launch.AttemptID == attemptID && strconv.FormatUint(launch.Generation, 10) == generation && launch.PolicyDigest == digest
}

func (m *Manager) sandboxMCPLive(launch *sandbox.Launch) bool {
	owner, err := m.SandboxOwner()
	return err == nil && owner.AttemptID == launch.AttemptID && owner.Generation == launch.Generation && owner.PolicyDigest == launch.PolicyDigest
}
