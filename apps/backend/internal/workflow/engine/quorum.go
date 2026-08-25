package engine

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/quorummetrics"
	"go.uber.org/zap"
)

// ErrParticipantNotFound is returned by ResolveParticipantRole when the
// caller does not occupy a reviewer or approver seat with
// decision_required = 1 at the given step (AC-3).
var ErrParticipantNotFound = errors.New("workflow engine: participant not found")

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

// recordGuardNotFired emits the AC-24/24a observability unit for a guarded
// transition that evaluated and did not fire: the quorummetrics counter
// (unconditional) and, when a logger is wired, a structured log record.
// Called from both the ordinary HandleTrigger/evaluateActions path and the
// AC-65-scoped decision re-evaluation path — the two "engine-path"
// evaluators AC-24 covers. The AC-24b/57d read-only diagnostic snapshot
// (EvaluateStepQuorum) must never call this.
func (e *Engine) recordGuardNotFired(state MachineState, action Action, outcome GuardOutcome) {
	quorummetrics.RecordGuardNotFired(outcome.Reason)
	if e.logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("task_id", state.TaskID),
		zap.String("step_id", state.CurrentStepID),
		zap.Int("required_count", outcome.RequiredCount),
		zap.Int("received_count", outcome.ReceivedCount),
		zap.String("reason", outcome.Reason),
	}
	if action.Guard != nil && action.Guard.WaitForQuorum != nil {
		fields = append(fields,
			zap.String("guard_role", action.Guard.WaitForQuorum.Role),
			zap.String("guard_threshold", action.Guard.WaitForQuorum.Threshold),
		)
	}
	if outcome.Err != nil {
		fields = append(fields, zap.Error(outcome.Err))
	}
	e.logger.Warn("workflow quorum guard did not fire", fields...)
}

// recordReevaluationSkip emits the AC-24/24a observability unit for the
// AC-16a/F39 skip: a decision was recorded but re-evaluation could not run
// because the task's active session is unresolvable. Task-level only — no
// guard was evaluated, so guard-scoped fields (role, threshold, counts) are
// omitted.
func (e *Engine) recordReevaluationSkip(taskID, stepID string) {
	quorummetrics.RecordGuardNotFired(ReasonSessionUnresolvable)
	if e.logger == nil {
		return
	}
	e.logger.Warn("workflow quorum reevaluation skipped: session unresolvable",
		zap.String("task_id", taskID),
		zap.String("step_id", stepID),
		zap.String("reason", ReasonSessionUnresolvable),
	)
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

	seats, err := e.requiredSeatsForWorkflow(ctx, state.CurrentStepID, state.TaskID, state.WorkflowID, guard.Role)
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
	return e.requiredSeatsForWorkflow(ctx, stepID, taskID, "", role)
}

func (e *Engine) requiredSeatsForWorkflow(ctx context.Context, stepID, taskID, workflowID, role string) ([]ParticipantInfo, error) {
	gathered, err := gatherParticipantSlate(ctx, e.participants, stepID, taskID, workflowID)
	if err != nil {
		return nil, err
	}

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

// gatherParticipantSlate implements AC-50 step 1, shared by the quorum guard
// and every fan-out/resolution call site that must see the same participant
// population the guard counts: per-task rows for taskID at ANY step
// (workflow-scoped when the store supports it and workflowID is known)
// unioned with template rows (task_id="") at the evaluating step only. It
// does not filter by role/decision_required or dedupe — callers apply their
// own filter and canonicalizeByTaskRoleAgent/collapseByRoleAgent afterward.
func gatherParticipantSlate(ctx context.Context, store ParticipantStore, stepID, taskID, workflowID string) ([]ParticipantInfo, error) {
	var perTask []ParticipantInfo
	var err error
	if scoped, ok := store.(WorkflowScopedParticipantStore); ok && workflowID != "" {
		perTask, err = scoped.ListTaskParticipantsForWorkflow(ctx, taskID, workflowID)
	} else {
		perTask, err = store.ListTaskParticipants(ctx, taskID)
	}
	if err != nil {
		return nil, fmt.Errorf("list task participants: %w", err)
	}
	template, err := store.ListStepParticipants(ctx, stepID, "")
	if err != nil {
		return nil, fmt.Errorf("list step participants: %w", err)
	}

	gathered := make([]ParticipantInfo, 0, len(perTask)+len(template))
	gathered = append(gathered, perTask...)
	gathered = append(gathered, template...)
	return gathered, nil
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

// RecordDecisionResult reports the outcome of RecordParticipantDecision: the
// stamped decision identity (AC-66/57b-i) plus what happened to
// re-evaluation.
type RecordDecisionResult struct {
	DecisionID string
	DecidedAt  time.Time

	// ReevaluationSkipped is true under AC-16a/F39: the decision was
	// recorded but re-evaluation did not run because sessionID was blank
	// (the caller could not resolve the task's active session). SkipReason
	// is ReasonSessionUnresolvable in that case.
	ReevaluationSkipped bool
	SkipReason          string

	// Transitioned / TransitionAbandoned / FromStepID / ToStepID mirror
	// HandleResult's fields for the transition (if any) the re-evaluation
	// applied.
	Transitioned        bool
	TransitionAbandoned bool
	FromStepID          string
	ToStepID            string
	// Guards is the pre-transition quorum snapshot used by the decision
	// response. It remains available after a successful CAS move.
	Guards []QuorumGuardState
}

// RecordParticipantDecision is the engine's single decision-recording entry
// point (AC-57): it writes a new decision row, stamping ID and DecidedAt
// itself (AC-66/57b-i) since DecisionStore.RecordStepDecision takes
// DecisionInfo by value and never echoes generated fields back. When
// sessionID resolves, it then re-evaluates — in "committing mode", applying
// at most one satisfied transition itself (AC-46/47/65) — only the guarded
// on_turn_complete transition actions at the decision's step, never a plain
// HandleTrigger(TriggerOnTurnComplete) call, which would re-fire every
// on_turn_complete action (queue_run, clear_decisions, ...) once per
// decision instead of once per completed turn.
//
// in.ID and in.DecidedAt are ignored — always overwritten with a fresh
// UUID and the current time. sessionID is optional: blank means the caller
// could not resolve the task's active session (AC-16a), so the decision is
// recorded and re-evaluation is skipped without erroring, reported under
// AC-23 as ReasonSessionUnresolvable so it's distinguishable from
// "re-evaluated and found threshold unmet."
func (e *Engine) RecordParticipantDecision(
	ctx context.Context,
	sessionID string,
	in DecisionInfo,
) (RecordDecisionResult, error) {
	if e.decisions == nil {
		return RecordDecisionResult{}, fmt.Errorf("workflow engine: decision store not wired")
	}
	if in.TaskID == "" || in.StepID == "" || in.ParticipantID == "" {
		return RecordDecisionResult{}, fmt.Errorf("record decision requires task_id, step_id, and participant_id")
	}
	if in.Decision == "" {
		return RecordDecisionResult{}, fmt.Errorf("record decision verdict must not be empty")
	}

	in.ID = uuid.New().String()
	in.DecidedAt = time.Now().UTC()
	if err := e.decisions.RecordStepDecision(ctx, in); err != nil {
		return RecordDecisionResult{}, err
	}

	result := RecordDecisionResult{DecisionID: in.ID, DecidedAt: in.DecidedAt}
	if sessionID == "" {
		result.ReevaluationSkipped = true
		result.SkipReason = ReasonSessionUnresolvable
		e.recordReevaluationSkip(in.TaskID, in.StepID)
		return result, nil
	}

	handled, err := e.reevaluateGuardedTransitions(ctx, in.TaskID, sessionID, in.StepID, in.ID)
	if err != nil {
		return result, err
	}
	result.Transitioned = handled.Transitioned
	result.TransitionAbandoned = handled.TransitionAbandoned
	result.FromStepID = handled.FromStepID
	result.ToStepID = handled.ToStepID
	result.Guards = handled.Guards
	return result, nil
}

// reevaluateGuardedTransitions implements the AC-13-17/AC-65 re-evaluation
// scope: it loads the step the decision was recorded against directly (not
// the task's possibly-since-moved current step — the CAS apply below is
// what protects against that race) and applies at most the first satisfied
// guarded on_turn_complete transition action, in configured order. Actions
// without a wait_for_quorum guard, and every non-transition action
// (queue_run, clear_decisions, ...), are out of scope for this path — they
// already ran once for the turn that produced these decisions and must not
// re-run per decision.
//
// The AC-30 operation id is marked applied only once this function has
// fully determined the outcome (transitioned, abandoned, or nothing
// satisfied) — never before, so a crash mid-evaluation does not leave a
// decision's re-evaluation permanently skipped.
func (e *Engine) reevaluateGuardedTransitions(
	ctx context.Context, taskID, sessionID, stepID, decisionID string,
) (HandleResult, error) {
	opID := fmt.Sprintf("decision:%s:%s:%s", taskID, stepID, decisionID)
	applied, err := e.store.IsOperationApplied(ctx, opID)
	if err != nil {
		return HandleResult{}, err
	}
	if applied {
		return HandleResult{Idempotent: true}, nil
	}

	state, err := e.store.LoadState(ctx, taskID, sessionID)
	if err != nil {
		return HandleResult{}, err
	}
	state.CurrentStepID = stepID
	step, err := e.store.LoadStep(ctx, state.WorkflowID, stepID)
	if err != nil {
		return HandleResult{}, err
	}

	result, err := e.applyFirstSatisfiedGuardedTransition(ctx, taskID, sessionID, state, step)
	if err != nil {
		return HandleResult{}, err
	}
	if err := e.markOperationApplied(ctx, opID); err != nil {
		return HandleResult{}, err
	}
	return result, nil
}

// applyFirstSatisfiedGuardedTransition evaluates on_turn_complete's guarded
// transition actions in order and, for the first one whose wait_for_quorum
// guard is satisfied, applies it via the AC-46/48 compare-and-swap. Emits
// the AC-24/24a observability unit for every guard that is evaluated and
// does not fire, same as the ordinary HandleTrigger path.
//
//nolint:cyclop // ordered guard evaluation keeps first-transition semantics explicit.
func (e *Engine) applyFirstSatisfiedGuardedTransition(
	ctx context.Context, taskID, sessionID string, state MachineState, step StepSpec,
) (HandleResult, error) {
	guards := make([]QuorumGuardState, 0)
	var selectedTarget string
	for _, action := range step.Events[TriggerOnTurnComplete] {
		if !isTransitionAction(action.Kind) || action.RequiresApproval {
			continue
		}
		if action.Guard == nil || action.Guard.WaitForQuorum == nil {
			continue
		}
		entry, targetStepID, targetErr, satisfied := e.evaluateGuardForTransition(ctx, state, step, action)
		guards = append(guards, entry)
		if targetErr != nil {
			// Preserve the old write-side behavior for the first satisfied
			// guard, which returned this target-resolution error. A malformed
			// later guard must not prevent an earlier satisfied transition from
			// firing, and an unmet guard can still report its diagnostic state.
			if satisfied && selectedTarget == "" {
				return HandleResult{}, targetErr
			}
			e.recordGuardNotFired(state, action, GuardOutcome{Reason: entry.Reason, Err: entry.Error, RequiredCount: entry.RequiredCount, ReceivedCount: entry.ReceivedCount})
			continue
		}
		if !satisfied {
			e.recordGuardNotFired(state, action, GuardOutcome{Reason: entry.Reason, Err: entry.Error, RequiredCount: entry.RequiredCount, ReceivedCount: entry.ReceivedCount})
			continue
		}
		if targetStepID == "" || targetStepID == state.CurrentStepID {
			continue
		}
		if selectedTarget == "" {
			selectedTarget = targetStepID
		}
	}
	if selectedTarget == "" {
		return HandleResult{Guards: guards}, nil
	}
	applied, err := e.store.ApplyTransitionIfAtStep(
		ctx, taskID, sessionID, state.CurrentStepID, selectedTarget, TriggerOnTurnComplete,
	)
	if err != nil {
		return HandleResult{}, err
	}
	if !applied {
		return HandleResult{Guards: guards, TransitionAbandoned: true, FromStepID: state.CurrentStepID, ToStepID: selectedTarget}, nil
	}
	return HandleResult{Guards: guards, Transitioned: true, FromStepID: state.CurrentStepID, ToStepID: selectedTarget}, nil
}

func (e *Engine) evaluateGuardForTransition(
	ctx context.Context, state MachineState, step StepSpec, action Action,
) (QuorumGuardState, string, error, bool) {
	outcome := e.evaluateTransitionGuard(ctx, state, action)
	targetStepID, targetErr := e.resolveTransitionTarget(ctx, state, step, action)
	if targetErr != nil {
		return QuorumGuardState{
			TargetStepID: targetStepID,
			Role:         action.Guard.WaitForQuorum.Role,
			Threshold:    action.Guard.WaitForQuorum.Threshold,
			Satisfied:    false,
			Reason:       ReasonEvaluationError,
			Error:        targetErr,
		}, targetStepID, targetErr, outcome.Satisfied
	}
	return QuorumGuardState{
		TargetStepID:  targetStepID,
		Role:          action.Guard.WaitForQuorum.Role,
		Threshold:     action.Guard.WaitForQuorum.Threshold,
		RequiredCount: outcome.RequiredCount,
		ReceivedCount: outcome.ReceivedCount,
		Satisfied:     outcome.Satisfied,
		Reason:        outcome.Reason,
		Error:         outcome.Err,
	}, targetStepID, nil, outcome.Satisfied
}

// QuorumGuardState is one guarded transition's read-only quorum state,
// returned by EvaluateStepQuorum (AC-54/57d). Satisfied=true entries omit
// Reason entirely (AC-60).
type QuorumGuardState struct {
	TargetStepID  string
	Role          string
	Threshold     string
	RequiredCount int
	ReceivedCount int
	Satisfied     bool
	Reason        string
	Error         error
}

// QuorumSnapshot is the AC-54/57d read-only diagnostic snapshot for a
// task's current step.
type QuorumSnapshot struct {
	StepID string
	// ReevaluationBlocked reflects only the "task has at least one decision
	// at its current step" conjunct (AC-62) — the engine has no access to
	// the AC-16 active-session query, so it cannot compute the full
	// conjunction with "no active session" by itself. The caller
	// (shared.WorkflowEngineDispatcher's EvaluateStepQuorum, which does have
	// session-resolution access) ANDs in that second conjunct.
	ReevaluationBlocked bool
	// Guards is ordered to match the step's configured on_turn_complete
	// action order (AC-57d), tying AC-61's selection rule to AC-17's
	// first-transition-wins.
	Guards []QuorumGuardState
}

// EvaluateStepQuorum computes the read-only AC-54/57d snapshot for the step
// the given session currently resolves to. sessionID is used only to
// satisfy TransitionStore.LoadState's signature — MachineState.CurrentStepID
// is derived from the task row, not the session, so any valid session for
// the task yields the correct step (see production TransitionStore
// implementations). Applies no transition, writes no row, executes no
// action callback, emits no AC-24 log, and increments no AC-24a counter — a
// diagnostic read never fails on, or mutates, the state it exists to
// diagnose.
//
// A store error while evaluating one guard surfaces as that guard's
// ReasonEvaluationError entry, not a method-level error, so one failing
// guard does not blank the others. A task with no bound current step
// returns an empty, no-error snapshot (AC-24c). sessionID == "" (a task
// that has never had a session) is NOT special-cased to an empty snapshot:
// TransitionStore.LoadState derives CurrentStepID from the task row, not
// the session, so a blank sessionID still yields the correct step, and
// AC-62's ReevaluationBlocked must be computed live from that step's
// decisions rather than short-circuited to false.
func (e *Engine) EvaluateStepQuorum(ctx context.Context, taskID, sessionID string) (QuorumSnapshot, error) {
	state, err := e.store.LoadState(ctx, taskID, sessionID)
	if err != nil {
		return QuorumSnapshot{}, err
	}
	if state.CurrentStepID == "" {
		return QuorumSnapshot{}, nil
	}
	step, err := e.store.LoadStep(ctx, state.WorkflowID, state.CurrentStepID)
	if err != nil {
		return QuorumSnapshot{}, err
	}

	snapshot := QuorumSnapshot{StepID: state.CurrentStepID}
	for _, action := range step.Events[TriggerOnTurnComplete] {
		if !isTransitionAction(action.Kind) || action.Guard == nil {
			continue
		}
		snapshot.Guards = append(snapshot.Guards, e.evaluateGuardStateReadOnly(ctx, state, step, action))
	}

	if e.decisions != nil {
		decisions, derr := e.decisions.ListStepDecisions(ctx, taskID, state.CurrentStepID)
		if derr == nil {
			snapshot.ReevaluationBlocked = len(decisions) > 0
		}
	}

	return snapshot, nil
}

// evaluateGuardStateReadOnly computes one QuorumGuardState entry without any
// side effect (no logging, no counters, no apply).
func (e *Engine) evaluateGuardStateReadOnly(
	ctx context.Context, state MachineState, step StepSpec, action Action,
) QuorumGuardState {
	outcome := e.evaluateTransitionGuard(ctx, state, action)
	targetStepID, err := e.resolveTransitionTarget(ctx, state, step, action)
	if err != nil {
		outcome = GuardOutcome{Reason: ReasonEvaluationError, Err: err}
	}
	entry := QuorumGuardState{
		TargetStepID:  targetStepID,
		Satisfied:     outcome.Satisfied,
		Reason:        outcome.Reason,
		Error:         outcome.Err,
		RequiredCount: outcome.RequiredCount,
		ReceivedCount: outcome.ReceivedCount,
	}
	if action.Guard.WaitForQuorum != nil {
		entry.Role = action.Guard.WaitForQuorum.Role
		entry.Threshold = action.Guard.WaitForQuorum.Threshold
	}
	return entry
}

// ResolveParticipantRole resolves the role and seat id an agent decider
// occupies at stepID, for the agent decision tool's AC-2 (seat id),
// AC-3 (permission), AC-4 (approver-wins) and AC-4a (slate-consistent
// population) requirements. It reuses requiredSeats — the exact AC-50
// slate construction the quorum evaluator itself uses — so role
// resolution and slate membership can never disagree about who
// participates, per AC-57's "exactly once, in the engine" mandate.
// Approver is checked before reviewer, matching the existing
// approver-wins precedence in office/dashboard/decisions.go's
// resolveDeciderRole (AC-4). Returns ErrParticipantNotFound when the
// agent occupies neither seat.
func (e *Engine) ResolveParticipantRole(
	ctx context.Context, taskID, stepID, agentProfileID string,
) (role, participantID string, err error) {
	if agentProfileID == "" {
		return "", "", ErrParticipantNotFound
	}
	workflowID := ""
	if state, stateErr := e.store.LoadState(ctx, taskID, ""); stateErr == nil {
		workflowID = state.WorkflowID
	}
	for _, r := range []wfmodels.ParticipantRole{
		wfmodels.ParticipantRoleApprover,
		wfmodels.ParticipantRoleReviewer,
	} {
		seats, seatsErr := e.requiredSeatsForWorkflow(ctx, stepID, taskID, workflowID, string(r))
		if seatsErr != nil {
			return "", "", fmt.Errorf("resolve participant role: %w", seatsErr)
		}
		if id, ok := seatIDFor(seats, agentProfileID); ok {
			return string(r), id, nil
		}
	}
	return "", "", ErrParticipantNotFound
}

// seatIDFor scans seats for one occupied by agentProfileID, returning its
// AC-50 canonical seat id.
func seatIDFor(seats []ParticipantInfo, agentProfileID string) (string, bool) {
	for _, s := range seats {
		if s.AgentProfileID == agentProfileID {
			return s.ID, true
		}
	}
	return "", false
}
