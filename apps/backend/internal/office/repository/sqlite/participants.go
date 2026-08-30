package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/workflow/models"
)

// Participant is the office-flavour view of a task participant row. After
// ADR 0005 Wave C the data lives in workflow_step_participants — we keep
// this struct shape so existing office callers (dashboard, scheduler)
// don't need to learn about workflow_step rows. CreatedAt is best-effort:
// workflow_step_participants doesn't store it, so we emit zero time.
type Participant struct {
	TaskID           string    `db:"task_id" json:"task_id"`
	AgentProfileID   string    `db:"agent_profile_id" json:"agent_profile_id"`
	Role             string    `db:"role" json:"role"`
	DecisionRequired bool      `db:"decision_required" json:"decision_required"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
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

// GetWorkflowStepStageType returns the persisted stage type for a workflow
// step. It returns an empty string when the step does not exist so callers
// can apply compatibility fallbacks for older run payloads.
func (r *Repository) GetWorkflowStepStageType(ctx context.Context, stepID string) (string, error) {
	if stepID == "" {
		return "", nil
	}
	var stageType sql.NullString
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT stage_type FROM workflow_steps WHERE id = ?`,
	), stepID).Scan(&stageType)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !stageType.Valid {
		return "", nil
	}
	return stageType.String, nil
}

// AddTaskParticipant inserts a (task, agent, role) row idempotently into
// workflow_step_participants under the task's current workflow step. A
// second call with the same natural key is a no-op. Returns nil silently
// when the task has no workflow_step_id yet — the caller surface gives no
// way to reject creation, and a later step-set will not retroactively
// add the participant.
//
// When the engine's on_enter ensure_participant_seat action has already
// auto-cast a fallback seat for this exact (step, task, role) — see
// EnsureRoleSeat in internal/workflow/repository — a manual registration
// claims that seat in place (reassigning its agent_profile_id and marking
// it "manual") instead of inserting a second seat for the same role. Two
// seats for one role silently doubles the decision count an all_approve
// quorum guard requires, which then never resolves from a single human
// decision. Claiming only ever applies when exactly one "auto" seat
// exists at this identity and it has no recorded decisions yet — a
// legitimate multi-reviewer registration (two humans each manually
// adding a distinct reviewer before either decides) never matches
// because neither of their seats carries provenance "auto".
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

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Idempotent: probe for an existing exact-match row before inserting.
	var existing string
	err = tx.QueryRowContext(ctx, tx.Rebind(`
		SELECT id FROM workflow_step_participants
		WHERE step_id = ? AND task_id = ? AND role = ? AND agent_profile_id = ?
	`), stepID, taskID, role, agentID).Scan(&existing)
	if err == nil {
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("probe existing participant: %w", err)
	}

	claimID, err := r.findClaimableAutoSeat(ctx, tx, stepID, taskID, role)
	if err != nil {
		return err
	}
	if claimID != "" {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE workflow_step_participants
			SET agent_profile_id = ?, provenance = ?
			WHERE id = ?
		`), agentID, string(models.ParticipantProvenanceManual), claimID); err != nil {
			return fmt.Errorf("claim auto-cast participant: %w", err)
		}
		return tx.Commit()
	}

	id := uuid.New().String()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, provenance)
		VALUES (?, ?, ?, ?, ?, 1, 0, ?)
	`), id, stepID, taskID, role, agentID, string(models.ParticipantProvenanceManual)); err != nil {
		return fmt.Errorf("insert participant: %w", err)
	}
	return tx.Commit()
}

// findClaimableAutoSeat returns the id of the sole "auto"-provenance seat
// at (stepID, taskID, role) that has zero recorded decisions, or "" when
// no such single, undecided auto seat exists. Runs inside AddTaskParticipant's
// transaction so the check-then-claim is atomic against a concurrent
// EnsureRoleSeat or second manual registration.
func (r *Repository) findClaimableAutoSeat(ctx context.Context, tx *sqlx.Tx, stepID, taskID, role string) (string, error) {
	rows, err := tx.QueryContext(ctx, tx.Rebind(`
		SELECT id FROM workflow_step_participants
		WHERE step_id = ? AND task_id = ? AND role = ? AND provenance = ?
	`), stepID, taskID, role, string(models.ParticipantProvenanceAuto))
	if err != nil {
		return "", fmt.Errorf("find claimable auto seat: %w", err)
	}
	var candidates []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return "", fmt.Errorf("scan claimable auto seat: %w", err)
		}
		candidates = append(candidates, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return "", err
	}
	_ = rows.Close()
	if len(candidates) != 1 {
		return "", nil
	}

	var decisionCount int
	if err := tx.QueryRowContext(ctx, tx.Rebind(`
		SELECT COUNT(*) FROM workflow_step_decisions WHERE participant_id = ?
	`), candidates[0]).Scan(&decisionCount); err != nil {
		return "", fmt.Errorf("check auto seat decisions: %w", err)
	}
	if decisionCount != 0 {
		return "", nil
	}
	return candidates[0], nil
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
		SELECT task_id, agent_profile_id, role, decision_required
		FROM workflow_step_participants
		WHERE step_id = ? AND role = ?
		  AND (task_id = '' OR task_id = ?)
		ORDER BY position ASC, agent_profile_id ASC, id ASC
	`), stepID, role, taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Participant{}
	for rows.Next() {
		var p Participant
		var rowTask sql.NullString
		var decisionRequired int
		if err := rows.Scan(
			&rowTask,
			&p.AgentProfileID,
			&p.Role,
			&decisionRequired,
		); err != nil {
			return nil, err
		}
		// Project the canonical task_id into the row (template-level rows
		// have task_id = ''; we surface them as participants of the input task).
		if rowTask.Valid {
			p.TaskID = rowTask.String
		}
		p.DecisionRequired = decisionRequired != 0
		out = append(out, p)
	}
	return projectOfficeParticipants(taskID, mergeOfficeParticipants(out)), rows.Err()
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
		SELECT task_id, agent_profile_id, role, decision_required
		FROM workflow_step_participants
		WHERE step_id = ?
		  AND (task_id = '' OR task_id = ?)
		ORDER BY role ASC, position ASC, agent_profile_id ASC, id ASC
	`), stepID, taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Participant{}
	for rows.Next() {
		var p Participant
		var rowTask sql.NullString
		var decisionRequired int
		if err := rows.Scan(
			&rowTask,
			&p.AgentProfileID,
			&p.Role,
			&decisionRequired,
		); err != nil {
			return nil, err
		}
		if rowTask.Valid {
			p.TaskID = rowTask.String
		}
		p.DecisionRequired = decisionRequired != 0
		out = append(out, p)
	}
	return projectOfficeParticipants(taskID, mergeOfficeParticipants(out)), rows.Err()
}

// mergeOfficeParticipants enforces per-task precedence: when a
// template-level row and a per-task row share (role, agent_profile_id),
// the per-task row wins. workflow_step_participants stores both kinds; we
// post-filter here because the SQL query returns the union.
func mergeOfficeParticipants(rows []Participant) []Participant {
	if len(rows) <= 1 {
		return rows
	}
	type key struct{ role, agent string }
	chosen := make(map[key]int, len(rows))
	out := make([]Participant, 0, len(rows))
	for _, p := range rows {
		k := key{p.Role, p.AgentProfileID}
		if idx, ok := chosen[k]; ok {
			if out[idx].TaskID == "" && p.TaskID != "" {
				out[idx] = p
			}
			continue
		}
		chosen[k] = len(out)
		out = append(out, p)
	}
	return out
}

func projectOfficeParticipants(taskID string, rows []Participant) []Participant {
	for i := range rows {
		rows[i].TaskID = taskID
	}
	return rows
}
