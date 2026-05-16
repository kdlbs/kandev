package models

import "time"

// Phase 2 (task model unification, ADR-0004) introduces multi-agent
// participation and decision tracking on workflow steps. The schemas and
// repository methods land here in the preparation slice; callbacks that
// write to them are wired in a later slice.

// ParticipantRole identifies the part a workflow_step_participants row plays
// in a multi-agent step.
type ParticipantRole string

const (
	ParticipantRoleReviewer     ParticipantRole = "reviewer"
	ParticipantRoleApprover     ParticipantRole = "approver"
	ParticipantRoleWatcher      ParticipantRole = "watcher"
	ParticipantRoleCollaborator ParticipantRole = "collaborator"
	// ParticipantRoleRunner marks the agent currently responsible for
	// driving the task at this step. ADR 0005 Wave D introduced this
	// role as the storage location for per-task assignee overrides:
	// reassigning a task without editing the workflow inserts a
	// `runner` participant row for (step, task) instead of writing to
	// the legacy `tasks.assignee_agent_profile_id` column. The
	// ResolveCurrentRunner helper resolves a task's effective runner by
	// preferring runner participants over the step's primary agent.
	ParticipantRoleRunner ParticipantRole = "runner"
)

// StageType classifies a workflow_step's UX role. A semantic hint for the
// frontend; the engine itself does not branch on it.
type StageType string

const (
	StageTypeWork     StageType = "work"
	StageTypeReview   StageType = "review"
	StageTypeApproval StageType = "approval"
	StageTypeCustom   StageType = "custom"
)

// WorkflowStyle classifies a workflow's UX presentation. Read by the frontend
// only — backend code MUST NOT branch on this value.
type WorkflowStyle string

const (
	WorkflowStyleKanban WorkflowStyle = "kanban"
	WorkflowStyleOffice WorkflowStyle = "office"
	WorkflowStyleCustom WorkflowStyle = "custom"
)

// WorkflowStepParticipant represents an agent profile bound to a workflow step
// in a multi-agent capacity. An empty list of participants on a step keeps
// today's single-agent kanban behaviour.
//
// TaskID is a Wave 8 (ADR-0004) extension: when empty the row is a
// template-level participant that applies to every task at this step;
// when non-empty the row is a per-task override that only applies to
// that task. Per-task rows take precedence over template-level rows
// when both define the same (role, agent_profile_id) pair.
type WorkflowStepParticipant struct {
	ID               string          `json:"id"`
	StepID           string          `json:"step_id"`
	TaskID           string          `json:"task_id,omitempty"`
	Role             ParticipantRole `json:"role"`
	AgentProfileID   string          `json:"agent_profile_id"`
	DecisionRequired bool            `json:"decision_required"`
	Position         int             `json:"position"`
}

// WorkflowStepDecision records a participant's verdict on a (task, step) pair.
// The decision string is open-ended (e.g. 'approved', 'rejected', 'comment',
// 'no_decision') so workflows can define their own quorum semantics on top.
//
// SupersededAt marks a decision as no longer current. ADR 0005 Wave D folded
// the legacy office_task_approval_decisions table into this model; the office
// rework flow uses Repository.SupersedeStepDecisions to mark prior rows
// superseded so a new review round starts from a clean slate without losing
// the audit trail. ListStepDecisions returns rows regardless of supersede
// state — quorum guards and active-decision listings filter at the call site.
type WorkflowStepDecision struct {
	ID            string     `json:"id"`
	TaskID        string     `json:"task_id"`
	StepID        string     `json:"step_id"`
	ParticipantID string     `json:"participant_id"`
	Decision      string     `json:"decision"`
	Note          string     `json:"note,omitempty"`
	DecidedAt     time.Time  `json:"decided_at"`
	SupersededAt  *time.Time `json:"superseded_at,omitempty"`

	// DeciderType / DeciderID / Role are denormalised projection fields
	// captured from the participant when the decision is recorded. Office
	// callers (decisions service, run-detail timeline, activity log) read
	// these directly off the row instead of joining back to the
	// participants table on every read. Empty values indicate a row
	// recorded before ADR 0005 Wave D or by an engine-side caller that
	// supplied only the participant id.
	DeciderType string `json:"decider_type,omitempty"`
	DeciderID   string `json:"decider_id,omitempty"`
	Role        string `json:"role,omitempty"`
	Comment     string `json:"comment,omitempty"`
}
