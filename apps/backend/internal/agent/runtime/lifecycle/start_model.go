package lifecycle

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/sessionmodel"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// StartModelPolicy carries the profile's model-selection settings for the
// executor-authoritative policy applied at session start and when a fresh ACP
// session is created.
type StartModelPolicy struct {
	// Model is the configured start model (ACP model ID). Empty means "use
	// the agent's default" and does not produce a selection decision.
	Model string
	// FallbackModel is the optional single model to switch to when Model is
	// not advertised. It is used only when the executor advertises it.
	FallbackModel string
	// AutoFallback keeps selection best-effort after an advertised model
	// rejects SetModel. It does not bypass the executor catalog.
	AutoFallback bool
}

// ModelSelectionOutcome describes the executor-authoritative result.
type ModelSelectionOutcome string

const (
	ModelSelectionOutcomeNone             ModelSelectionOutcome = ""
	ModelSelectionOutcomeApplied          ModelSelectionOutcome = "applied"
	ModelSelectionOutcomeExplicitFallback ModelSelectionOutcome = "explicit_fallback"
	ModelSelectionOutcomeProviderDefault  ModelSelectionOutcome = "provider_default"
)

// Reasons are stable values stored in warning metadata and consumed by the
// frontend. Keep them provider-neutral.
const (
	ModelSelectionReasonRequestedNotAdvertised      = "requested_not_advertised"
	ModelSelectionReasonFallbackNotAdvertised       = "fallback_not_advertised"
	ModelSelectionReasonCatalogEmpty                = "catalog_empty"
	ModelSelectionReasonSelectionUnsupported        = "selection_unsupported"
	ModelSelectionReasonSelectionFailedAutoFallback = "selection_failed_auto_fallback"
)

// ModelSelectionDecision is the single model-selection result shared by
// initial launch, context reset, and workspace rebind.
type ModelSelectionDecision struct {
	RequestedModel string                `json:"requested_model,omitempty"`
	FallbackModel  string                `json:"fallback_model,omitempty"`
	EffectiveModel string                `json:"effective_model,omitempty"`
	Outcome        ModelSelectionOutcome `json:"outcome"`
	Reason         string                `json:"reason,omitempty"`
	Warning        bool                  `json:"warning"`
	SetModelCalled bool                  `json:"set_model_called"`
}

// modelApplier abstracts the agentctl surface applyStartModelPolicy needs so
// tests can script SetModel without a live agentctl server.
type modelApplier interface {
	SetModel(ctx context.Context, modelID string) error
}

// advertisedModelIDs returns the session's currently advertised model IDs.
// An empty slice means the executor has no usable model catalog.
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

func providerDefaultDecision(state *CachedModelState, policy StartModelPolicy, reason string) ModelSelectionDecision {
	decision := ModelSelectionDecision{
		RequestedModel: policy.Model,
		FallbackModel:  policy.FallbackModel,
		Outcome:        ModelSelectionOutcomeProviderDefault,
		Reason:         reason,
		Warning:        policy.Model != "",
	}
	if state != nil {
		decision.EffectiveModel = state.CurrentModelID
	}
	return decision
}

// applyStartModelPolicy applies the profile model only when the executor's
// ACP catalog advertises it. If the requested model is absent, it may apply an
// explicitly configured fallback only when that fallback is also advertised.
// All other mismatches continue on the agent's current/default model and
// return a warning decision. Errors from an advertised model remain explicit,
// except for method-not-supported and legacy auto-fallback mode.
func applyStartModelPolicy(
	ctx context.Context,
	log *logger.Logger,
	applier modelApplier,
	state *CachedModelState,
	policy StartModelPolicy,
) (ModelSelectionDecision, error) {
	if policy.Model == "" {
		return ModelSelectionDecision{Outcome: ModelSelectionOutcomeNone}, nil
	}

	decision := ModelSelectionDecision{
		RequestedModel: policy.Model,
		FallbackModel:  policy.FallbackModel,
	}
	advertised := advertisedModelIDs(state)
	if len(advertised) == 0 {
		return providerDefaultDecision(state, policy, ModelSelectionReasonCatalogEmpty), nil
	}

	if !containsModel(advertised, policy.Model) {
		if policy.FallbackModel != "" && containsModel(advertised, policy.FallbackModel) {
			return applyAdvertisedFallback(ctx, log, applier, state, policy, decision)
		}
		reason := ModelSelectionReasonRequestedNotAdvertised
		if policy.FallbackModel != "" {
			reason = ModelSelectionReasonFallbackNotAdvertised
		}
		return providerDefaultDecision(state, policy, reason), nil
	}

	decision.SetModelCalled = true
	if err := applier.SetModel(ctx, policy.Model); err != nil {
		if sessionmodel.IsMethodNotFound(err) {
			log.Debug("agent does not support model selection, continuing on provider default",
				zap.String("model", policy.Model), zap.Error(err))
			decision = providerDefaultDecision(state, policy, ModelSelectionReasonSelectionUnsupported)
			decision.SetModelCalled = true
			return decision, nil
		}
		if policy.AutoFallback {
			log.Warn("failed to set profile model via ACP (auto-fallback)",
				zap.String("model", policy.Model), zap.Error(err))
			decision = providerDefaultDecision(state, policy, ModelSelectionReasonSelectionFailedAutoFallback)
			decision.SetModelCalled = true
			return decision, nil
		}
		return decision, fmt.Errorf("failed to set start model %q: %w", policy.Model, err)
	}

	decision.EffectiveModel = policy.Model
	decision.Outcome = ModelSelectionOutcomeApplied
	return decision, nil
}

func applyAdvertisedFallback(
	ctx context.Context,
	log *logger.Logger,
	applier modelApplier,
	state *CachedModelState,
	policy StartModelPolicy,
	decision ModelSelectionDecision,
) (ModelSelectionDecision, error) {
	decision.SetModelCalled = true
	if err := applier.SetModel(ctx, policy.FallbackModel); err != nil {
		if sessionmodel.IsMethodNotFound(err) {
			decision = providerDefaultDecision(state, policy, ModelSelectionReasonSelectionUnsupported)
			decision.SetModelCalled = true
			return decision, nil
		}
		return decision, fmt.Errorf("failed to set fallback model %q: %w", policy.FallbackModel, err)
	}
	decision.EffectiveModel = policy.FallbackModel
	decision.Outcome = ModelSelectionOutcomeExplicitFallback
	decision.Reason = ModelSelectionReasonRequestedNotAdvertised
	decision.Warning = true
	log.Info("start model unavailable, using advertised fallback model",
		zap.String("start_model", policy.Model),
		zap.String("fallback_model", policy.FallbackModel))
	return decision, nil
}
