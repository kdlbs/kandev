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
	_, err := b.ObserveInferenceStopDetailed(ctx, snapshot)
	return err
}

// ObserveInferenceStopDetailed reports the existing settlement's actual scope.
func (b *sharedInferenceConsumer) ObserveInferenceStopDetailed(ctx context.Context, snapshot *utility.AttemptSnapshot) (*utility.InferenceStopSettlementReport, error) {
	if snapshot == nil || snapshot.InstanceID == "" {
		return nil, nil
	}
	report := &utility.InferenceStopSettlementReport{Scope: "managed_bindings_for_instance", Status: "unavailable"}
	b.refreshMu.Lock()
	defer b.refreshMu.Unlock()
	bindings, err := b.capacity.InferenceCapacityBindings(ctx)
	if err != nil {
		return report, err
	}
	proofs := matchingStopProofs(snapshot, bindings)
	countStopSettlementScope(report, snapshot, bindings, len(proofs))
	if len(proofs) == 0 {
		report.Status = "observed"
		return report, nil
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
		return report, err
	}
	var failures []error
	for key, status := range proofs {
		settled, evidenced := b.settleStopProof(ctx, bindings[key], &status)
		if settled {
			report.Settled++
			report.Unresolved--
			continue
		}
		if evidenced {
			failures = append(failures, fmt.Errorf("settle final inference attempt %s", key))
		}
	}
	report.Status = "observed"
	return report, errors.Join(failures...)
}

// settleStopProof settles one recorded terminal proof only while its exact held
// claim is present. A missing row is not evidence that this stop settled it, so
// absence never increments Settled or decrements Unresolved.
func (b *sharedInferenceConsumer) settleStopProof(ctx context.Context, binding sessioncapacity.InferenceBinding, status *utility.AttemptStatus) (settled, evidenced bool) {
	if !matchesInferenceProof(binding, status) {
		return false, false
	}
	current, err := b.capacity.InferenceCapacityBindings(ctx)
	if err != nil {
		return false, false
	}
	if _, present := current[binding.Key]; !present {
		return false, false
	}
	return b.settleLocked(ctx, binding, status), true
}

func countStopSettlementScope(report *utility.InferenceStopSettlementReport, snapshot *utility.AttemptSnapshot, bindings map[string]sessioncapacity.InferenceBinding, matched int) {
	for _, held := range bindings {
		if held.InstanceID == snapshot.InstanceID {
			report.ManagedClaims++
		}
	}
	report.MatchedTerminal = matched
	report.Unresolved = report.ManagedClaims
	for _, child := range snapshot.Attempts {
		if child.Retained {
			if _, held := bindings[inferenceBindingKey(child.OwnerID, child.AttemptID)]; !held {
				report.UnmatchedRetained++
				report.Unresolved++
			}
		}
	}
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
