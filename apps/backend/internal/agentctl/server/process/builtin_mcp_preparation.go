package process

import (
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

// PrepareBuiltinProfile serializes initial package binding against process start.
func (m *Manager) PrepareBuiltinProfile(profile p.Context, apply func(p.Context) error) error {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if m.cfg.AgentCommand != "" || len(m.cfg.AgentArgs) != 0 || m.ProcessGeneration() != 0 || m.passthrough.Live() {
		return errors.New("built-in MCP preparation requires an unused workspace helper")
	}
	profile = p.Normalize(profile)
	if _, err := p.MarshalBuiltinBinding(profile); err != nil {
		return err
	}
	if profile.BuiltinPackages.TaskID != m.cfg.TaskID {
		return errors.New("built-in MCP binding belongs to another task")
	}
	servers, err := config.BuiltinMCPServers(profile.BuiltinPackages, m.cfg.MCPHost, m.cfg.Port)
	if err != nil {
		return err
	}
	for _, existing := range m.cfg.McpServers {
		if existing.Name == "kandev" {
			continue
		}
		if p.ReservedBuiltinRuntimeName(existing.Name) {
			if m.cfg.McpProfile != nil && m.cfg.McpProfile.BuiltinPackages != nil {
				continue
			}
			return fmt.Errorf("reserved built-in MCP name %q must be renamed", existing.Name)
		}
		servers = append(servers, existing)
	}
	if err := apply(profile); err != nil {
		return err
	}
	m.cfg.McpProfile = &profile
	m.cfg.McpServers = servers
	return nil
}
