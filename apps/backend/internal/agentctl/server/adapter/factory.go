package adapter

import (
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

// NewAdapter creates a new protocol adapter based on the specified protocol type.
// It returns an error if the protocol is not supported.
func NewAdapter(protocol agent.Protocol, cfg *Config, log *logger.Logger) (AgentAdapter, error) {
	// Convert to shared config for transport adapters
	sharedCfg := cfg.ToSharedConfig()

	switch protocol {
	case agent.ProtocolACP:
		return newACPAdapterWrapper(acp.NewAdapter(sharedCfg, log)), nil
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}
}

// Adapter wrappers to convert transport-specific adapters to the common AgentAdapter interface.
// These wrappers handle the type conversion between transport-specific AgentInfo and the
// common adapter.AgentInfo type.

// acpAdapterWrapper wraps acp.Adapter to implement AgentAdapter
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
