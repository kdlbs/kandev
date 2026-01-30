package adapter

import (
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/codex"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/opencode"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/streamjson"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

// NewAdapter creates a new protocol adapter based on the specified protocol type.
// It returns an error if the protocol is not supported.
//
// The protocol determines which transport adapter to use:
//   - ProtocolACP: ACP adapter (JSON-RPC 2.0 over stdin/stdout)
//   - ProtocolClaudeCode: Stream-json adapter (streaming JSON over stdin/stdout)
//   - ProtocolCodex: Codex adapter (JSON-RPC variant over stdin/stdout)
//   - ProtocolOpenCode: OpenCode adapter (REST/SSE over HTTP)
func NewAdapter(protocol agent.Protocol, cfg *Config, log *logger.Logger) (AgentAdapter, error) {
	// Convert to shared config for transport adapters
	sharedCfg := cfg.ToSharedConfig()

	switch protocol {
	case agent.ProtocolACP:
		return newACPAdapterWrapper(acp.NewAdapter(sharedCfg, log)), nil
	case agent.ProtocolClaudeCode:
		return newStreamJSONAdapterWrapper(streamjson.NewAdapter(sharedCfg, log)), nil
	case agent.ProtocolCodex:
		return newCodexAdapterWrapper(codex.NewAdapter(sharedCfg, log)), nil
	case agent.ProtocolOpenCode:
		return newOpenCodeAdapterWrapper(opencode.NewAdapter(sharedCfg, log)), nil
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

// streamJSONAdapterWrapper wraps streamjson.Adapter to implement AgentAdapter
type streamJSONAdapterWrapper struct {
	*streamjson.Adapter
}

func newStreamJSONAdapterWrapper(a *streamjson.Adapter) *streamJSONAdapterWrapper {
	return &streamJSONAdapterWrapper{Adapter: a}
}

func (w *streamJSONAdapterWrapper) GetAgentInfo() *AgentInfo {
	info := w.Adapter.GetAgentInfo()
	if info == nil {
		return nil
	}
	return &AgentInfo{Name: info.Name, Version: info.Version}
}

// SetStderrProvider implements StderrProviderSetter
func (w *streamJSONAdapterWrapper) SetStderrProvider(provider StderrProvider) {
	w.Adapter.SetStderrProvider(provider)
}

// codexAdapterWrapper wraps codex.Adapter to implement AgentAdapter
type codexAdapterWrapper struct {
	*codex.Adapter
}

func newCodexAdapterWrapper(a *codex.Adapter) *codexAdapterWrapper {
	return &codexAdapterWrapper{Adapter: a}
}

func (w *codexAdapterWrapper) GetAgentInfo() *AgentInfo {
	info := w.Adapter.GetAgentInfo()
	if info == nil {
		return nil
	}
	return &AgentInfo{Name: info.Name, Version: info.Version}
}

// SetStderrProvider implements StderrProviderSetter
func (w *codexAdapterWrapper) SetStderrProvider(provider StderrProvider) {
	w.Adapter.SetStderrProvider(provider)
}

// openCodeAdapterWrapper wraps opencode.Adapter to implement AgentAdapter
type openCodeAdapterWrapper struct {
	*opencode.Adapter
}

func newOpenCodeAdapterWrapper(a *opencode.Adapter) *openCodeAdapterWrapper {
	return &openCodeAdapterWrapper{Adapter: a}
}

func (w *openCodeAdapterWrapper) GetAgentInfo() *AgentInfo {
	info := w.Adapter.GetAgentInfo()
	if info == nil {
		return nil
	}
	return &AgentInfo{Name: info.Name, Version: info.Version}
}
