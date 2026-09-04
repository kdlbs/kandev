package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
)

const planCommentSelectCols = `id, task_id, plan_id, body, selected_text, anchor_from, anchor_to, version, created_at, updated_at`

// ListTaskPlanComments returns the complete pending-comment snapshot for the current plan.
func (r *Repository) ListTaskPlanComments(ctx context.Context, taskID string) (*models.TaskPlanCommentSnapshot, error) {
	tx, err := r.ro.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin task plan comment snapshot read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	snapshot, err := r.readPlanCommentSnapshot(ctx, tx, taskID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit task plan comment snapshot read: %w", err)
	}
	return snapshot, nil
}

// CreateTaskPlanComment inserts a caller-identified comment and increments the collection revision.
func (r *Repository) CreateTaskPlanComment(
	ctx context.Context,
	comment *models.TaskPlanComment,
) (*models.TaskPlanCommentSnapshot, error) {
	fingerprint, err := planCommentCreateFingerprint(comment)
	if err != nil {
		return nil, err
	}
	tx, err := r.beginPlanCommentTx(ctx, comment.TaskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	planID, err := currentPlanID(ctx, tx, r.db, comment.TaskID)
	if err != nil {
		return nil, err
	}
	if planID != comment.PlanID {
		return r.commitPlanCommentConflict(ctx, tx, comment.TaskID)
	}
	snapshot, handled, err := r.resolvePlanCommentAdmissionReplay(
		ctx, tx, comment, fingerprint,
	)
	if err != nil || handled {
		return snapshot, err
	}

	existing, err := getPlanCommentInTx(ctx, tx, r.db, comment.ID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if samePlanCommentCreate(existing, comment) {
			if err := insertPlanCommentAdmission(ctx, tx, r.db, comment, fingerprint); err != nil {
				return nil, err
			}
			return r.commitPlanCommentRead(ctx, tx, comment.TaskID)
		}
		return r.commitPlanCommentConflict(ctx, tx, comment.TaskID)
	}

	now := time.Now().UTC()
	comment.Version = 1
	comment.CreatedAt = now
	comment.UpdatedAt = now
	if err := insertPlanCommentAdmission(ctx, tx, r.db, comment, fingerprint); err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_plan_comments
			(id, task_id, plan_id, body, selected_text, anchor_from, anchor_to, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), comment.ID, comment.TaskID, comment.PlanID, comment.Body, comment.SelectedText,
		comment.AnchorFrom, comment.AnchorTo, comment.Version, comment.CreatedAt, comment.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert task plan comment: %w", err)
	}
	return r.incrementAndCommitPlanCommentSnapshot(ctx, tx, comment.TaskID)
}

type planCommentAdmission struct {
	taskID      string
	planID      string
	fingerprint string
}

func (r *Repository) resolvePlanCommentAdmissionReplay(
	ctx context.Context,
	tx *sqlx.Tx,
	comment *models.TaskPlanComment,
	fingerprint string,
) (*models.TaskPlanCommentSnapshot, bool, error) {
	admission, err := getPlanCommentAdmissionInTx(ctx, tx, r.db, comment.ID)
	if err != nil {
		return nil, true, err
	}
	if admission == nil {
		return nil, false, nil
	}
	if admission.taskID == comment.TaskID && admission.planID == comment.PlanID &&
		admission.fingerprint == fingerprint {
		snapshot, err := r.commitPlanCommentRead(ctx, tx, comment.TaskID)
		return snapshot, true, err
	}
	snapshot, err := r.commitPlanCommentConflict(ctx, tx, comment.TaskID)
	return snapshot, true, err
}

func planCommentCreateFingerprint(comment *models.TaskPlanComment) (string, error) {
	return plancomments.Fingerprint(struct {
		ID           string `json:"id"`
		TaskID       string `json:"task_id"`
		PlanID       string `json:"plan_id"`
		Body         string `json:"body"`
		SelectedText string `json:"selected_text"`
		AnchorFrom   int    `json:"anchor_from"`
		AnchorTo     int    `json:"anchor_to"`
	}{
		ID: comment.ID, TaskID: comment.TaskID, PlanID: comment.PlanID,
		Body: comment.Body, SelectedText: comment.SelectedText,
		AnchorFrom: comment.AnchorFrom, AnchorTo: comment.AnchorTo,
	})
}

func getPlanCommentAdmissionInTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	commentID string,
) (*planCommentAdmission, error) {
	admission := &planCommentAdmission{}
	err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT task_id, plan_id, request_fingerprint
		FROM task_plan_comment_admissions WHERE id = ?
	`), commentID).Scan(&admission.taskID, &admission.planID, &admission.fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read task plan comment admission: %w", err)
	}
	return admission, nil
}

func insertPlanCommentAdmission(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	comment *models.TaskPlanComment,
	fingerprint string,
) error {
	_, err := tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO task_plan_comment_admissions
			(id, task_id, plan_id, request_fingerprint, created_at)
		VALUES (?, ?, ?, ?, ?)
	`), comment.ID, comment.TaskID, comment.PlanID, fingerprint, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("insert task plan comment admission: %w", err)
	}
	return nil
}

// UpdateTaskPlanComment changes a comment body when its plan and row version still match.
func (r *Repository) UpdateTaskPlanComment(
	ctx context.Context,
	comment *models.TaskPlanComment,
	expectedVersion int64,
) (*models.TaskPlanCommentSnapshot, error) {
	tx, err := r.beginPlanCommentTx(ctx, comment.TaskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	planID, err := currentPlanID(ctx, tx, r.db, comment.TaskID)
	if err != nil {
		return nil, err
	}
	if planID != comment.PlanID {
		return r.commitPlanCommentConflict(ctx, tx, comment.TaskID)
	}

	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_plan_comments
		SET body = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND task_id = ? AND plan_id = ? AND version = ?
	`), comment.Body, now, comment.ID, comment.TaskID, comment.PlanID, expectedVersion)
	if err != nil {
		return nil, fmt.Errorf("update task plan comment: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return r.commitPlanCommentConflict(ctx, tx, comment.TaskID)
	}

	stored, err := getPlanCommentInTx(ctx, tx, r.db, comment.ID)
	if err != nil {
		return nil, err
	}
	*comment = *stored
	return r.incrementAndCommitPlanCommentSnapshot(ctx, tx, comment.TaskID)
}

// DeleteTaskPlanComment removes a comment when its plan and row version still match.
func (r *Repository) DeleteTaskPlanComment(
	ctx context.Context,
	taskID, planID, commentID string,
	expectedVersion int64,
) (*models.TaskPlanCommentSnapshot, error) {
	tx, err := r.beginPlanCommentTx(ctx, taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	currentID, err := currentPlanID(ctx, tx, r.db, taskID)
	if err != nil {
		return nil, err
	}
	if currentID != planID {
		return r.commitPlanCommentConflict(ctx, tx, taskID)
	}

	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_plan_comments
		WHERE id = ? AND task_id = ? AND plan_id = ? AND version = ?
	`), commentID, taskID, planID, expectedVersion)
	if err != nil {
		return nil, fmt.Errorf("delete task plan comment: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return r.commitPlanCommentConflict(ctx, tx, taskID)
	}
	return r.incrementAndCommitPlanCommentSnapshot(ctx, tx, taskID)
}

func (r *Repository) beginPlanCommentTx(ctx context.Context, taskID string) (*sqlx.Tx, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin task plan comment transaction: %w", err)
	}
	if err := r.lockTaskRowInTx(ctx, tx, taskID); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func currentPlanID(ctx context.Context, q sqlx.QueryerContext, db *sqlx.DB, taskID string) (string, error) {
	var planID string
	err := sqlx.GetContext(ctx, q, &planID, db.Rebind(`SELECT id FROM task_plans WHERE task_id = ?`), taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrTaskPlanNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read current task plan: %w", err)
	}
	return planID, nil
}

func getPlanCommentInTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	commentID string,
) (*models.TaskPlanComment, error) {
	comment := &models.TaskPlanComment{}
	err := tx.QueryRowContext(ctx, db.Rebind(
		`SELECT `+planCommentSelectCols+` FROM task_plan_comments WHERE id = ?`,
	), commentID).Scan(
		&comment.ID, &comment.TaskID, &comment.PlanID, &comment.Body, &comment.SelectedText,
		&comment.AnchorFrom, &comment.AnchorTo, &comment.Version, &comment.CreatedAt, &comment.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read task plan comment: %w", err)
	}
	return comment, nil
}

func samePlanCommentCreate(a, b *models.TaskPlanComment) bool {
	return a.ID == b.ID && a.TaskID == b.TaskID && a.PlanID == b.PlanID &&
		a.Body == b.Body && a.SelectedText == b.SelectedText &&
		a.AnchorFrom == b.AnchorFrom && a.AnchorTo == b.AnchorTo
}

func (r *Repository) incrementAndCommitPlanCommentSnapshot(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) (*models.TaskPlanCommentSnapshot, error) {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_plans SET comments_revision = comments_revision + 1 WHERE task_id = ?
	`), taskID)
	if err != nil {
		return nil, fmt.Errorf("increment task plan comments revision: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return nil, ErrTaskPlanNotFound
	}
	return r.commitPlanCommentRead(ctx, tx, taskID)
}

func (r *Repository) commitPlanCommentRead(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) (*models.TaskPlanCommentSnapshot, error) {
	snapshot, err := r.readPlanCommentSnapshot(ctx, tx, taskID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit task plan comment transaction: %w", err)
	}
	return snapshot, nil
}

func (r *Repository) commitPlanCommentConflict(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) (*models.TaskPlanCommentSnapshot, error) {
	snapshot, err := r.commitPlanCommentRead(ctx, tx, taskID)
	if err != nil {
		return nil, err
	}
	return snapshot, ErrTaskPlanCommentsChanged
}

func (r *Repository) readPlanCommentSnapshot(
	ctx context.Context,
	q sqlx.QueryerContext,
	taskID string,
) (*models.TaskPlanCommentSnapshot, error) {
	snapshot := &models.TaskPlanCommentSnapshot{TaskID: taskID, Comments: make([]*models.TaskPlanComment, 0)}
	err := sqlx.GetContext(ctx, q, snapshot, r.db.Rebind(`
		SELECT id AS plan_id, comments_revision AS revision
		FROM task_plans WHERE task_id = ?
	`), taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTaskPlanNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read task plan comment snapshot head: %w", err)
	}

	rows, err := q.QueryContext(ctx, r.db.Rebind(`
		SELECT `+planCommentSelectCols+`
		FROM task_plan_comments WHERE task_id = ? AND plan_id = ?
		ORDER BY created_at, id
	`), taskID, snapshot.PlanID)
	if err != nil {
		return nil, fmt.Errorf("list task plan comments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		comment := &models.TaskPlanComment{}
		if err := rows.Scan(
			&comment.ID, &comment.TaskID, &comment.PlanID, &comment.Body, &comment.SelectedText,
			&comment.AnchorFrom, &comment.AnchorTo, &comment.Version, &comment.CreatedAt, &comment.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan task plan comment: %w", err)
		}
		snapshot.Comments = append(snapshot.Comments, comment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task plan comments: %w", err)
	}
	return snapshot, nil
}
