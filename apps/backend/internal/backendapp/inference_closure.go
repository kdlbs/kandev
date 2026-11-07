package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
)

func closureBindingRelated(held sessioncapacity.InferenceBinding, sid, instance string) bool {
	return held.SessionID == "" || held.SessionID == sid || held.InstanceID == instance
}

// CaptureInferenceClosureClaims is persisted before any claim is removed.
func (b *sharedInferenceConsumer) CaptureInferenceClosureClaims(ctx context.Context, sid, instance string) (json.RawMessage, error) {
	b.refreshMu.Lock()
	defer b.refreshMu.Unlock()
	all, err := b.capacity.InferenceCapacityBindings(ctx)
	if err != nil {
		return nil, err
	}
	result := map[string]sessioncapacity.InferenceBinding{}
	for key, held := range all {
		if closureBindingRelated(held, sid, instance) {
			result[key] = held
		}
	}
	return json.Marshal(result)
}

// SettleInferenceClosure replays exact recorded applicability, never inferring
// provider settlement from a missing map entry after a cross-store crash.
func (b *sharedInferenceConsumer) SettleInferenceClosure(ctx context.Context, client *agentruntime.InferenceClient, observation *utility.InferenceClosureResponse, expected json.RawMessage) error {
	if observation == nil || observation.Snapshot.Closure == nil || !observation.Snapshot.Known || !utility.RecordedAttemptsTerminal(&observation.Snapshot) {
		return utility.ErrInferenceClosureIncomplete
	}
	c := observation.Snapshot.Closure
	var claims map[string]sessioncapacity.InferenceBinding
	if json.Unmarshal(expected, &claims) != nil || claims == nil {
		return utility.ErrInferenceClosureIncomplete
	}
	b.refreshMu.Lock()
	defer b.refreshMu.Unlock()
	all, err := b.capacity.InferenceCapacityBindings(ctx)
	if err != nil {
		return err
	}
	if err := validateClosureClaimPopulation(all, claims, c); err != nil {
		return err
	}
	statuses := map[string]utility.AttemptStatus{}
	for _, status := range observation.Snapshot.Attempts {
		statuses[inferenceBindingKey(status.OwnerID, status.AttemptID)] = status
	}
	for key, held := range claims {
		status, ok := statuses[key]
		if !ok || held.SessionID != c.SessionID || held.InstanceID != c.SourceInstanceID || !matchesInferenceProof(held, &status) {
			return utility.ErrInferenceClosureIncomplete
		}
		if err = b.settleClosureClaim(ctx, held, status, all); err != nil {
			return err
		}
	}
	if err = b.verifyInferenceClosureClaimsLocked(ctx, c.SessionID, c.SourceInstanceID); err != nil {
		return err
	}
	for key, status := range statuses {
		if !status.Retained {
			continue
		}
		if _, ok := claims[key]; !ok && (status.State != utility.AttemptNotStarted || status.PromptDispatch != utility.PromptNotSent) {
			return utility.ErrInferenceClosureIncomplete
		}
		released, releaseErr := client.ReleaseInferenceAttempt(ctx, status.AttemptID)
		if releaseErr != nil {
			return releaseErr
		}
		if released == nil || released.Retained || !definitelySettledInference(released) {
			return utility.ErrInferenceClosureIncomplete
		}
	}
	return nil
}

func (b *sharedInferenceConsumer) settleClosureClaim(ctx context.Context, held sessioncapacity.InferenceBinding, status utility.AttemptStatus, all map[string]sessioncapacity.InferenceBinding) error {
	if current, exists := all[held.Key]; exists {
		if current.SettlementKind != held.SettlementKind || current.ProviderKey != held.ProviderKey || current.ProviderKnown != held.ProviderKnown {
			return utility.ErrInferenceClosureIncomplete
		}
		if !b.settleLocked(ctx, held, &status) {
			return fmt.Errorf("closure claim settlement unavailable")
		}
		return nil
	}
	held.TerminalState, held.TerminalPromptDispatch = status.State, status.PromptDispatch
	return b.finishInferenceClaim(ctx, held)
}

func (b *sharedInferenceConsumer) VerifyInferenceClosureClaims(ctx context.Context, sid, instance string) error {
	b.refreshMu.Lock()
	defer b.refreshMu.Unlock()
	return b.verifyInferenceClosureClaimsLocked(ctx, sid, instance)
}

func (b *sharedInferenceConsumer) verifyInferenceClosureClaimsLocked(ctx context.Context, sid, instance string) error {
	all, err := b.capacity.InferenceCapacityBindings(ctx)
	if err != nil {
		return err
	}
	for _, held := range all {
		if closureBindingRelated(held, sid, instance) {
			return fmt.Errorf("closure has unresolved related claims")
		}
	}
	return nil
}

func validateClosureClaimPopulation(all, claims map[string]sessioncapacity.InferenceBinding, c *utility.InferenceScopeClosure) error {
	for key, held := range all {
		if !closureBindingRelated(held, c.SessionID, c.SourceInstanceID) {
			continue
		}
		prior, ok := claims[key]
		if !ok || !sameHeldInference(prior, held) {
			return fmt.Errorf("closure claim population changed")
		}
	}
	return nil
}
