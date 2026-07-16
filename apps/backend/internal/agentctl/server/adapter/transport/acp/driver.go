package acp

import (
	"context"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/sessionmodel"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// ACPDriver owns agent-specific ACP request and response translation while the
// Adapter retains transport, session lifecycle, and event delivery.
type ACPDriver interface {
	sessionConfigOptions(
		meta map[string]any,
		options []acp.SessionConfigOption,
		models []modelInfo,
		currentModelID string,
	) []streams.ConfigOption
	modelConfigAfterChange([]streams.ConfigOption, []modelInfo, string) []streams.ConfigOption
	handleModelChange(context.Context, *Adapter, driverSessionConn, driverConfigChange) (bool, error)
	handleConfigOption(context.Context, *Adapter, driverSessionConn, driverConfigChange) (bool, error)
	suppressNotification(acp.SessionNotification) bool
	onModelChanged(string)
	supplementalEvents(*Adapter, string, map[string]any) []AgentEvent
	normalizePromptUsage(*streams.PromptUsage, map[string]any) *streams.PromptUsage
}

type driverSessionConn interface {
	sessionmodel.SDKConn
}

type driverConfigChange struct {
	sessionID string
	configID  string
	value     string
	models    []modelInfo
	config    []streams.ConfigOption
}

func newACPDriver(agentID string) ACPDriver {
	if agentID == grokAgentID {
		return newGrokACPDriver()
	}
	return standardACPDriver{}
}

type standardACPDriver struct{}

func (standardACPDriver) sessionConfigOptions(
	meta map[string]any,
	options []acp.SessionConfigOption,
	_ []modelInfo,
	_ string,
) []streams.ConfigOption {
	typed := convertACPConfigOptions(options)
	if len(typed) > 0 {
		return typed
	}
	return extractConfigOptions(meta)
}

func (standardACPDriver) modelConfigAfterChange(
	config []streams.ConfigOption,
	_ []modelInfo,
	_ string,
) []streams.ConfigOption {
	return config
}

func (standardACPDriver) handleConfigOption(
	context.Context,
	*Adapter,
	driverSessionConn,
	driverConfigChange,
) (bool, error) {
	return false, nil
}

func (standardACPDriver) handleModelChange(
	context.Context,
	*Adapter,
	driverSessionConn,
	driverConfigChange,
) (bool, error) {
	return false, nil
}

func (standardACPDriver) suppressNotification(acp.SessionNotification) bool {
	return false
}

func (standardACPDriver) onModelChanged(string) {}

func (standardACPDriver) supplementalEvents(*Adapter, string, map[string]any) []AgentEvent {
	return nil
}

func (standardACPDriver) normalizePromptUsage(
	usage *streams.PromptUsage,
	_ map[string]any,
) *streams.PromptUsage {
	return usage
}
