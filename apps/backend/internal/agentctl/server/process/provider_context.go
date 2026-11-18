package process

import (
	"errors"
	"reflect"

	p "github.com/kandev/kandev/internal/mcp/profile"
)

func (m *Manager) CurrentProviderMCPScope() (p.ProviderMCPScope, bool) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if m.cfg.ProviderMCP == nil {
		return p.ProviderMCPScope{}, false
	}
	return m.cfg.ProviderMCP.Scope, true
}

// RefreshBuiltinRuntimeContext retains the immutable scoped package binding.
// Initial helpers still track the current legacy context for source fencing.
func (m *Manager) RefreshBuiltinRuntimeContext(profile p.Context) error {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if m.cfg.McpProfile == nil || m.cfg.McpProfile.BuiltinPackages == nil {
		return nil
	}
	if profile.Surface != m.cfg.McpProfile.Surface || !reflect.DeepEqual(profile.BuiltinPackages, m.cfg.McpProfile.BuiltinPackages) {
		return errors.New("built-in runtime context cannot change package selection")
	}
	if m.cfg.ProviderMCP == nil {
		copy := p.Normalize(profile)
		m.cfg.McpProfile = &copy
	}
	m.configRevision++
	return nil
}
