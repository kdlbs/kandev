package models

import "time"

// DeferredAssignment is a task assignment that was blocked by an active
// workspace pause (docs/specs/office/requirements/paused-assignment-replay.md,
// docs/specs/office/system-design/workspace-kill-switch-02.md "Deferred
// assignments"). One row exists per task, keyed on TaskID: a later deferred
// occurrence for the same task (a reassignment during the same pause, or a
// fresh pause after an earlier one resolved) overwrites the row rather than
// adding a second one, so replay always acts on the latest assigning
// occurrence. Pending means ResolvedAt is nil; Outcome ("replayed" |
// "dropped") is set together with ResolvedAt.
type DeferredAssignment struct {
	TaskID               string     `json:"task_id" db:"task_id"`
	WorkspaceID          string     `json:"workspace_id" db:"workspace_id"`
	AgentProfileID       string     `json:"agent_profile_id" db:"agent_profile_id"`
	AssignmentGeneration int64      `json:"assignment_generation" db:"assignment_generation"`
	PauseID              string     `json:"pause_id" db:"pause_id"`
	ActorType            string     `json:"actor_type,omitempty" db:"actor_type"`
	ActorID              string     `json:"actor_id,omitempty" db:"actor_id"`
	CreatedAt            time.Time  `json:"created_at" db:"created_at"`
	ResolvedAt           *time.Time `json:"resolved_at,omitempty" db:"resolved_at"`
	Outcome              string     `json:"outcome,omitempty" db:"outcome"`
}
