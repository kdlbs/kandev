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
	// agentProfileConfigKey is the manifest config_schema field that selects a
	// profile directly for plugins that do not need a utility-agent record.
	agentProfileConfigKey = "agent_profile"
)

// ErrUtilityAgentNotFound lets backend adapters identify the one lookup error
// that plugin calls should translate into a configuration failure.
var ErrUtilityAgentNotFound = errors.New("utility agent not found")

// ErrAgentProfileNotFound lets backend adapters identify a deleted direct
// profile and translate it into the configured-plugin failure contract.
var ErrAgentProfileNotFound = errors.New("agent profile not found")

// UtilityAgent is the execution-relevant portion of a configured utility
// agent. backendapp adapts internal/utility/service.Service to this shape.
type UtilityAgent struct {
	Name                string
	AgentID             string
	Model               string
	AgentProfileID      string
	ProfileBindingState string
	Enabled             bool
}

type utilityAgentSource interface {
	GetAgentByID(ctx context.Context, id string) (*UtilityAgent, error)
}

// AgentProfile is the execution-relevant portion of a direct profile
// selection. backendapp adapts the agent-settings repository to this shape.
type AgentProfile struct {
	Enabled        bool
	CLIPassthrough bool
	WorkspaceID    string
}

type agentProfileSource interface {
	GetProfileByID(ctx context.Context, id string) (*AgentProfile, error)
}

// utilityRunner runs a one-shot completion for an agent type + model and
// returns the response text.
type utilityRunner interface {
	ExecuteProfilePrompt(ctx context.Context, profileID, prompt string) (string, error)
}

// InvokeUtilityAgent runs the named utility agent selected in this plugin's
// configuration. Missing, stale, and disabled selections are FailedPrecondition
// so plugins cannot silently fall back to an unrelated model.
func (h *pluginHost) InvokeUtilityAgent(ctx context.Context, prompt string) (string, error) {
	if !h.capabilities.AgentInvoke {
		return "", permissionDenied(capabilityAgentInvoke)
	}
	var agents utilityAgentSource
	var profiles agentProfileSource
	var runner utilityRunner
	if h.utilityDeps != nil {
		agents, profiles, runner = h.utilityDeps()
	}
	if h.configs == nil {
		return h.UnimplementedHostData.InvokeUtilityAgent(ctx, prompt)
	}
	config, err := h.configs.GetConfig(h.pluginID)
	if err != nil {
		return "", fmt.Errorf("plugins: read plugin config: %w", err)
	}
	profileID, _ := config[agentProfileConfigKey].(string)
	if profileID != "" {
		return h.invokeConfiguredAgentProfile(ctx, profileID, profiles, runner, prompt)
	}
	agentID, _ := config[utilityAgentConfigKey].(string)
	if agentID != "" {
		return h.invokeConfiguredUtilityAgent(ctx, agentID, agents, runner, prompt)
	}
	if hasAgentProfileConfig(h.configSchema) {
		return "", errNoAgentProfile()
	}
	return "", errNoUtilityAgent()
}

func (h *pluginHost) invokeConfiguredUtilityAgent(
	ctx context.Context,
	agentID string,
	agents utilityAgentSource,
	runner utilityRunner,
	prompt string,
) (string, error) {
	if agents == nil || runner == nil {
		return h.UnimplementedHostData.InvokeUtilityAgent(ctx, prompt)
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
	if agent.ProfileBindingState == "unconfigured" || agent.AgentProfileID == "" {
		return "", status.Errorf(codes.FailedPrecondition, "configured utility agent %q has no usable agent profile", agentID)
	}
	return runner.ExecuteProfilePrompt(ctx, agent.AgentProfileID, prompt)
}

func (h *pluginHost) invokeConfiguredAgentProfile(
	ctx context.Context,
	profileID string,
	profiles agentProfileSource,
	runner utilityRunner,
	prompt string,
) (string, error) {
	if profiles == nil || runner == nil {
		return h.UnimplementedHostData.InvokeUtilityAgent(ctx, prompt)
	}
	profile, err := profiles.GetProfileByID(ctx, profileID)
	if errors.Is(err, ErrAgentProfileNotFound) {
		return "", status.Errorf(codes.FailedPrecondition, "configured agent profile %q not found", profileID)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "", status.FromContextError(err).Err()
	}
	if err != nil {
		return "", fmt.Errorf("plugins: load configured agent profile %q: %w", profileID, err)
	}
	// Current adapters translate a deleted profile into ErrAgentProfileNotFound,
	// but keep this guard so future implementations cannot silently treat a
	// nil success result as an eligible profile.
	if profile == nil {
		return "", status.Errorf(codes.FailedPrecondition, "configured agent profile %q not found", profileID)
	}
	if !profile.Enabled || profile.CLIPassthrough || profile.WorkspaceID != "" {
		return "", status.Errorf(codes.FailedPrecondition, "configured agent profile %q is not eligible for utility execution", profileID)
	}
	return runner.ExecuteProfilePrompt(ctx, profileID, prompt)
}

func hasAgentProfileConfig(schema map[string]any) bool {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	property, ok := properties[agentProfileConfigKey].(map[string]any)
	return ok && property["format"] == "agent-profile"
}

func errNoUtilityAgent() error {
	return status.Error(codes.FailedPrecondition, "no utility agent configured for this plugin")
}

func errNoAgentProfile() error {
	return status.Error(codes.FailedPrecondition, "no agent profile configured for this plugin")
}
