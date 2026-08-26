package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/runs/commentkeys"
	"github.com/kandev/kandev/internal/workflow/quorummetrics"
	"go.uber.org/zap"
)

// ErrActionNotYetWired is the sentinel returned by Phase 2 callbacks when a
// required engine dependency (RunQueueAdapter, ParticipantStore, …) has not
// been wired. It is intentionally exported so the orchestrator can detect
// "kanban-only" engines vs office-wired ones in tests, and so callers see a
// loud, distinctive error rather than a silent no-op.
var ErrActionNotYetWired = errors.New("workflow action not yet wired")

// ErrMalformedParticipantRole is the sentinel EnsureParticipantSeatCallback
// wraps when an ensure_participant_seat action's configured role is empty
// or not one of the recognised participant roles. Configuration is
// operator-editable and survives template changes, so this is detected and
// reported at runtime rather than skipped at compile time
// (AC-OFFICE-REVIEW-SEATS-001.11) — the callback writes no seat, and
// DispatchStepEntry's record-and-continue loop treats the returned error as
// non-fatal so the entry proceeds.
var ErrMalformedParticipantRole = errors.New("ensure_participant_seat: malformed role")

// ErrParticipantSeatUnfillable is the sentinel EnsureParticipantSeatCallback
// wraps when ParticipantSeatCaster could not resolve any agent for the role
// (REQ-002 step 2's no-runner branch). The callback writes no seat.
var ErrParticipantSeatUnfillable = errors.New("ensure_participant_seat: role unfillable")

// Target prefixes / sentinels recognised by QueueRunCallback.
const (
	TargetPrimary       = "primary"
	TargetParticipant   = "participant_role:"
	TargetAgentProfile  = "agent_profile_id:"
	TargetWorkspaceCEO  = "workspace.ceo_agent"
	TaskIDThis          = "this"
	defaultQueueReasonR = "queue_run"
)

// PrimaryAgentResolver resolves the task's "primary" agent profile id. The
// engine asks via this interface when a queue_run target is "primary". The
// answer is task-aware so office steps can prefer the task's current runner
// participant while kanban-style steps still fall back to the step primary.
// The indirection keeps the engine package free of model imports.
type PrimaryAgentResolver interface {
	PrimaryAgentProfileID(ctx context.Context, stepID, taskID string) (string, error)
}

// TargetTaskStepResolver resolves a task's current workflow step for cross-task
// queue_run actions. The engine only has the triggering task's StepSpec, so
// adapters that read task storage provide this for target task lookups.
type TargetTaskStepResolver interface {
	WorkflowStepIDForTask(ctx context.Context, taskID string) (string, error)
}

// QueueRunCallback executes the queue_run action by resolving Target/TaskID
// then enqueuing a run via RunQueueAdapter.
type QueueRunCallback struct {
	Adapter      RunQueueAdapter
	Participants ParticipantStore
	CEOResolver  CEOAgentResolver
	Primary      PrimaryAgentResolver
	TaskSteps    TargetTaskStepResolver
}

// Execute satisfies ActionCallback.
func (c QueueRunCallback) Execute(ctx context.Context, in ActionInput) (ActionResult, error) {
	if c.Adapter == nil {
		return ActionResult{}, fmt.Errorf("%w: queue_run requires RunQueueAdapter", ErrActionNotYetWired)
	}
	if in.Action.QueueRun == nil {
		return ActionResult{}, fmt.Errorf("queue_run action missing QueueRun config")
	}
	taskID := resolveTaskID(in.Action.QueueRun.TaskID, in.State.TaskID)
	agentIDs, workflowStepID, err := c.resolveTarget(ctx, in, taskID)
	if err != nil {
		return ActionResult{}, err
	}
	for _, agentID := range agentIDs {
		req := QueueRunRequest{
			AgentProfileID: agentID,
			TaskID:         taskID,
			WorkflowStepID: workflowStepID,
			Reason:         queueRunReason(in),
			IdempotencyKey: idempotencyKey(in, agentID, taskID),
			Payload:        queueRunPayload(in, in.Action.QueueRun.Payload, taskID),
		}
		if _, err := c.Adapter.QueueRun(ctx, req); err != nil {
			return ActionResult{}, fmt.Errorf("queue_run for agent %s: %w", agentID, err)
		}
	}
	return ActionResult{}, nil
}

func (c QueueRunCallback) resolveTarget(
	ctx context.Context, in ActionInput, taskID string,
) ([]string, string, error) {
	target := strings.TrimSpace(in.Action.QueueRun.Target)
	switch {
	case target == "" || target == TargetPrimary:
		stepID, err := c.resolveTargetStepID(ctx, in, taskID, pickStepResolver(c.Primary, c.TaskSteps))
		if err != nil {
			return nil, "", err
		}
		agentIDs, err := c.resolvePrimary(ctx, taskID, stepID)
		return agentIDs, stepID, err
	case strings.HasPrefix(target, TargetParticipant):
		role := strings.TrimPrefix(target, TargetParticipant)
		stepID, err := c.resolveTargetStepID(ctx, in, taskID, pickStepResolver(c.Participants, c.TaskSteps))
		if err != nil {
			return nil, "", err
		}
		var agentIDs []string
		if taskID == in.State.TaskID {
			agentIDs, err = c.resolveParticipantRole(ctx, stepID, taskID, in.State.WorkflowID, role)
		} else {
			agentIDs, err = c.resolveParticipantRoleStepScoped(ctx, stepID, taskID, role)
		}
		return agentIDs, stepID, err
	case strings.HasPrefix(target, TargetAgentProfile):
		id := strings.TrimPrefix(target, TargetAgentProfile)
		if id == "" {
			return nil, "", fmt.Errorf("queue_run agent_profile_id target is empty")
		}
		stepID, err := c.resolveTargetStepID(ctx, in, taskID, c.TaskSteps)
		if err != nil {
			return nil, "", err
		}
		return []string{id}, stepID, nil
	case target == TargetWorkspaceCEO:
		stepID, err := c.resolveTargetStepID(ctx, in, taskID, c.TaskSteps)
		if err != nil {
			return nil, "", err
		}
		agentIDs, err := c.resolveCEO(ctx, taskID)
		return agentIDs, stepID, err
	default:
		return nil, "", fmt.Errorf("queue_run: unsupported target %q", target)
	}
}

func (c QueueRunCallback) resolveTargetStepID(
	ctx context.Context, in ActionInput, taskID string, resolver TargetTaskStepResolver,
) (string, error) {
	if taskID == in.State.TaskID {
		return in.Step.ID, nil
	}
	if resolver == nil {
		return "", fmt.Errorf("%w: queue_run cross-task target requires TargetTaskStepResolver", ErrActionNotYetWired)
	}
	stepID, err := resolver.WorkflowStepIDForTask(ctx, taskID)
	if err != nil {
		return "", fmt.Errorf("queue_run resolve target task step: %w", err)
	}
	if stepID == "" {
		return "", fmt.Errorf("queue_run: task %s has no workflow step", taskID)
	}
	return stepID, nil
}

func pickStepResolver(v any, fallback TargetTaskStepResolver) TargetTaskStepResolver {
	if resolver, ok := v.(TargetTaskStepResolver); ok {
		return resolver
	}
	return fallback
}

func (c QueueRunCallback) resolvePrimary(ctx context.Context, taskID, stepID string) ([]string, error) {
	if c.Primary == nil {
		return nil, fmt.Errorf("%w: queue_run target=primary requires PrimaryAgentResolver", ErrActionNotYetWired)
	}
	id, err := c.Primary.PrimaryAgentProfileID(ctx, stepID, taskID)
	if err != nil {
		return nil, fmt.Errorf("queue_run resolve primary: %w", err)
	}
	if id == "" {
		return nil, fmt.Errorf("queue_run: step %s has no primary agent profile", stepID)
	}
	return []string{id}, nil
}

func (c QueueRunCallback) resolveParticipantRole(ctx context.Context, stepID, taskID, workflowID, role string) ([]string, error) {
	if c.Participants == nil {
		return nil, fmt.Errorf("%w: queue_run target=participant_role requires ParticipantStore", ErrActionNotYetWired)
	}
	seats, err := roleSeatsForFanOut(ctx, c.Participants, stepID, taskID, workflowID, role)
	if err != nil {
		return nil, fmt.Errorf("queue_run list participants: %w", err)
	}
	ids := make([]string, 0, len(seats))
	for _, p := range seats {
		ids = append(ids, p.AgentProfileID)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("queue_run: no participants with role %q on step %s", role, stepID)
	}
	return ids, nil
}

// resolveParticipantRoleStepScoped resolves a CROSS-task participant_role
// target — the pre-WO-30 behaviour, unchanged. It must NOT widen to the
// any-step/any-workflow slate resolveParticipantRole uses: that slate is
// only safe for the same-task case, where in.State.WorkflowID scopes
// gatherParticipantSlate's ListTaskParticipantsForWorkflow lookup to the
// task's actual current workflow. For a cross-task target the engine has no
// way to resolve the TARGET task's own workflow id here, so an any-step read
// would return per-task rows stamped on a step from a workflow the target
// task has since left (see phase2_sqlite.go's ListParticipantsForTaskAnyStep
// doc comment: overrides must not leak into quorum evaluation for the new
// workflow). Staying step-scoped to the target task's current step is the
// only safe read available.
func (c QueueRunCallback) resolveParticipantRoleStepScoped(ctx context.Context, stepID, taskID, role string) ([]string, error) {
	if c.Participants == nil {
		return nil, fmt.Errorf("%w: queue_run target=participant_role requires ParticipantStore", ErrActionNotYetWired)
	}
	all, err := c.Participants.ListStepParticipants(ctx, stepID, taskID)
	if err != nil {
		return nil, fmt.Errorf("queue_run list participants: %w", err)
	}
	ids := make([]string, 0, len(all))
	for _, p := range all {
		if p.Role == role {
			ids = append(ids, p.AgentProfileID)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("queue_run: no participants with role %q on step %s", role, stepID)
	}
	return ids, nil
}

// roleSeatsForFanOut gathers the same participant population the quorum
// guard counts (gatherParticipantSlate — per-task rows at any step, unioned
// with template rows at the evaluating step), filters to role (deliberately
// NOT DecisionRequired, unlike the guard's own slate — a fan-out wakes every
// seat in the role, not just decision-required ones), then dedupes via the
// guard's own canonicalize/collapse so a reviewer present at multiple steps
// or as both a per-task and template row is woken exactly once. The result
// is sorted by position ascending then agent profile id ascending
// (AC-OFFICE-REVIEW-SEATS-003.3) — explicitly, in Go, rather than trusting
// the underlying stores' query order, since gatherParticipantSlate unions
// two separately-ordered queries and canonicalize/collapse iterate in
// first-seen order.
func roleSeatsForFanOut(ctx context.Context, store ParticipantStore, stepID, taskID, workflowID, role string) ([]ParticipantInfo, error) {
	gathered, err := gatherParticipantSlate(ctx, store, stepID, taskID, workflowID)
	if err != nil {
		return nil, err
	}
	filtered := make([]ParticipantInfo, 0, len(gathered))
	for _, p := range gathered {
		if p.Role == role {
			filtered = append(filtered, p)
		}
	}
	seats := collapseByRoleAgent(canonicalizeByTaskRoleAgent(filtered, stepID))
	sort.SliceStable(seats, func(i, j int) bool {
		if seats[i].Position != seats[j].Position {
			return seats[i].Position < seats[j].Position
		}
		return seats[i].AgentProfileID < seats[j].AgentProfileID
	})
	return seats, nil
}

func (c QueueRunCallback) resolveCEO(ctx context.Context, taskID string) ([]string, error) {
	if c.CEOResolver == nil {
		return nil, fmt.Errorf("%w: queue_run target=workspace.ceo_agent requires CEOAgentResolver", ErrActionNotYetWired)
	}
	id, err := c.CEOResolver.ResolveCEOAgentProfileID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("queue_run resolve workspace.ceo_agent: %w", err)
	}
	if id == "" {
		return nil, fmt.Errorf("queue_run: workspace has no CEO agent profile for task %s", taskID)
	}
	return []string{id}, nil
}

// resolveTaskID maps the action's TaskID string into a concrete id, honouring
// the "this" sentinel and the default-empty-means-this convention.
func resolveTaskID(target, currentTaskID string) string {
	t := strings.TrimSpace(target)
	if t == "" || t == TaskIDThis {
		return currentTaskID
	}
	return t
}

// queueRunReason picks the action-supplied reason, falling back to the
// trigger type so logs and telemetry get a meaningful default.
func queueRunReason(in ActionInput) string {
	if in.Action.QueueRun != nil && in.Action.QueueRun.Reason != "" {
		return in.Action.QueueRun.Reason
	}
	if in.Trigger != "" {
		return string(in.Trigger)
	}
	return defaultQueueReasonR
}

// idempotencyKey synthesises a deterministic key from the engine's
// operation id (already idempotent across retries) plus action-specific
// salt. Comment status lookups map runs back through payload.comment_id, so
// even same-task primary comment wakes keep agent/task/action salt. That lets
// a later wake for the same comment reach a newly resolved runner instead of
// being suppressed by a bare task_comment:<comment_id> key.
// When OperationID is empty, the adapter sees an empty key and is expected to
// dedupe via its own mechanism (or accept the duplicate).
func idempotencyKey(in ActionInput, agentID, taskID string) string {
	// EntryID (the step-transition ledger row's own identifier) takes
	// priority: it is the durable identity DispatchStepEntry carries for a
	// step-entry action, and is stable across a redelivery of the same
	// arrival in a way OperationID is not for that path. OperationID
	// remains the key for every non-step-entry trigger, where EntryID is
	// always empty. See AC-OFFICE-STEP-ENTRY-001.2/.7/.10 and
	// docs/specs/office/system-design/step-entry-sequence-execution.md.
	opKey := in.EntryID
	if opKey == "" {
		opKey = in.OperationID
	}
	if opKey == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s:%s:%s:%s",
		opKey, in.Step.ID, taskID, agentID, queueActionDigest(in))
}

func queueActionDigest(in ActionInput) string {
	key := struct {
		Kind    ActionKind     `json:"kind"`
		Target  string         `json:"target,omitempty"`
		TaskID  string         `json:"task_id,omitempty"`
		Role    string         `json:"role,omitempty"`
		Reason  string         `json:"reason"`
		Payload map[string]any `json:"payload,omitempty"`
	}{
		Kind: in.Action.Kind,
	}
	switch in.Action.Kind {
	case ActionQueueRun:
		if in.Action.QueueRun != nil {
			key.Target = strings.TrimSpace(in.Action.QueueRun.Target)
			key.TaskID = strings.TrimSpace(in.Action.QueueRun.TaskID)
			key.Reason = queueRunReason(in)
			key.Payload = in.Action.QueueRun.Payload
		}
	case ActionQueueRunForEachParticipant:
		if in.Action.QueueRunForEachParticipant != nil {
			cfg := in.Action.QueueRunForEachParticipant
			key.Role = strings.TrimSpace(cfg.Role)
			key.Reason = queueRunForEachParticipantReason(in)
			key.Payload = cfg.Payload
		}
	}
	b, err := json.Marshal(key)
	if err != nil {
		b = []byte(string(key.Kind) + "\x00" + key.Target + "\x00" +
			key.TaskID + "\x00" + key.Role + "\x00" + key.Reason)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

func queueRunPayload(in ActionInput, actionPayload map[string]any, targetTaskID string) map[string]any {
	out := make(map[string]any, len(actionPayload))
	comment, ok := commentPayload(in.Payload)
	if ok {
		if comment.CommentID != "" {
			out["comment_id"] = comment.CommentID
		}
		if comment.AuthorID != "" {
			out["author_id"] = comment.AuthorID
		}
	}
	// Workflow-authored payload fields are explicit overrides. The trigger's
	// comment_id/author_id only provide defaults for ordinary comment wakes.
	for k, v := range actionPayload {
		out[k] = v
	}
	if targetTaskID != "" &&
		in.State.TaskID != "" &&
		targetTaskID != in.State.TaskID &&
		(ok || commentkeys.HasTaskCommentPrefix(in.OperationID)) {
		out["source_task_id"] = in.State.TaskID
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func commentPayload(payload any) (OnCommentPayload, bool) {
	switch p := payload.(type) {
	case OnCommentPayload:
		return p, true
	case *OnCommentPayload:
		if p != nil {
			return *p, true
		}
	}
	return OnCommentPayload{}, false
}

// ClearDecisionsCallback executes the clear_decisions action by deleting all
// recorded decisions for the trigger's (task, step) pair.
type ClearDecisionsCallback struct {
	Decisions DecisionStore
}

// Execute satisfies ActionCallback.
func (c ClearDecisionsCallback) Execute(ctx context.Context, in ActionInput) (ActionResult, error) {
	if c.Decisions == nil {
		return ActionResult{}, fmt.Errorf("%w: clear_decisions requires DecisionStore", ErrActionNotYetWired)
	}
	if _, err := c.Decisions.ClearStepDecisions(ctx, in.State.TaskID, in.Step.ID); err != nil {
		return ActionResult{}, fmt.Errorf("clear_decisions: %w", err)
	}
	return ActionResult{}, nil
}

// QueueRunForEachParticipantCallback fans out queue_run over every participant
// in the task's workflow-scoped participant slate matching the configured role.
type QueueRunForEachParticipantCallback struct {
	Adapter      RunQueueAdapter
	Participants ParticipantStore
}

// Execute satisfies ActionCallback.
func (c QueueRunForEachParticipantCallback) Execute(ctx context.Context, in ActionInput) (ActionResult, error) {
	if c.Adapter == nil {
		return ActionResult{}, fmt.Errorf("%w: queue_run_for_each_participant requires RunQueueAdapter", ErrActionNotYetWired)
	}
	if c.Participants == nil {
		return ActionResult{}, fmt.Errorf("%w: queue_run_for_each_participant requires ParticipantStore", ErrActionNotYetWired)
	}
	cfg := in.Action.QueueRunForEachParticipant
	if cfg == nil || cfg.Role == "" {
		return ActionResult{}, fmt.Errorf("queue_run_for_each_participant missing role")
	}
	taskID := in.State.TaskID
	seats, err := roleSeatsForFanOut(ctx, c.Participants, in.Step.ID, taskID, in.State.WorkflowID, cfg.Role)
	if err != nil {
		return ActionResult{}, fmt.Errorf("queue_run_for_each_participant list participants: %w", err)
	}
	reason := queueRunForEachParticipantReason(in)
	for _, p := range seats {
		req := QueueRunRequest{
			AgentProfileID: p.AgentProfileID,
			TaskID:         taskID,
			WorkflowStepID: in.Step.ID,
			Reason:         reason,
			IdempotencyKey: idempotencyKey(in, p.AgentProfileID, taskID),
			Payload:        queueRunPayload(in, cfg.Payload, taskID),
		}
		if _, err := c.Adapter.QueueRun(ctx, req); err != nil {
			return ActionResult{}, fmt.Errorf("queue_run for participant %s: %w", p.ID, err)
		}
	}
	return ActionResult{}, nil
}

func queueRunForEachParticipantReason(in ActionInput) string {
	if in.Action.QueueRunForEachParticipant != nil && in.Action.QueueRunForEachParticipant.Reason != "" {
		return in.Action.QueueRunForEachParticipant.Reason
	}
	return string(in.Trigger)
}

// Outcome labels for the office_review_seat_ensure_total counter
// (REQ-OFFICE-REVIEW-SEATS-004.1, AC-004.6, AC-004.9). SeatOutcomeNoCandidate
// is declared for the counter's documented enum but is currently unreachable:
// the casting algorithm folds "no eligible candidates" and "runner does not
// resolve" into a single Unfillable result, which always maps to
// SeatOutcomeNoRunner.
const (
	SeatOutcomeSeated          = "seated"
	SeatOutcomeAlreadySeated   = "already_seated"
	SeatOutcomeNoCandidate     = "no_candidate"
	SeatOutcomeNoRunner        = "no_runner"
	SeatOutcomeError           = "error"
	SeatOutcomeMalformedConfig = "malformed_config"
)

// SeatRoleLabelInvalid is the fixed role label used on the
// office_review_seat_ensure_total counter when the declared role is missing,
// empty or unrecognized (AC-004.11) — the arbitrary operator-supplied string
// appears only as a typed field on the accompanying warning record, never as
// a counter label.
const SeatRoleLabelInvalid = "invalid"

// EnsureParticipantSeatCallback executes the ensure_participant_seat action:
// it guarantees a decision-required seat exists for the configured role
// somewhere in the task's workflow before the fan-out that follows it in the
// compiled step queues a run for that role (REQ-OFFICE-REVIEW-SEATS-001;
// Control flow steps 3-4 of
// docs/specs/office/system-design/review-participant-seats-01.md). Writer's
// HasRoleSeatForTaskWorkflow is workflow-scoped, not step-scoped, so a seat
// recorded against an earlier step in the same workflow already satisfies
// this and Caster is never invoked (AC-001.5, AC-003.5). Logger is nil-safe
// (AC-24 convention): a nil Logger skips warning emission but the counters in
// REQ-OFFICE-REVIEW-SEATS-004 are still recorded unconditionally.
type EnsureParticipantSeatCallback struct {
	Writer ParticipantSeatWriter
	Caster ParticipantSeatCaster
	Logger *logger.Logger
}

// Execute satisfies ActionCallback.
func (c EnsureParticipantSeatCallback) Execute(ctx context.Context, in ActionInput) (ActionResult, error) {
	taskID := in.State.TaskID
	stepID := in.Step.ID

	var declaredRole string
	if in.Action.EnsureParticipantSeat != nil {
		declaredRole = strings.TrimSpace(in.Action.EnsureParticipantSeat.Role)
	}
	if declaredRole == "" || !ValidParticipantRole(declaredRole) {
		c.recordSeatEnsureMalformedRole(taskID, stepID, declaredRole)
		return ActionResult{}, fmt.Errorf("%w: %q", ErrMalformedParticipantRole, declaredRole)
	}
	role := declaredRole

	if c.Writer == nil {
		err := fmt.Errorf("%w: ensure_participant_seat requires ParticipantSeatWriter", ErrActionNotYetWired)
		c.recordSeatEnsureError(taskID, stepID, role, err)
		return ActionResult{}, err
	}
	workflowID := in.State.WorkflowID
	seated, err := c.Writer.HasRoleSeatForTaskWorkflow(ctx, workflowID, taskID, role)
	if err != nil {
		c.recordSeatEnsureError(taskID, stepID, role, err)
		return ActionResult{}, fmt.Errorf("ensure_participant_seat check existing seat: %w", err)
	}
	if seated {
		quorummetrics.RecordSeatEnsureOutcome(role, SeatOutcomeAlreadySeated)
		return ActionResult{}, nil
	}
	if c.Caster == nil {
		err := fmt.Errorf("%w: ensure_participant_seat requires ParticipantSeatCaster", ErrActionNotYetWired)
		c.recordSeatEnsureError(taskID, stepID, role, err)
		return ActionResult{}, err
	}
	cast, err := c.Caster.CastParticipantSeat(ctx, taskID, stepID, role)
	if err != nil {
		c.recordSeatEnsureError(taskID, stepID, role, err)
		return ActionResult{}, fmt.Errorf("ensure_participant_seat cast: %w", err)
	}
	if cast.Unfillable {
		c.recordSeatEnsureUnfillable(cast.WorkspaceID, taskID, stepID, role)
		return ActionResult{}, fmt.Errorf("%w: role %q", ErrParticipantSeatUnfillable, role)
	}
	if _, err := c.Writer.EnsureRoleSeat(ctx, workflowID, stepID, taskID, role, cast.AgentProfileID); err != nil {
		c.recordSeatEnsureError(taskID, stepID, role, err)
		return ActionResult{}, fmt.Errorf("ensure_participant_seat write seat: %w", err)
	}
	quorummetrics.RecordSeatEnsureOutcome(role, SeatOutcomeSeated)
	quorummetrics.RecordSeatProvenance(role, string(cast.Provenance), cast.SelfReview)
	return ActionResult{}, nil
}

// recordSeatEnsureMalformedRole emits AC-001.11/004.1/004.11's counter and
// warning record for a declared role that is missing, empty or unrecognized.
// The counter uses the fixed SeatRoleLabelInvalid label; declaredRole (which
// may be an arbitrary operator-supplied string) appears only as a typed field
// on the warning record, never as a counter label (AC-004.6).
func (c EnsureParticipantSeatCallback) recordSeatEnsureMalformedRole(taskID, stepID, declaredRole string) {
	quorummetrics.RecordSeatEnsureOutcome(SeatRoleLabelInvalid, SeatOutcomeMalformedConfig)
	if c.Logger == nil {
		return
	}
	c.Logger.Warn("ensure_participant_seat: declared role malformed",
		zap.String("task_id", taskID),
		zap.String("step_id", stepID),
		zap.String("declared_role", declaredRole),
	)
}

// recordSeatEnsureUnfillable emits AC-004.1's counter and warning record for
// a role whose casting resolution produced no agent to seat.
func (c EnsureParticipantSeatCallback) recordSeatEnsureUnfillable(workspaceID, taskID, stepID, role string) {
	quorummetrics.RecordSeatEnsureOutcome(role, SeatOutcomeNoRunner)
	if c.Logger == nil {
		return
	}
	c.Logger.Warn("ensure_participant_seat: role unfillable",
		zap.String("workspace_id", workspaceID),
		zap.String("task_id", taskID),
		zap.String("step_id", stepID),
		zap.String("role", role),
	)
}

// recordSeatEnsureError emits AC-004.9's counter and warning record for a
// genuine failure — as opposed to a confirmed empty result — while ensuring
// role's seat. Its outcome value is distinct from the empty-result outcomes
// above, and its warning record carries the underlying error.
func (c EnsureParticipantSeatCallback) recordSeatEnsureError(taskID, stepID, role string, err error) {
	quorummetrics.RecordSeatEnsureOutcome(role, SeatOutcomeError)
	if c.Logger == nil {
		return
	}
	c.Logger.Warn("ensure_participant_seat: error",
		zap.String("task_id", taskID),
		zap.String("step_id", stepID),
		zap.String("role", role),
		zap.Error(err),
	)
}

// Compile-time interface assertions.
var (
	_ ActionCallback = QueueRunCallback{}
	_ ActionCallback = ClearDecisionsCallback{}
	_ ActionCallback = QueueRunForEachParticipantCallback{}
	_ ActionCallback = EnsureParticipantSeatCallback{}
)
