// host_utility.go implements pluginHost.InvokeUtilityAgent — the agent_invoke
// Host capability. Plugins can use the platform default or provide a profile
// override for one invocation.
package plugins

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const capabilityAgentInvoke = "agent_invoke"

// ErrAgentProfileNotFound identifies a deleted profile during host selection
// or final runner validation.
var ErrAgentProfileNotFound = errors.New("agent profile not found")

// ErrAgentProfileIneligible identifies a profile that cannot run a utility
// completion during host selection or final runner validation.
var ErrAgentProfileIneligible = errors.New("agent profile is not eligible for utility execution")

// utilityDefaultProfileSource reads the platform default selected in user
// settings. It is deliberately narrower than the utility-agent service.
type utilityDefaultProfileSource interface {
	GetDefaultUtilityAgentProfileID(ctx context.Context) (string, error)
}

// AgentProfile is the execution-relevant portion of a profile selection.
// backendapp adapts the agent-settings resolver to this shape.
type AgentProfile struct {
	Enabled          bool
	CLIPassthrough   bool
	WorkspaceID      string
	InferenceCapable bool
}

type agentProfileSource interface {
	GetProfileByID(ctx context.Context, id string) (*AgentProfile, error)
}

// utilityRunner runs a one-shot completion for a profile and returns the
// response text.
type utilityRunner interface {
	ExecuteProfilePrompt(ctx context.Context, profileID, prompt string) (string, error)
}

// InvokeUtilityAgent runs a one-shot, non-interactive completion. An empty
// profile override resolves the current platform default for this call.
func (h *pluginHost) InvokeUtilityAgent(
	ctx context.Context,
	prompt string,
	options ...pluginsdk.UtilityAgentOptions,
) (string, error) {
	if !h.capabilities.AgentInvoke {
		return "", permissionDenied(capabilityAgentInvoke)
	}
	if len(options) > 1 {
		return "", status.Error(codes.InvalidArgument, "InvokeUtilityAgent accepts at most one options value")
	}

	var defaultProfile utilityDefaultProfileSource
	var profiles agentProfileSource
	var runner utilityRunner
	if h.utilityDeps != nil {
		defaultProfile, profiles, runner = h.utilityDeps()
	}

	profileID := ""
	if len(options) == 1 {
		profileID = options[0].ProfileID
	}
	if profileID == "" {
		if defaultProfile == nil || profiles == nil || runner == nil {
			return h.UnimplementedHostData.InvokeUtilityAgent(ctx, prompt, options...)
		}
		var err error
		profileID, err = defaultProfile.GetDefaultUtilityAgentProfileID(ctx)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", status.FromContextError(err).Err()
		}
		if err != nil {
			return "", fmt.Errorf("plugins: load default agent profile: %w", err)
		}
		if profileID == "" {
			return "", status.Error(codes.FailedPrecondition, "no default agent profile configured")
		}
	}

	return h.invokeAgentProfile(ctx, profileID, profiles, runner, prompt)
}

func (h *pluginHost) invokeAgentProfile(
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
		return "", status.Errorf(codes.FailedPrecondition, "agent profile %q not found", profileID)
	}
	if errors.Is(err, ErrAgentProfileIneligible) {
		return "", status.Errorf(codes.FailedPrecondition, "agent profile %q is not eligible for utility execution", profileID)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "", status.FromContextError(err).Err()
	}
	if err != nil {
		return "", fmt.Errorf("plugins: load agent profile %q: %w", profileID, err)
	}
	if profile == nil {
		return "", status.Errorf(codes.FailedPrecondition, "agent profile %q not found", profileID)
	}
	if !profile.Enabled || profile.CLIPassthrough || profile.WorkspaceID != "" || !profile.InferenceCapable {
		return "", status.Errorf(codes.FailedPrecondition, "agent profile %q is not eligible for utility execution", profileID)
	}

	response, err := runner.ExecuteProfilePrompt(ctx, profileID, prompt)
	if errors.Is(err, ErrAgentProfileNotFound) {
		return "", status.Errorf(codes.FailedPrecondition, "agent profile %q not found", profileID)
	}
	if errors.Is(err, ErrAgentProfileIneligible) {
		return "", status.Errorf(codes.FailedPrecondition, "agent profile %q is not eligible for utility execution", profileID)
	}
	return response, err
}
