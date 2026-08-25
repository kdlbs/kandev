package engine

import (
	"context"
	"strings"
)

// sessionShapedActionKinds are the step-entry action kinds a route with a
// live arriving session already executes today — through the orchestrator's
// own inline entry handler on most routes, and through
// HandleTriggerSessionShapedOnly on the workflow-switch route. DispatchStepEntry
// never runs these: running them a second time would launch or prompt an
// agent twice (AC-OFFICE-STEP-ENTRY-001.3, .5).
var sessionShapedActionKinds = map[ActionKind]bool{
	ActionEnablePlanMode:    true,
	ActionAutoStartAgent:    true,
	ActionResetAgentContext: true,
	ActionSetSessionMode:    true,
}

// sessionIndependentActionKinds are the step-entry action kinds
// DispatchStepEntry executes. None of them reads or mutates the arriving
// session, so every one is safe to run for an arrival with no live session
// (AC-OFFICE-STEP-ENTRY-001.1).
var sessionIndependentActionKinds = map[ActionKind]bool{
	ActionClearDecisions:             true,
	ActionQueueRunForEachParticipant: true,
	ActionQueueRun:                   true,
	ActionRunCodeReview:              true,
	ActionEnsureParticipantSeat:      true,
}

func isSessionShapedActionKind(kind ActionKind) bool {
	return sessionShapedActionKinds[kind]
}

func isSessionIndependentActionKind(kind ActionKind) bool {
	return sessionIndependentActionKinds[kind]
}

// StepEntryActionResult records the outcome of one action DispatchStepEntry
// attempted. Err is nil for an action that succeeded.
type StepEntryActionResult struct {
	Kind ActionKind
	Err  error
}

// DispatchStepEntry executes the session-independent half of the step's
// declared on_enter sequence for one committed arrival at (workflowID,
// stepID). Callers are the registered step-transition ledger writers, once
// per committed row that names a destination step, synchronously after
// their own commit (AC-OFFICE-STEP-ENTRY-001.1, .8, .9).
//
// Unlike HandleTrigger, this requires no live session, uses a
// record-and-continue loop instead of aborting on the first failed action
// (AC-OFFICE-STEP-ENTRY-001.4, .6), and carries entryID — the ledger row's
// own identifier — rather than an operation id
// (AC-OFFICE-STEP-ENTRY-001.2, .7). It bypasses HandleTrigger's
// already-applied/mark-applied bookkeeping entirely: per design, there is no
// separate "this entry already ran" marker. Each dispatched action is
// individually idempotent against durable state keyed by entryID
// (AC-OFFICE-STEP-ENTRY-001.10) — see
// docs/specs/office/system-design/step-entry-sequence-execution.md
// ("Where 'this entry already ran' is recorded").
func (e *Engine) DispatchStepEntry(ctx context.Context, taskID, workflowID, stepID, entryID string) []StepEntryActionResult {
	step, err := e.store.LoadStep(ctx, workflowID, stepID)
	if err != nil {
		return []StepEntryActionResult{{Err: err}}
	}
	actions := step.Events[TriggerOnEnter]
	if len(actions) == 0 {
		return nil
	}

	state := MachineState{TaskID: taskID, WorkflowID: workflowID, CurrentStepID: stepID}
	results := make([]StepEntryActionResult, 0, len(actions))
	seenParticipantRoles := map[string]bool{}
	for _, action := range actions {
		if !isSessionIndependentActionKind(action.Kind) {
			// Exclusion is the contract, not an anomaly: session-shaped
			// kinds (and any kind not yet classified) are silently
			// skipped here, not recorded as skipped work.
			continue
		}
		if action.Kind == ActionEnsureParticipantSeat && seenEnsureParticipantSeatRole(seenParticipantRoles, action) {
			// AC-OFFICE-REVIEW-SEATS-001.12/004.7: a step that declares the
			// seat-ensuring action more than once for the same participant
			// role writes at most one seat and emits at most one record for
			// it. Re-running the callback for a repeat declaration would
			// re-count an outcome (e.g. already_seated, or a second
			// malformed-role warning) that AC-004.7 requires collapsed to
			// one per role per entry.
			continue
		}
		results = append(results, StepEntryActionResult{
			Kind: action.Kind,
			Err:  e.executeStepEntryAction(ctx, state, step, action, entryID),
		})
	}
	return results
}

// seenEnsureParticipantSeatRole reports whether action declares the same
// raw (unvalidated) participant role as an earlier ActionEnsureParticipantSeat
// action already dispatched in this DispatchStepEntry call, recording it if
// not. Deduping on the raw string — rather than only on validated roles —
// also collapses repeated malformed declarations (AC-OFFICE-REVIEW-SEATS-001.11)
// to a single warning record, matching AC-OFFICE-REVIEW-SEATS-004.7's "at
// most one record per participant role for each condition" for both the
// repeated-declaration and malformed-declaration conditions.
func seenEnsureParticipantSeatRole(seen map[string]bool, action Action) bool {
	var role string
	if action.EnsureParticipantSeat != nil {
		role = strings.TrimSpace(action.EnsureParticipantSeat.Role)
	}
	if seen[role] {
		return true
	}
	seen[role] = true
	return false
}

// executeStepEntryAction runs one action's callback for DispatchStepEntry.
// An unregistered kind is a no-op, matching Engine.executeCallback. There is
// no DataPatch handling: none of the session-independent kinds ever produce
// one (only set_workflow_data does, and it is not compiled for step entry),
// and DispatchStepEntry has no session to persist one against.
func (e *Engine) executeStepEntryAction(ctx context.Context, state MachineState, step StepSpec, action Action, entryID string) error {
	callback, ok := e.callbacks.Get(action.Kind)
	if !ok {
		return nil
	}
	_, err := callback.Execute(ctx, ActionInput{
		Trigger: TriggerOnEnter,
		State:   state,
		Step:    step,
		Action:  action,
		EntryID: entryID,
	})
	return err
}
