package backendapp

import (
	"context"
	"maps"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
)

const inferenceBindingActiveState = "active"

type inferenceObservation struct {
	status utility.AttemptStatus
	ref    agentruntime.InferenceClientRef
}

func (b *sharedInferenceConsumer) Refresh(ctx context.Context) { b.refresh(ctx) }

// Reuse native recovered clients and the existing ceiling sweep. An absent owner,
// failed snapshot or missing retained token never proves an empty population.
func (b *sharedInferenceConsumer) refresh(ctx context.Context, extra ...agentruntime.InferenceClientRef) {
	b.refreshMu.Lock()
	defer b.refreshMu.Unlock()
	refs, known := b.inferenceRefs(ctx, extra)
	defer releaseInferenceRefs(refs)
	observations, observedKnown := observeInferenceRefs(ctx, refs)
	known = known && observedKnown
	bindings, err := b.capacity.InferenceCapacityBindings(ctx)
	if err != nil {
		_ = b.capacity.SyncInferenceCapacity(ctx, false, func(map[string]sessioncapacity.InferenceBinding) error { return nil })
		return
	}
	original := maps.Clone(bindings)
	known = b.reconcileHeldInference(ctx, bindings, observations) && known
	known = b.importUnboundInference(ctx, bindings, observations) && known
	err = b.capacity.SyncInferenceCapacity(ctx, known, func(active map[string]sessioncapacity.InferenceBinding) error {
		return mergeObservedInference(active, original, bindings)
	})
	if err == nil {
		b.ackInferenceObservations(ctx, original, bindings, observations)
	}
}

func (b *sharedInferenceConsumer) inferenceRefs(ctx context.Context, extra []agentruntime.InferenceClientRef) ([]agentruntime.InferenceClientRef, bool) {
	known := true
	refs := append([]agentruntime.InferenceClientRef{}, extra...)
	for _, source := range b.sources {
		clients, err := source.InferenceClients(ctx)
		if err != nil {
			known = false
		}
		refs = append(refs, clients...)
	}
	return refs, known
}

func releaseInferenceRefs(refs []agentruntime.InferenceClientRef) {
	for _, ref := range refs {
		if ref.Release != nil {
			ref.Release()
		}
	}
}

func observeInferenceRefs(ctx context.Context, refs []agentruntime.InferenceClientRef) (map[string]inferenceObservation, bool) {
	known := true
	observations := make(map[string]inferenceObservation)
	seen := make(map[string]bool)
	for _, ref := range refs {
		if ref.Client == nil {
			known = false
			continue
		}
		instance := ref.Client.InferenceInstanceID()
		if seen[instance] {
			continue
		}
		seen[instance] = true
		snapshot, err := ref.Client.InferenceAttempts(ctx)
		if err != nil {
			known = false
			continue
		}
		if !snapshot.Known || snapshot.InstanceID != instance {
			known = false
		}
		for _, status := range snapshot.Attempts {
			if status.OwnerID == "" || status.InstanceID != instance {
				known = false
				continue
			}
			key := inferenceBindingKey(status.OwnerID, status.AttemptID)
			observations[key] = inferenceObservation{status: status, ref: ref}
		}
	}
	return observations, known
}

func (b *sharedInferenceConsumer) reconcileHeldInference(ctx context.Context, bindings map[string]sessioncapacity.InferenceBinding, observations map[string]inferenceObservation) bool {
	known := true
	for key, held := range bindings {
		if held.TerminalState != "" {
			if b.finishRetainedInferenceStop(ctx, held) {
				delete(bindings, key)
			} else {
				known = false
			}
			continue
		}
		observed, found := observations[key]
		if !found || observed.status.InstanceID != held.InstanceID {
			known = false
			held.State = inferenceStateUnknown
			bindings[key] = held
			continue
		}
		status := &observed.status
		if status.State == utility.AttemptPrepared && !b.ownsWorker(key) {
			cancelled, cancelErr := observed.ref.Client.CancelPreparedInferenceAttempt(ctx, status.AttemptID)
			if cancelErr == nil {
				status = cancelled
				observed.status = *cancelled
				observations[key] = observed
			}
		}
		if definitelySettledInference(status) {
			if matchesInferenceProof(held, status) {
				held.TerminalState, held.TerminalPromptDispatch = status.State, status.PromptDispatch
				bindings[key] = held
			}
			if !b.settleLocked(ctx, held, status) {
				known = false
				continue
			}
			delete(bindings, key)
			continue
		}
		if !status.Known || status.State == utility.AttemptUnknown {
			known = false
			held.State = inferenceStateUnknown
		} else if status.State != utility.AttemptPrepared {
			held.State = inferenceBindingActiveState
		}
		bindings[key] = held
	}
	return known
}

func (b *sharedInferenceConsumer) importUnboundInference(ctx context.Context, bindings map[string]sessioncapacity.InferenceBinding, observations map[string]inferenceObservation) bool {
	known := true
	for key, observed := range observations {
		status := observed.status
		if _, held := bindings[key]; held || status.Settled {
			continue
		}
		if status.State == utility.AttemptPrepared {
			if status.Retained && !b.ownsWorker(key) {
				if cancelled, err := observed.ref.Client.CancelPreparedInferenceAttempt(ctx, status.AttemptID); err == nil {
					observed.status = *cancelled
					observations[key] = observed
				}
			}
			continue
		}
		if !status.Known {
			known = false
		}
		identity := observed.ref.Identity
		bindings[key] = sessioncapacity.InferenceBinding{Key: key, InstanceID: status.InstanceID, OwnerID: status.OwnerID, AttemptID: status.AttemptID,
			PreparationID: status.PreparationID, TaskID: status.TaskID, SessionID: status.SessionID, CallID: status.CallID,
			OwnerKind: identity.OwnerKind, ParentExecutionID: identity.ParentExecutionID, ExecutorType: identity.ExecutorType, State: inferenceStateUnknown}
	}
	return known
}

func (b *sharedInferenceConsumer) ackInferenceObservations(ctx context.Context, original, bindings map[string]sessioncapacity.InferenceBinding, observations map[string]inferenceObservation) {
	for key, observed := range observations {
		if _, held := bindings[key]; held || !observed.status.Retained {
			continue
		}
		prior, owned := original[key]
		if !owned || !matchesInferenceProof(prior, &observed.status) {
			continue
		}
		_, _ = observed.ref.Client.ReleaseInferenceAttempt(ctx, observed.status.AttemptID)
	}
}

func definitelySettledInference(status *utility.AttemptStatus) bool {
	return status != nil && status.Known && status.Settled && (status.State == utility.AttemptNotStarted || status.State == utility.AttemptTerminated)
}
