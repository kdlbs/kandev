package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/cliflags"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

// ErrInferenceAgentIDRequired is returned when ExecuteInferencePrompt is called
// without an agent ID. Callers should treat this as a client validation error
// (HTTP 400) rather than a server-side failure.
var ErrInferenceAgentIDRequired = errors.New("agent_id is required")

// ExecuteInferencePrompt executes an inference prompt via an active session's agentctl.
// It looks up the inference config from the agent registry and passes it to agentctl.
func (m *Manager) ExecuteInferencePrompt(ctx context.Context, sessionID, agentID, model, prompt string) (*utility.PromptResponse, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if agentID == "" {
		return nil, ErrInferenceAgentIDRequired
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

	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return nil, fmt.Errorf("agentctl client not available for session %s", sessionID)
	}

	// Build request with inference config. StripEnv is derived from
	// Runtime().StripEnv via agents.StripEnvFor, not declared separately.
	req := &utility.PromptRequest{
		Prompt:  prompt,
		AgentID: agentID,
		Model:   model,
		InferenceConfig: &utility.InferenceConfigDTO{
			Command:   cfg.Command.Args(),
			ModelFlag: cfg.ModelFlag.Args(),
			WorkDir:   execution.WorkspacePath,
			StripEnv:  agents.StripEnvFor(ia),
		},
	}

	return client.InferencePrompt(ctx, req)
}

// ExecuteInferenceProfilePrompt resolves a profile once at dispatch time and
// applies its model, mode, config options, environment, flags, prefix, and
// permission policy to a session-bound utility call.
//
//nolint:cyclop // profile execution validates several independent runtime inputs.
func (m *Manager) ExecuteInferenceProfilePrompt(ctx context.Context, sessionID, profileID, prompt string) (*utility.PromptResponse, error) {
	if m.profileResolver == nil {
		return nil, fmt.Errorf("agent profile resolver is not configured")
	}
	profile, err := m.profileResolver.ResolveProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.AgentID == "" {
		return nil, fmt.Errorf("agent profile %q is not executable", profileID)
	}
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	// The registry is keyed by the agent name (claude-acp, codex-acp, ...),
	// while AgentID is the agents row's generated UUID. AgentName is empty only
	// for resolvers that never populated it, whose AgentID is already a name.
	agentName := profile.AgentName
	if agentName == "" {
		agentName = profile.AgentID
	}
	ia, ok := m.registry.GetInferenceAgent(agentName)
	if !ok {
		return nil, fmt.Errorf("agent %q does not support inference", agentName)
	}
	cfg := ia.InferenceConfig()
	if cfg == nil || !cfg.Supported {
		return nil, fmt.Errorf("agent %q inference not supported", agentName)
	}
	execution, err := m.GetOrEnsureExecution(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("no execution available for session %s: %w", sessionID, err)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return nil, fmt.Errorf("agentctl client not available for session %s", sessionID)
	}
	flags, err := cliflags.Resolve(profile.CLIFlags)
	if err != nil {
		return nil, fmt.Errorf("resolve profile cli flags: %w", err)
	}
	var prefix []string
	if profile.CommandPrefix != "" {
		if err := cliflags.ValidateCommandPrefix(profile.CommandPrefix); err != nil {
			return nil, err
		}
		prefix, err = cliflags.Tokenise(profile.CommandPrefix)
		if err != nil {
			return nil, err
		}
	}
	env := make(map[string]string, len(profile.EnvVars))
	for _, value := range profile.EnvVars {
		if value.Key != "" && value.SecretID == "" {
			env[value.Key] = value.Value
		}
	}
	autoApprove := profile.AutoApprove
	return client.InferencePrompt(ctx, &utility.PromptRequest{
		Prompt: prompt, AgentID: agentName, Model: profile.Model, Mode: profile.Mode,
		AutoApprovePermissions: &autoApprove,
		InferenceConfig: &utility.InferenceConfigDTO{
			Command: cfg.Command.Args(), ModelFlag: cfg.ModelFlag.Args(), WorkDir: execution.WorkspacePath,
			Env: env, StripEnv: agents.StripEnvFor(ia), CLIFlags: flags, CommandPrefix: prefix,
		},
	})
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
		})
	}

	return result
}

// InferenceAgentInfo contains info about an inference-capable agent.
// Models are no longer listed here — consumers should read them from the
// host utility capability cache directly.
type InferenceAgentInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}
