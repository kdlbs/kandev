package process

import (
	"os"
	"strings"

	"github.com/kandev/kandev/internal/agent/antigravityruntime"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func scanAntigravityNativeMCPNames(cfg *config.InstanceConfig) ([]string, bool) {
	if cfg == nil {
		return nil, false
	}
	home := ""
	for _, entry := range cfg.AgentEnv {
		key, value, found := strings.Cut(entry, "=")
		if found && key == "HOME" {
			home = value
		}
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return antigravityruntime.ScanNativeMCPServerNames(cfg.WorkDir, home)
}

func (m *Manager) refreshAntigravityMCPInventory() {
	if m.cfg == nil || m.cfg.AgentType != acpcompat.AntigravityAgentID || m.cfg.AuthPolicy == nil ||
		m.cfg.AuthPolicy.RuntimeVariant != acpcompat.AntigravityRuntimeCLI {
		return
	}

	names, known := scanAntigravityNativeMCPNames(m.cfg)
	m.antigravityMCPInventoryMu.Lock()
	m.antigravityMCPNames = names
	m.antigravityMCPNamesKnown = known
	m.antigravityMCPInventoryMu.Unlock()
	if !known {
		m.logger.Warn("Antigravity native MCP registrations are unavailable; implicit tool approval is disabled")
	}
}

func (m *Manager) antigravityMCPInventorySnapshot() ([]string, bool) {
	m.antigravityMCPInventoryMu.RLock()
	names := append([]string(nil), m.antigravityMCPNames...)
	known := m.antigravityMCPNamesKnown
	m.antigravityMCPInventoryMu.RUnlock()
	return names, known
}
