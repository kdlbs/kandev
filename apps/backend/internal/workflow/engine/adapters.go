package engine

import (
	"context"
	"time"
)

// RunQueueAdapter is the engine's contract with the runs queue. The Phase 2
// final integration emits QueueRun requests through this interface; Phase 3
// implements it against the runs table (renamed from office_runs).
//
// Implementations MUST be safe for concurrent use and SHOULD treat
// (IdempotencyKey) as a uniqueness key when set: if a request with the same
// idempotency key has already been queued the implementation returns
// QueueOutcomeDeduped (nil error) without enqueuing a duplicate.
type RunQueueAdapter interface {
	QueueRun(ctx context.Context, req QueueRunRequest) (QueueOutcome, error)
}

// QueueOutcome reports what QueueRun actually did with a request, so a
// caller that needs to log or act on the result — not just detect an error —
// doesn't have to infer it from side effects. A duplicated declaration lives
// in internal/runs/service (see that package's RunQueueAdapter doc); both
// MUST match.
type QueueOutcome string

const (
	// QueueOutcomeQueued means a new runs row was inserted.
	QueueOutcomeQueued QueueOutcome = "queued"
	// QueueOutcomeDeduped means an existing row with the same
	// IdempotencyKey already exists within the dedupe window, so nothing
	// was inserted.
	QueueOutcomeDeduped QueueOutcome = "deduped"
	// QueueOutcomeCoalesced means the request was merged into an existing
	// queued row for the same agent + reason within the coalescing
	// window, so nothing new was inserted.
	QueueOutcomeCoalesced QueueOutcome = "coalesced"
)

// QueueRunRequest is the typed payload the engine hands to RunQueueAdapter.
//
// AgentProfileID is always populated — the engine resolves the action's
// Target string into a concrete agent profile id before invoking the
// adapter. TaskID is similarly resolved (defaulting to the trigger's
// task id when the action specifies "this" or leaves it blank).
type QueueRunRequest struct {
	AgentProfileID string
	TaskID         string
	WorkflowStepID string
	Reason         string
	IdempotencyKey string
	Payload        map[string]any
}

// ParticipantInfo is a lightweight projection of a workflow_step_participants
// row — enough for the engine's resolver/quorum logic without coupling the
// engine package to the workflow models package.
//
// TaskID is "" for template-level rows and non-empty for per-task overrides.
type ParticipantInfo struct {
	ID               string
	StepID           string
	TaskID           string
	Role             string
	AgentProfileID   string
	DecisionRequired bool
	Position         int
}

// DecisionInfo is a lightweight projection of a workflow_step_decisions row.
//
// DeciderType, DeciderID, Role and Comment carry the decider's identity and
// human-facing reason. Role is the role the decision was recorded under
// (which may differ from a guard's role — AC-42/58). Comment is the
// human-facing reason column; Note is a separate, unrelated field no Office
// surface reads.
//
// ID and DecidedAt are stamped by Engine.RecordParticipantDecision (AC-66,
// AC-57b-i) before the write — DecisionStore.RecordStepDecision receives
// DecisionInfo by value and never generates or echoes back either field, so
// a caller cannot read them off the row after the call.
type DecisionInfo struct {
	ID            string
	TaskID        string
	StepID        string
	ParticipantID string
	Decision      string
	Note          string
	DecidedAt     time.Time

	DeciderType string
	DeciderID   string
	Role        string
	Comment     string
}

// ParticipantStore reads the workflow_step_participants table for an engine
// step + task. Wave 8 of the task-model-unification ADR introduced
// dual-scoped participants: rows with task_id=” apply to every task at the
// step (template-level), rows with task_id != ” apply only to that task
// (per-task override). Per-task rows take precedence on (role, agent) ties.
//
// The taskID argument is the trigger's task; implementations MUST merge
// template-level rows with per-task rows for that task and return the
// resolved set. Returning nil/empty list is valid and signals a
// single-agent step for that task.
//
// ListTaskParticipants returns every per-task row (task_id = taskID)
// regardless of step_id — the AC-49 port that makes AC-18's cross-step
// counting expressible. Template rows are never returned by this method;
// callers combine it with ListStepParticipants(stepID, "") to gather
// template rows scoped to one step (AC-50 step 1). An empty result is
// valid, not an error.
type ParticipantStore interface {
	ListStepParticipants(ctx context.Context, stepID, taskID string) ([]ParticipantInfo, error)
	ListTaskParticipants(ctx context.Context, taskID string) ([]ParticipantInfo, error)
}

// WorkflowScopedParticipantStore is an optional refinement of
// ParticipantStore. It limits per-task overrides to the task's active
// workflow, so rows left by a previous workflow cannot contribute to a new
// workflow's quorum. The base interface remains small for plugins and test
// doubles that do not have workflow-aware storage.
type WorkflowScopedParticipantStore interface {
	ListTaskParticipantsForWorkflow(ctx context.Context, taskID, workflowID string) ([]ParticipantInfo, error)
}

// ParticipantSeatWriter is the engine's contract for guaranteeing a
// decision-required seat exists for a role somewhere in the task's workflow
// (REQ-OFFICE-REVIEW-SEATS-001). Both methods are workflow+role scoped, not
// step-scoped: a seat recorded against an earlier step in the same workflow
// still counts, so re-entry after a rejection round never seats the role
// twice (AC-001.5, AC-003.5).
//
// EnsureParticipantSeatCallback uses HasRoleSeatForTaskWorkflow as a cheap
// existence peek before invoking the caster (skipping it entirely when a
// seat already exists), and EnsureRoleSeat as the durable, transactional,
// concurrency-safe write once the caster has resolved an agent to seat.
// EnsureRoleSeat is itself the callback's idempotence mechanism: repeated
// calls for the same (workflowID, taskID, role) converge on the one row a
// single-transaction check-then-insert produced, so no separate durable
// idempotency marker is needed.
type ParticipantSeatWriter interface {
	HasRoleSeatForTaskWorkflow(ctx context.Context, workflowID, taskID, role string) (bool, error)
	EnsureRoleSeat(ctx context.Context, workflowID, stepID, taskID, role, agentProfileID string) (ParticipantInfo, error)
}

// SeatProvenance records why a particular agent was selected to fill a
// role's seat, for the AC-004 observability counters. It is not persisted
// on the participant row itself.
type SeatProvenance string

const (
	// SeatProvenanceEligiblePool means the caster selected an agent from the
	// role's normal eligible-agent pool (REQ-002 steps 3-4).
	SeatProvenanceEligiblePool SeatProvenance = "eligible_pool"
	// SeatProvenanceRunnerFallback means the eligible pool was empty and the
	// caster fell back to seating the task's runner (REQ-002 step 2).
	SeatProvenanceRunnerFallback SeatProvenance = "runner_fallback"
)

// ParticipantSeatCastResult is the typed outcome of a casting resolution
// attempt. AgentProfileID and Provenance are meaningless when Unfillable is
// true — REQ-002 step 2's no-runner branch, meaning no agent could be
// resolved for the role at all. WorkspaceID is populated on every result,
// including Unfillable ones, so the AC-OFFICE-REVIEW-SEATS-004.1 warning
// record can identify the workspace without a second lookup.
//
// SelfReview is true both when the chosen agent is the task's runner and
// when it already holds another seat on this task (the office seat
// caster's best-effort cross-step exclusion, AC-OFFICE-REVIEW-SEATS-002.3,
// ran out of alternatives). It is a counter label on RecordSeatProvenance,
// not a persisted or user-visible field.
type ParticipantSeatCastResult struct {
	AgentProfileID string
	WorkspaceID    string
	Provenance     SeatProvenance
	SelfReview     bool
	Unfillable     bool
}

// ParticipantSeatCaster resolves which agent should fill a role's seat when
// none exists yet, per REQ-002's five-step deterministic algorithm. The
// office package implements this against role-specific workspace agent pools;
// the engine treats the result as opaque.
type ParticipantSeatCaster interface {
	// stepID is the immutable workflow step that the task entered. Callers
	// must pass this value instead of asking the adapter to re-read mutable
	// task state after the transition commits. workflowID scopes any
	// cross-step exclusion read the caster performs to the task's current
	// workflow, so participant rows left over from a workflow the task has
	// since switched away from (switch_workflow durably keeps them; see
	// ListParticipantsForTaskWorkflow's doc comment) never count as already
	// seated for the new workflow's casting decision.
	CastParticipantSeat(ctx context.Context, workflowID, taskID, stepID, role string) (ParticipantSeatCastResult, error)
}

// AgentProfileResolver answers whether an agent profile id still resolves to
// a live (non-deleted) agent profile. The quorum guard uses it to drop a
// required seat whose agent was deleted after the seat was cast, rather than
// waiting forever on an agent that can never decide
// (REQ-OFFICE-REVIEW-SEATS-004.3). Nil-safe like every other optional Engine
// dependency: when unwired, every seat is kept exactly as before this
// capability existed.
type AgentProfileResolver interface {
	AgentProfileExists(ctx context.Context, agentProfileID string) (bool, error)
}

// DecisionStore reads and writes workflow_step_decisions rows. The engine
// uses it from the wait_for_quorum guard, the clear_decisions action, and
// Engine.RecordParticipantDecision.
type DecisionStore interface {
	ListStepDecisions(ctx context.Context, taskID, stepID string) ([]DecisionInfo, error)
	RecordStepDecision(ctx context.Context, d DecisionInfo) error
	ClearStepDecisions(ctx context.Context, taskID, stepID string) (int64, error)
}

// CEOAgentResolver resolves the workspace's CEO agent profile for the
// "workspace.ceo_agent" QueueRun target. Implementations look the workspace
// up via the trigger's task. Phase 2 final exposes the contract; office
// integration provides the implementation.
type CEOAgentResolver interface {
	ResolveCEOAgentProfileID(ctx context.Context, taskID string) (string, error)
}

// ChildTaskSpec is the typed payload TaskCreator receives. The engine
// resolves blank fields to defaults (parent task workflow / first runnable
// step / inherited assignee) inside the adapter — keeping the engine
// package free of model imports.
type ChildTaskSpec struct {
	Title          string
	Description    string
	WorkflowID     string
	StepID         string
	AgentProfileID string
}

// TaskCreator is the engine's contract with whoever knows how to create a
// task with a parent. Defined here so CreateChildTaskCallback can stay in
// the engine package; the office side implements the interface against
// the task service.
//
// Implementations MUST:
//
//  1. Set parent_id to parentTaskID on the new task.
//  2. Persist the new task with the supplied workflow + step + assignee.
//  3. Return the new task id (non-empty) or a non-nil error.
//
// The engine does not call CreateChildTask in EvaluateOnly mode — it
// always commits.
type TaskCreator interface {
	CreateChildTask(ctx context.Context, parentTaskID string, spec ChildTaskSpec) (taskID string, err error)
}

// WorkflowSwitcher is the engine's contract for in-place workflow swap.
// Implementations mutate tasks.workflow_id and tasks.workflow_step_id and
// return the resolved step id (defaulting blank stepID to the workflow's
// first runnable step).
//
// The engine drives on_exit on the old step before the swap and on_enter
// on the new step after — the implementation is responsible only for the
// row update.
type WorkflowSwitcher interface {
	SwitchTaskWorkflow(ctx context.Context, taskID, newWorkflowID, newStepID string) (resolvedStepID string, err error)
}
