package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/agents"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/acpprovider"
)

// ErrProviderMisconfigured marks an OpenAI-compatible provider profile that
// cannot be launched: the agent does not support a gateway provider, the base
// URL is invalid, or the API-key secret could not be revealed. The launch is
// aborted rather than letting the agent fall back to its vendor endpoint.
var ErrProviderMisconfigured = errors.New("PROVIDER_MISCONFIGURED")

// resolveProviderGatewayAuth builds the ACP gateway authenticate params for a
// profile whose ProviderKind is openai_compatible. It returns (nil, nil) for a
// native profile or a nil profileInfo. The revealed API key, when present, is
// also returned so the caller can mirror it into the agent environment for the
// utility/inference path and the agent's own credential fallback.
func (m *Manager) resolveProviderGatewayAuth(
	ctx context.Context,
	profileInfo *AgentProfileInfo,
	agentConfig agents.Agent,
	runtime agentruntime.Runtime,
) (auth *acpprovider.GatewayAuth, keyEnvVar, apiKey string, err error) {
	if profileInfo == nil || profileInfo.ProviderKind != settingsmodels.ProviderKindOpenAICompatible {
		return nil, "", "", nil
	}
	spec := agents.OpenAICompatibleProviderSpecFor(agentConfig)
	if spec == nil {
		return nil, "", "", fmt.Errorf("%w: agent %q does not support an OpenAI-compatible provider",
			ErrProviderMisconfigured, profileInfo.AgentName)
	}
	validateBaseURL := acpprovider.ValidateBaseURL
	if profileInfo.ProviderAPIKeySecretID != "" {
		// The revealed key rides in an Authorization header to this URL; refuse a
		// cleartext non-loopback destination rather than leak it on the wire.
		validateBaseURL = acpprovider.ValidateCredentialedBaseURL
	}
	if verr := validateBaseURL(profileInfo.ProviderBaseURL); verr != nil {
		return nil, "", "", fmt.Errorf("%w: %v", ErrProviderMisconfigured, verr)
	}
	baseURL, verr := providerBaseURLForRuntime(profileInfo.ProviderBaseURL, runtime)
	if verr != nil {
		return nil, "", "", verr
	}
	key := ""
	if profileInfo.ProviderAPIKeySecretID != "" {
		revealed, revErr := m.revealGlobalSecret(ctx, profileInfo.ProviderAPIKeySecretID)
		if revErr != nil {
			if errors.Is(revErr, context.Canceled) || errors.Is(revErr, context.DeadlineExceeded) {
				return nil, "", "", revErr
			}
			return nil, "", "", fmt.Errorf("%w: could not reveal the provider API key", ErrProviderMisconfigured)
		}
		key = revealed
	}
	gw := acpprovider.BuildGatewayAuth(spec.AuthMethodID, spec.ProviderName, baseURL, key)
	return &gw, spec.KeyEnvVar, key, nil
}

// providerBaseURLForRuntime adapts a loopback provider base URL to the agent's
// execution environment. An agent in a local Docker container reaches a service
// on the developer's host through host.docker.internal, so a loopback URL is
// rewritten. Other containerized runtimes (remote Docker, Sprites) have no route
// to the developer's loopback at all, so a loopback URL is rejected with
// actionable guidance instead of failing opaquely at connect time. Host runtimes
// keep the URL as-is.
func providerBaseURLForRuntime(rawURL string, runtime agentruntime.Runtime) (string, error) {
	if !acpprovider.IsLoopbackBaseURL(rawURL) {
		return rawURL, nil
	}
	switch runtime {
	case agentruntime.RuntimeDocker:
		rewritten, _ := acpprovider.RewriteLoopbackHostForDocker(rawURL)
		return rewritten, nil
	case agentruntime.RuntimeRemoteDocker, agentruntime.RuntimeSprites:
		return "", fmt.Errorf(
			"%w: provider base URL %q is on localhost, which an agent running in a %s environment cannot reach; use a hostname or IP reachable from that environment",
			ErrProviderMisconfigured, rawURL, runtime)
	default:
		return rawURL, nil
	}
}
