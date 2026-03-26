package lifecycle

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

// ExecuteInferencePrompt executes an inference prompt via an active session's agentctl.
// It looks up the inference config from the agent registry and passes it to agentctl.
func (m *Manager) ExecuteInferencePrompt(ctx context.Context, sessionID, agentID, model, prompt string) (*utility.PromptResponse, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	// Get inference agent from registry
	ia, ok := m.registry.GetInferenceAgent(agentID)
	if !ok {
		return nil, fmt.Errorf("agent %q does not support inference", agentID)
	}

	cfg := ia.InferenceConfig()
	if cfg == nil || !cfg.Supported {
		return nil, fmt.Errorf("agent %q inference not supported", agentID)
	}

	// Get or create execution on-demand (survives backend restart)
	execution, err := m.GetOrEnsureExecution(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("no execution available for session %s: %w", sessionID, err)
	}

	client := execution.GetAgentCtlClient()
	if client == nil {
		return nil, fmt.Errorf("agentctl client not available for session %s", sessionID)
	}

	// Build request with inference config
	req := &utility.PromptRequest{
		Prompt:  prompt,
		AgentID: agentID,
		Model:   model,
		InferenceConfig: &utility.InferenceConfigDTO{
			Command:   cfg.Command.Args(),
			ModelFlag: cfg.ModelFlag.Args(),
			WorkDir:   execution.WorkspacePath,
		},
	}

	return client.InferencePrompt(ctx, req)
}

// ListInferenceAgents returns agents that support inference with their models.
// Only returns agents that are actually installed on the system.
func (m *Manager) ListInferenceAgents() []InferenceAgentInfo {
	return m.ListInferenceAgentsWithContext(context.Background())
}

// ListInferenceAgentsWithContext returns installed inference agents using the provided context.
func (m *Manager) ListInferenceAgentsWithContext(ctx context.Context) []InferenceAgentInfo {
	inferenceAgents := m.registry.ListInferenceAgents()
	result := make([]InferenceAgentInfo, 0, len(inferenceAgents))

	for _, ia := range inferenceAgents {
		// Get base agent for metadata
		ag, ok := ia.(agents.Agent)
		if !ok {
			continue
		}

		// Only include agents that are installed
		installed, err := ag.IsInstalled(ctx)
		if err != nil || installed == nil || !installed.Available {
			continue
		}

		result = append(result, InferenceAgentInfo{
			ID:          ag.ID(),
			Name:        ag.Name(),
			DisplayName: ag.DisplayName(),
			Models:      ia.InferenceModels(),
		})
	}

	return result
}

// InferenceAgentInfo contains info about an inference-capable agent.
type InferenceAgentInfo struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	DisplayName string                  `json:"display_name"`
	Models      []agents.InferenceModel `json:"models"`
}
