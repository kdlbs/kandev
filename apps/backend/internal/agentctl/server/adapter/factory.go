package adapter

import (
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

// NewAdapter creates a new protocol adapter based on the specified protocol type.
// Only ACP is supported; non-ACP variants were removed in the ACP-first migration.
func NewAdapter(protocol agent.Protocol, cfg *Config, log *logger.Logger) (AgentAdapter, error) {
	sharedCfg := cfg.ToSharedConfig()

	switch protocol {
	case agent.ProtocolACP:
		return newACPAdapterWrapper(acp.NewAdapter(sharedCfg, log)), nil
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}
}

// acpAdapterWrapper wraps acp.Adapter to implement AgentAdapter.
type acpAdapterWrapper struct {
	*acp.Adapter
}

func newACPAdapterWrapper(a *acp.Adapter) *acpAdapterWrapper {
	return &acpAdapterWrapper{Adapter: a}
}

func (w *acpAdapterWrapper) GetAgentInfo() *AgentInfo {
	info := w.Adapter.GetAgentInfo()
	if info == nil {
		return nil
	}
	return &AgentInfo{Name: info.Name, Version: info.Version}
}
