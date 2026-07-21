// host_utility.go implements pluginHost.InvokeUtilityAgent — the agent_invoke
// Host capability (ADR 0048). A plugin delegates a one-shot, non-interactive
// LLM step to the operator-configured "utility agent" (Settings > System), so
// it needs no API key of its own. The heavy lifting reuses the sessionless
// host-utility inference tier (ADR 0002) via a narrow utilityRunner interface,
// wired by backendapp so this package never imports the agent runtime.
package plugins

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const capabilityAgentInvoke = "agent_invoke"

// utilitySettingsSource reads the operator-configured utility agent profile id
// (internal/user's UtilityAgentProfileID). Empty means "no utility agent
// configured".
type utilitySettingsSource interface {
	UtilityAgentProfileID(ctx context.Context) (string, error)
}

// utilityRunner runs a one-shot completion for an agent type + model and
// returns the response text. Satisfied by a thin adapter over
// internal/agent/hostutility.Manager (ADR 0002), wired in backendapp so
// internal/plugins does not import the agent runtime.
type utilityRunner interface {
	ExecutePrompt(ctx context.Context, agentType, model, mode, prompt string) (string, error)
}

// InvokeUtilityAgent runs a one-shot, non-interactive completion using the
// operator-configured utility agent, gated by the agent_invoke capability. It
// resolves the configured profile to its agent type + model and delegates to
// the sessionless host-utility runner. A missing capability is
// PermissionDenied; an unconfigured (or since-deleted) utility agent is
// FailedPrecondition — never a silent empty completion.
func (h *pluginHost) InvokeUtilityAgent(ctx context.Context, prompt string) (string, error) {
	if !h.capabilities.AgentInvoke {
		return "", permissionDenied(capabilityAgentInvoke)
	}
	if h.utilitySettings == nil || h.utilityRunner == nil || h.agentProfiles == nil {
		// Not wired (e.g. a bare test pluginHost) — fall back to the embedded
		// Unimplemented default rather than nil-dereferencing.
		return h.UnimplementedHostData.InvokeUtilityAgent(ctx, prompt)
	}

	profileID, err := h.utilitySettings.UtilityAgentProfileID(ctx)
	if err != nil {
		return "", fmt.Errorf("plugins: read utility agent setting: %w", err)
	}
	if profileID == "" {
		return "", errNoUtilityAgent()
	}

	agentType, model, err := h.resolveUtilityAgentProfile(ctx, profileID)
	if err != nil {
		return "", err
	}
	return h.utilityRunner.ExecutePrompt(ctx, agentType, model, "", prompt)
}

// resolveUtilityAgentProfile finds the configured profile id among the
// instance's agent profiles and returns its agent type + model. A profile id
// that no longer resolves is FailedPrecondition (treated like "unconfigured"),
// not an internal error — the operator's selection has simply gone stale.
func (h *pluginHost) resolveUtilityAgentProfile(ctx context.Context, profileID string) (agentType, model string, err error) {
	resp, err := h.agentProfiles.ListAgents(ctx)
	if err != nil {
		return "", "", fmt.Errorf("plugins: list agent profiles: %w", err)
	}
	for _, agent := range resp.Agents {
		for _, profile := range agent.Profiles {
			if profile.ID == profileID {
				return profile.AgentID, profile.Model, nil
			}
		}
	}
	return "", "", status.Errorf(codes.FailedPrecondition, "configured utility agent profile %q not found", profileID)
}

// errNoUtilityAgent is the typed error a plugin gets when the operator has not
// selected a utility agent in Settings > System.
func errNoUtilityAgent() error {
	return status.Error(codes.FailedPrecondition, "no utility agent configured")
}
