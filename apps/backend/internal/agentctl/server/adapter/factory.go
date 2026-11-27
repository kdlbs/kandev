package adapter

import (
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acpv2"
	codexappserver "github.com/kandev/kandev/internal/agentctl/server/adapter/transport/codexappserver"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

// NewAdapter creates a protocol adapter for the selected agent runtime.
func NewAdapter(protocol agent.Protocol, cfg *Config, log *logger.Logger) (AgentAdapter, error) {
	sharedCfg := cfg.ToSharedConfig()

	switch protocol {
	case agent.ProtocolACP:
		if cfg.ACPProtocolVersion == 2 {
			return &acpV2AdapterWrapper{acpv2.NewAdapter(sharedCfg, log)}, nil
		}
		return newACPAdapterWrapper(acp.NewAdapter(sharedCfg, log)), nil
	case agent.ProtocolCodexAppServer:
		return newCodexAppServerAdapterWrapper(codexappserver.NewAdapter(sharedCfg, log)), nil
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}
}

type acpV2AdapterWrapper struct{ *acpv2.Adapter }

func (w *acpV2AdapterWrapper) GetAgentInfo() *AgentInfo {
	info := w.Adapter.GetAgentInfo()
	if info == nil {
		return nil
	}
	return &AgentInfo{Name: info.Name, Version: info.Version}
}

type codexAppServerAdapterWrapper struct {
	*codexappserver.Adapter
}

var (
	_ AgentAdapter           = (*codexAppServerAdapterWrapper)(nil)
	_ BackgroundWorkProvider = (*codexAppServerAdapterWrapper)(nil)
)

func newCodexAppServerAdapterWrapper(a *codexappserver.Adapter) *codexAppServerAdapterWrapper {
	return &codexAppServerAdapterWrapper{Adapter: a}
}

func (w *codexAppServerAdapterWrapper) GetAgentInfo() *AgentInfo {
	info := w.Adapter.GetAgentInfo()
	if info == nil {
		return nil
	}
	return &AgentInfo{Name: info.Name, Version: info.Version}
}

// acpAdapterWrapper wraps acp.Adapter to implement AgentAdapter.
type acpAdapterWrapper struct {
	*acp.Adapter
}

func (*acpAdapterWrapper) ACPProtocolVersion() uint16 { return 1 }

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
