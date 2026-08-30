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
