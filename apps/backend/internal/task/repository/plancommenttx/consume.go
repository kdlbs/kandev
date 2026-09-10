// Package plancommenttx shares atomic pending-comment validation and consumption.
package plancommenttx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const commentSelectCols = `id, task_id, plan_id, body, selected_text, anchor_from, anchor_to, version, created_at, updated_at`

// Resolution is the authoritative prompt expansion captured under the task lock.
type Resolution struct {
	TaskID   string
	PlanID   string
	Content  string
	Comments []*models.TaskPlanComment
	Before   *models.TaskPlanCommentSnapshot
}

// CommentsChangedError carries the committed snapshot that invalidated a request.
type CommentsChangedError struct {
	Snapshot *models.TaskPlanCommentSnapshot
}

func (e *CommentsChangedError) Error() string { return repoerrors.ErrTaskPlanCommentsChanged.Error() }
func (e *CommentsChangedError) Unwrap() error { return repoerrors.ErrTaskPlanCommentsChanged }

// PrimarySessionChangedError reports the safe current primary, when one exists.
type PrimarySessionChangedError struct {
	SessionID string
	State     models.TaskSessionState
}

func (e *PrimarySessionChangedError) Error() string {
	return repoerrors.ErrPrimarySessionChanged.Error()
}
func (e *PrimarySessionChangedError) Unwrap() error { return repoerrors.ErrPrimarySessionChanged }

// SessionUnavailableError reports a target that cannot accept a prompt at
// the final message/queue admission boundary.
type SessionUnavailableError struct {
	SessionID string
	State     models.TaskSessionState
}

func (e *SessionUnavailableError) Error() string { return repoerrors.ErrTaskSessionUnavailable.Error() }
func (e *SessionUnavailableError) Unwrap() error { return repoerrors.ErrTaskSessionUnavailable }

// LockTask establishes the global task-before-session mutation order.
func LockTask(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, taskID string) error {
	result, err := tx.ExecContext(ctx, db.Rebind(`
		UPDATE tasks SET updated_at = updated_at WHERE id = ?
	`), taskID)
	if err != nil {
		return fmt.Errorf("lock task for plan comment admission: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect task plan comment lock: %w", err)
	}
	if changed != 1 {
		return fmt.Errorf("%w: %s", repoerrors.ErrTaskNotFound, taskID)
	}
	return nil
}

// Resolve validates exact references and expands the server-owned prompt content.
func ResolveDirect(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	taskID string,
	sessionID string,
	contentTemplate string,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
) (*Resolution, error) {
	return resolve(
		ctx, tx, db, taskID, sessionID, contentTemplate, refs, requirePrimary, expectedState,
	)
}

// ResolveQueue validates a queue target without requiring one exact active
// state. Busy and promptable sessions may both accept queued work.
func ResolveQueue(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	taskID string,
	sessionID string,
	contentTemplate string,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
) (*Resolution, error) {
	return resolve(ctx, tx, db, taskID, sessionID, contentTemplate, refs, requirePrimary, "")
}

func resolve(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	taskID string,
	sessionID string,
	contentTemplate string,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
) (*Resolution, error) {
	if err := validateTargetSession(
		ctx, tx, db, taskID, sessionID, requirePrimary, expectedState,
	); err != nil {
		return nil, err
	}
	snapshot, err := readSnapshot(ctx, tx, db, taskID)
	if err != nil {
		return nil, err
	}
	comments, ok := resolveReferences(snapshot, refs)
	if !ok {
		return nil, &CommentsChangedError{Snapshot: snapshot}
	}
	content, err := plancomments.ResolvePlaceholder(contentTemplate, comments)
	if err != nil {
		return nil, fmt.Errorf("format task plan comments: %w", err)
	}
	if err := plancomments.ValidateRenderedPrompt(content); err != nil {
		return nil, err
	}
	return &Resolution{
		TaskID: taskID, PlanID: snapshot.PlanID, Content: content,
		Comments: comments, Before: snapshot,
	}, nil
}

// Consume deletes resolved rows and advances the collection revision.
func Consume(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	resolved *Resolution,
) (*models.TaskPlanCommentSnapshot, error) {
	if resolved == nil || len(resolved.Comments) == 0 {
		return nil, errors.New("resolved plan comments are required")
	}
	query, args := consumeQuery(resolved)
	result, err := tx.ExecContext(ctx, db.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("consume task plan comments: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("inspect consumed task plan comments: %w", err)
	}
	if changed != int64(len(resolved.Comments)) {
		return nil, &CommentsChangedError{Snapshot: resolved.Before}
	}
	result, err = tx.ExecContext(ctx, db.Rebind(`
		UPDATE task_plans SET comments_revision = comments_revision + 1
		WHERE task_id = ? AND id = ?
	`), resolved.TaskID, resolved.PlanID)
	if err != nil {
		return nil, fmt.Errorf("advance task plan comment revision: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return nil, repoerrors.ErrTaskPlanNotFound
	}
	return readSnapshot(ctx, tx, db, resolved.TaskID)
}

func validateTargetSession(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	taskID, sessionID string,
	requirePrimary bool,
	expectedState models.TaskSessionState,
) error {
	var (
		state models.TaskSessionState
		err   error
	)
	if requirePrimary {
		state, err = validatePrimarySession(ctx, tx, db, taskID, sessionID)
	} else {
		state, err = validateTaskSession(ctx, tx, db, taskID, sessionID)
	}
	if err != nil {
		return err
	}
	if !isActiveSessionState(state) || expectedState != "" && state != expectedState {
		return &SessionUnavailableError{SessionID: sessionID, State: state}
	}
	return nil
}

func validateTaskSession(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	taskID, sessionID string,
) (models.TaskSessionState, error) {
	var (
		storedTaskID string
		state        models.TaskSessionState
	)
	query := `SELECT task_id, state FROM task_sessions WHERE id = ?`
	if dialect.IsPostgres(db.DriverName()) {
		query += ` FOR UPDATE`
	}
	err := tx.QueryRowContext(ctx, db.Rebind(query), sessionID).Scan(&storedTaskID, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", repoerrors.ErrTaskSessionMismatch
	}
	if err != nil {
		return "", fmt.Errorf("validate task session: %w", err)
	}
	if storedTaskID != taskID {
		return "", repoerrors.ErrTaskSessionMismatch
	}
	return state, nil
}

func validatePrimarySession(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	taskID, sessionID string,
) (models.TaskSessionState, error) {
	query := `SELECT id, state FROM task_sessions WHERE task_id = ? AND is_primary = 1 LIMIT 1`
	if dialect.IsPostgres(db.DriverName()) {
		query += ` FOR UPDATE`
	}
	var currentID, state string
	err := tx.QueryRowContext(ctx, db.Rebind(query), taskID).Scan(&currentID, &state)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("read current primary session: %w", err)
	}
	if err == nil && currentID == sessionID {
		return models.TaskSessionState(state), nil
	}
	return "", &PrimarySessionChangedError{SessionID: currentID, State: models.TaskSessionState(state)}
}

func isActiveSessionState(state models.TaskSessionState) bool {
	switch state {
	case models.TaskSessionStateCreated,
		models.TaskSessionStateStarting,
		models.TaskSessionStateRunning,
		models.TaskSessionStateIdle,
		models.TaskSessionStateWaitingForInput:
		return true
	default:
		return false
	}
}

func resolveReferences(
	snapshot *models.TaskPlanCommentSnapshot,
	refs []models.TaskPlanCommentRef,
) ([]*models.TaskPlanComment, bool) {
	if len(refs) == 0 {
		return nil, false
	}
	byID := make(map[string]*models.TaskPlanComment, len(snapshot.Comments))
	for _, comment := range snapshot.Comments {
		byID[comment.ID] = comment
	}
	seen := make(map[string]struct{}, len(refs))
	comments := make([]*models.TaskPlanComment, 0, len(refs))
	for _, ref := range refs {
		comment := byID[ref.ID]
		if _, duplicate := seen[ref.ID]; duplicate || comment == nil || comment.Version != ref.Version {
			return nil, false
		}
		seen[ref.ID] = struct{}{}
		comments = append(comments, comment)
	}
	sort.Slice(comments, func(i, j int) bool {
		if comments[i].CreatedAt.Equal(comments[j].CreatedAt) {
			return comments[i].ID < comments[j].ID
		}
		return comments[i].CreatedAt.Before(comments[j].CreatedAt)
	})
	return comments, true
}

func consumeQuery(resolved *Resolution) (string, []interface{}) {
	conditions := make([]string, 0, len(resolved.Comments))
	args := []interface{}{resolved.TaskID, resolved.PlanID}
	for _, comment := range resolved.Comments {
		conditions = append(conditions, `(id = ? AND version = ?)`)
		args = append(args, comment.ID, comment.Version)
	}
	return `DELETE FROM task_plan_comments WHERE task_id = ? AND plan_id = ? AND (` +
		strings.Join(conditions, ` OR `) + `)`, args
}

func readSnapshot(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	taskID string,
) (*models.TaskPlanCommentSnapshot, error) {
	snapshot := &models.TaskPlanCommentSnapshot{TaskID: taskID, Comments: make([]*models.TaskPlanComment, 0)}
	err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT id, comments_revision FROM task_plans WHERE task_id = ?
	`), taskID).Scan(&snapshot.PlanID, &snapshot.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repoerrors.ErrTaskPlanNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read task plan comment head: %w", err)
	}
	rows, err := tx.QueryContext(ctx, db.Rebind(`
		SELECT `+commentSelectCols+` FROM task_plan_comments
		WHERE task_id = ? AND plan_id = ? ORDER BY created_at, id
	`), taskID, snapshot.PlanID)
	if err != nil {
		return nil, fmt.Errorf("read task plan comments: %w", err)
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
