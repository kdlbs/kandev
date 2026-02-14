package agents

import (
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/codex"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/opencode"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/streamjson"
)

// Adapter wrappers convert transport-specific adapters to the common AgentAdapter interface.
// These handle the type conversion between transport-specific AgentInfo and adapter.AgentInfo.

// --- ACP ---

type acpAdapterWrapper struct {
	*acp.Adapter
}

func newACPAdapterWrapper(a *acp.Adapter) *acpAdapterWrapper {
	return &acpAdapterWrapper{Adapter: a}
}

func (w *acpAdapterWrapper) GetAgentInfo() *adapter.AgentInfo {
	info := w.Adapter.GetAgentInfo()
	if info == nil {
		return nil
	}
	return &adapter.AgentInfo{Name: info.Name, Version: info.Version}
}

// --- Stream JSON (Claude Code, Amp) ---

type streamJSONAdapterWrapper struct {
	*streamjson.Adapter
}

func newStreamJSONAdapterWrapper(a *streamjson.Adapter) *streamJSONAdapterWrapper {
	return &streamJSONAdapterWrapper{Adapter: a}
}

func (w *streamJSONAdapterWrapper) GetAgentInfo() *adapter.AgentInfo {
	info := w.Adapter.GetAgentInfo()
	if info == nil {
		return nil
	}
	return &adapter.AgentInfo{Name: info.Name, Version: info.Version}
}

func (w *streamJSONAdapterWrapper) SetStderrProvider(provider adapter.StderrProvider) {
	w.Adapter.SetStderrProvider(provider)
}

// --- Codex ---

type codexAdapterWrapper struct {
	*codex.Adapter
}

func newCodexAdapterWrapper(a *codex.Adapter) *codexAdapterWrapper {
	return &codexAdapterWrapper{Adapter: a}
}

func (w *codexAdapterWrapper) GetAgentInfo() *adapter.AgentInfo {
	info := w.Adapter.GetAgentInfo()
	if info == nil {
		return nil
	}
	return &adapter.AgentInfo{Name: info.Name, Version: info.Version}
}

func (w *codexAdapterWrapper) SetStderrProvider(provider adapter.StderrProvider) {
	w.Adapter.SetStderrProvider(provider)
}

// --- OpenCode ---

type openCodeAdapterWrapper struct {
	*opencode.Adapter
}

func newOpenCodeAdapterWrapper(a *opencode.Adapter) *openCodeAdapterWrapper {
	return &openCodeAdapterWrapper{Adapter: a}
}

func (w *openCodeAdapterWrapper) GetAgentInfo() *adapter.AgentInfo {
	info := w.Adapter.GetAgentInfo()
	if info == nil {
		return nil
	}
	return &adapter.AgentInfo{Name: info.Name, Version: info.Version}
}
