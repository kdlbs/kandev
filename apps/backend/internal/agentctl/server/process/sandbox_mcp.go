package process

import (
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/mcp/profile"
	"strconv"
)

const sandboxMCPSSETransport = "sse"

// configuredSandboxMCP computes automatic access before configuration changes.
func configuredSandboxMCP(cfg *config.InstanceConfig, launch *sandbox.Launch) ([]config.McpServerConfig, bool, error) {
	if launch == nil {
		return cfg.McpServers, cfg.InjectedKandevMCP, nil
	}
	var manifest *profile.BuiltinManifest
	if cfg.McpProfile != nil {
		manifest = cfg.McpProfile.BuiltinPackages
	}
	servers, err := config.BuiltinMCPServersForLaunch(manifest, cfg.MCPHost, cfg.Port, launch)
	if err != nil {
		return nil, false, err
	}
	injected := len(servers) > 0
	for _, server := range cfg.McpServers {
		if server.Name == injectedKandevMCPServerName || (manifest != nil && profile.ReservedBuiltinRuntimeName(server.Name)) {
			continue
		}
		servers = append(servers, server)
	}
	return servers, injected, nil
}

func canonicalKandevMCPURL(cfg *config.InstanceConfig, transport string) string {
	if cfg.Sandbox != nil {
		if transport == managedMCPHTTPTransport {
			return cfg.Sandbox.MCPURL
		}
		return ""
	}
	if transport == managedMCPHTTPTransport {
		return config.MCPServerURL(cfg.MCPHost, cfg.Port, "/mcp")
	}
	if transport == sandboxMCPSSETransport {
		return config.MCPServerURL(cfg.MCPHost, cfg.Port, "/sse")
	}
	return ""
}

// AdmitSandboxMCP fences the control route to the retained, live launch owner.
func (m *Manager) AdmitSandboxMCP(attemptID, generation, digest string) bool {
	return m.AdmitSandboxMCPPath(attemptID, generation, digest, sandbox.MCPRootPath)
}

// AdmitSandboxMCPPath also bounds the request to the captured package routes.
func (m *Manager) AdmitSandboxMCPPath(attemptID, generation, digest, path string) bool {
	m.mu.RLock()
	launch := m.cfg.Sandbox.Clone()
	m.mu.RUnlock()
	if _, err := launch.MCPURLForPath(path); err != nil {
		return false
	}
	return sandboxMCPMatches(launch, attemptID, generation, digest) && m.sandboxMCPLive(launch)
}

func sandboxMCPMatches(launch *sandbox.Launch, attemptID, generation, digest string) bool {
	return launch != nil && launch.MCPURL != "" && launch.AttemptID == attemptID && strconv.FormatUint(launch.Generation, 10) == generation && launch.PolicyDigest == digest
}

func (m *Manager) sandboxMCPLive(launch *sandbox.Launch) bool {
	owner, err := m.SandboxOwner()
	return err == nil && owner.AttemptID == launch.AttemptID && owner.Generation == launch.Generation && owner.PolicyDigest == launch.PolicyDigest
}
