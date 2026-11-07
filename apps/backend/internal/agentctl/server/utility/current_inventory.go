package utility

// RequireCapacity serializes the mode change with every token consumption.
// The instance manager applies it before publishing future instances.
func (o *InferenceOwner) RequireCapacity(required bool) {
	o.attempts.mu.Lock()
	defer o.attempts.mu.Unlock()
	o.attempts.capacityRequired = required
}

func (o *attemptOwner) currentInventoryLocked() *CurrentAttemptInventory {
	current := &CurrentAttemptInventory{Version: 1, Known: true, CapacityManaged: o.capacityRequired, Reason: "complete"}
	if o.storageUnavailable {
		current.Known, current.Reason = false, "owner_unavailable"
		return current
	}
	for _, a := range o.state.Attempts {
		if !a.Status.Known || a.Status.State == AttemptUnknown || (!a.Status.Settled && a.Status.OwnerID != o.id) {
			current.Known, current.Reason = false, "attempt_unknown"
			break
		}
	}
	return current
}

// admitCapacity records the backend's completed reservation before consumption.
func (o *attemptOwner) admitCapacity(id string) (AttemptStatus, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	a := o.state.Attempts[id]
	if o.storageUnavailable || a == nil || a.Status.OwnerID != o.id {
		return AttemptStatus{AttemptID: id, State: AttemptUnknown}, errAttemptUnavailable
	}
	if a.Consumed || a.Status.Settled || !a.Status.Retained || !a.Status.Known {
		return a.Status, errAttemptExists
	}
	a.CapacityAdmitted = true
	// This permission is process-local. Recovered tokens cannot be consumed by a new owner.
	return a.Status, nil
}
