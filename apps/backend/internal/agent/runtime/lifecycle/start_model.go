package lifecycle

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/sessionmodel"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// StartModelPolicy carries the profile's model-selection settings for the
// no-silent-model-fallback policy applied at session start (and when a
// context reset re-applies the effective model to a fresh ACP session).
type StartModelPolicy struct {
	// Model is the configured start model (ACP model ID). Empty means "use
	// the agent's default" — nothing is applied.
	Model string
	// FallbackModel is the optional single model to switch to when Model is
	// unavailable. Ignored when AutoFallback is enabled.
	FallbackModel string
	// AutoFallback restores the legacy best-effort behavior: SetModel
	// failures log and the session continues on the provider default.
	AutoFallback bool
}

// modelApplier abstracts the agentctl surface applyStartModelPolicy needs so
// tests can script SetModel without a live agentctl server.
type modelApplier interface {
	SetModel(ctx context.Context, modelID string) error
}

// advertisedModelIDs returns the session's currently advertised model IDs.
// An empty slice means the agent has not advertised a model list (unknown).
func advertisedModelIDs(state *CachedModelState) []string {
	if state == nil {
		return nil
	}
	ids := make([]string, 0, len(state.Models))
	for _, m := range state.Models {
		ids = append(ids, m.ModelID)
	}
	return ids
}

func containsModel(ids []string, id string) bool {
	for _, existing := range ids {
		if existing == id {
			return true
		}
	}
	return false
}

// applyStartModelPolicy enforces the no-silent-model-fallback rules when
// applying the profile's start model to a session:
//
//   - AutoFallback: legacy best-effort (attempt SetModel; on failure warn
//     and continue on the provider default).
//   - Start model gone (absent from the advertised list, when known):
//     apply FallbackModel when set (returning usingFallback=true), else
//     fail with the actionable model-unavailable message.
//   - Start model present / list unknown: apply it; a method-not-found
//     error means the agent has no model selection — continue silently;
//     any other error fails explicitly.
//
// Returns the model actually applied ("" when none was applied / the agent
// default remains) and whether the fallback model was used.
func applyStartModelPolicy(
	ctx context.Context,
	log *logger.Logger,
	applier modelApplier,
	state *CachedModelState,
	policy StartModelPolicy,
) (applied string, usingFallback bool, err error) {
	if policy.Model == "" {
		return "", false, nil
	}
	if policy.AutoFallback {
		if err := applier.SetModel(ctx, policy.Model); err != nil {
			log.Warn("failed to set profile model via ACP (auto-fallback)",
				zap.String("model", policy.Model), zap.Error(err))
			return "", false, nil
		}
		return policy.Model, false, nil
	}

	advertised := advertisedModelIDs(state)
	if len(advertised) > 0 && !containsModel(advertised, policy.Model) {
		if policy.FallbackModel != "" {
			if err := applier.SetModel(ctx, policy.FallbackModel); err != nil {
				return "", false, fmt.Errorf(
					"start model %q is unavailable and fallback model %q failed to apply: %w",
					policy.Model, policy.FallbackModel, err)
			}
			log.Info("start model unavailable, using fallback model",
				zap.String("start_model", policy.Model),
				zap.String("fallback_model", policy.FallbackModel))
			return policy.FallbackModel, true, nil
		}
		return "", false, fmt.Errorf("%s", routingerr.ModelUnavailableMessage(policy.Model))
	}

	if err := applier.SetModel(ctx, policy.Model); err != nil {
		if sessionmodel.IsMethodNotFound(err) {
			// The agent declares no model-selection support — nothing to
			// enforce; the session continues on the agent default.
			log.Debug("agent does not support model selection, continuing",
				zap.String("model", policy.Model), zap.Error(err))
			return "", false, nil
		}
		// SetModel failed even though the model appeared available (the
		// advertised list may not have arrived yet at session start, or the
		// agent rejected the model itself). The explicit fallback exists
		// exactly for this: switch to it instead of failing — but never
		// fall through to the provider default.
		if policy.FallbackModel != "" {
			if ferr := applier.SetModel(ctx, policy.FallbackModel); ferr != nil {
				return "", false, fmt.Errorf(
					"start model %q failed and fallback model %q failed to apply: %w",
					policy.Model, policy.FallbackModel, ferr)
			}
			log.Info("start model unavailable, using fallback model",
				zap.String("start_model", policy.Model),
				zap.String("fallback_model", policy.FallbackModel))
			return policy.FallbackModel, true, nil
		}
		return "", false, fmt.Errorf("failed to set start model %q: %w", policy.Model, err)
	}
	return policy.Model, false, nil
}
