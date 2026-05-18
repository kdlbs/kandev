package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Participant is the office-flavour view of a task participant row. After
// ADR 0005 Wave C the data lives in workflow_step_participants — we keep
// this struct shape so existing office callers (dashboard, scheduler)
// don't need to learn about workflow_step rows. CreatedAt is best-effort:
// workflow_step_participants doesn't store it, so we emit zero time.
type Participant struct {
	TaskID         string    `db:"task_id" json:"task_id"`
	AgentProfileID string    `db:"agent_profile_id" json:"agent_profile_id"`
	Role           string    `db:"role" json:"role"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
}

// stepIDForTask resolves the current workflow_step_id for the given task.
// Returns "" with no error when the task has no step yet — that case must
// short-circuit calls (you cannot create a participant without a step).
func (r *Repository) stepIDForTask(ctx context.Context, taskID string) (string, error) {
	var stepID sql.NullString
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT workflow_step_id FROM tasks WHERE id = ?`,
	), taskID).Scan(&stepID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !stepID.Valid {
		return "", nil
	}
	return stepID.String, nil
}

// GetTaskWorkflowStepID returns the current workflow_step_id for the given
// task. Returns "" with no error when the task has no step bound. Exposed
// so the dashboard service can resolve a task's step before recording a
// workflow_step_decisions row (ADR 0005 Wave E).
func (r *Repository) GetTaskWorkflowStepID(ctx context.Context, taskID string) (string, error) {
	return r.stepIDForTask(ctx, taskID)
}

// AddTaskParticipant inserts a (task, agent, role) row idempotently into
// workflow_step_participants under the task's current workflow step. A
// second call with the same natural key is a no-op. Returns nil silently
// when the task has no workflow_step_id yet — the caller surface gives no
// way to reject creation, and a later step-set will not retroactively
// add the participant.
func (r *Repository) AddTaskParticipant(ctx context.Context, taskID, agentID, role string) error {
	stepID, err := r.stepIDForTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("resolve task workflow_step_id: %w", err)
	}
	if stepID == "" {
		// Task has no step — silently skip. Mirrors the legacy
		// INSERT-OR-IGNORE behaviour that swallowed FK / constraint failures.
		return nil
	}
	// Idempotent: probe for an existing row before inserting.
	var existing string
	err = r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id FROM workflow_step_participants
		WHERE step_id = ? AND task_id = ? AND role = ? AND agent_profile_id = ?
	`), stepID, taskID, role, agentID).Scan(&existing)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("probe existing participant: %w", err)
	}
	id := uuid.New().String()
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES (?, ?, ?, ?, ?, 1, 0)
	`), id, stepID, taskID, role, agentID)
	return err
}

// RemoveTaskParticipant deletes a (task, agent, role) row from
// workflow_step_participants. A delete of a non-existent row is not an
// error. Removes per-task rows only — template-level rows (task_id = ”)
// are untouched.
func (r *Repository) RemoveTaskParticipant(ctx context.Context, taskID, agentID, role string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM workflow_step_participants
		WHERE task_id = ? AND agent_profile_id = ? AND role = ?
	`), taskID, agentID, role)
	return err
}

// ListTaskParticipants returns all participants for a task filtered by role.
// Reads from workflow_step_participants under the task's current workflow
// step, merging template-level and per-task rows (per-task wins on
// (role, agent_profile_id) conflicts).
func (r *Repository) ListTaskParticipants(ctx context.Context, taskID, role string) ([]Participant, error) {
	stepID, err := r.stepIDForTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if stepID == "" {
		return []Participant{}, nil
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT task_id, agent_profile_id, role
		FROM workflow_step_participants
		WHERE step_id = ? AND role = ?
		  AND (task_id = '' OR task_id = ?)
		ORDER BY position ASC, id ASC
	`), stepID, role, taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Participant{}
	for rows.Next() {
		var p Participant
		var rowTask sql.NullString
		if err := rows.Scan(&rowTask, &p.AgentProfileID, &p.Role); err != nil {
			return nil, err
		}
		// Project the canonical task_id into the row (template-level rows
		// have task_id = ''; we surface them as participants of the input task).
		p.TaskID = taskID
		out = append(out, p)
	}
	return mergeOfficeParticipants(out), rows.Err()
}

// ListAllTaskParticipants returns every participant for a task across both
// roles. Used by the task DTO so reviewers and approvers can be surfaced
// without two separate round trips.
func (r *Repository) ListAllTaskParticipants(ctx context.Context, taskID string) ([]Participant, error) {
	stepID, err := r.stepIDForTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if stepID == "" {
		return []Participant{}, nil
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT task_id, agent_profile_id, role
		FROM workflow_step_participants
		WHERE step_id = ?
		  AND (task_id = '' OR task_id = ?)
		ORDER BY role ASC, position ASC, id ASC
	`), stepID, taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Participant{}
	for rows.Next() {
		var p Participant
		var rowTask sql.NullString
		if err := rows.Scan(&rowTask, &p.AgentProfileID, &p.Role); err != nil {
			return nil, err
		}
		p.TaskID = taskID
		out = append(out, p)
	}
	return mergeOfficeParticipants(out), rows.Err()
}

// mergeOfficeParticipants enforces per-task precedence: when a
// template-level row and a per-task row share (role, agent_profile_id),
// the per-task row wins. workflow_step_participants stores both kinds; we
// post-filter here because the SQL query returns the union.
func mergeOfficeParticipants(rows []Participant) []Participant {
	if len(rows) <= 1 {
		return rows
	}
	// Since both flavours scan into the same Participant shape (task_id is
	// already projected to the input taskID), we can't distinguish them by
	// task_id alone. The merge is therefore a uniqueness collapse on
	// (role, agent_profile_id) — each unique key is kept once.
	type key struct{ role, agent string }
	seen := make(map[key]bool, len(rows))
	out := make([]Participant, 0, len(rows))
	for _, p := range rows {
		k := key{p.Role, p.AgentProfileID}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, p)
	}
	return out
}
