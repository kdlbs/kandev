package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	managed "github.com/kandev/kandev/internal/task/repository/managedconversation"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func (r *Repository) managedDeletionJobTx(ctx context.Context, tx *sqlx.Tx, operation string) (*models.TaskResourceCleanupJob, error) {
	query := `SELECT ` + taskResourceCleanupColumns + ` FROM task_resource_cleanup_jobs WHERE operation_id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	job := &models.TaskResourceCleanupJob{}
	err := tx.QueryRowContext(ctx, tx.Rebind(query), managed.DeleteOperationID(operation)).Scan(
		&job.ID, &job.OperationID, &job.TaskID, &job.Trigger, &job.State, &job.ResourceSnapshot, &job.Attempts,
		&job.NextAttemptAt, &job.LastError, &job.CreatedAt, &job.UpdatedAt, &job.CompletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return job, err
}

func validateDeletionRequest(task *models.Task, request managed.DeleteRequest) error {
	if task == nil || !managed.Matches(task, request.Identity) || !task.CreatedAt.Equal(request.TaskCreatedAt) {
		return managed.ErrNotFound
	}
	if !request.Ordinary && task.Metadata[models.MetaKeyManagedConversationDetached] == true {
		return managed.ErrNotFound
	}
	if !request.Ordinary && managed.Revision(task) != request.ExpectedRevision {
		return managed.ErrRevision
	}
	return nil
}

func (r *Repository) managedDeletionTaskTx(ctx context.Context, tx *sqlx.Tx, request managed.DeleteRequest) (*models.Task, error) {
	if err := r.lockTaskHierarchy(ctx, tx, []string{request.WorkspaceID}, []string{request.TaskID}); err != nil {
		return nil, err
	}
	if err := r.lockTaskStepForWrite(ctx, tx, request.TaskID); err != nil {
		return nil, err
	}
	query := `SELECT ` + taskSelectColumns("t") + ` FROM tasks t WHERE t.id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE OF t`
	}
	task, err := r.scanSingleTask(tx.QueryRowContext(ctx, tx.Rebind(query), request.TaskID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, managed.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return task, validateDeletionRequest(task, request)
}

func (r *Repository) AdmitManagedDeletion(ctx context.Context, request managed.DeleteRequest, owner string) (*managed.DeleteClaim, error) {
	if request.OperationID == "" || request.PayloadDigest == "" || owner == "" {
		return nil, managed.ErrUnavailable
	}
	tx, err := r.db.BeginTxx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	task, err := r.managedDeletionTaskTx(ctx, tx, request)
	if err != nil {
		return nil, err
	}
	if request.Ordinary {
		request.ExpectedRevision = managed.Revision(task)
	}
	if err := r.validateManagedDeletionEligibilityTx(ctx, tx, request.TaskID); err != nil {
		return nil, err
	}
	previous, err := r.managedDeletionJobTx(ctx, tx, request.OperationID)
	if err != nil {
		return nil, err
	}
	if envelope, err := validatePreviousManagedDeletion(previous, request); err != nil {
		return envelope, err
	}
	if err := r.taskCleanupBarrierLocked(ctx, tx, request.TaskID); err != nil {
		return nil, err
	}
	claim := &managed.DeleteClaim{DeleteRequest: request, Version: 1, JobID: uuid.NewString(), Owner: owner, Phase: managed.DeleteReserved}
	if previous != nil {
		claim.JobID = previous.ID
	}
	if err := r.reserveManagedDeletionTx(ctx, tx, *claim, previous); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return claim, tx.Commit()
}

func validatePreviousManagedDeletion(previous *models.TaskResourceCleanupJob, request managed.DeleteRequest) (*managed.DeleteClaim, error) {
	if previous == nil {
		return nil, nil
	}
	envelope, err := managed.DeletionEnvelope(previous.ResourceSnapshot)
	if err != nil || !sameDeletionOperation(envelope, request) || envelope.TaskID != request.TaskID || !envelope.TaskCreatedAt.Equal(request.TaskCreatedAt) || envelope.ExpectedRevision != request.ExpectedRevision {
		return nil, managed.ErrUnavailable
	}
	if previous.State != models.TaskResourceCleanupStateCancelled {
		return envelope, managed.ErrDeletionOwned
	}
	return nil, nil
}

func (r *Repository) validateTaskDeletionTx(ctx context.Context, tx *sqlx.Tx, id string, claim *managed.DeleteClaim) error {
	if claim != nil {
		if err := r.markManagedDeletionTx(ctx, tx, *claim); err != nil {
			return err
		}
	} else if err := r.managedDeletionBarrierTx(ctx, tx, id); err != nil {
		return err
	}
	return r.validateManagedDeletionEligibilityTx(ctx, tx, id)
}

func (r *Repository) validateManagedDeletionEligibilityTx(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	var children bool
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM tasks WHERE parent_id = ?)`), taskID).Scan(&children); err != nil {
		return err
	}
	if children {
		return repoerrors.ErrTaskHierarchyConflict
	}
	return recoveryclaim.EnsureTaskAvailableTx(ctx, r.db, tx, taskID)
}

func (r *Repository) reserveManagedDeletionTx(ctx context.Context, tx *sqlx.Tx, claim managed.DeleteClaim, previous *models.TaskResourceCleanupJob) error {
	snapshot, err := managed.WithDeletionEnvelope(`{}`, claim)
	if err != nil {
		return err
	}
	now := r.nowUTC()
	if previous == nil {
		_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO task_resource_cleanup_jobs (`+taskResourceCleanupColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`), claim.JobID, managed.DeleteOperationID(claim.OperationID), claim.TaskID, models.TaskResourceCleanupTriggerDelete,
			models.TaskResourceCleanupStatePrepared, snapshot, 0, nil, "", now, now, nil)
		return err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_resource_cleanup_jobs SET state = ?, resource_snapshot = ?, completed_at = NULL, next_attempt_at = NULL, last_error = '', updated_at = ? WHERE id = ? AND operation_id = ? AND state = ? AND resource_snapshot = ?`),
		models.TaskResourceCleanupStatePrepared, snapshot, now, previous.ID, previous.OperationID, models.TaskResourceCleanupStateCancelled, previous.ResourceSnapshot)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return managed.ErrDeletionOwned
	}
	return nil
}

func sameDeletionOperation(claim *managed.DeleteClaim, request managed.DeleteRequest) bool {
	return claim != nil && claim.Version == 1 && claim.OperationID == request.OperationID && claim.PayloadDigest == request.PayloadDigest &&
		claim.InstallationID == request.InstallationID && claim.WorkspaceID == request.WorkspaceID && claim.InstanceKey == request.InstanceKey && claim.Ordinary == request.Ordinary && (request.Ordinary || claim.ExpectedRevision == request.ExpectedRevision)
}

func (r *Repository) InspectManagedDeletion(ctx context.Context, request managed.DeleteRequest) (*managed.DeleteClaim, *models.TaskResourceCleanupJob, error) {
	tx, err := r.db.BeginTxx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := r.managedDeletionJobTx(ctx, tx, request.OperationID)
	if err != nil || job == nil {
		return nil, job, err
	}
	claim, err := managed.DeletionEnvelope(job.ResourceSnapshot)
	if err != nil {
		return nil, job, err
	}
	if !sameDeletionOperation(claim, request) || claim.JobID != job.ID || claim.TaskID != job.TaskID || claim.Owner == "" {
		return nil, job, managed.ErrUnavailable
	}
	return claim, job, nil
}

func (r *Repository) validateDeletionOwnerTx(ctx context.Context, tx *sqlx.Tx, expected managed.DeleteClaim) (*models.TaskResourceCleanupJob, *managed.DeleteClaim, error) {
	job, err := r.managedDeletionJobTx(ctx, tx, expected.OperationID)
	if err != nil {
		return nil, nil, err
	}
	if job == nil || job.ID != expected.JobID || job.State != models.TaskResourceCleanupStatePrepared {
		return nil, nil, managed.ErrDeletionOwned
	}
	claim, err := managed.DeletionEnvelope(job.ResourceSnapshot)
	if err != nil {
		return nil, nil, err
	}
	if !sameDeletionOperation(claim, expected.DeleteRequest) || claim.Owner != expected.Owner || claim.JobID != expected.JobID || claim.TaskID != expected.TaskID || !claim.TaskCreatedAt.Equal(expected.TaskCreatedAt) || claim.ExpectedRevision != expected.ExpectedRevision {
		return nil, nil, managed.ErrDeletionOwned
	}
	return job, claim, nil
}

func (r *Repository) PrepareManagedDeletion(ctx context.Context, expected managed.DeleteClaim, snapshot string) (*models.TaskResourceCleanupJob, error) {
	tx, err := r.db.BeginTxx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := r.managedDeletionTaskTx(ctx, tx, expected.DeleteRequest); err != nil {
		return nil, err
	}
	job, claim, err := r.validateDeletionOwnerTx(ctx, tx, expected)
	if err != nil {
		return nil, err
	}
	if claim.Phase != managed.DeleteReserved {
		return nil, managed.ErrDeletionOwned
	}
	if err := r.validateManagedDeletionInventoryTx(ctx, tx, expected, snapshot); err != nil {
		return nil, err
	}
	claim.Phase = managed.DeletePrepared
	encoded, err := managed.WithDeletionEnvelope(snapshot, *claim)
	if err != nil {
		return nil, err
	}
	if err := r.updateDeletionSnapshotTx(ctx, tx, job, encoded); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	job.ResourceSnapshot = encoded
	return job, nil
}

func (r *Repository) updateDeletionSnapshotTx(ctx context.Context, tx *sqlx.Tx, job *models.TaskResourceCleanupJob, snapshot string) error {
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_resource_cleanup_jobs SET resource_snapshot = ?, updated_at = ? WHERE id = ? AND operation_id = ? AND state = ? AND resource_snapshot = ?`),
		snapshot, r.nowUTC(), job.ID, job.OperationID, models.TaskResourceCleanupStatePrepared, job.ResourceSnapshot)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return managed.ErrDeletionOwned
	}
	return nil
}

func (r *Repository) FinalizeManagedDeletion(ctx context.Context, claim managed.DeleteClaim) (string, error) {
	return r.deleteTaskWithVacatedStep(ctx, claim.TaskID, &claim)
}

func (r *Repository) ReleaseManagedDeletion(ctx context.Context, expected managed.DeleteClaim) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := r.managedDeletionTaskTx(ctx, tx, expected.DeleteRequest); err != nil {
		return false, err
	}
	job, claim, err := r.validateDeletionOwnerTx(ctx, tx, expected)
	if err != nil {
		return false, err
	}
	if claim.Phase != managed.DeleteReserved && claim.Phase != managed.DeletePrepared {
		return false, managed.ErrDeletionOwned
	}
	now := r.nowUTC()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_resource_cleanup_jobs SET state = ?, completed_at = ?, updated_at = ? WHERE id = ? AND operation_id = ? AND state = ? AND resource_snapshot = ?`),
		models.TaskResourceCleanupStateCancelled, now, now, job.ID, job.OperationID, models.TaskResourceCleanupStatePrepared, job.ResourceSnapshot)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return false, errors.Join(err, managed.ErrDeletionOwned)
	}
	return true, tx.Commit()
}

func (r *Repository) markManagedDeletionTx(ctx context.Context, tx *sqlx.Tx, expected managed.DeleteClaim) error {
	task, err := r.scanSingleTask(tx.QueryRowContext(ctx, tx.Rebind(`SELECT `+taskSelectColumns("t")+` FROM tasks t WHERE t.id = ?`), expected.TaskID))
	if err != nil {
		return err
	}
	if err := validateDeletionRequest(task, expected.DeleteRequest); err != nil {
		return err
	}
	if managed.Revision(task) != expected.ExpectedRevision {
		return managed.ErrRevision
	}
	job, claim, err := r.validateDeletionOwnerTx(ctx, tx, expected)
	if err != nil {
		return err
	}
	if claim.Phase != managed.DeletePrepared {
		return managed.ErrDeletionOwned
	}
	claim.Phase = managed.DeleteCommitted
	snapshot, err := managed.WithDeletionEnvelope(job.ResourceSnapshot, *claim)
	if err != nil {
		return err
	}
	return r.updateDeletionSnapshotTx(ctx, tx, job, snapshot)
}

var _ managed.DeletionRepository = (*Repository)(nil)

func (r *Repository) validateManagedDeletionInventoryTx(ctx context.Context, tx *sqlx.Tx, claim managed.DeleteClaim, snapshot string) error {
	var inventory struct {
		WorkspaceID string                `json:"workspace_id"`
		Sessions    []*models.TaskSession `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(snapshot), &inventory); err != nil {
		return err
	}
	if inventory.WorkspaceID != claim.WorkspaceID {
		return managed.ErrUnavailable
	}
	var currentIDs []string
	if err := tx.SelectContext(ctx, &currentIDs, tx.Rebind(`SELECT id FROM task_sessions WHERE task_id = ? ORDER BY id`), claim.TaskID); err != nil {
		return err
	}
	capturedIDs := make([]string, 0, len(inventory.Sessions))
	for _, session := range inventory.Sessions {
		if session == nil || session.TaskID != claim.TaskID {
			return managed.ErrUnavailable
		}
		capturedIDs = append(capturedIDs, session.ID)
	}
	slices.Sort(capturedIDs)
	if !slices.Equal(currentIDs, capturedIDs) {
		return managed.ErrUnavailable
	}
	return nil
}

func (r *Repository) cleanupDeletionEnvelope(ctx context.Context, id string) (*managed.DeleteClaim, error) {
	var snapshot string
	err := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT resource_snapshot FROM task_resource_cleanup_jobs WHERE id = ?`), id).Scan(&snapshot)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return managed.DeletionEnvelope(snapshot)
}

func (r *Repository) rejectManagedCleanupMutation(ctx context.Context, id string) error {
	claim, err := r.cleanupDeletionEnvelope(ctx, id)
	if err != nil {
		return err
	}
	if claim != nil {
		return managed.ErrDeletionOwned
	}
	return nil
}

func (r *Repository) validateManagedCleanupActivation(ctx context.Context, id string) error {
	claim, err := r.cleanupDeletionEnvelope(ctx, id)
	if err != nil {
		return err
	}
	if claim != nil && (claim.Version != 1 || claim.JobID != id || claim.Phase != managed.DeleteCommitted) {
		return managed.ErrDeletionOwned
	}
	return nil
}

func (r *Repository) rejectOtherCleanupJobsTx(ctx context.Context, tx *sqlx.Tx, taskID, jobID string) error {
	var active bool
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM task_resource_cleanup_jobs WHERE task_id = ? AND id <> ? AND state IN (?, ?, ?, ?, ?))`), taskID, jobID,
		models.TaskResourceCleanupStatePrepared, models.TaskResourceCleanupStatePending, models.TaskResourceCleanupStateRunning, models.TaskResourceCleanupStateRetryWait, models.TaskResourceCleanupStateWaitingForClean).Scan(&active)
	if err != nil {
		return err
	}
	if active {
		return repoerrors.ErrTaskCleanupInProgress
	}
	return nil
}

// managedDeletionBarrierTx protects canonical writers without changing ordinary
// cleanup authority. The task lock precedes the cleanup-row observation.
func (r *Repository) managedDeletionBarrierTx(ctx context.Context, tx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, taskID string) error {
	if taskID == "" {
		return nil
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		var id string
		err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT id FROM tasks WHERE id = ? FOR UPDATE`), taskID).Scan(&id)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	} else if _, err := tx.ExecContext(ctx, `UPDATE tasks SET id = id WHERE 0`); err != nil {
		return err
	}
	var active bool
	marker := dialect.JSONExtract(r.db.DriverName(), "COALESCE(NULLIF(resource_snapshot, ''), '{}')", "managed_delete")
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT EXISTS (SELECT 1 FROM task_resource_cleanup_jobs WHERE task_id = ? AND state IN (?, ?, ?, ?, ?) AND `+marker+` IS NOT NULL)`), taskID,
		models.TaskResourceCleanupStatePrepared, models.TaskResourceCleanupStatePending, models.TaskResourceCleanupStateRunning, models.TaskResourceCleanupStateRetryWait, models.TaskResourceCleanupStateWaitingForClean).Scan(&active)
	if err != nil {
		return err
	}
	if active {
		return fmt.Errorf("%w: retained task %s has an exclusive deletion owner", repoerrors.ErrTaskCleanupInProgress, taskID)
	}
	return nil
}
