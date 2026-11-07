package backendapp

import (
	"context"
	"maps"
	"strconv"

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
// failed inventory/snapshot or missing retained token never proves an empty
// population.
//
// A failed source or per-execution snapshot marks the population unknown so new
// admission fails closed, while held bindings remain in the store and stay
// counted. One unreadable execution therefore never erases the known bindings.
func (b *sharedInferenceConsumer) refresh(ctx context.Context, extra ...agentruntime.InferenceClientRef) {
	b.refreshMu.Lock()
	defer b.refreshMu.Unlock()
	refs, known := b.inferenceRefs(ctx, extra)
	defer releaseInferenceRefs(refs)
	observations, observedKnown := b.observeInferenceRefs(ctx, refs)
	known = known && observedKnown
	bindings, err := b.capacity.InferenceCapacityBindings(ctx)
	if err != nil {
		b.logPopulationUnknown("inference_bindings", inferencePopulationReadFailure(err), sessioncapacity.InferenceBinding{})
		_ = b.capacity.SyncInferenceCapacity(ctx, false, func(map[string]sessioncapacity.InferenceBinding) error { return nil })
		return
	}
	if !observedKnown {
		b.logPopulationUnknown("inference_bindings", "snapshot_unknown_context", sessioncapacity.InferenceBinding{}, inferenceBindingDiagnosticFields(bindings)...)
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
	all, finite := b.capacity.InferenceCapacityPolicy()
	for index, source := range b.sources {
		clients, err := inferenceSourceClients(ctx, source, all && finite)
		if err != nil {
			b.logPopulationUnknown("inference_inventory_"+strconv.Itoa(index), inferencePopulationReadFailure(err), sessioncapacity.InferenceBinding{})
			known = false
		}
		refs = append(refs, clients...)
	}
	return refs, known
}

func inferenceSourceClients(ctx context.Context, source inferenceClientSource, required bool) ([]agentruntime.InferenceClientRef, error) {
	if current, ok := source.(interface {
		InferenceCapacityClients(context.Context, bool) ([]agentruntime.InferenceClientRef, error)
	}); ok {
		return current.InferenceCapacityClients(ctx, required)
	}
	return source.InferenceClients(ctx)
}

func currentInferencePopulationKnown(snapshot *utility.AttemptSnapshot, required bool) bool {
	if snapshot.Current == nil {
		return snapshot.Known
	}
	return snapshot.Current.Version == 1 && snapshot.Current.Known && (!required || snapshot.Current.CapacityManaged)
}

func releaseInferenceRefs(refs []agentruntime.InferenceClientRef) {
	for _, ref := range refs {
		if ref.Release != nil {
			ref.Release()
		}
	}
}

func (b *sharedInferenceConsumer) observeInferenceRefs(ctx context.Context, refs []agentruntime.InferenceClientRef) (map[string]inferenceObservation, bool) {
	known := true
	all, finite := b.capacity.InferenceCapacityPolicy()
	observations := make(map[string]inferenceObservation)
	seen := make(map[string]bool)
	for _, ref := range refs {
		locator := inferencePopulationLocator(ref, nil)
		if ref.Client == nil {
			b.logPopulationUnknown("inference_snapshot", "client_missing", locator)
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
			b.logPopulationUnknown("inference_snapshot", inferencePopulationReadFailure(err), locator)
			known = false
			continue
		}
		if !currentInferencePopulationKnown(snapshot, all && finite) || snapshot.InstanceID != instance {
			reason := "snapshot_unknown"
			if snapshot.InstanceID != instance {
				reason = "instance_mismatch"
			}
			b.logPopulationUnknown("inference_snapshot", reason, locator, inferenceSnapshotDiagnosticFields(snapshot)...)
			known = false
		}
		known = b.collectSnapshotAttempts(observations, ref, snapshot, instance) && known
	}
	return observations, known
}

func (b *sharedInferenceConsumer) collectSnapshotAttempts(observations map[string]inferenceObservation, ref agentruntime.InferenceClientRef, snapshot *utility.AttemptSnapshot, instance string) bool {
	known := true
	for _, status := range snapshot.Attempts {
		if status.OwnerID == "" || status.InstanceID != instance {
			b.logPopulationUnknown("inference_snapshot", "attempt_identity_unavailable", inferencePopulationLocator(ref, &status))
			known = false
			continue
		}
		key := inferenceBindingKey(status.OwnerID, status.AttemptID)
		observations[key] = inferenceObservation{status: status, ref: ref}
	}
	return known
}

func (b *sharedInferenceConsumer) reconcileHeldInference(ctx context.Context, bindings map[string]sessioncapacity.InferenceBinding, observations map[string]inferenceObservation) bool {
	known := true
	for key, held := range bindings {
		if held.TerminalState != "" {
			if b.finishRetainedInferenceStop(ctx, held) {
				delete(bindings, key)
			} else {
				b.logPopulationUnknown("inference_settlement", "retained_settlement_unconfirmed", held)
				known = false
			}
			continue
		}
		observed, found := observations[key]
		if !found || observed.status.InstanceID != held.InstanceID {
			b.logPopulationUnknown("inference_owner", "observation_unavailable", held)
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
				b.logPopulationUnknown("inference_settlement", "settlement_unconfirmed", held)
				known = false
				continue
			}
			delete(bindings, key)
			continue
		}
		if !status.Known || status.State == utility.AttemptUnknown {
			b.logPopulationUnknown("inference_attempt", "state_unknown", held)
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
			b.logPopulationUnknown("inference_attempt", "unbound_state_unknown", inferencePopulationLocator(observed.ref, &status))
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
		status := observed.status
		if definitelySettledInference(&status) {
			// Settled tokens whose binding was already removed still need their ACK
			// retried. They are not replayable and belong to this live instance.
			_, _ = observed.ref.Client.ReleaseInferenceAttempt(ctx, status.AttemptID)
			continue
		}
		prior, owned := original[key]
		if !owned || !matchesInferenceProof(prior, &status) {
			continue
		}
		_, _ = observed.ref.Client.ReleaseInferenceAttempt(ctx, status.AttemptID)
	}
}

func definitelySettledInference(status *utility.AttemptStatus) bool {
	return status != nil && status.Known && status.Settled && (status.State == utility.AttemptNotStarted || status.State == utility.AttemptTerminated)
}
