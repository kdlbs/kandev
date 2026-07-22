// host_utility.go implements pluginHost.InvokeUtilityAgent — the agent_invoke
// Host capability (ADR 0048). Plugins delegate one-shot, non-interactive LLM
// steps to the utility agent selected in their own configuration.
package plugins

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	capabilityAgentInvoke = "agent_invoke"
	// utilityAgentConfigKey is the manifest config_schema field plugins with
	// agent_invoke declare. Its value is the selected utility agent's ID.
	utilityAgentConfigKey = "utility_agent"
)

// ErrUtilityAgentNotFound lets backend adapters identify the one lookup error
// that plugin calls should translate into a configuration failure.
var ErrUtilityAgentNotFound = errors.New("utility agent not found")

// UtilityAgent is the execution-relevant portion of a configured utility
// agent. backendapp adapts internal/utility/service.Service to this shape.
type UtilityAgent struct {
	Name    string
	AgentID string
	Model   string
	Enabled bool
}

type utilityAgentSource interface {
	GetAgentByID(ctx context.Context, id string) (*UtilityAgent, error)
}

// utilityRunner runs a one-shot completion for an agent type + model and
// returns the response text.
type utilityRunner interface {
	ExecutePrompt(ctx context.Context, agentType, model, mode, prompt string) (string, error)
}

// InvokeUtilityAgent runs the named utility agent selected in this plugin's
// configuration. Missing, stale, and disabled selections are FailedPrecondition
// so plugins cannot silently fall back to an unrelated model.
func (h *pluginHost) InvokeUtilityAgent(ctx context.Context, prompt string) (string, error) {
	if !h.capabilities.AgentInvoke {
		return "", permissionDenied(capabilityAgentInvoke)
	}
	var agents utilityAgentSource
	var runner utilityRunner
	if h.utilityDeps != nil {
		agents, runner = h.utilityDeps()
	}
	if agents == nil || runner == nil || h.configs == nil {
		return h.UnimplementedHostData.InvokeUtilityAgent(ctx, prompt)
	}
	config, err := h.configs.GetConfig(h.pluginID)
	if err != nil {
		return "", fmt.Errorf("plugins: read plugin config: %w", err)
	}
	agentID, _ := config[utilityAgentConfigKey].(string)
	if agentID == "" {
		return "", errNoUtilityAgent()
	}
	agent, err := agents.GetAgentByID(ctx, agentID)
	if errors.Is(err, ErrUtilityAgentNotFound) {
		return "", status.Errorf(codes.FailedPrecondition, "configured utility agent %q not found", agentID)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "", status.FromContextError(err).Err()
	}
	if err != nil {
		return "", fmt.Errorf("plugins: load configured utility agent %q: %w", agentID, err)
	}
	if !agent.Enabled {
		return "", status.Errorf(codes.FailedPrecondition, "configured utility agent %q is disabled", agentID)
	}
	return runner.ExecutePrompt(ctx, agent.AgentID, agent.Model, "", prompt)
}

func errNoUtilityAgent() error {
	return status.Error(codes.FailedPrecondition, "no utility agent configured for this plugin")
}
