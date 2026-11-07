package backendapp

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
)

// ObserveInferenceStop retains exact terminal proof in the existing CAS owner before settlement.
// Snapshot continuity never turns an absent or unrelated child into a released binding.
func (b *sharedInferenceConsumer) ObserveInferenceStop(ctx context.Context, snapshot *utility.AttemptSnapshot) error {
	if snapshot == nil || snapshot.InstanceID == "" {
		return nil
	}
	b.refreshMu.Lock()
	defer b.refreshMu.Unlock()
	bindings, err := b.capacity.InferenceCapacityBindings(ctx)
	if err != nil {
		return err
	}
	proofs := matchingStopProofs(snapshot, bindings)
	if len(proofs) == 0 {
		return nil
	}
	if err := b.capacity.UpdateInferenceCapacity(ctx, func(active map[string]sessioncapacity.InferenceBinding) error {
		for key, status := range proofs {
			held, ok := active[key]
			if !ok || held.InstanceID != status.InstanceID || held.OwnerID != status.OwnerID || held.AttemptID != status.AttemptID {
				continue
			}
			held.TerminalState, held.TerminalPromptDispatch = status.State, status.PromptDispatch
			active[key] = held
		}
		return nil
	}); err != nil {
		return err
	}
	var failures []error
	for key, status := range proofs {
		if !b.settleLocked(ctx, bindings[key], &status) {
			failures = append(failures, fmt.Errorf("settle final inference attempt %s", key))
		}
	}
	return errors.Join(failures...)
}

func matchingStopProofs(snapshot *utility.AttemptSnapshot, bindings map[string]sessioncapacity.InferenceBinding) map[string]utility.AttemptStatus {
	proofs := make(map[string]utility.AttemptStatus)
	for _, status := range snapshot.Attempts {
		if status.InstanceID != snapshot.InstanceID || !definitelySettledInference(&status) {
			continue
		}
		key := inferenceBindingKey(status.OwnerID, status.AttemptID)
		if held, ok := bindings[key]; ok && matchesInferenceProof(held, &status) {
			proofs[key] = status
		}
	}
	return proofs
}

func (b *sharedInferenceConsumer) finishRetainedInferenceStop(ctx context.Context, held sessioncapacity.InferenceBinding) bool {
	if held.TerminalState != utility.AttemptTerminated && held.TerminalState != utility.AttemptNotStarted {
		return false
	}
	return b.finishInferenceClaim(ctx, held) == nil
}
