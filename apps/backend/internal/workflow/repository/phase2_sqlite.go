package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/workflow/models"
)

// initPhase2Schema creates the workflow_step_participants and
// workflow_step_decisions tables introduced in Phase 2 (ADR-0004). Both are
// created with `IF NOT EXISTS` so this migration is idempotent and safe to
// run on every backend startup.
//
// Wave 8 deviation from the original ADR: workflow_step_participants is
// dual-scoped — a row with task_id=” is a template-level participant that
// applies to every task at the step, while task_id != ” is a per-task
// override that only applies to that task. Per-task rows take precedence
// when both define the same (role, agent_profile_id) pair (the per-task row
// wins). The office dashboard uses per-task rows; workflow templates
// (currently none in production) would use template-level rows.
func (r *Repository) initPhase2Schema() error {
	participantsSchema := `
	CREATE TABLE IF NOT EXISTS workflow_step_participants (
		id TEXT PRIMARY KEY,
		step_id TEXT NOT NULL REFERENCES workflow_steps(id) ON DELETE CASCADE,
		task_id TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL CHECK (role IN ('reviewer','approver','watcher','collaborator','runner')),
		agent_profile_id TEXT NOT NULL,
		decision_required INTEGER NOT NULL DEFAULT 0,
		position INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00',
		provenance TEXT NOT NULL DEFAULT 'manual'
	);
	CREATE INDEX IF NOT EXISTS idx_workflow_step_participants_step ON workflow_step_participants(step_id);
	CREATE INDEX IF NOT EXISTS idx_workflow_step_participants_role ON workflow_step_participants(step_id, role);
	CREATE INDEX IF NOT EXISTS idx_workflow_step_participants_task ON workflow_step_participants(task_id) WHERE task_id != '';
	CREATE INDEX IF NOT EXISTS idx_workflow_step_participants_task_role ON workflow_step_participants(task_id, role) WHERE task_id != '';
	`
	if _, err := r.db.Exec(participantsSchema); err != nil {
		return fmt.Errorf("failed to create workflow_step_participants table: %w", err)
	}
	// created_at predates this column on any database that already had this
	// table; the ADD COLUMN default backfills existing rows to one constant
	// timestamp so ResolveCurrentRunner's tiebreak still resolves them
	// deterministically (see participantsNaturalKeyIndexName below).
	r.migrate.Apply("workflow_step_participants.created_at", `
		ALTER TABLE workflow_step_participants
		ADD COLUMN created_at TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00'
	`)
	// provenance predates this column on any database that already had this
	// table; the ADD COLUMN default backfills existing rows to "manual" so a
	// pre-existing seat is never treated as claimable (AddTaskParticipant
	// only claims a seat whose provenance is "auto").
	r.migrate.Apply("workflow_step_participants.provenance", `
		ALTER TABLE workflow_step_participants
		ADD COLUMN provenance TEXT NOT NULL DEFAULT 'manual'
	`)

	decisionsSchema := `
	CREATE TABLE IF NOT EXISTS workflow_step_decisions (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		step_id TEXT NOT NULL,
		participant_id TEXT NOT NULL,
		decision TEXT NOT NULL,
		note TEXT DEFAULT '',
		decided_at TIMESTAMP NOT NULL,
		superseded_at TIMESTAMP NULL,
		decider_type TEXT NOT NULL DEFAULT '',
		decider_id TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT '',
		comment TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_workflow_step_decisions_task_step ON workflow_step_decisions(task_id, step_id);
	CREATE INDEX IF NOT EXISTS idx_workflow_step_decisions_participant ON workflow_step_decisions(participant_id);
	CREATE INDEX IF NOT EXISTS idx_workflow_step_decisions_active
		ON workflow_step_decisions(task_id, role) WHERE superseded_at IS NULL;
	`
	// Created before the participant dedupe below: dedupeParticipantsBeforeUniqueIndex
	// remaps workflow_step_decisions.participant_id off a losing duplicate, which
	// needs this table to already exist (including on a fresh database's first
	// initPhase2Schema run).
	if _, err := r.db.Exec(decisionsSchema); err != nil {
		return fmt.Errorf("failed to create workflow_step_decisions table: %w", err)
	}

	// Must run before participantsNaturalKeyIndexName is created below: a
	// database that predates the unique index can already hold rows sharing
	// the same (step_id, task_id, role, agent_profile_id) identity, and
	// CREATE UNIQUE INDEX over them fails outright, aborting backend startup.
	if err := r.dedupeParticipantsBeforeUniqueIndex(); err != nil {
		return fmt.Errorf("failed to dedupe workflow_step_participants before unique index: %w", err)
	}
	if _, err := r.db.Exec(`
	CREATE UNIQUE INDEX IF NOT EXISTS ` + participantsNaturalKeyIndexName + `
		ON workflow_step_participants(step_id, task_id, role, agent_profile_id);
	`); err != nil {
		return fmt.Errorf("failed to create %s: %w", participantsNaturalKeyIndexName, err)
	}

	// Must run before decisionActiveDeciderIndexName is created below: an
	// install that hit the pre-fix AC-27/29 concurrent double-insert race
	// (commit 73226d29b) can already hold duplicate active rows for the same
	// (task_id, step_id, decider_id, role) identity, and CREATE UNIQUE INDEX
	// over them fails outright, aborting backend startup.
	if err := r.dedupeActiveStepDecisionsBeforeUniqueIndex(); err != nil {
		return fmt.Errorf("failed to dedupe workflow_step_decisions before unique index: %w", err)
	}

	if _, err := r.db.Exec(`
	CREATE UNIQUE INDEX IF NOT EXISTS ` + decisionActiveDeciderIndexName + `
		ON workflow_step_decisions(task_id, step_id, decider_id, role)
		WHERE superseded_at IS NULL AND decider_id != '' AND role != '';
	`); err != nil {
		return fmt.Errorf("failed to create %s: %w", decisionActiveDeciderIndexName, err)
	}

	return nil
}

// dedupeActiveStepDecisionsBeforeUniqueIndex supersedes every active
// workflow_step_decisions row except the newest per (task_id, step_id,
// decider_id, role) group, using the same last-row-wins ordering
// (decided_at DESC, id DESC) the engine already uses to evaluate quorum.
// Idempotent: a database with no duplicate active rows leaves this a no-op.
// Mirrors normalizeDuplicateStartSteps below.
func (r *Repository) dedupeActiveStepDecisionsBeforeUniqueIndex() error {
	_, err := r.db.Exec(`
		WITH ranked AS (
			SELECT
				id,
				ROW_NUMBER() OVER (
					PARTITION BY task_id, step_id, decider_id, role
					ORDER BY decided_at DESC, id DESC
				) AS decision_rank
			FROM workflow_step_decisions
			WHERE superseded_at IS NULL AND decider_id != '' AND role != ''
		)
		UPDATE workflow_step_decisions
		SET superseded_at = decided_at
		WHERE id IN (SELECT id FROM ranked WHERE decision_rank > 1)
	`)
	return err
}

// ----------------------------------------------------------------------------
// WorkflowStepParticipant CRUD
// ----------------------------------------------------------------------------

// participantsNaturalKeyIndexName is the unique index enforcing at most one
// participant row per (step_id, task_id, role, agent_profile_id) identity.
// EnsureRoleSeat relies on this to turn a lost check-then-insert race into a
// retryable constraint violation instead of a silent duplicate seat — see
// isParticipantsNaturalKeyViolation.
const participantsNaturalKeyIndexName = "idx_workflow_step_participants_natural_key"

// dedupeParticipantsBeforeUniqueIndex keeps one row per (step_id, task_id,
// role, agent_profile_id) group — the earliest by position, tiebroken by id
// for determinism — and deletes the rest. Before deleting, it remaps any
// workflow_step_decisions.participant_id pointing at a losing duplicate onto
// the surviving row: a decision already recorded against a duplicate would
// otherwise be left with a dangling participant_id after the delete, making
// mapDecisionsToSeats drop it and a completed review look undecided. A
// database with no duplicate rows leaves this a no-op. Mirrors
// dedupeActiveStepDecisionsBeforeUniqueIndex.
func (r *Repository) dedupeParticipantsBeforeUniqueIndex() error {
	if _, err := r.db.Exec(`
		WITH ranked AS (
			SELECT
				id,
				FIRST_VALUE(id) OVER (
					PARTITION BY step_id, task_id, role, agent_profile_id
					ORDER BY position ASC, id ASC
				) AS survivor_id
			FROM workflow_step_participants
		)
		UPDATE workflow_step_decisions
		SET participant_id = (
			SELECT survivor_id FROM ranked WHERE ranked.id = workflow_step_decisions.participant_id
		)
		WHERE participant_id IN (SELECT id FROM ranked WHERE ranked.id != ranked.survivor_id)
	`); err != nil {
		return fmt.Errorf("remap decisions off duplicate participants: %w", err)
	}

	_, err := r.db.Exec(`
		WITH ranked AS (
			SELECT
				id,
				ROW_NUMBER() OVER (
					PARTITION BY step_id, task_id, role, agent_profile_id
					ORDER BY position ASC, id ASC
				) AS participant_rank
			FROM workflow_step_participants
		)
		DELETE FROM workflow_step_participants
		WHERE id IN (SELECT id FROM ranked WHERE participant_rank > 1)
	`)
	return err
}

// UpsertStepParticipant inserts a new participant or updates an existing one
// by id. If id is empty a UUID is generated.
func (r *Repository) UpsertStepParticipant(ctx context.Context, p *models.WorkflowStepParticipant) error {
	if p == nil {
		return errors.New("participant must not be nil")
	}
	if p.StepID == "" {
		return errors.New("participant.step_id is required")
	}
	if !validParticipantRole(p.Role) {
		return fmt.Errorf("invalid participant role %q", p.Role)
	}
	if p.AgentProfileID == "" {
		return errors.New("participant.agent_profile_id is required")
	}
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workflow_step_participants (id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			step_id = excluded.step_id,
			task_id = excluded.task_id,
			role = excluded.role,
			agent_profile_id = excluded.agent_profile_id,
			decision_required = excluded.decision_required,
			position = excluded.position
	`), p.ID, p.StepID, p.TaskID, string(p.Role), p.AgentProfileID,
		dialect.BoolToInt(p.DecisionRequired), p.Position, p.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert step participant: %w", err)
	}
	return nil
}

// UpsertTaskParticipant inserts or updates a per-task participant row.
// task_id MUST be non-empty; for template-level rows use UpsertStepParticipant
// directly. Idempotent on (step_id, task_id, role, agent_profile_id): a second
// call with the same key is a no-op (it returns the existing row's ID).
func (r *Repository) UpsertTaskParticipant(
	ctx context.Context, stepID, taskID, role, agentProfileID string,
) (string, error) {
	if stepID == "" || taskID == "" || role == "" || agentProfileID == "" {
		return "", errors.New("step_id, task_id, role, and agent_profile_id are required")
	}
	if !validParticipantRole(models.ParticipantRole(role)) {
		return "", fmt.Errorf("invalid participant role %q", role)
	}
	// Look for existing row by natural key.
	var existing string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id FROM workflow_step_participants
		WHERE step_id = ? AND task_id = ? AND role = ? AND agent_profile_id = ?
	`), stepID, taskID, role, agentProfileID).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("lookup existing task participant: %w", err)
	}
	id := uuid.New().String()
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workflow_step_participants (id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at)
		VALUES (?, ?, ?, ?, ?, 1, 0, ?)
	`), id, stepID, taskID, role, agentProfileID, time.Now().UTC()); err != nil {
		return "", fmt.Errorf("insert task participant: %w", err)
	}
	return id, nil
}

// DeleteTaskParticipant removes a per-task participant row by natural key.
// A delete of a non-existent row is not an error.
func (r *Repository) DeleteTaskParticipant(
	ctx context.Context, stepID, taskID, role, agentProfileID string,
) error {
	if stepID == "" || taskID == "" || role == "" || agentProfileID == "" {
		return errors.New("step_id, task_id, role, and agent_profile_id are required")
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM workflow_step_participants
		WHERE step_id = ? AND task_id = ? AND role = ? AND agent_profile_id = ?
	`), stepID, taskID, role, agentProfileID); err != nil {
		return fmt.Errorf("delete task participant: %w", err)
	}
	return nil
}

// ListStepParticipantsForTask returns every participant configured for the
// given (step, task) pair. The result merges template-level rows
// (task_id = ”) with per-task overrides (task_id = task). When both define
// the same (role, agent_profile_id) the per-task row takes precedence.
//
// The merged list is ordered by (role, position, id) so consumers see a
// stable layout matching ListStepParticipants.
func (r *Repository) ListStepParticipantsForTask(
	ctx context.Context, stepID, taskID string,
) ([]*models.WorkflowStepParticipant, error) {
	if stepID == "" {
		return nil, errors.New("step_id is required")
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at
		FROM workflow_step_participants
		WHERE step_id = ? AND (task_id = '' OR task_id = ?)
		ORDER BY role ASC, position ASC, agent_profile_id ASC, id ASC
	`), stepID, taskID)
	if err != nil {
		return nil, fmt.Errorf("list step participants for task: %w", err)
	}
	defer func() { _ = rows.Close() }()

	all := make([]*models.WorkflowStepParticipant, 0)
	for rows.Next() {
		p := &models.WorkflowStepParticipant{}
		var role string
		var decisionRequired int
		if err := rows.Scan(&p.ID, &p.StepID, &p.TaskID, &role, &p.AgentProfileID, &decisionRequired, &p.Position, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan step participant: %w", err)
		}
		p.Role = models.ParticipantRole(role)
		p.DecisionRequired = decisionRequired == 1
		all = append(all, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return mergeParticipantRows(all), nil
}

// ListParticipantsForTaskAnyStep returns every per-task participant row for
// a task (task_id = taskID) regardless of step_id, unmerged with any
// template-level row. Used by the quorum engine's requiredSeats to build the
// AC-49/50 required slate, which combines this with template rows scoped to
// the evaluating step. An empty result is valid, not an error.
func (r *Repository) ListParticipantsForTaskAnyStep(
	ctx context.Context, taskID string,
) ([]*models.WorkflowStepParticipant, error) {
	if taskID == "" {
		return nil, errors.New("task_id is required")
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at
		FROM workflow_step_participants
		WHERE task_id = ?
		ORDER BY role ASC, position ASC, agent_profile_id ASC, id ASC
	`), taskID)
	if err != nil {
		return nil, fmt.Errorf("list participants for task: %w", err)
	}
	defer func() { _ = rows.Close() }()

	all := make([]*models.WorkflowStepParticipant, 0)
	for rows.Next() {
		p := &models.WorkflowStepParticipant{}
		var role string
		var decisionRequired int
		if err := rows.Scan(&p.ID, &p.StepID, &p.TaskID, &role, &p.AgentProfileID, &decisionRequired, &p.Position, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan step participant: %w", err)
		}
		p.Role = models.ParticipantRole(role)
		p.DecisionRequired = decisionRequired == 1
		all = append(all, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return all, nil
}

// ListParticipantsForTaskWorkflow returns every per-task participant row for
// a task whose step belongs to workflowID. Participant overrides remain
// durable when a task changes workflow, but they must not leak into quorum
// evaluation for the new workflow.
func (r *Repository) ListParticipantsForTaskWorkflow(
	ctx context.Context, taskID, workflowID string,
) ([]*models.WorkflowStepParticipant, error) {
	if taskID == "" {
		return nil, errors.New("task_id is required")
	}
	if workflowID == "" {
		return nil, errors.New("workflow_id is required")
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT p.id, p.step_id, p.task_id, p.role, p.agent_profile_id, p.decision_required, p.position, p.created_at
		FROM workflow_step_participants p
		JOIN workflow_steps ws ON ws.id = p.step_id
		WHERE p.task_id = ? AND ws.workflow_id = ?
		ORDER BY p.role ASC, p.position ASC, p.agent_profile_id ASC, p.id ASC
	`), taskID, workflowID)
	if err != nil {
		return nil, fmt.Errorf("list participants for task workflow: %w", err)
	}
	defer func() { _ = rows.Close() }()

	all := make([]*models.WorkflowStepParticipant, 0)
	for rows.Next() {
		p := &models.WorkflowStepParticipant{}
		var role string
		var decisionRequired int
		if err := rows.Scan(&p.ID, &p.StepID, &p.TaskID, &role, &p.AgentProfileID, &decisionRequired, &p.Position, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan step participant: %w", err)
		}
		p.Role = models.ParticipantRole(role)
		p.DecisionRequired = decisionRequired == 1
		all = append(all, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return all, nil
}

// mergeParticipantRows enforces the per-task-precedence rule. Given a set of
// rows for one step (mixed template-level and per-task), drop any
// template-level row whose (role, agent_profile_id) is also present in a
// per-task row.
func mergeParticipantRows(rows []*models.WorkflowStepParticipant) []*models.WorkflowStepParticipant {
	if len(rows) <= 1 {
		return rows
	}
	type key struct{ role, agent string }
	perTask := make(map[key]bool)
	for _, p := range rows {
		if p.TaskID != "" {
			perTask[key{string(p.Role), p.AgentProfileID}] = true
		}
	}
	if len(perTask) == 0 {
		return rows
	}
	out := make([]*models.WorkflowStepParticipant, 0, len(rows))
	for _, p := range rows {
		if p.TaskID == "" && perTask[key{string(p.Role), p.AgentProfileID}] {
			continue
		}
		out = append(out, p)
	}
	return out
}

// DeleteStepParticipant removes a participant by id. Returns nil even if no
// row matches; callers that need stricter semantics can read first.
func (r *Repository) DeleteStepParticipant(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("participant id is required")
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM workflow_step_participants WHERE id = ?`,
	), id); err != nil {
		return fmt.Errorf("delete step participant: %w", err)
	}
	return nil
}

// ListStepParticipants returns every template-level participant configured
// for the step, ordered by (role, position) so consumers see a stable layout.
//
// Note: this method only returns rows with task_id=” (template-level).
// To include per-task overrides for a specific task, use
// ListStepParticipantsForTask.
func (r *Repository) ListStepParticipants(ctx context.Context, stepID string) ([]*models.WorkflowStepParticipant, error) {
	if stepID == "" {
		return nil, errors.New("step_id is required")
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at
		FROM workflow_step_participants
		WHERE step_id = ? AND task_id = ''
		ORDER BY role ASC, position ASC, agent_profile_id ASC, id ASC
	`), stepID)
	if err != nil {
		return nil, fmt.Errorf("list step participants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []*models.WorkflowStepParticipant
	for rows.Next() {
		p := &models.WorkflowStepParticipant{}
		var role string
		var decisionRequired int
		if err := rows.Scan(&p.ID, &p.StepID, &p.TaskID, &role, &p.AgentProfileID, &decisionRequired, &p.Position, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan step participant: %w", err)
		}
		p.Role = models.ParticipantRole(role)
		p.DecisionRequired = decisionRequired == 1
		result = append(result, p)
	}
	return result, rows.Err()
}

// ListTaskParticipantsByRole returns the merged template + per-task
// participants for a (task, step) pair, filtered to a single role.
// Used by the office dashboard's reviewer/approver listings.
func (r *Repository) ListTaskParticipantsByRole(
	ctx context.Context, stepID, taskID, role string,
) ([]*models.WorkflowStepParticipant, error) {
	all, err := r.ListStepParticipantsForTask(ctx, stepID, taskID)
	if err != nil {
		return nil, err
	}
	filtered := make([]*models.WorkflowStepParticipant, 0, len(all))
	for _, p := range all {
		if string(p.Role) == role {
			filtered = append(filtered, p)
		}
	}
	return filtered, nil
}

// GetStepParticipant returns a single participant by id.
func (r *Repository) GetStepParticipant(ctx context.Context, id string) (*models.WorkflowStepParticipant, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at, provenance
		FROM workflow_step_participants WHERE id = ?
	`), id)
	p := &models.WorkflowStepParticipant{}
	var role string
	var decisionRequired int
	var provenance string
	err := row.Scan(&p.ID, &p.StepID, &p.TaskID, &role, &p.AgentProfileID, &decisionRequired, &p.Position, &p.CreatedAt, &provenance)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("workflow step participant not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	p.Role = models.ParticipantRole(role)
	p.DecisionRequired = decisionRequired == 1
	p.Provenance = models.ParticipantProvenance(provenance)
	return p, nil
}

// ResolveCurrentRunner returns the agent profile id currently driving a
// task at the given workflow step. ADR 0005 Wave D moved per-task
// reassignment from `tasks.assignee_agent_profile_id` to a
// `runner` participant row on (step, task). The resolution rule:
//
//  1. A `runner` participant for (step_id, task_id). Exactly one row
//     wins; multiple rows fall back to the lowest-position id-tiebroken
//     pick (ambiguity, but deterministic).
//  2. The step's `agent_profile_id` (the workflow's primary).
//  3. The most recently assigned runner participant for the task. This
//     preserves the task's effective runner after terminal transitions
//     to steps that do not carry their own participant rows, such as the
//     default Done step.
//
// Returns "" without error when neither exists. Empty step_id or
// task_id is an error — callers must supply both.
func (r *Repository) ResolveCurrentRunner(
	ctx context.Context, stepID, taskID string,
) (string, error) {
	if stepID == "" || taskID == "" {
		return "", errors.New("step_id and task_id are required")
	}
	// Per-task runner row takes precedence. Order by position then id so
	// the pick is deterministic when more than one row exists.
	var agentID string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT agent_profile_id FROM workflow_step_participants
		WHERE step_id = ? AND task_id = ? AND role = 'runner'
		ORDER BY position ASC, id ASC
		LIMIT 1
	`), stepID, taskID).Scan(&agentID)
	if err == nil && agentID != "" {
		return agentID, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("lookup runner participant: %w", err)
	}
	// Fall back to the step's primary.
	var primary sql.NullString
	err = r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT COALESCE(agent_profile_id, '')
		FROM workflow_steps
		WHERE id = ?
	`), stepID).Scan(&primary)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("lookup step primary: %w", err)
	}
	if primary.Valid {
		if primary.String != "" {
			return primary.String, nil
		}
	}
	err = r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT agent_profile_id FROM workflow_step_participants
		WHERE task_id = ? AND role = 'runner'
		ORDER BY created_at DESC, agent_profile_id ASC
		LIMIT 1
	`), taskID).Scan(&agentID)
	if err == nil {
		if agentID != "" {
			return agentID, nil
		}
		return "", nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return "", fmt.Errorf("lookup task runner participant: %w", err)
}

// GetTaskWorkflowStepID returns the task's current workflow step. Cross-task
// queue_run primary resolution uses this before resolving the target task's
// current runner.
func (r *Repository) GetTaskWorkflowStepID(ctx context.Context, taskID string) (string, error) {
	if taskID == "" {
		return "", errors.New("task_id is required")
	}
	var stepID string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT COALESCE(workflow_step_id, '')
		FROM tasks
		WHERE id = ?
	`), taskID).Scan(&stepID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("lookup task workflow step: %w", err)
	}
	return stepID, nil
}

// SetTaskRunner writes (or replaces) the runner participant for
// (step_id, task_id). Idempotent: if a runner participant already
// exists for that pair the existing row is updated to point at the new
// agent_profile_id; otherwise a new row is inserted with position=0 and
// decision_required=0. Reading code SHOULD use ResolveCurrentRunner.
func (r *Repository) SetTaskRunner(
	ctx context.Context, stepID, taskID, agentProfileID string,
) error {
	if stepID == "" || taskID == "" || agentProfileID == "" {
		return errors.New("step_id, task_id, and agent_profile_id are required")
	}
	// Probe for existing row by natural key (step, task, role=runner).
	var existing string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id FROM workflow_step_participants
		WHERE step_id = ? AND task_id = ? AND role = 'runner'
		LIMIT 1
	`), stepID, taskID).Scan(&existing)
	if err == nil {
		_, uerr := r.db.ExecContext(ctx, r.db.Rebind(`
			UPDATE workflow_step_participants SET agent_profile_id = ? WHERE id = ?
		`), agentProfileID, existing)
		if uerr != nil {
			return fmt.Errorf("update runner participant: %w", uerr)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("lookup runner participant: %w", err)
	}
	id := uuid.New().String()
	_, ierr := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workflow_step_participants
		(id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at)
		VALUES (?, ?, ?, 'runner', ?, 0, 0, ?)
	`), id, stepID, taskID, agentProfileID, time.Now().UTC())
	if ierr != nil {
		return fmt.Errorf("insert runner participant: %w", ierr)
	}
	return nil
}

// ClearTaskRunner removes the runner participant for (step_id, task_id).
// A no-op when no runner row exists. Used when a task is unassigned.
func (r *Repository) ClearTaskRunner(ctx context.Context, stepID, taskID string) error {
	if stepID == "" || taskID == "" {
		return errors.New("step_id and task_id are required")
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM workflow_step_participants
		WHERE step_id = ? AND task_id = ? AND role = 'runner'
	`), stepID, taskID); err != nil {
		return fmt.Errorf("clear runner participant: %w", err)
	}
	return nil
}

// FindParticipantID looks up an existing participant id by natural key
// (step_id, task_id, role, agent_profile_id). Used by RecordDecision when
// converting a (task, decider, role) decision into a workflow_step_decisions
// row. Returns "" without an error when no row matches.
func (r *Repository) FindParticipantID(
	ctx context.Context, stepID, taskID, role, agentProfileID string,
) (string, error) {
	if stepID == "" || role == "" || agentProfileID == "" {
		return "", errors.New("step_id, role, and agent_profile_id are required")
	}
	var id string
	// Prefer per-task row when the natural key matches it; fall back to
	// template-level row when only that exists.
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id FROM workflow_step_participants
		WHERE step_id = ? AND role = ? AND agent_profile_id = ?
		  AND (task_id = ? OR task_id = '')
		ORDER BY (CASE WHEN task_id = '' THEN 1 ELSE 0 END) ASC
		LIMIT 1
	`), stepID, role, agentProfileID, taskID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

// sqliteParticipantsNaturalKeyViolationMessage is the substring go-sqlite3
// puts in a UNIQUE-constraint error for participantsNaturalKeyIndexName's
// column list, in the index's own column order.
const sqliteParticipantsNaturalKeyViolationMessage = "UNIQUE constraint failed: workflow_step_participants.step_id, " +
	"workflow_step_participants.task_id, workflow_step_participants.role, workflow_step_participants.agent_profile_id"

// isParticipantsNaturalKeyViolation reports whether err is a violation of
// participantsNaturalKeyIndexName specifically, not any unique violation. On
// PostgreSQL it inspects the typed pgconn.PgError's constraint name; on
// SQLite (no typed access to the constraint name) it matches the
// column-list message documented above. Mirrors isDecisionActiveDeciderViolation.
func isParticipantsNaturalKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == participantsNaturalKeyIndexName
	}
	return strings.Contains(err.Error(), sqliteParticipantsNaturalKeyViolationMessage)
}

const participantsLockNamespace = "workflow-participant-role-seat:"

// ensureRoleSeatMaxAttempts bounds EnsureRoleSeat's retry loop. Mirrors
// recordStepDecisionMaxAttempts: one retry resolves the race the unique
// index exists for (a second concurrent entry's check now observes the
// first entry's committed row and writes nothing); further attempts only
// matter if a third writer interleaves in the same narrow window.
const ensureRoleSeatMaxAttempts = 3

// HasRoleSeatForTaskWorkflow reports whether any participant row in the
// given role already exists for the task anywhere in the given workflow —
// the same workflow-scoped (not step-scoped) existence question
// EnsureRoleSeat answers inside its transaction. Callers use this cheap,
// non-transactional peek to decide whether resolving a candidate agent (an
// Office caster call) is worth doing at all; it is not itself a
// concurrency guard — EnsureRoleSeat's own transactional check is what
// closes the race (see EnsureRoleSeat's doc comment).
func (r *Repository) HasRoleSeatForTaskWorkflow(ctx context.Context, workflowID, taskID, role string) (bool, error) {
	if workflowID == "" || taskID == "" || role == "" {
		return false, errors.New("workflow_id, task_id, and role are required")
	}
	var exists int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT 1 FROM workflow_step_participants p
		JOIN workflow_steps ws ON ws.id = p.step_id
		WHERE p.task_id = ? AND ws.workflow_id = ? AND p.role = ?
		LIMIT 1
	`), taskID, workflowID, role).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check existing role seat: %w", err)
	}
	return true, nil
}

// EnsureRoleSeat is the participant writer review-participant-seats
// introduces (system-design "Components": "A new writer, not a reuse").
// Unlike UpsertTaskParticipant (natural key scoped to step+role+agent) or
// UpsertStepParticipant (same, template-level), this writer's existence
// check is scoped to workflow+role: it asks whether the task already holds
// a seat in this role anywhere in the given workflow, not just at this
// step, so an operator's manually-placed seat at an earlier step is never
// duplicated at a later gated step (AC-OFFICE-REVIEW-SEATS-001.5,
// -003.5).
//
// The check and the insert run in one transaction on the write handle —
// deliberately, not the read-then-write split UpsertTaskParticipant uses.
// That is what lets two concurrent entries for the same task+step be
// bounded to one seat at the role level: the second transaction's check
// observes the first transaction's committed insert and writes nothing
// (system-design "Failure and recovery"). The unique natural-key index
// remains the backstop for the case two entries resolve the same
// (step, agent) pair concurrently despite the check — EnsureRoleSeat
// retries the whole transaction on that specific violation so the loser
// re-observes the winner's row instead of surfacing an error.
//
// agentProfileID must already be resolved by the caller (the Office
// caster) before calling this — EnsureRoleSeat only ever writes the seat
// it is given; it does not resolve a candidate itself.
//
// Returns the seat (existing or newly written) and whether this call
// inserted it. On PostgreSQL, an advisory transaction lock keyed on
// (workflow, task, role) serializes concurrent callers before the SELECT,
// matching recordStepDecisionTx's pattern — SQLite's single-writer lock
// already provides that serialization.
func (r *Repository) EnsureRoleSeat(
	ctx context.Context, workflowID, stepID, taskID, role, agentProfileID string,
) (*models.WorkflowStepParticipant, bool, error) {
	if workflowID == "" || stepID == "" || taskID == "" || agentProfileID == "" {
		return nil, false, errors.New("workflow_id, step_id, task_id, and agent_profile_id are required")
	}
	if !validParticipantRole(models.ParticipantRole(role)) {
		return nil, false, fmt.Errorf("invalid participant role %q", role)
	}

	var (
		seat *models.WorkflowStepParticipant
		ins  bool
		err  error
	)
	for attempt := 0; attempt < ensureRoleSeatMaxAttempts; attempt++ {
		seat, ins, err = r.ensureRoleSeatTx(ctx, workflowID, stepID, taskID, role, agentProfileID)
		if err == nil || !isParticipantsNaturalKeyViolation(err) {
			return seat, ins, err
		}
	}
	return nil, false, fmt.Errorf("ensure role seat: exhausted retries on natural-key race: %w", err)
}

// ensureRoleSeatTx runs one check-then-insert attempt in its own
// transaction. A caller that loses the race gets its INSERT rejected by
// participantsNaturalKeyIndexName; EnsureRoleSeat's retry loop then re-runs
// this from scratch so the SELECT sees the winner's now-committed row.
func (r *Repository) ensureRoleSeatTx(
	ctx context.Context, workflowID, stepID, taskID, role, agentProfileID string,
) (*models.WorkflowStepParticipant, bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if dialect.IsPostgres(r.db.DriverName()) {
		lockKey := strings.Join([]string{participantsLockNamespace, workflowID, taskID, role}, "|")
		if _, err := tx.ExecContext(ctx, r.db.Rebind("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))"), lockKey); err != nil {
			return nil, false, fmt.Errorf("lock role seat identity: %w", err)
		}
	}

	existing := &models.WorkflowStepParticipant{}
	var existingRole string
	var existingDecisionRequired int
	var existingProvenance string
	err = tx.QueryRowContext(ctx, tx.Rebind(`
		SELECT p.id, p.step_id, p.task_id, p.role, p.agent_profile_id, p.decision_required, p.position, p.created_at, p.provenance
		FROM workflow_step_participants p
		JOIN workflow_steps ws ON ws.id = p.step_id
		WHERE p.task_id = ? AND ws.workflow_id = ? AND p.role = ?
		ORDER BY p.position ASC, p.agent_profile_id ASC, p.id ASC
		LIMIT 1
	`), taskID, workflowID, role).Scan(
		&existing.ID, &existing.StepID, &existing.TaskID, &existingRole,
		&existing.AgentProfileID, &existingDecisionRequired, &existing.Position, &existing.CreatedAt, &existingProvenance,
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, false, fmt.Errorf("check existing role seat: %w", err)
	}
	if err == nil {
		existing.Role = models.ParticipantRole(existingRole)
		existing.DecisionRequired = existingDecisionRequired == 1
		existing.Provenance = models.ParticipantProvenance(existingProvenance)
		if cerr := tx.Commit(); cerr != nil {
			return nil, false, fmt.Errorf("commit: %w", cerr)
		}
		return existing, false, nil
	}

	seat := &models.WorkflowStepParticipant{
		ID:               uuid.New().String(),
		StepID:           stepID,
		TaskID:           taskID,
		Role:             models.ParticipantRole(role),
		AgentProfileID:   agentProfileID,
		DecisionRequired: true,
		Position:         0,
		CreatedAt:        time.Now().UTC(),
		Provenance:       models.ParticipantProvenanceAuto,
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at, provenance)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), seat.ID, seat.StepID, seat.TaskID, string(seat.Role), seat.AgentProfileID,
		dialect.BoolToInt(seat.DecisionRequired), seat.Position, seat.CreatedAt, string(seat.Provenance)); err != nil {
		if isParticipantsNaturalKeyViolation(err) {
			return nil, false, err
		}
		return nil, false, fmt.Errorf("insert role seat: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit: %w", err)
	}
	return seat, true, nil
}

// ----------------------------------------------------------------------------
// WorkflowStepDecision CRUD
// ----------------------------------------------------------------------------

// decisionActiveDeciderIndexName is the partial unique index enforcing at
// most one non-superseded decision per (task_id, step_id, decider_id, role).
// Naming it explicitly keeps isDecisionActiveDeciderViolation attributable
// to this constraint specifically (AC-27/29): under PostgreSQL READ
// COMMITTED, two concurrent RecordStepDecision calls for the same decider
// identity can both run the "supersede prior" UPDATE and see zero matching
// rows (neither has committed yet), then both INSERT — this index turns the
// second INSERT into a constraint violation instead of a silent duplicate
// active row, and recordStepDecisionTx retries on that specific violation so
// the loser's write still lands (properly superseding the winner's row)
// rather than surfacing an error to the caller.
const decisionActiveDeciderIndexName = "uniq_workflow_step_decisions_active_decider"

const decisionLockNamespace = "workflow-step-decision:"

// sqliteDecisionActiveDeciderViolationMessage is the substring go-sqlite3
// puts in a UNIQUE-constraint error for this index's column list.
const sqliteDecisionActiveDeciderViolationMessage = "UNIQUE constraint failed: workflow_step_decisions.task_id, " +
	"workflow_step_decisions.step_id, workflow_step_decisions.decider_id, workflow_step_decisions.role"

// isDecisionActiveDeciderViolation reports whether err is a violation of
// decisionActiveDeciderIndexName specifically, not any unique violation. On
// PostgreSQL it inspects the typed pgconn.PgError's constraint name; on
// SQLite (no typed access to the constraint name) it matches the
// column-list message documented above.
func isDecisionActiveDeciderViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == decisionActiveDeciderIndexName
	}
	return strings.Contains(err.Error(), sqliteDecisionActiveDeciderViolationMessage)
}

// recordStepDecisionMaxAttempts bounds the retry loop in RecordStepDecision.
// One retry resolves the race this index exists for (a second concurrent
// writer's supersede-then-insert observes the first writer's now-committed
// row and correctly supersedes it); further attempts only matter if a third
// writer interleaves in the same narrow window, which two spare attempts
// comfortably covers without ever looping unbounded.
const recordStepDecisionMaxAttempts = 3

// RecordStepDecision inserts a new decision row. If id is empty a UUID is
// generated. If decided_at is the zero value the current UTC time is used.
//
// ADR 0005 Wave D folded the legacy office_task_approval_decisions table
// into workflow_step_decisions. This method preserves the office "supersede
// prior" semantics: a row with a populated (decider_id, role) pair that
// matches an existing active row in the same (task, step) atomically marks
// the prior row superseded inside a transaction, then inserts the new one.
// When decider_id is empty (engine-side callers that only know the
// participant_id) the prior-row check falls back to (task, step,
// participant_id). The transaction guarantees readers never observe a row
// gap. See decisionActiveDeciderIndexName for why a decider_id+role write
// retries instead of erroring on a lost supersede race.
func (r *Repository) RecordStepDecision(ctx context.Context, d *models.WorkflowStepDecision) error {
	if d == nil {
		return errors.New("decision must not be nil")
	}
	if d.TaskID == "" || d.StepID == "" || d.ParticipantID == "" {
		return errors.New("decision requires task_id, step_id, and participant_id")
	}
	if d.Decision == "" {
		return errors.New("decision verdict must not be empty")
	}
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	if d.DecidedAt.IsZero() {
		d.DecidedAt = time.Now().UTC()
	}

	var err error
	for attempt := 0; attempt < recordStepDecisionMaxAttempts; attempt++ {
		err = r.recordStepDecisionTx(ctx, d)
		if err == nil || !isDecisionActiveDeciderViolation(err) {
			return err
		}
	}
	return fmt.Errorf("record step decision: exhausted retries on active-decider race: %w", err)
}

// recordStepDecisionTx runs one supersede-then-insert attempt in its own
// transaction. A caller that loses the AC-27/29 race gets its INSERT
// rejected by decisionActiveDeciderIndexName; RecordStepDecision's retry
// loop then re-runs this from scratch so the supersede UPDATE sees the
// winner's now-committed row.
func (r *Repository) recordStepDecisionTx(ctx context.Context, d *models.WorkflowStepDecision) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// PostgreSQL's READ COMMITTED isolation does not serialize the initial
	// supersede UPDATE when no active row exists yet. Lock the identity before
	// that UPDATE so concurrent writers observe the prior commit instead of
	// racing into the partial unique index. SQLite's single-writer lock already
	// provides this serialization.
	lockIdentity := d.ParticipantID
	if d.DeciderID != "" && d.Role != "" {
		lockIdentity = strings.Join([]string{d.DeciderID, d.Role}, "|")
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		lockKey := strings.Join([]string{decisionLockNamespace, d.TaskID, d.StepID, lockIdentity}, "|")
		if _, err := tx.ExecContext(ctx, r.db.Rebind("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))"), lockKey); err != nil {
			return fmt.Errorf("lock active decision identity: %w", err)
		}
	}

	if d.DeciderID != "" && d.Role != "" {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE workflow_step_decisions
			SET superseded_at = ?
			WHERE task_id = ? AND step_id = ? AND decider_id = ? AND role = ?
			  AND superseded_at IS NULL
		`), d.DecidedAt, d.TaskID, d.StepID, d.DeciderID, d.Role); err != nil {
			return fmt.Errorf("supersede prior decisions: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE workflow_step_decisions
			SET superseded_at = ?
			WHERE task_id = ? AND step_id = ? AND participant_id = ?
			  AND superseded_at IS NULL
		`), d.DecidedAt, d.TaskID, d.StepID, d.ParticipantID); err != nil {
			return fmt.Errorf("supersede prior decisions: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO workflow_step_decisions
			(id, task_id, step_id, participant_id, decision, note, decided_at,
			 superseded_at, decider_type, decider_id, role, comment)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?, ?)
	`), d.ID, d.TaskID, d.StepID, d.ParticipantID, d.Decision, d.Note, d.DecidedAt,
		d.DeciderType, d.DeciderID, d.Role, d.Comment); err != nil {
		if isDecisionActiveDeciderViolation(err) {
			return err
		}
		return fmt.Errorf("record step decision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// ListStepDecisions returns every decision recorded for a (task, step) pair,
// oldest first — the order quorum guards will need. Includes superseded rows
// so timelines render the full history; quorum guards filter superseded
// rows themselves.
func (r *Repository) ListStepDecisions(ctx context.Context, taskID, stepID string) ([]*models.WorkflowStepDecision, error) {
	if taskID == "" || stepID == "" {
		return nil, errors.New("task_id and step_id are required")
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, step_id, participant_id, decision, note, decided_at,
		       superseded_at, decider_type, decider_id, role, comment
		FROM workflow_step_decisions
		WHERE task_id = ? AND step_id = ?
		ORDER BY decided_at ASC, id ASC
	`), taskID, stepID)
	if err != nil {
		return nil, fmt.Errorf("list step decisions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanDecisionRows(rows)
}

// ListActiveTaskDecisions returns every non-superseded decision row for a
// task across all steps, oldest first. Mirrors the legacy office
// ListActiveDecisions API so the dashboard can surface pending decisions
// per task without filtering by step.
func (r *Repository) ListActiveTaskDecisions(ctx context.Context, taskID string) ([]*models.WorkflowStepDecision, error) {
	if taskID == "" {
		return nil, errors.New("task_id is required")
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, step_id, participant_id, decision, note, decided_at,
		       superseded_at, decider_type, decider_id, role, comment
		FROM workflow_step_decisions
		WHERE task_id = ? AND superseded_at IS NULL
		ORDER BY decided_at ASC, id ASC
	`), taskID)
	if err != nil {
		return nil, fmt.Errorf("list active task decisions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanDecisionRows(rows)
}

// SupersedeTaskDecisions marks every active decision for a task as
// superseded across all steps. Used by the rework / reopen paths so a
// fresh review round starts without losing the prior audit trail.
// A no-op when there are no active rows.
func (r *Repository) SupersedeTaskDecisions(ctx context.Context, taskID string) error {
	if taskID == "" {
		return errors.New("task_id is required")
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE workflow_step_decisions
		SET superseded_at = ?
		WHERE task_id = ? AND superseded_at IS NULL
	`), time.Now().UTC(), taskID)
	if err != nil {
		return fmt.Errorf("supersede task decisions: %w", err)
	}
	return nil
}

// scanDecisionRows pulls a workflow_step_decisions row set into the model.
// Hoisted so List* helpers stay short and identical in their projection.
func scanDecisionRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]*models.WorkflowStepDecision, error) {
	var result []*models.WorkflowStepDecision
	for rows.Next() {
		d := &models.WorkflowStepDecision{}
		var note, deciderType, deciderID, role, comment sql.NullString
		var supersededAt sql.NullTime
		if err := rows.Scan(&d.ID, &d.TaskID, &d.StepID, &d.ParticipantID,
			&d.Decision, &note, &d.DecidedAt,
			&supersededAt, &deciderType, &deciderID, &role, &comment); err != nil {
			return nil, fmt.Errorf("scan step decision: %w", err)
		}
		if note.Valid {
			d.Note = note.String
		}
		if supersededAt.Valid {
			t := supersededAt.Time
			d.SupersededAt = &t
		}
		if deciderType.Valid {
			d.DeciderType = deciderType.String
		}
		if deciderID.Valid {
			d.DeciderID = deciderID.String
		}
		if role.Valid {
			d.Role = role.String
		}
		if comment.Valid {
			d.Comment = comment.String
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// ClearStepDecisions hard-deletes every decision recorded for a (task, step)
// pair. Returns the number of rows removed. Used by the Phase 2
// ClearDecisions action so quorum starts fresh on Review re-entry.
//
// ADR 0005 Wave D note: this remains a hard delete, distinct from
// SupersedeTaskDecisions which marks rows superseded so the timeline can
// still surface them. Engine-driven clears (re-entering Review) drop the
// state entirely; office-driven clears (rework / reopen on a single task)
// preserve history via the supersede flag.
func (r *Repository) ClearStepDecisions(ctx context.Context, taskID, stepID string) (int64, error) {
	if taskID == "" || stepID == "" {
		return 0, errors.New("task_id and step_id are required")
	}
	res, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM workflow_step_decisions WHERE task_id = ? AND step_id = ?`,
	), taskID, stepID)
	if err != nil {
		return 0, fmt.Errorf("clear step decisions: %w", err)
	}
	rows, _ := res.RowsAffected()
	return rows, nil
}

// validParticipantRole mirrors the workflow_step_participants CHECK constraint.
func validParticipantRole(role models.ParticipantRole) bool {
	switch role {
	case models.ParticipantRoleReviewer,
		models.ParticipantRoleApprover,
		models.ParticipantRoleWatcher,
		models.ParticipantRoleCollaborator,
		models.ParticipantRoleRunner:
		return true
	}
	return false
}
