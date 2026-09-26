package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

func reserveManagedAgentOperationTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	operation *models.ManagedAgentOperation,
	expectedBindingRevision int64,
	leaseOwner string,
	leaseUntil time.Time,
) (*models.ManagedAgentBinding, *models.ManagedAgentOperation, bool, error) {
	queryBinding := `SELECT ` + managedAgentBindingColumns + ` FROM managed_agent_bindings WHERE id = ?` + managedAgentLockSuffix(db.DriverName())
	binding, err := scanManagedAgentBinding(tx.QueryRowContext(ctx, db.Rebind(queryBinding), operation.BindingID))
	if err != nil {
		return nil, nil, false, fmt.Errorf("load managed agent binding: %w", err)
	}
	existing, replayed, err := readManagedAgentOperationReplay(ctx, tx, db, operation)
	if err != nil {
		return nil, nil, false, err
	}
	if replayed {
		if err := validateManagedAgentOperationReplay(ctx, tx, db, binding, existing); err != nil {
			return nil, nil, false, err
		}
		return binding, existing, true, nil
	}
	now := time.Now().UTC()
	if err := validateManagedAgentOperationReservation(ctx, tx, db, binding, operation, expectedBindingRevision, leaseOwner, now); err != nil {
		return nil, nil, false, err
	}
	return persistManagedAgentOperationReservation(ctx, tx, db, binding, operation, expectedBindingRevision, leaseOwner, leaseUntil, now)
}

func validateManagedAgentOperationReplay(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	binding *models.ManagedAgentBinding,
	existing *models.ManagedAgentOperation,
) error {
	if existing.State != models.ManagedAgentSubmissionReserved {
		return nil
	}
	if binding.Lifecycle == models.ManagedAgentBindingArchived || binding.Lifecycle == models.ManagedAgentBindingTerminationPending {
		return ErrManagedAgentBindingConflict
	}
	archived, err := managedAgentTaskArchived(ctx, tx, db, binding.TaskID)
	if err != nil {
		return err
	}
	if archived {
		return ErrManagedAgentBindingConflict
	}
	return nil
}

func validateManagedAgentOperationReservation(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	expectedBindingRevision int64,
	leaseOwner string,
	now time.Time,
) error {
	archived, err := managedAgentTaskArchived(ctx, tx, db, binding.TaskID)
	if err != nil {
		return err
	}
	if archived {
		return ErrManagedAgentBindingConflict
	}
	if managedAgentBindingCannotDispatch(binding, operation) {
		return ErrManagedAgentBindingConflict
	}
	if binding.Revision != expectedBindingRevision {
		return ErrManagedAgentRevisionConflict
	}
	if err := validateManagedAgentRetryAcknowledgment(ctx, tx, db, binding, operation, now); err != nil {
		return err
	}
	if operation.RetryAcknowledgesOperationID != "" &&
		(binding.Lifecycle == models.ManagedAgentBindingArchived || binding.Lifecycle == models.ManagedAgentBindingTerminationPending) {
		binding.Lifecycle = models.ManagedAgentBindingReady
	}
	if managedAgentLeaseHeldByOther(binding, leaseOwner, now) {
		return ErrManagedAgentLeaseHeld
	}
	return nil
}

func managedAgentBindingCannotDispatch(binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation) bool {
	return (binding.Lifecycle == models.ManagedAgentBindingArchived ||
		binding.Lifecycle == models.ManagedAgentBindingTerminationPending) && operation.RetryAcknowledgesOperationID == ""
}

func validateManagedAgentRetryAcknowledgment(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	now time.Time,
) error {
	active, err := hasManagedAgentActiveOperation(ctx, tx, db, binding.ID)
	if err != nil {
		return err
	}
	if !active {
		if operation.RetryAcknowledgesOperationID != "" {
			return ErrManagedAgentOperationNotFound
		}
		return nil
	}
	if operation.RetryAcknowledgesOperationID == "" {
		return ErrManagedAgentActiveOperation
	}
	if err := acknowledgeManagedAgentUnknownRetryTx(
		ctx, tx, db, binding.ID, operation.RetryAcknowledgesOperationID, operation.Kind, now,
	); err != nil {
		return err
	}
	active, err = hasManagedAgentActiveOperation(ctx, tx, db, binding.ID)
	if err != nil {
		return err
	}
	if active {
		return ErrManagedAgentActiveOperation
	}
	return nil
}

func persistManagedAgentOperationReservation(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	expectedBindingRevision int64,
	leaseOwner string,
	leaseUntil time.Time,
	now time.Time,
) (*models.ManagedAgentBinding, *models.ManagedAgentOperation, bool, error) {
	binding.Revision++
	binding.DispatchGeneration++
	binding.DispatchOwner = leaseOwner
	binding.DispatchLeaseUntil = &leaseUntil
	binding.UpdatedAt = now
	result, err := tx.ExecContext(ctx, db.Rebind(`
		UPDATE managed_agent_bindings SET revision = ?, lifecycle = ?, dispatch_generation = ?, dispatch_owner = ?,
			dispatch_lease_until = ?, updated_at = ? WHERE id = ? AND revision = ?
	`), binding.Revision, binding.Lifecycle, binding.DispatchGeneration, binding.DispatchOwner, binding.DispatchLeaseUntil,
		binding.UpdatedAt, binding.ID, expectedBindingRevision)
	if err != nil {
		return nil, nil, false, fmt.Errorf("claim managed agent dispatch generation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, nil, false, fmt.Errorf("confirm managed agent dispatch generation: %w", err)
	}
	if rows != 1 {
		return nil, nil, false, ErrManagedAgentRevisionConflict
	}
	operation.State = models.ManagedAgentSubmissionReserved
	operation.DispatchGeneration = binding.DispatchGeneration
	operation.Revision = 1
	operation.UpdatedAt = now
	if operation.CreatedAt.IsZero() {
		operation.CreatedAt = now
	}
	if err := insertManagedAgentOperation(ctx, tx, db, operation); err != nil {
		if managedAgentUniqueViolation(err) {
			return nil, nil, false, ErrManagedAgentOperationConflict
		}
		return nil, nil, false, fmt.Errorf("insert managed agent operation: %w", err)
	}
	return binding, operation, false, nil
}
