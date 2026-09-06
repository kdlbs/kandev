package models

import "time"

// LaunchLedgerEntry is one durable, append-only record of a launch (a
// runs.status transition to 'claimed'), per REQ-OFFICE-LAUNCH-SAFETY-002.
// runs.claimed_at is cleared on retry/recovery, so the launch budgets of
// REQ-OFFICE-LAUNCH-SAFETY-005 count these rows instead.
type LaunchLedgerEntry struct {
	ID          string    `json:"id" db:"id"`
	RunID       string    `json:"run_id" db:"run_id"`
	WorkspaceID string    `json:"workspace_id" db:"workspace_id"`
	CausationID string    `json:"causation_id" db:"causation_id"`
	RoutineID   string    `json:"routine_id" db:"routine_id"`
	HumanRooted bool      `json:"human_rooted" db:"human_rooted"`
	ClaimedAt   time.Time `json:"claimed_at" db:"claimed_at"`
}

// GateFailureState tracks consecutive fail-closed evaluations of one gate
// for one workspace (AC-OFFICE-BACKPRESSURE-003.8), so escalation survives
// a process restart. Reset by exactly one successful evaluation of the
// same gate (AC-OFFICE-BACKPRESSURE-003.10's "successfully evaluated"
// meaning readable, not permissive) and by nothing else.
type GateFailureState struct {
	WorkspaceID         string     `json:"workspace_id" db:"workspace_id"`
	Gate                string     `json:"gate" db:"gate"`
	ConsecutiveFailures int        `json:"consecutive_failures" db:"consecutive_failures"`
	LastEscalationAt    *time.Time `json:"last_escalation_at" db:"last_escalation_at"`
	UpdatedAt           time.Time  `json:"updated_at" db:"updated_at"`
}
