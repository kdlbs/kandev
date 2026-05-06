package engine

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Threshold values understood by wait_for_quorum.
const (
	QuorumAllApprove      = "all_approve"
	QuorumAllDecide       = "all_decide"
	QuorumAnyReject       = "any_reject"
	QuorumMajorityApprove = "majority_approve"
	// QuorumNApprovePrefix matches thresholds of the form "n_approve:<N>".
	QuorumNApprovePrefix = "n_approve:"
)

// Decision verdict strings the quorum evaluator recognises. Other free-form
// verdicts are accepted and stored, but only "approved" and "rejected" are
// counted by the canned thresholds.
const (
	DecisionApproved = "approved"
	DecisionRejected = "rejected"
)

// evaluateTransitionGuard returns true if the action's guard permits the
// transition (or if the action carries no guard at all). When false, the
// engine skips this action and keeps looking for the next eligible
// transition — preserving the first-transition-wins rule.
//
// A guard that references quorum but has no DecisionStore wired returns
// false: better to stall a transition than fire one whose precondition the
// engine cannot evaluate.
func (e *Engine) evaluateTransitionGuard(ctx context.Context, state MachineState, action Action) (bool, error) {
	if action.Guard == nil {
		return true, nil
	}
	if action.Guard.WaitForQuorum != nil {
		return e.evaluateWaitForQuorum(ctx, state, action.Guard.WaitForQuorum)
	}
	// Unknown guard variant — fail closed.
	return false, nil
}

// evaluateWaitForQuorum reads the (task, step) decisions and the step's
// participants and applies the configured threshold.
func (e *Engine) evaluateWaitForQuorum(ctx context.Context, state MachineState, guard *WaitForQuorumGuard) (bool, error) {
	if e.decisions == nil || e.participants == nil {
		return false, nil
	}
	required, err := e.requiredParticipants(ctx, state.CurrentStepID, state.TaskID, guard.Role)
	if err != nil {
		return false, err
	}
	if len(required) == 0 {
		// No required reviewers — guard cannot be satisfied. Fail closed.
		return false, nil
	}
	decisions, err := e.decisions.ListStepDecisions(ctx, state.TaskID, state.CurrentStepID)
	if err != nil {
		return false, fmt.Errorf("load step decisions for quorum: %w", err)
	}
	return applyThreshold(guard.Threshold, required, decisions), nil
}

// requiredParticipants returns the participants for the (step, task) pair
// whose role matches and whose DecisionRequired flag is true.
func (e *Engine) requiredParticipants(ctx context.Context, stepID, taskID, role string) ([]ParticipantInfo, error) {
	all, err := e.participants.ListStepParticipants(ctx, stepID, taskID)
	if err != nil {
		return nil, fmt.Errorf("list step participants for quorum: %w", err)
	}
	required := make([]ParticipantInfo, 0, len(all))
	for _, p := range all {
		if p.Role != role || !p.DecisionRequired {
			continue
		}
		required = append(required, p)
	}
	return required, nil
}

// applyThreshold counts decisions against the required participant slate
// and reports whether the threshold is met. Decisions whose participant_id
// is not in the required slate are ignored — that's how mid-flight removal
// drops the missing slot from the count.
func applyThreshold(threshold string, required []ParticipantInfo, decisions []DecisionInfo) bool {
	requiredIDs := participantIDSet(required)
	latest := latestDecisionsPerParticipant(decisions, requiredIDs)

	approveCount := 0
	rejectCount := 0
	decidedCount := 0
	for _, d := range latest {
		decidedCount++
		switch d.Decision {
		case DecisionApproved:
			approveCount++
		case DecisionRejected:
			rejectCount++
		}
	}

	totalRequired := len(required)
	switch {
	case threshold == QuorumAllApprove:
		return approveCount == totalRequired
	case threshold == QuorumAllDecide:
		return decidedCount == totalRequired
	case threshold == QuorumAnyReject:
		return rejectCount > 0
	case threshold == QuorumMajorityApprove:
		return approveCount*2 > totalRequired
	case strings.HasPrefix(threshold, QuorumNApprovePrefix):
		n, err := strconv.Atoi(strings.TrimPrefix(threshold, QuorumNApprovePrefix))
		if err != nil || n <= 0 {
			return false
		}
		return approveCount >= n
	}
	return false
}

// participantIDSet returns the set of participant IDs in the slate.
func participantIDSet(participants []ParticipantInfo) map[string]struct{} {
	set := make(map[string]struct{}, len(participants))
	for _, p := range participants {
		set[p.ID] = struct{}{}
	}
	return set
}

// latestDecisionsPerParticipant keeps only the most recent decision per
// participant whose ID is present in the required set. Decisions for
// participants no longer required (removed mid-flight) are dropped.
func latestDecisionsPerParticipant(decisions []DecisionInfo, required map[string]struct{}) map[string]DecisionInfo {
	latest := make(map[string]DecisionInfo, len(required))
	for _, d := range decisions {
		if _, ok := required[d.ParticipantID]; !ok {
			continue
		}
		// Decisions arrive oldest-first per the repo contract; later writes
		// overwrite earlier ones so we keep the latest verdict per participant.
		latest[d.ParticipantID] = d
	}
	return latest
}

// RecordParticipantDecision writes a new decision row and immediately re-
// evaluates pending transitions for that (task, step) so any wait_for_quorum
// guard that just became satisfied can fire.
//
// The returned HandleResult mirrors HandleTrigger: Transitioned indicates a
// transition was applied. EvaluateOnly is honoured if set on the input.
//
// Callers that want to suppress re-evaluation (e.g. record-only flows) can
// pass an empty EvaluateInput by leaving Trigger blank — only the recording
// step runs in that case.
func (e *Engine) RecordParticipantDecision(
	ctx context.Context,
	taskID, sessionID, stepID, participantID, decision, note string,
) error {
	if e.decisions == nil {
		return fmt.Errorf("workflow engine: decision store not wired")
	}
	if taskID == "" || stepID == "" || participantID == "" {
		return fmt.Errorf("record decision requires task_id, step_id, and participant_id")
	}
	if decision == "" {
		return fmt.Errorf("record decision verdict must not be empty")
	}
	if err := e.decisions.RecordStepDecision(ctx, DecisionInfo{
		TaskID:        taskID,
		StepID:        stepID,
		ParticipantID: participantID,
		Decision:      decision,
		Note:          note,
	}); err != nil {
		return err
	}
	// Re-evaluate transitions: fire on_turn_complete in evaluate-only mode so
	// callers (the office service) receive the transition payload and decide
	// how to apply it. Idempotency is keyed off a synthetic operation id.
	//
	// SessionID is optional here — when blank we skip re-evaluation rather
	// than synthesise a fake session. Callers that want a re-eval pass the
	// canonical session id for the (task, step) — typically the task's
	// primary session.
	if sessionID == "" {
		return nil
	}
	_, err := e.HandleTrigger(ctx, HandleInput{
		TaskID:       taskID,
		SessionID:    sessionID,
		Trigger:      TriggerOnTurnComplete,
		EvaluateOnly: true,
		OperationID:  fmt.Sprintf("decision:%s:%s:%s:%d", taskID, stepID, participantID, time.Now().UnixNano()),
	})
	return err
}
