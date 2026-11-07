package instance

import "github.com/kandev/kandev/internal/agentctl/server/utility"

// FinalInferenceSnapshot forwards only the proof retained by this stopped owner.
func (i *Instance) FinalInferenceSnapshot() *utility.AttemptSnapshot {
	if owner, ok := i.manager.(interface {
		FinalInferenceSnapshot() *utility.AttemptSnapshot
	}); ok {
		return owner.FinalInferenceSnapshot()
	}
	return nil
}

// FinalManagedStopReceipt forwards the current manager's bounded stop result.
func (i *Instance) FinalManagedStopReceipt() *utility.ManagedProcessStopReceipt {
	if owner, ok := i.manager.(interface {
		FinalManagedStopReceipt() *utility.ManagedProcessStopReceipt
	}); ok {
		return owner.FinalManagedStopReceipt()
	}
	return nil
}
