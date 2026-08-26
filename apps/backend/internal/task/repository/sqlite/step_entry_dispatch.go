package sqlite

import "context"

// StepEntryDispatcher executes a step's session-independent on_enter
// sequence for one committed arrival. Implemented in production by an
// adapter over (*engine.Engine).DispatchStepEntry, wired at boot in
// internal/backendapp — see
// docs/specs/office/system-design/step-entry-sequence-execution.md.
type StepEntryDispatcher interface {
	DispatchStepEntry(ctx context.Context, taskID, workflowID, stepID, entryID string)
}

// SetStepEntryDispatcher wires the dispatcher every registered
// step-transition writer calls, synchronously, after its own commit
// (AC-OFFICE-STEP-ENTRY-001.9). Unset (nil) is safe: dispatchStepEntry
// becomes a no-op, which is the pre-boot and test-fixture default and keeps
// every repository test that doesn't care about step entry unaffected.
func (r *Repository) SetStepEntryDispatcher(d StepEntryDispatcher) {
	r.stepEntryDispatcher = d
}

// dispatchStepEntry is the no-op-safe call site every registered
// step-transition writer uses after its own tx.Commit() succeeds. It only
// fires when the writer actually produced a new ledger row naming a
// destination step: entryID is empty whenever recordStepTransition itself
// was a no-op (no step change, or a task with no workflow at all), and
// stepID is empty for a detach (RemoveTaskFromWorkflow, which names no
// destination and therefore has no entry sequence to run — see this
// requirement's Terminology and the "arrival" definition in
// docs/specs/office/requirements/step-entry-sequence-execution.md).
func (r *Repository) dispatchStepEntry(ctx context.Context, taskID, workflowID, stepID, entryID string) {
	if r.stepEntryDispatcher == nil || entryID == "" || stepID == "" {
		return
	}
	// The transition is committed before this hook runs. Do not let request
	// cancellation strand the committed entry before its on_enter actions run.
	r.stepEntryDispatcher.DispatchStepEntry(context.WithoutCancel(ctx), taskID, workflowID, stepID, entryID)
}
