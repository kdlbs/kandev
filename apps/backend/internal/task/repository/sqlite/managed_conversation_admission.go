package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	managed "github.com/kandev/kandev/internal/task/repository/managedconversation"
)

func (r *Repository) beginManagedAdmission(ctx context.Context, identity managed.Identity) (*sqlx.Tx, *models.Task, error) {
	tx, err := r.db.BeginTxx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return nil, nil, err
	}
	if err = r.lockTaskHierarchy(ctx, tx, []string{identity.WorkspaceID}, nil); err != nil {
		_ = tx.Rollback()
		return nil, nil, managedContextError(ctx, err)
	}
	query := `SELECT ` + taskSelectColumns("t") + ` FROM tasks t WHERE t.id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE OF t`
	}
	task, err := r.scanSingleTask(tx.QueryRowContext(ctx, tx.Rebind(query), identity.TaskID))
	if errors.Is(err, sql.ErrNoRows) {
		return tx, nil, nil
	}
	if err != nil {
		_ = tx.Rollback()
		return nil, nil, managedContextError(ctx, err)
	}
	if !managed.Matches(task, identity) {
		_ = tx.Rollback()
		return nil, nil, managed.ErrNotFound
	}
	if err := r.managedDeletionBarrierTx(ctx, tx, task.ID); err != nil {
		_ = tx.Rollback()
		return nil, nil, fmt.Errorf("%w: %v", managed.ErrBusy, err)
	}
	return tx, task, nil
}

func managedContextError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func (r *Repository) managedPrimaryTx(ctx context.Context, tx *sqlx.Tx, taskID string) (*models.TaskSession, error) {
	query := `SELECT id FROM task_sessions WHERE task_id = ? AND is_primary = 1 ORDER BY id`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	rows, err := tx.QueryContext(ctx, tx.Rebind(query), taskID)
	if err != nil {
		return nil, managedContextError(ctx, err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	if len(ids) != 1 {
		return nil, fmt.Errorf("managed conversation has multiple primary sessions")
	}
	if err := r.lockManagedExecutorTx(ctx, tx, ids[0]); err != nil {
		return nil, err
	}
	return managedSessionProjectionTx(ctx, tx, ids[0])
}

func (r *Repository) lockManagedExecutorTx(ctx context.Context, tx *sqlx.Tx, sessionID string) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}
	var id string
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT id FROM executors_running WHERE session_id = ? FOR UPDATE`), sessionID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return managedContextError(ctx, err)
}

func managedSessionProjectionTx(ctx context.Context, tx *sqlx.Tx, id string) (*models.TaskSession, error) {
	rows, err := tx.QueryContext(ctx, tx.Rebind(`SELECT `+taskSessionSelectCols+` `+taskSessionFromClause+` WHERE ts.id = ?`), id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, models.ErrTaskSessionNotFound
	}
	return scanTaskSessionRow(rows)
}

func (r *Repository) EnsureManagedConversation(ctx context.Context, input managed.EnsureRequest) (managed.Result, error) {
	tx, task, err := r.beginManagedAdmission(ctx, input.Identity)
	if err != nil {
		return managed.Result{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var result managed.Result
	if task == nil {
		result, err = r.createManagedConversationTx(ctx, tx, input)
	} else {
		result, err = r.reconcileManagedConversationTx(ctx, tx, task, input)
	}
	if err != nil {
		return managed.Result{}, managedContextError(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return managed.Result{}, err
	}
	if err := tx.Commit(); err != nil {
		return managed.Result{}, managedContextError(ctx, err)
	}
	return result, nil
}

func (r *Repository) createManagedConversationTx(ctx context.Context, tx *sqlx.Tx, input managed.EnsureRequest) (managed.Result, error) {
	if input.ExpectedRevision != 0 {
		return managed.Result{}, managed.ErrRevision
	}
	if input.Task == nil || !managed.Matches(input.Task, input.Identity) {
		return managed.Result{}, managed.ErrNotFound
	}
	if err := r.prepareTaskForCreate(input.Task); err != nil {
		return managed.Result{}, err
	}
	if _, err := r.insertTaskTx(ctx, tx.Tx, input.Task); err != nil {
		return managed.Result{}, err
	}
	primary, err := r.repairManagedPrimaryTx(ctx, tx, input.Task, input.PrimaryID, input.Configuration)
	return managed.Result{Task: input.Task, Primary: primary, Created: true, Changed: true}, err
}

func (r *Repository) reconcileManagedConversationTx(ctx context.Context, tx *sqlx.Tx, task *models.Task, input managed.EnsureRequest) (managed.Result, error) {
	if task.Metadata[models.MetaKeyManagedConversationDetached] == true {
		return managed.Result{}, managed.ErrNotFound
	}
	primary, err := r.managedPrimaryTx(ctx, tx, task.ID)
	if err != nil {
		return managed.Result{}, err
	}
	config, changed, replayed, err := managedEnsureConfiguration(task, input)
	if err != nil {
		return managed.Result{}, err
	}
	if changed && !managedPrimaryIdle(primary) {
		return managed.Result{}, managed.ErrBusy
	}
	missing := primary == nil
	if missing {
		primary, err = r.repairManagedPrimaryTx(ctx, tx, task, input.PrimaryID, config)
	} else if changed {
		err = r.updateManagedPrimaryTx(ctx, tx, primary, config)
	}
	if err != nil {
		return managed.Result{}, err
	}
	changed = !replayed && (changed || missing)
	if changed {
		values := config.Values()
		values[models.MetaKeyManagedByPlugin] = input.Task.Metadata[models.MetaKeyManagedByPlugin]
		stampManagedOperation(values, managed.Revision(task)+1, input.OperationID, input.PayloadDigest)
		if err := r.patchManagedTaskTx(ctx, tx, task, values); err != nil {
			return managed.Result{}, err
		}
	}
	return managed.Result{Task: task, Primary: primary, Changed: changed, Replayed: replayed}, nil
}

func managedEnsureConfiguration(task *models.Task, input managed.EnsureRequest) (managed.Configuration, bool, bool, error) {
	if managed.Replay(task, input.OperationID, input.PayloadDigest) {
		return managed.FromTask(task), false, true, nil
	}
	if managed.Revision(task) != input.ExpectedRevision {
		return managed.Configuration{}, false, false, managed.ErrRevision
	}
	current := managed.FromTask(task)
	changed := !input.Equal(current) || task.Metadata[models.MetaKeyManagedPolicyInvalidated] == true
	return input.Configuration, changed, false, nil
}

func managedPrimaryIdle(primary *models.TaskSession) bool {
	return primary == nil || (primary.State != models.TaskSessionStateRunning && primary.State != models.TaskSessionStateStarting && primary.AgentExecutionID == "")
}

func (r *Repository) repairManagedPrimaryTx(ctx context.Context, tx *sqlx.Tx, task *models.Task, primaryID string, config managed.Configuration) (*models.TaskSession, error) {
	if err := r.taskCleanupBarrierLocked(ctx, tx, task.ID); err != nil {
		return nil, err
	}
	primary := &models.TaskSession{ID: primaryID, TaskID: task.ID, IsPrimary: true,
		AgentProfileID: config.AgentProfileID, ExecutorID: config.ExecutorID, ExecutorProfileID: config.ExecutorProfileID,
		State: models.TaskSessionStateCreated}
	if err := r.createTaskSession(ctx, tx, primary); err != nil {
		return nil, err
	}
	return managedSessionProjectionTx(ctx, tx, primary.ID)
}

func (r *Repository) updateManagedPrimaryTx(ctx context.Context, tx *sqlx.Tx, primary *models.TaskSession, config managed.Configuration) error {
	if err := r.taskCleanupBarrierLocked(ctx, tx, primary.TaskID); err != nil {
		return err
	}
	now := r.nowUTC()
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_sessions SET agent_profile_id = ?, executor_id = ?, executor_profile_id = ?, updated_at = ? WHERE id = ?`),
		config.AgentProfileID, config.ExecutorID, config.ExecutorProfileID, now, primary.ID)
	if err == nil {
		primary.AgentProfileID, primary.ExecutorID, primary.ExecutorProfileID = config.AgentProfileID, config.ExecutorID, config.ExecutorProfileID
		primary.UpdatedAt = now
	}
	return err
}

func stampManagedOperation(values map[string]interface{}, revision uint64, operation, digest string) {
	values[models.MetaKeyManagedConversationRevision] = strconv.FormatUint(revision, 10)
	values[managed.OperationKey], values[managed.PayloadKey] = operation, digest
}

func (r *Repository) patchManagedTaskTx(ctx context.Context, tx *sqlx.Tx, task *models.Task, values map[string]interface{}) error {
	current, err := r.currentMetadataForMerge(ctx, tx.Tx, task.ID)
	if err != nil {
		return err
	}
	for key, value := range values {
		if value == "" {
			delete(current, key)
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		current[key] = encoded
	}
	encoded, err := json.Marshal(current)
	if err != nil {
		return err
	}
	now := r.nowUTC()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE tasks SET metadata = ?, updated_at = ? WHERE id = ?`), string(encoded), now, task.ID); err != nil {
		return err
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		return err
	}
	task.Metadata = metadata
	task.UpdatedAt = now
	return nil
}
