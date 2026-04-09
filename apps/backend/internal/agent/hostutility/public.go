package hostutility

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
)

// GetAll returns a snapshot of every probed agent type's capabilities.
func (m *Manager) GetAll() []AgentCapabilities {
	if m == nil || m.cache == nil {
		return nil
	}
	return m.cache.all()
}

// Get returns the cached capabilities for a single agent type.
func (m *Manager) Get(agentType string) (AgentCapabilities, bool) {
	if m == nil || m.cache == nil {
		return AgentCapabilities{}, false
	}
	return m.cache.get(agentType)
}

// Refresh re-probes the given agent type, refreshes the cache, and returns the
// new capabilities. If the warm instance is missing (never bootstrapped or
// crashed), it is lazily recreated.
func (m *Manager) Refresh(ctx context.Context, agentType string) (AgentCapabilities, error) {
	inst, ia, err := m.getInstance(ctx, agentType)
	if err != nil {
		return AgentCapabilities{}, err
	}
	caps := m.probe(ctx, inst, ia)
	m.cache.set(caps)
	return caps, nil
}

// ExecutePrompt runs a sessionless utility prompt against the warm instance
// for the given agent type. The caller picks the model (explicit from the
// utility agent record, user default, or probe cache fallback).
func (m *Manager) ExecutePrompt(
	ctx context.Context,
	agentType, model, mode, prompt string,
) (*PromptResult, error) {
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	inst, ia, err := m.getInstance(ctx, agentType)
	if err != nil {
		return nil, err
	}
	cfg := ia.InferenceConfig()

	resolved := m.resolveModel(agentType, model, ia)
	if resolved == "" {
		return nil, fmt.Errorf("no model specified for agent %q and no default found", agentType)
	}

	req := &agentctlutil.PromptRequest{
		Prompt:  prompt,
		AgentID: agentType,
		Model:   resolved,
		Mode:    mode,
		InferenceConfig: &agentctlutil.InferenceConfigDTO{
			Command:   cfg.Command.Args(),
			ModelFlag: cfg.ModelFlag.Args(),
			WorkDir:   inst.workDir,
		},
	}
	resp, err := inst.client.InferencePrompt(ctx, req)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	return &PromptResult{
		Response:       resp.Response,
		Model:          resp.Model,
		PromptTokens:   resp.PromptTokens,
		ResponseTokens: resp.ResponseTokens,
		DurationMs:     resp.DurationMs,
	}, nil
}

// resolveModel picks the model to use for an ExecutePrompt call.
// Precedence: explicit argument > cached probe currentModelID > agent's default inference model.
func (m *Manager) resolveModel(agentType, explicit string, ia agents.InferenceAgent) string {
	if explicit != "" {
		return explicit
	}
	if caps, ok := m.cache.get(agentType); ok && caps.CurrentModelID != "" {
		return caps.CurrentModelID
	}
	for _, im := range ia.InferenceModels() {
		if im.IsDefault {
			return im.ID
		}
	}
	return ""
}
