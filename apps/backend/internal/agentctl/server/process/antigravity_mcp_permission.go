package process

import (
	"slices"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/mcp/profile"
)

func (m *Manager) resolveAntigravityMCPToolAlias(name string) (string, string, bool) {
	nativeMCPNames, inventoryKnown := m.antigravityMCPInventorySnapshot()
	tool, ok := m.antigravityMCPToolCandidate(name)
	if !ok || !inventoryKnown || !m.antigravityMCPRegistrationIsUnambiguous(name, nativeMCPNames) {
		return "", "", false
	}
	return injectedKandevMCPServerName, tool, true
}

func (m *Manager) antigravityMCPToolCandidate(name string) (string, bool) {
	if m.cfg == nil || m.cfg.AgentType != acpcompat.AntigravityAgentID || m.cfg.AuthPolicy == nil ||
		m.cfg.AuthPolicy.RuntimeVariant != acpcompat.AntigravityRuntimeCLI {
		return "", false
	}
	const injectedPrefix = "mcp_kandev_"
	if !strings.HasPrefix(name, injectedPrefix) {
		return "", false
	}
	tool := strings.TrimPrefix(name, injectedPrefix)
	if tool == "" || !slices.Contains(m.liveCoreToolNames(), tool) {
		return "", false
	}
	return tool, true
}

func (m *Manager) antigravityMCPRegistrationIsUnambiguous(name string, nativeMCPNames []string) bool {
	hasInjectedRegistration := false
	for _, server := range m.cfg.McpServers {
		if server.Name == injectedKandevMCPServerName {
			if !injectedKandevMCPServerMatches(m.cfg, server) {
				return false
			}
			hasInjectedRegistration = true
			continue
		}
		if server.Name != "" && strings.HasPrefix(name, "mcp_"+server.Name+"_") {
			return false
		}
	}
	for _, serverName := range nativeMCPNames {
		if serverName != "" && strings.HasPrefix(name, "mcp_"+serverName+"_") {
			return false
		}
	}
	return hasInjectedRegistration
}

func (m *Manager) liveCoreToolNames() []string {
	m.builtinCatalogMu.RLock()
	catalog := m.builtinCatalog
	m.builtinCatalogMu.RUnlock()
	if catalog == nil {
		return nil
	}
	return catalog()[profile.PackageCore]
}
