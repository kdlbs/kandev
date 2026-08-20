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
// verdicts are accepted and stored, but only "approved" and "rejected" (and,
// per AC-9, "changes_requested" as a synonym for "rejected") are counted by
// the canned thresholds.
const (
	DecisionApproved         = "approved"
	DecisionRejected         = "rejected"
	DecisionChangesRequested = "changes_requested"
)

// DeciderTypeAgent / DeciderTypeUser are the recognized decider_type values.
const (
	DeciderTypeAgent = "agent"
	DeciderTypeUser  = "user"
)

// Reason codes for a guarded transition that does not fire. This is the
// closed set defined by AC-23; AC-24a's expvar keys are exactly these plus
// ReasonSessionUnresolvable (a recording-time skip, not a guard-evaluation
// outcome — see AC-52).
const (
	ReasonThresholdNotMet          = "threshold_not_met"
	ReasonSlateEmpty               = "slate_empty"
	ReasonDecisionStoreUnwired     = "decision_store_unwired"
	ReasonParticipantStoreUnwired  = "participant_store_unwired"
	ReasonGuardVariantUnrecognized = "guard_variant_unrecognized"
	ReasonThresholdUnrecognized    = "threshold_unrecognized"
	ReasonThresholdUnsatisfiable   = "threshold_unsatisfiable"
	ReasonEvaluationError          = "evaluation_error"
	ReasonSessionUnresolvable      = "session_unresolvable"
)

// GuardOutcome is the result of evaluating one wait_for_quorum guard.
// Satisfied=true means the transition may fire; Reason is populated
// whenever Satisfied is false, with exactly one AC-23 code. Err carries the
// underlying error when Reason == ReasonEvaluationError.
type GuardOutcome struct {
	Satisfied     bool
	Reason        string
	Err           error
	RequiredCount int
	ReceivedCount int
}

// isRejectionVerdict reports whether a stored verdict counts as a rejection.
// AC-9: "changes_requested" is a synonym for "rejected" everywhere a
// rejection is counted.
func isRejectionVerdict(decision string) bool {
	return decision == DecisionRejected || decision == DecisionChangesRequested
}

// evaluateTransitionGuard returns the guard outcome for an action. A nil
// guard always permits the transition (kanban semantics, unchanged).
func (e *Engine) evaluateTransitionGuard(ctx context.Context, state MachineState, action Action) GuardOutcome {
	if action.Guard == nil {
		return GuardOutcome{Satisfied: true}
	}
	if action.Guard.WaitForQuorum == nil {
		// Unknown guard variant — fail closed (AC-23/AC-53's sibling code).
		return GuardOutcome{Reason: ReasonGuardVariantUnrecognized}
	}
	return e.computeGuardOutcome(ctx, state, action.Guard.WaitForQuorum)
}

// computeGuardOutcome evaluates a single wait_for_quorum guard against the
// engine's wired stores, following the AC-52 precedence order: store-unwired
// checks first, then (for approve-style thresholds) slate construction
// errors, then an empty slate, then threshold recognition, then
// unsatisfiability, then a genuinely unmet threshold. any_reject does not
// use the seat slate at all (AC-43/AC-59) and so skips the slate_empty step
// entirely.
func (e *Engine) computeGuardOutcome(ctx context.Context, state MachineState, guard *WaitForQuorumGuard) GuardOutcome {
	if e.decisions == nil {
		return GuardOutcome{Reason: ReasonDecisionStoreUnwired}
	}
	if e.participants == nil {
		return GuardOutcome{Reason: ReasonParticipantStoreUnwired}
	}

	if guard.Threshold == QuorumAnyReject {
		decisions, err := e.decisions.ListStepDecisions(ctx, state.TaskID, state.CurrentStepID)
		if err != nil {
			return GuardOutcome{Reason: ReasonEvaluationError, Err: fmt.Errorf("load step decisions for quorum: %w", err)}
		}
		satisfied, received := evaluateAnyReject(decisions, guard.Role)
		outcome := GuardOutcome{Satisfied: satisfied, RequiredCount: 1, ReceivedCount: received}
		if !satisfied {
			outcome.Reason = ReasonThresholdNotMet
		}
		return outcome
	}

	seats, err := e.requiredSeats(ctx, state.CurrentStepID, state.TaskID, guard.Role)
	if err != nil {
		return GuardOutcome{Reason: ReasonEvaluationError, Err: fmt.Errorf("build required slate for quorum: %w", err)}
	}
	if len(seats) == 0 {
		return GuardOutcome{Reason: ReasonSlateEmpty}
	}
	decisions, err := e.decisions.ListStepDecisions(ctx, state.TaskID, state.CurrentStepID)
	if err != nil {
		return GuardOutcome{Reason: ReasonEvaluationError, Err: fmt.Errorf("load step decisions for quorum: %w", err)}
	}
	return evaluateApproveStyle(guard.Threshold, seats, decisions)
}

// requiredSeats builds the AC-50 required slate for a guard's role at the
// evaluating step: gather (per-task rows at any step, plus template rows at
// the evaluating step only) -> filter (role + decision_required) ->
// canonicalize (AC-44, one row per (task_id, role, agent_profile_id)) ->
// collapse (AC-20, one row per (role, agent_profile_id), per-task winning
// over template).
func (e *Engine) requiredSeats(ctx context.Context, stepID, taskID, role string) ([]ParticipantInfo, error) {
	perTask, err := e.participants.ListTaskParticipants(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("list task participants for quorum: %w", err)
	}
	template, err := e.participants.ListStepParticipants(ctx, stepID, "")
	if err != nil {
		return nil, fmt.Errorf("list step participants for quorum: %w", err)
	}

	gathered := make([]ParticipantInfo, 0, len(perTask)+len(template))
	gathered = append(gathered, perTask...)
	gathered = append(gathered, template...)

	filtered := make([]ParticipantInfo, 0, len(gathered))
	for _, p := range gathered {
		if p.Role != role || !p.DecisionRequired {
			continue
		}
		filtered = append(filtered, p)
	}

	canonical := canonicalizeByTaskRoleAgent(filtered, stepID)
	return collapseByRoleAgent(canonical), nil
}

// seatCanonKey identifies a row for AC-44 canonicalization: (task_id, role,
// agent_profile_id), except a row with no agent_profile_id has no identity
// to canonicalize on and is kept standalone, keyed by its own row id.
type seatCanonKey struct {
	taskID, role, agentProfileID, standaloneID string
}

func canonKeyFor(p ParticipantInfo) seatCanonKey {
	if p.AgentProfileID == "" {
		return seatCanonKey{taskID: p.TaskID, role: p.Role, standaloneID: p.ID}
	}
	return seatCanonKey{taskID: p.TaskID, role: p.Role, agentProfileID: p.AgentProfileID}
}

// canonicalizeByTaskRoleAgent implements AC-44: one row per (task_id, role,
// agent_profile_id), preferring the row at the evaluating step, else the
// row with the lowest id in ASCII order.
func canonicalizeByTaskRoleAgent(rows []ParticipantInfo, evaluatingStepID string) []ParticipantInfo {
	groups := map[seatCanonKey][]ParticipantInfo{}
	var order []seatCanonKey
	for _, p := range rows {
		k := canonKeyFor(p)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], p)
	}
	out := make([]ParticipantInfo, 0, len(order))
	for _, k := range order {
		out = append(out, pickCanonicalRow(groups[k], evaluatingStepID))
	}
	return out
}

func pickCanonicalRow(rows []ParticipantInfo, evaluatingStepID string) ParticipantInfo {
	for _, r := range rows {
		if r.StepID == evaluatingStepID {
			return r
		}
	}
	best := rows[0]
	for _, r := range rows[1:] {
		if r.ID < best.ID {
			best = r
		}
	}
	return best
}

// seatCollapseKey identifies a row for AC-20 collapse: (role,
// agent_profile_id), except a row with no agent_profile_id is kept
// standalone, keyed by its own row id.
type seatCollapseKey struct {
	role, agentProfileID, standaloneID string
}

func collapseKeyFor(p ParticipantInfo) seatCollapseKey {
	if p.AgentProfileID == "" {
		return seatCollapseKey{role: p.Role, standaloneID: p.ID}
	}
	return seatCollapseKey{role: p.Role, agentProfileID: p.AgentProfileID}
}

// collapseByRoleAgent implements AC-50 step 4 / AC-20: collapse across
// task_id to one row per (role, agent_profile_id), the per-task row (non
// empty task_id) winning over a template row.
func collapseByRoleAgent(rows []ParticipantInfo) []ParticipantInfo {
	groups := map[seatCollapseKey][]ParticipantInfo{}
	var order []seatCollapseKey
	for _, p := range rows {
		k := collapseKeyFor(p)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], p)
	}
	out := make([]ParticipantInfo, 0, len(order))
	for _, k := range order {
		out = append(out, pickPerTaskOverTemplate(groups[k]))
	}
	return out
}

func pickPerTaskOverTemplate(rows []ParticipantInfo) ParticipantInfo {
	for _, r := range rows {
		if r.TaskID != "" {
			return r
		}
	}
	best := rows[0]
	for _, r := range rows[1:] {
		if r.ID < best.ID {
			best = r
		}
	}
	return best
}

// mapDecisionsToSeats implements AC-51/AC-26: maps each decision to at most
// one seat — by participant_id when it names a seat, otherwise by
// (role, decider_id) for decider_type=agent — and keeps the latest mapped
// decision per seat under the AC-26 ordering (decisions arrive oldest
// first; a later write overwrites an earlier one). A decision that maps to
// no seat is dropped, per AC-43a.
func mapDecisionsToSeats(seats []ParticipantInfo, decisions []DecisionInfo) map[string]DecisionInfo {
	seatByID := make(map[string]struct{}, len(seats))
	seatByRoleAgent := make(map[[2]string]string, len(seats))
	for _, s := range seats {
		seatByID[s.ID] = struct{}{}
		if s.AgentProfileID != "" {
			seatByRoleAgent[[2]string{s.Role, s.AgentProfileID}] = s.ID
		}
	}

	latest := make(map[string]DecisionInfo, len(seats))
	for _, d := range decisions {
		seatID := ""
		if _, ok := seatByID[d.ParticipantID]; ok && d.ParticipantID != "" {
			seatID = d.ParticipantID
		} else if d.DeciderType == DeciderTypeAgent {
			if sid, ok := seatByRoleAgent[[2]string{d.Role, d.DeciderID}]; ok {
				seatID = sid
			}
		}
		if seatID == "" {
			continue
		}
		latest[seatID] = d
	}
	return latest
}

// evaluateAnyReject implements AC-43/AC-43a/AC-43b/AC-58/AC-59: any_reject
// is a decider-identity veto, not a seated quorum contribution. The LAST
// decision per (decider_type, decider_id) under the AC-26 ordering decides
// whether that decider currently vetoes. Agent deciders are filtered to the
// guard's role (AC-42); user deciders are role-agnostic (AC-58), waiving
// resolveDeciderRole's approver-wins quirk so a human rejection counts
// regardless of the stored role. The slate is never consulted — a seatless
// veto is exactly what this threshold exists to allow.
func evaluateAnyReject(decisions []DecisionInfo, guardRole string) (satisfied bool, receivedCount int) {
	type identity struct{ deciderType, deciderID string }
	latest := map[identity]DecisionInfo{}
	for _, d := range decisions {
		if d.DeciderType == "" || d.DeciderID == "" {
			continue
		}
		if d.DeciderType == DeciderTypeAgent && d.Role != guardRole {
			continue
		}
		latest[identity{d.DeciderType, d.DeciderID}] = d
	}
	vetoCount := 0
	for _, d := range latest {
		if isRejectionVerdict(d.Decision) {
			vetoCount++
		}
	}
	return vetoCount > 0, vetoCount
}

// evaluateApproveStyle implements AC-21/AC-39/AC-40/AC-41/AC-43a/AC-53: the
// approve-style thresholds (all_approve, all_decide, majority_approve,
// n_approve:<N>), counting only decisions mapped to an AC-50 seat.
func evaluateApproveStyle(threshold string, seats []ParticipantInfo, decisions []DecisionInfo) GuardOutcome {
	mapped := mapDecisionsToSeats(seats, decisions)
	approveCount, decidedCount := 0, 0
	for _, d := range mapped {
		decidedCount++
		if d.Decision == DecisionApproved {
			approveCount++
		}
	}
	total := len(seats)

	switch {
	case threshold == QuorumAllApprove:
		return finishApproveStyle(approveCount == total, total, approveCount)
	case threshold == QuorumAllDecide:
		return finishApproveStyle(decidedCount == total, total, decidedCount)
	case threshold == QuorumMajorityApprove:
		return finishApproveStyle(approveCount*2 > total, total, approveCount)
	case strings.HasPrefix(threshold, QuorumNApprovePrefix):
		return evaluateNApprove(threshold, total, approveCount)
	default:
		return GuardOutcome{Reason: ReasonThresholdUnrecognized}
	}
}

func evaluateNApprove(threshold string, total, approveCount int) GuardOutcome {
	n, err := strconv.Atoi(strings.TrimPrefix(threshold, QuorumNApprovePrefix))
	if err != nil || n <= 0 {
		return GuardOutcome{Reason: ReasonThresholdUnrecognized}
	}
	if n > total {
		return GuardOutcome{Reason: ReasonThresholdUnsatisfiable, RequiredCount: n, ReceivedCount: approveCount}
	}
	return finishApproveStyle(approveCount >= n, n, approveCount)
}

func finishApproveStyle(satisfied bool, required, received int) GuardOutcome {
	outcome := GuardOutcome{Satisfied: satisfied, RequiredCount: required, ReceivedCount: received}
	if !satisfied {
		outcome.Reason = ReasonThresholdNotMet
	}
	return outcome
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
