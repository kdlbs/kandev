package process

import (
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/mcp/profile"
	"slices"
)

// SetBuiltinToolCatalog binds permission checks to the live server registry.
// A missing registry fails closed for a package-mode execution.
func (m *Manager) SetBuiltinToolCatalog(catalog func() map[profile.PackageID][]string) {
	m.builtinCatalogMu.Lock()
	defer m.builtinCatalogMu.Unlock()
	m.builtinCatalog = catalog
}

func (m *Manager) builtinPermissionAllowed(name, tool string) bool {
	id := profile.BuiltinPackageForRuntimeName(name)
	if id == "" || m.cfg.McpProfile.BuiltinPackages.Validate() != nil {
		return false
	}
	m.builtinCatalogMu.RLock()
	catalog := m.builtinCatalog
	m.builtinCatalogMu.RUnlock()
	if catalog == nil || !slices.Contains(catalog()[id], tool) {
		return false
	}
	expected, err := config.BuiltinMCPServers(m.cfg.McpProfile.BuiltinPackages, m.cfg.MCPHost, m.cfg.Port)
	if err != nil {
		return false
	}
	for _, candidate := range m.cfg.McpServers {
		if candidate.Name != name || !builtinEndpointOnly(candidate) {
			continue
		}
		for _, injected := range expected {
			if candidate.Name == injected.Name && candidate.Type == injected.Type && candidate.URL == injected.URL {
				return true
			}
		}
	}
	return false
}

func builtinEndpointOnly(server config.McpServerConfig) bool {
	return server.Command == "" && len(server.Args) == 0 && len(server.Env) == 0 && len(server.Headers) == 0
}
