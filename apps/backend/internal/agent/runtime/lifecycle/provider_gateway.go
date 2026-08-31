package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/agents"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
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
) (auth *acpprovider.GatewayAuth, keyEnvVar, apiKey string, err error) {
	if profileInfo == nil || profileInfo.ProviderKind != settingsmodels.ProviderKindOpenAICompatible {
		return nil, "", "", nil
	}
	spec := agents.OpenAICompatibleProviderSpecFor(agentConfig)
	if spec == nil {
		return nil, "", "", fmt.Errorf("%w: agent %q does not support an OpenAI-compatible provider",
			ErrProviderMisconfigured, profileInfo.AgentName)
	}
	if verr := acpprovider.ValidateBaseURL(profileInfo.ProviderBaseURL); verr != nil {
		return nil, "", "", fmt.Errorf("%w: %v", ErrProviderMisconfigured, verr)
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
	gw := acpprovider.BuildGatewayAuth(spec.AuthMethodID, spec.ProviderName, profileInfo.ProviderBaseURL, key)
	return &gw, spec.KeyEnvVar, key, nil
}
