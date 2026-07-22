package acp

import (
	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// acpDialect contains pure compatibility decisions for an agent's observed
// ACP implementation. The Adapter remains responsible for RPC execution,
// session state, serialization, and event delivery.
type acpDialect struct {
	normalizeSessionConfig func(
		[]streams.ConfigOption,
		[]modelInfo,
		string,
	) []streams.ConfigOption
	configAfterModelChange func(
		[]streams.ConfigOption,
		[]modelInfo,
		string,
	) []streams.ConfigOption
	modelChangeRequest   func(dialectConfigChange) (*dialectRPC, error)
	configChangeRequest  func(dialectConfigChange) (*dialectRPC, error)
	suppressNotification func(acp.SessionNotification) bool
	contextWindow        func(map[string]any, []modelInfo, []streams.ConfigOption) (contextWindowSample, bool)
	normalizePromptUsage func(*streams.PromptUsage, map[string]any) *streams.PromptUsage
	subagentFrame        func(map[string]any, string, any) (subagentFrame, bool)
}

type dialectConfigChange struct {
	sessionID string
	configID  string
	value     string
	models    []modelInfo
	config    []streams.ConfigOption
}

type dialectRPC struct {
	request     acp.UnstableSetSessionModelRequest
	formatError func(error) error
}

type contextWindowSample struct {
	used int64
	size int64
}

func newACPDialect(agentID string) acpDialect {
	switch agentID {
	case grokAgentID:
		return newGrokACPDialect()
	case codexAgentID:
		return newCodexACPDialect()
	}
	return acpDialect{}
}

func (d acpDialect) parseSubagentFrame(meta map[string]any, title string, rawInput any) (subagentFrame, bool) {
	if d.subagentFrame == nil {
		return subagentFrame{}, false
	}
	return d.subagentFrame(meta, title, rawInput)
}

func (d acpDialect) sessionConfigOptions(
	meta map[string]any,
	options []acp.SessionConfigOption,
	models []modelInfo,
	currentModelID string,
) []streams.ConfigOption {
	base := convertACPConfigOptions(options)
	if len(base) == 0 {
		base = extractConfigOptions(meta)
	}
	if d.normalizeSessionConfig == nil {
		return base
	}
	return d.normalizeSessionConfig(base, models, currentModelID)
}

func (d acpDialect) modelConfigAfterChange(
	config []streams.ConfigOption,
	models []modelInfo,
	modelID string,
) []streams.ConfigOption {
	if d.configAfterModelChange == nil {
		return config
	}
	return d.configAfterModelChange(config, models, modelID)
}

func (d acpDialect) modelRequest(change dialectConfigChange) (*dialectRPC, error) {
	if d.modelChangeRequest == nil {
		return nil, nil
	}
	return d.modelChangeRequest(change)
}

func (d acpDialect) configRequest(change dialectConfigChange) (*dialectRPC, error) {
	if d.configChangeRequest == nil {
		return nil, nil
	}
	return d.configChangeRequest(change)
}

func (d acpDialect) suppresses(notification acp.SessionNotification) bool {
	return d.suppressNotification != nil && d.suppressNotification(notification)
}

func (d acpDialect) contextSample(
	meta map[string]any,
	models []modelInfo,
	config []streams.ConfigOption,
) (contextWindowSample, bool) {
	if d.contextWindow == nil {
		return contextWindowSample{}, false
	}
	return d.contextWindow(meta, models, config)
}

func (d acpDialect) promptUsage(
	usage *streams.PromptUsage,
	meta map[string]any,
) *streams.PromptUsage {
	if d.normalizePromptUsage == nil {
		return usage
	}
	return d.normalizePromptUsage(usage, meta)
}
