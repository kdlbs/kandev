package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// revisionSelectCols lists the task_plan_revisions columns in the fixed order used by
// every SELECT in this file (and by scanRevisionRow / scanRevisionRows).
const revisionSelectCols = `id, task_id, revision_number, title, content, author_kind, author_name, revert_of_revision_id, created_at, updated_at`

// CreateTaskPlan creates a new task plan.
func (r *Repository) CreateTaskPlan(ctx context.Context, plan *models.TaskPlan) error {
	if plan.ID == "" {
		plan.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	plan.CreatedAt = now
	plan.UpdatedAt = now

	if plan.Title == "" {
		plan.Title = "Plan"
	}
	if plan.CreatedBy == "" {
		plan.CreatedBy = "agent"
	}

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_plans (id, task_id, title, content, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`), plan.ID, plan.TaskID, plan.Title, plan.Content, plan.CreatedBy, plan.CreatedAt, plan.UpdatedAt)
	return err
}

// GetTaskPlan retrieves a task plan by task ID.
func (r *Repository) GetTaskPlan(ctx context.Context, taskID string) (*models.TaskPlan, error) {
	plan := &models.TaskPlan{}
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, title, content, created_by, created_at, updated_at
		FROM task_plans WHERE task_id = ?
	`), taskID).Scan(&plan.ID, &plan.TaskID, &plan.Title, &plan.Content, &plan.CreatedBy, &plan.CreatedAt, &plan.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil // Return nil, nil when no plan exists
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task plan: %w", err)
	}
	return plan, nil
}

// UpdateTaskPlan updates an existing task plan.
func (r *Repository) UpdateTaskPlan(ctx context.Context, plan *models.TaskPlan) error {
	plan.UpdatedAt = time.Now().UTC()

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_plans SET title = ?, content = ?, created_by = ?, updated_at = ?
		WHERE task_id = ?
	`), plan.Title, plan.Content, plan.CreatedBy, plan.UpdatedAt, plan.TaskID)
	if err != nil {
		return fmt.Errorf("failed to update task plan: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task plan not found for task: %s", plan.TaskID)
	}
	return nil
}

// DeleteTaskPlan deletes a task plan by task ID.
func (r *Repository) DeleteTaskPlan(ctx context.Context, taskID string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_plans WHERE task_id = ?`), taskID)
	if err != nil {
		return fmt.Errorf("failed to delete task plan: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task plan not found for task: %s", taskID)
	}
	return nil
}

// Revision history

// InsertTaskPlanRevision inserts a new revision row.
func (r *Repository) InsertTaskPlanRevision(ctx context.Context, rev *models.TaskPlanRevision) error {
	if rev.ID == "" {
		rev.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if rev.CreatedAt.IsZero() {
		rev.CreatedAt = now
	}
	rev.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_plan_revisions
			(id, task_id, revision_number, title, content, author_kind, author_name, revert_of_revision_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		rev.ID, rev.TaskID, rev.RevisionNumber, rev.Title, rev.Content,
		rev.AuthorKind, rev.AuthorName, rev.RevertOfRevisionID, rev.CreatedAt, rev.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert task plan revision: %w", err)
	}
	return nil
}

// UpdateTaskPlanRevision updates title/content/updated_at on an existing revision (coalesce merge).
func (r *Repository) UpdateTaskPlanRevision(ctx context.Context, rev *models.TaskPlanRevision) error {
	rev.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_plan_revisions
		SET title = ?, content = ?, updated_at = ?
		WHERE id = ?
	`), rev.Title, rev.Content, rev.UpdatedAt, rev.ID)
	if err != nil {
		return fmt.Errorf("failed to update task plan revision: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task plan revision not found: %s", rev.ID)
	}
	return nil
}

// GetTaskPlanRevision fetches a single revision by ID.
func (r *Repository) GetTaskPlanRevision(ctx context.Context, id string) (*models.TaskPlanRevision, error) {
	return r.scanRevisionRow(r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+revisionSelectCols+` FROM task_plan_revisions WHERE id = ?`,
	), id))
}

// GetLatestTaskPlanRevision returns the newest revision for a task (by revision_number DESC).
func (r *Repository) GetLatestTaskPlanRevision(ctx context.Context, taskID string) (*models.TaskPlanRevision, error) {
	return r.scanRevisionRow(r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+revisionSelectCols+` FROM task_plan_revisions WHERE task_id = ? ORDER BY revision_number DESC LIMIT 1`,
	), taskID))
}

// ListTaskPlanRevisions returns revisions newest-first. limit <= 0 returns all.
func (r *Repository) ListTaskPlanRevisions(ctx context.Context, taskID string, limit int) ([]*models.TaskPlanRevision, error) {
	query := `SELECT ` + revisionSelectCols + ` FROM task_plan_revisions WHERE task_id = ? ORDER BY revision_number DESC`
	args := []interface{}{taskID}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list task plan revisions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*models.TaskPlanRevision
	for rows.Next() {
		rev := &models.TaskPlanRevision{}
		var revertOf sql.NullString
		if err := rows.Scan(
			&rev.ID, &rev.TaskID, &rev.RevisionNumber, &rev.Title, &rev.Content,
			&rev.AuthorKind, &rev.AuthorName, &revertOf, &rev.CreatedAt, &rev.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan task plan revision: %w", err)
		}
		if revertOf.Valid {
			v := revertOf.String
			rev.RevertOfRevisionID = &v
		}
		out = append(out, rev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task plan revisions: %w", err)
	}
	return out, nil
}

// NextTaskPlanRevisionNumber returns max(revision_number)+1 for a task, or 1 if none exist.
//
// Note: prefer WritePlanRevision when writing a new revision — it computes the next number
// atomically with the insert. This helper reads via the RO replica and is only safe against
// TOCTOU when followed by a write under a BEGIN IMMEDIATE-equivalent (e.g., within
// WritePlanRevision). Direct callers should be confined to read-only inspection / tests.
func (r *Repository) NextTaskPlanRevisionNumber(ctx context.Context, taskID string) (int, error) {
	var maxNum sql.NullInt64
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT MAX(revision_number) FROM task_plan_revisions WHERE task_id = ?
	`), taskID).Scan(&maxNum)
	if err != nil {
		return 0, fmt.Errorf("failed to get next revision number: %w", err)
	}
	if !maxNum.Valid {
		return 1, nil
	}
	return int(maxNum.Int64) + 1, nil
}

// WritePlanRevision atomically upserts HEAD (task_plans) and either appends a new revision
// or merges into an existing one in a single write transaction keyed by task_id. This closes
// the TOCTOU window on revision_number (MAX+1 is computed inside the tx) and prevents HEAD
// from disagreeing with history on partial failure.
//
// Coalesce behavior: when coalesceLatestID is non-nil and non-empty, the revision with that
// ID has title/content/updated_at merged in-place and its other fields (revision_number,
// author, created_at) are preserved. When nil or empty, a new revision row is inserted with
// revision_number = MAX(existing)+1 and populated from rev.
//
// On success, rev is mutated to reflect the persisted state (ID, RevisionNumber, CreatedAt,
// UpdatedAt).
func (r *Repository) WritePlanRevision(
	ctx context.Context,
	head *models.TaskPlan,
	rev *models.TaskPlanRevision,
	coalesceLatestID *string,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin plan revision tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	if err := upsertPlanHead(ctx, tx, r.db, head, now); err != nil {
		return err
	}
	if coalesceLatestID != nil && *coalesceLatestID != "" {
		if err := mergeRevisionInTx(ctx, tx, r.db, rev, *coalesceLatestID, now); err != nil {
			return err
		}
	} else {
		if err := insertNewRevisionInTx(ctx, tx, r.db, rev, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func upsertPlanHead(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, head *models.TaskPlan, now time.Time) error {
	if head.ID == "" {
		head.ID = uuid.New().String()
	}
	if head.Title == "" {
		head.Title = "Plan"
	}
	if head.CreatedBy == "" {
		head.CreatedBy = "agent"
	}
	if head.CreatedAt.IsZero() {
		head.CreatedAt = now
	}
	head.UpdatedAt = now
	if _, err := tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO task_plans (id, task_id, title, content, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			title = excluded.title,
			content = excluded.content,
			created_by = excluded.created_by,
			updated_at = excluded.updated_at
	`), head.ID, head.TaskID, head.Title, head.Content, head.CreatedBy, head.CreatedAt, head.UpdatedAt); err != nil {
		return fmt.Errorf("upsert task plan head: %w", err)
	}
	return nil
}

func mergeRevisionInTx(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, rev *models.TaskPlanRevision, latestID string, now time.Time) error {
	result, err := tx.ExecContext(ctx, db.Rebind(`
		UPDATE task_plan_revisions
		SET title = ?, content = ?, updated_at = ?
		WHERE id = ?
	`), rev.Title, rev.Content, now, latestID)
	if err != nil {
		return fmt.Errorf("merge plan revision: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task plan revision not found: %s", latestID)
	}
	rev.ID = latestID
	rev.UpdatedAt = now
	return nil
}

func insertNewRevisionInTx(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, rev *models.TaskPlanRevision, now time.Time) error {
	var maxNum sql.NullInt64
	if err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT MAX(revision_number) FROM task_plan_revisions WHERE task_id = ?
	`), rev.TaskID).Scan(&maxNum); err != nil {
		return fmt.Errorf("compute next revision number: %w", err)
	}
	rev.RevisionNumber = 1
	if maxNum.Valid {
		rev.RevisionNumber = int(maxNum.Int64) + 1
	}
	if rev.ID == "" {
		rev.ID = uuid.New().String()
	}
	if rev.CreatedAt.IsZero() {
		rev.CreatedAt = now
	}
	rev.UpdatedAt = now
	if _, err := tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO task_plan_revisions
			(`+revisionSelectCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		rev.ID, rev.TaskID, rev.RevisionNumber, rev.Title, rev.Content,
		rev.AuthorKind, rev.AuthorName, rev.RevertOfRevisionID, rev.CreatedAt, rev.UpdatedAt); err != nil {
		return fmt.Errorf("insert plan revision: %w", err)
	}
	return nil
}

func (r *Repository) scanRevisionRow(row *sql.Row) (*models.TaskPlanRevision, error) {
	rev := &models.TaskPlanRevision{}
	var revertOf sql.NullString
	err := row.Scan(
		&rev.ID, &rev.TaskID, &rev.RevisionNumber, &rev.Title, &rev.Content,
		&rev.AuthorKind, &rev.AuthorName, &revertOf, &rev.CreatedAt, &rev.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan task plan revision: %w", err)
	}
	if revertOf.Valid {
		v := revertOf.String
		rev.RevertOfRevisionID = &v
	}
	return rev, nil
}
