package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) ReserveManagedAgentStart(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	leaseOwner string,
	leaseUntil time.Time,
) (*models.ManagedAgentBinding, *models.ManagedAgentOperation, bool, error) {
	if err := validateManagedAgentBinding(binding); err != nil {
		return nil, nil, false, err
	}
	if err := validateManagedAgentOperation(operation); err != nil {
		return nil, nil, false, err
	}
	if operation.BindingID != binding.ID || operation.Kind != models.ManagedAgentOperationCreate ||
		binding.Lifecycle != models.ManagedAgentBindingCreating || leaseOwner == "" || !leaseUntil.After(time.Now()) {
		return nil, nil, false, fmt.Errorf("managed agent start reservation is invalid")
	}
	prepareManagedAgentStart(binding, operation, leaseOwner, leaseUntil)

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, false, fmt.Errorf("begin managed agent start reservation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	reservedBinding, reservedOperation, replayed, err := reserveManagedAgentStartTx(ctx, tx, r.db, binding, operation)
	if err != nil && managedAgentUniqueViolation(err) {
		_ = tx.Rollback()
		return r.resolveManagedAgentStartRace(ctx, binding, operation)
	}
	if err != nil {
		return nil, nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, false, fmt.Errorf("commit managed agent start reservation: %w", err)
	}
	return reservedBinding, reservedOperation, replayed, nil
}

func prepareManagedAgentStart(binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation, owner string, leaseUntil time.Time) {
	now := time.Now().UTC()
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = now
	}
	binding.UpdatedAt = now
	binding.Revision = 1
	binding.DispatchGeneration = 1
	binding.DispatchOwner = owner
	binding.DispatchLeaseUntil = &leaseUntil
	operation.State = models.ManagedAgentSubmissionReserved
	operation.DispatchGeneration = binding.DispatchGeneration
	operation.Revision = 1
	operation.UpdatedAt = now
	if operation.CreatedAt.IsZero() {
		operation.CreatedAt = now
	}
}

func reserveManagedAgentStartTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
) (*models.ManagedAgentBinding, *models.ManagedAgentOperation, bool, error) {
	existing, existingOperation, replayed, err := readManagedAgentStartReplay(ctx, tx, db, binding, operation)
	if err != nil || replayed {
		return existing, existingOperation, replayed, err
	}
	archived, err := managedAgentTaskArchived(ctx, tx, db, binding.TaskID)
	if err != nil {
		return nil, nil, false, err
	}
	if archived {
		return nil, nil, false, ErrManagedAgentBindingConflict
	}
	if err := insertManagedAgentBinding(ctx, tx, db, binding); err != nil {
		return nil, nil, false, fmt.Errorf("insert managed agent binding: %w", err)
	}
	if err := insertManagedAgentOperation(ctx, tx, db, operation); err != nil {
		if managedAgentUniqueViolation(err) {
			return nil, nil, false, ErrManagedAgentOperationConflict
		}
		return nil, nil, false, fmt.Errorf("insert managed agent create operation: %w", err)
	}
	return binding, operation, false, nil
}

func readManagedAgentStartReplay(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
) (*models.ManagedAgentBinding, *models.ManagedAgentOperation, bool, error) {
	query := `SELECT ` + managedAgentBindingColumns + ` FROM managed_agent_bindings WHERE session_id = ?` + managedAgentLockSuffix(db.DriverName())
	existing, err := scanManagedAgentBinding(tx.QueryRowContext(ctx, db.Rebind(query), binding.SessionID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, false, nil
	}
	if err != nil {
		return nil, nil, false, fmt.Errorf("read managed agent start binding: %w", err)
	}
	if !sameManagedAgentLaunch(existing, binding) {
		return nil, nil, false, ErrManagedAgentBindingConflict
	}
	existingOperation, err := scanManagedAgentOperation(tx.QueryRowContext(ctx, db.Rebind(
		`SELECT `+managedAgentOperationColumns+` FROM managed_agent_operations WHERE prompt_turn_id = ?`,
	), operation.PromptTurnID))
	if err != nil || existingOperation.BindingID != existing.ID || existingOperation.Kind != operation.Kind ||
		existingOperation.RequestDigest != operation.RequestDigest {
		return nil, nil, false, ErrManagedAgentBindingConflict
	}
	return existing, existingOperation, true, nil
}

func (r *Repository) resolveManagedAgentStartRace(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
) (*models.ManagedAgentBinding, *models.ManagedAgentOperation, bool, error) {
	existing, err := r.GetManagedAgentBindingBySession(ctx, binding.SessionID)
	if err != nil {
		return nil, nil, false, ErrManagedAgentBindingConflict
	}
	existingOperation, err := r.GetManagedAgentOperationByPromptTurnID(ctx, operation.PromptTurnID)
	if err != nil || !sameManagedAgentLaunch(existing, binding) || existingOperation.BindingID != existing.ID ||
		existingOperation.Kind != operation.Kind || existingOperation.RequestDigest != operation.RequestDigest {
		return nil, nil, false, ErrManagedAgentBindingConflict
	}
	return existing, existingOperation, true, nil
}

func insertManagedAgentBinding(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, binding *models.ManagedAgentBinding) error {
	_, err := tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO managed_agent_bindings (
			id, session_id, task_id, workspace_id, user_id, execution_id, provider_kind, executor_id,
			executor_profile_id, credential_ref, remote_agent_id, lifecycle, repository_id, repository_url,
			starting_ref, model, callback_url, auto_create_pr, revision, dispatch_generation, dispatch_owner,
			dispatch_lease_until, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), binding.ID, binding.SessionID, binding.TaskID, binding.WorkspaceID, binding.UserID, binding.ExecutionID,
		binding.ProviderKind, binding.ExecutorID, binding.ExecutorProfileID, binding.CredentialRef,
		binding.RemoteAgentID, binding.Lifecycle, binding.Launch.RepositoryID, binding.Launch.RepositoryURL,
		binding.Launch.StartingRef, binding.Launch.Model, binding.Launch.CallbackURL,
		managedAgentBool(binding.Launch.AutoCreatePR), binding.Revision, binding.DispatchGeneration, binding.DispatchOwner,
		binding.DispatchLeaseUntil, binding.CreatedAt, binding.UpdatedAt)
	return err
}

func insertManagedAgentOperation(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, operation *models.ManagedAgentOperation) error {
	requestSnapshot, err := json.Marshal(operation.RequestSnapshot)
	if err != nil {
		return fmt.Errorf("encode managed agent request snapshot: %w", err)
	}
	resultSnapshot, err := json.Marshal(operation.ResultSnapshot)
	if err != nil {
		return fmt.Errorf("encode managed agent result snapshot: %w", err)
	}
	_, err = tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO managed_agent_operations (
			id, binding_id, prompt_turn_id, operation_kind, request_digest, request_snapshot,
			submission_state, remote_run_id, pre_submit_run_id, dispatch_generation, revision,
			created_at, dispatch_started_at, accepted_at, settled_at, updated_at, sanitized_error, result_snapshot,
			completion_pending
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), operation.ID, operation.BindingID, operation.PromptTurnID, operation.Kind, operation.RequestDigest,
		string(requestSnapshot), operation.State, operation.RemoteRunID, operation.PreSubmitRunID,
		operation.DispatchGeneration, operation.Revision, operation.CreatedAt, operation.DispatchStartedAt,
		operation.AcceptedAt, operation.SettledAt, operation.UpdatedAt, operation.SanitizedError, string(resultSnapshot),
		managedAgentBool(operation.CompletionPending))
	return err
}

func (r *Repository) ReserveManagedAgentOperation(
	ctx context.Context,
	operation *models.ManagedAgentOperation,
	expectedBindingRevision int64,
	leaseOwner string,
	leaseUntil time.Time,
) (*models.ManagedAgentBinding, *models.ManagedAgentOperation, bool, error) {
	if err := validateManagedAgentOperation(operation); err != nil {
		return nil, nil, false, err
	}
	if operation.Kind != models.ManagedAgentOperationFollowup || leaseOwner == "" || !leaseUntil.After(time.Now()) {
		return nil, nil, false, fmt.Errorf("managed agent operation reservation is invalid")
	}
	if operation.DuplicationRiskAcknowledged != (operation.RetryAcknowledgesOperationID != "") {
		return nil, nil, false, fmt.Errorf("managed agent retry acknowledgment is invalid")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, false, fmt.Errorf("begin managed agent operation reservation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	binding, reserved, replayed, err := reserveManagedAgentOperationTx(
		ctx, tx, r.db, operation, expectedBindingRevision, leaseOwner, leaseUntil,
	)
	if err != nil {
		return nil, nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, false, fmt.Errorf("commit managed agent operation reservation: %w", err)
	}
	return binding, reserved, replayed, nil
}

func acknowledgeManagedAgentUnknownRetryTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	bindingID, operationID string,
	at time.Time,
) error {
	query := `SELECT operation_kind, submission_state FROM managed_agent_operations WHERE id = ? AND binding_id = ?` + managedAgentLockSuffix(db.DriverName())
	var kind models.ManagedAgentOperationKind
	var state models.ManagedAgentSubmissionState
	if err := tx.QueryRowContext(ctx, db.Rebind(query), operationID, bindingID).Scan(&kind, &state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrManagedAgentOperationNotFound
		}
		return fmt.Errorf("load uncertain operation for acknowledged retry: %w", err)
	}
	if kind != models.ManagedAgentOperationFollowup ||
		state != models.ManagedAgentSubmissionUnknown && state != models.ManagedAgentSubmissionSubmitting {
		return ErrManagedAgentOperationTransition
	}
	result, err := tx.ExecContext(ctx, db.Rebind(`
		UPDATE managed_agent_operations
		SET submission_state = ?, revision = revision + 1, settled_at = ?, updated_at = ?,
			sanitized_error = ?
		WHERE id = ? AND binding_id = ? AND submission_state IN (?, ?)
	`), models.ManagedAgentSubmissionRetryAcked, at, at,
		"User acknowledged that retrying may create duplicate remote work.", operationID, bindingID,
		models.ManagedAgentSubmissionUnknown, models.ManagedAgentSubmissionSubmitting)
	if err != nil {
		return fmt.Errorf("record explicit Cursor Cloud retry acknowledgment: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm explicit Cursor Cloud retry acknowledgment: %w", err)
	}
	if rows != 1 {
		return ErrManagedAgentRevisionConflict
	}
	return nil
}

func readManagedAgentOperationReplay(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	operation *models.ManagedAgentOperation,
) (*models.ManagedAgentOperation, bool, error) {
	existing, err := scanManagedAgentOperation(tx.QueryRowContext(ctx, db.Rebind(
		`SELECT `+managedAgentOperationColumns+` FROM managed_agent_operations WHERE prompt_turn_id = ?`,
	), operation.PromptTurnID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read managed agent operation identity: %w", err)
	}
	if existing.BindingID != operation.BindingID || existing.Kind != operation.Kind || existing.RequestDigest != operation.RequestDigest {
		return nil, false, ErrManagedAgentOperationConflict
	}
	return existing, true, nil
}

func hasManagedAgentActiveOperation(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, bindingID string) (bool, error) {
	var operationID string
	err := tx.QueryRowContext(ctx, db.Rebind(`
	SELECT id FROM managed_agent_operations
		WHERE binding_id = ? AND (submission_state IN ('reserved', 'submitting', 'accepted', 'cancelling', 'unknown') OR completion_pending = 1)
		LIMIT 1
	`), bindingID).Scan(&operationID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check active managed agent operation: %w", err)
	}
	return true, nil
}

func managedAgentTaskArchived(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, taskID string) (bool, error) {
	var archivedAt sql.NullTime
	query := "SELECT archived_at FROM tasks WHERE id = ?" + managedAgentLockSuffix(db.DriverName())
	err := tx.QueryRowContext(ctx, db.Rebind(query), taskID).Scan(&archivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("managed agent task %s was not found", taskID)
	}
	if err != nil {
		return false, fmt.Errorf("read task archive state for managed agent: %w", err)
	}
	return archivedAt.Valid, nil
}

func transitionManagedAgentBindingLifecycleTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	operation *models.ManagedAgentOperation,
	at time.Time,
) error {
	var (
		query string
		args  []any
	)
	switch {
	case operation.Kind == models.ManagedAgentOperationCreate && operation.State == models.ManagedAgentSubmissionAccepted:
		query = "UPDATE managed_agent_bindings SET lifecycle = ?, revision = revision + 1, updated_at = ? WHERE id = ? AND lifecycle = ?"
		args = []any{models.ManagedAgentBindingReady, at, operation.BindingID, models.ManagedAgentBindingCreating}
	case !managedAgentOperationActive(operation.State):
		query = "UPDATE managed_agent_bindings SET lifecycle = ?, revision = revision + 1, updated_at = ? WHERE id = ? AND lifecycle = ?"
		args = []any{models.ManagedAgentBindingArchived, at, operation.BindingID, models.ManagedAgentBindingTerminationPending}
	default:
		return nil
	}
	if _, err := tx.ExecContext(ctx, db.Rebind(query), args...); err != nil {
		return fmt.Errorf("transition managed agent binding lifecycle: %w", err)
	}
	return nil
}

func managedAgentLeaseHeldByOther(binding *models.ManagedAgentBinding, owner string, now time.Time) bool {
	return binding.DispatchOwner != "" && binding.DispatchLeaseUntil != nil &&
		binding.DispatchLeaseUntil.After(now) && binding.DispatchOwner != owner
}

func (r *Repository) ClaimManagedAgentDispatchLease(
	ctx context.Context,
	bindingID, operationID string,
	expectedBindingRevision int64,
	leaseOwner string,
	leaseUntil time.Time,
) (*models.ManagedAgentBinding, error) {
	if bindingID == "" || operationID == "" || leaseOwner == "" || !leaseUntil.After(time.Now()) {
		return nil, fmt.Errorf("managed agent dispatch lease claim is invalid")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin managed agent dispatch lease claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	binding, err := managedAgentLeaseCandidate(ctx, tx, r.db, bindingID, operationID, expectedBindingRevision)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if managedAgentLeaseHeldByOther(binding, leaseOwner, now) {
		return nil, ErrManagedAgentLeaseHeld
	}
	binding.Revision++
	binding.DispatchOwner = leaseOwner
	binding.DispatchLeaseUntil = &leaseUntil
	binding.UpdatedAt = now
	if err := updateManagedAgentLeaseTx(ctx, tx, r.db, binding, expectedBindingRevision); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit managed agent dispatch lease: %w", err)
	}
	return binding, nil
}

func managedAgentLeaseCandidate(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	bindingID, operationID string,
	expectedRevision int64,
) (*models.ManagedAgentBinding, error) {
	query := `SELECT ` + managedAgentBindingColumns + ` FROM managed_agent_bindings WHERE id = ?` + managedAgentLockSuffix(db.DriverName())
	binding, err := scanManagedAgentBinding(tx.QueryRowContext(ctx, db.Rebind(query), bindingID))
	if err != nil {
		return nil, fmt.Errorf("load binding for managed agent lease: %w", err)
	}
	if binding.Revision != expectedRevision {
		return nil, ErrManagedAgentRevisionConflict
	}
	if binding.Lifecycle == models.ManagedAgentBindingArchived || binding.Lifecycle == models.ManagedAgentBindingTerminationPending {
		return nil, ErrManagedAgentBindingConflict
	}
	archived, err := managedAgentTaskArchived(ctx, tx, db, binding.TaskID)
	if err != nil {
		return nil, err
	}
	if archived {
		return nil, ErrManagedAgentBindingConflict
	}
	var state models.ManagedAgentSubmissionState
	err = tx.QueryRowContext(ctx, db.Rebind(`
		SELECT submission_state FROM managed_agent_operations WHERE id = ? AND binding_id = ?
	`), operationID, bindingID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !managedAgentOperationActive(state) {
		return nil, ErrManagedAgentOperationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load operation for managed agent lease: %w", err)
	}
	return binding, nil
}

func updateManagedAgentLeaseTx(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, binding *models.ManagedAgentBinding, expectedRevision int64) error {
	result, err := tx.ExecContext(ctx, db.Rebind(`
		UPDATE managed_agent_bindings SET revision = ?, dispatch_owner = ?, dispatch_lease_until = ?, updated_at = ?
		WHERE id = ? AND revision = ?
	`), binding.Revision, binding.DispatchOwner, binding.DispatchLeaseUntil, binding.UpdatedAt, binding.ID, expectedRevision)
	if err != nil {
		return fmt.Errorf("update managed agent dispatch lease: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm managed agent dispatch lease: %w", err)
	}
	if rows != 1 {
		return ErrManagedAgentRevisionConflict
	}
	return nil
}

func (r *Repository) GetManagedAgentBindingBySession(ctx context.Context, sessionID string) (*models.ManagedAgentBinding, error) {
	binding, err := scanManagedAgentBinding(r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+managedAgentBindingColumns+` FROM managed_agent_bindings WHERE session_id = ?`,
	), sessionID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrManagedAgentBindingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get managed agent binding by session: %w", err)
	}
	return binding, nil
}

func (r *Repository) GetManagedAgentBindingByExecution(ctx context.Context, executionID string) (*models.ManagedAgentBinding, error) {
	binding, err := scanManagedAgentBinding(r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+managedAgentBindingColumns+` FROM managed_agent_bindings WHERE execution_id = ?`,
	), executionID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrManagedAgentBindingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get managed agent binding by execution: %w", err)
	}
	return binding, nil
}

func (r *Repository) GetManagedAgentBinding(ctx context.Context, bindingID string) (*models.ManagedAgentBinding, error) {
	query := `SELECT ` + managedAgentBindingColumns + ` FROM managed_agent_bindings WHERE id = ?`
	binding, err := scanManagedAgentBinding(r.ro.QueryRowContext(ctx, r.ro.Rebind(query), bindingID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrManagedAgentBindingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get managed agent binding: %w", err)
	}
	return binding, nil
}

func (r *Repository) GetManagedAgentOperationByPromptTurnID(ctx context.Context, promptTurnID string) (*models.ManagedAgentOperation, error) {
	operation, err := scanManagedAgentOperation(r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+managedAgentOperationColumns+` FROM managed_agent_operations WHERE prompt_turn_id = ?`,
	), promptTurnID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrManagedAgentOperationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get managed agent operation by prompt turn: %w", err)
	}
	return operation, nil
}

func (r *Repository) GetManagedAgentOperation(ctx context.Context, operationID string) (*models.ManagedAgentOperation, error) {
	query := `SELECT ` + managedAgentOperationColumns + ` FROM managed_agent_operations WHERE id = ?`
	operation, err := scanManagedAgentOperation(r.ro.QueryRowContext(ctx, r.ro.Rebind(query), operationID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrManagedAgentOperationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get managed agent operation: %w", err)
	}
	return operation, nil
}

func (r *Repository) GetManagedAgentLatestOperation(ctx context.Context, bindingID string) (*models.ManagedAgentOperation, error) {
	operation, err := scanManagedAgentOperation(r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+managedAgentOperationColumns+` FROM managed_agent_operations WHERE binding_id = ? ORDER BY dispatch_generation DESC LIMIT 1`,
	), bindingID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrManagedAgentOperationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get latest managed agent operation: %w", err)
	}
	return operation, nil
}

func (r *Repository) ListActiveManagedAgentBindings(ctx context.Context) ([]*models.ManagedAgentBinding, error) {
	rows, err := r.ro.QueryxContext(ctx, `
		SELECT `+managedAgentBindingColumns+`
		FROM managed_agent_bindings b
		WHERE EXISTS (
			SELECT 1 FROM managed_agent_operations o
			WHERE o.binding_id = b.id
				AND (o.submission_state IN ('reserved', 'submitting', 'accepted', 'cancelling', 'unknown') OR o.completion_pending = 1)
		)
		ORDER BY b.updated_at, b.id
	`)
	if err != nil {
		return nil, fmt.Errorf("list active managed agent bindings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	bindings := make([]*models.ManagedAgentBinding, 0)
	for rows.Next() {
		binding, scanErr := scanManagedAgentBinding(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan active managed agent binding: %w", scanErr)
		}
		bindings = append(bindings, binding)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read active managed agent bindings: %w", err)
	}
	return bindings, nil
}

func (r *Repository) AcknowledgeManagedAgentCompletion(ctx context.Context, operationID string) error {
	if operationID == "" {
		return fmt.Errorf("managed agent completion operation ID is required")
	}
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE managed_agent_operations SET completion_pending = 0, revision = revision + 1, updated_at = ?
		WHERE id = ? AND completion_pending = 1 AND submission_state IN ('succeeded', 'failed', 'cancelled')
	`), now, operationID)
	if err != nil {
		return fmt.Errorf("acknowledge managed agent completion: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm managed agent completion acknowledgement: %w", err)
	}
	if rows == 1 {
		return nil
	}
	operation, err := r.GetManagedAgentOperation(ctx, operationID)
	if err != nil {
		return err
	}
	if models.ManagedAgentOperationTerminal(operation.State) && !operation.CompletionPending {
		return nil
	}
	return fmt.Errorf("managed agent completion is not pending for operation %q", operationID)
}

func (r *Repository) DeleteManagedAgentBindingIfTerminal(ctx context.Context, bindingID string) error {
	if bindingID == "" {
		return fmt.Errorf("managed agent binding ID is required")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin managed agent binding deletion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	bindingQuery := "SELECT " + managedAgentBindingColumns + " FROM managed_agent_bindings WHERE id = ?" + managedAgentLockSuffix(r.db.DriverName())
	if _, err := scanManagedAgentBinding(tx.QueryRowContext(ctx, r.db.Rebind(bindingQuery), bindingID)); errors.Is(err, sql.ErrNoRows) {
		return ErrManagedAgentBindingNotFound
	} else if err != nil {
		return fmt.Errorf("load managed agent binding for deletion: %w", err)
	}
	operationQuery := "SELECT submission_state, completion_pending FROM managed_agent_operations WHERE binding_id = ? ORDER BY dispatch_generation DESC LIMIT 1" + managedAgentLockSuffix(r.db.DriverName())
	var state models.ManagedAgentSubmissionState
	var completionPending int
	err = tx.QueryRowContext(ctx, r.db.Rebind(operationQuery), bindingID).Scan(&state, &completionPending)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrManagedAgentOperationNotFound
	}
	if err != nil {
		return fmt.Errorf("load managed agent operation for deletion: %w", err)
	}
	if managedAgentOperationActive(state) || completionPending != 0 {
		return ErrManagedAgentActiveOperation
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind("DELETE FROM managed_agent_bindings WHERE id = ?"), bindingID)
	if err != nil {
		return fmt.Errorf("delete terminal managed agent binding: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm managed agent binding deletion: %w", err)
	}
	if rows != 1 {
		return ErrManagedAgentBindingNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit managed agent binding deletion: %w", err)
	}
	return nil
}

func (r *Repository) CompareAndSwapManagedAgentOperation(
	ctx context.Context,
	update models.ManagedAgentOperationUpdate,
) (*models.ManagedAgentOperation, error) {
	if update.OperationID == "" || update.State == "" {
		return nil, fmt.Errorf("managed agent operation update is incomplete")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin managed agent operation update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	operation, err := loadManagedAgentOperationForUpdate(ctx, tx, r.db, update.OperationID)
	if err != nil {
		return nil, err
	}
	if err := validateManagedAgentOperationUpdate(ctx, tx, r.db, operation, update); err != nil {
		return nil, err
	}
	if managedAgentUpdateIsNoop(operation, update) {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit managed agent operation no-op: %w", err)
		}
		return operation, nil
	}
	now := time.Now().UTC()
	applyManagedAgentOperationUpdate(operation, update, now)
	if err := persistManagedAgentOperationUpdate(ctx, tx, r.db, operation, update.ExpectedRevision); err != nil {
		return nil, err
	}
	if operation.State == models.ManagedAgentSubmissionAccepted || operation.State == models.ManagedAgentSubmissionUnknown ||
		!managedAgentOperationActive(operation.State) {
		if err := releaseManagedAgentDispatchLeaseTx(ctx, tx, r.db, operation.BindingID, now); err != nil {
			return nil, err
		}
	}
	if err := transitionManagedAgentBindingLifecycleTx(ctx, tx, r.db, operation, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit managed agent operation update: %w", err)
	}
	return operation, nil
}

func loadManagedAgentOperationForUpdate(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, id string) (*models.ManagedAgentOperation, error) {
	query := `SELECT ` + managedAgentOperationColumns + ` FROM managed_agent_operations WHERE id = ?` + managedAgentLockSuffix(db.DriverName())
	operation, err := scanManagedAgentOperation(tx.QueryRowContext(ctx, db.Rebind(query), id))
	if err != nil {
		return nil, fmt.Errorf("load managed agent operation for update: %w", err)
	}
	return operation, nil
}

func validateManagedAgentOperationUpdate(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	operation *models.ManagedAgentOperation,
	update models.ManagedAgentOperationUpdate,
) error {
	if operation.Revision != update.ExpectedRevision {
		return ErrManagedAgentRevisionConflict
	}
	if update.RemoteRunID != "" && operation.RemoteRunID != "" && operation.RemoteRunID != update.RemoteRunID {
		return ErrManagedAgentStreamIdentityConflict
	}
	if !validManagedAgentTransition(operation.State, update.State) {
		return ErrManagedAgentOperationTransition
	}
	if operation.State != models.ManagedAgentSubmissionReserved && operation.State != models.ManagedAgentSubmissionSubmitting {
		return nil
	}
	return validateManagedAgentDispatchAuthority(ctx, tx, db, operation.BindingID, update)
}

func validateManagedAgentDispatchAuthority(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	bindingID string,
	update models.ManagedAgentOperationUpdate,
) error {
	query := `SELECT ` + managedAgentBindingColumns + ` FROM managed_agent_bindings WHERE id = ?` + managedAgentLockSuffix(db.DriverName())
	binding, err := scanManagedAgentBinding(tx.QueryRowContext(ctx, db.Rebind(query), bindingID))
	if err != nil {
		return fmt.Errorf("load managed agent binding for dispatch update: %w", err)
	}
	if binding.Revision != update.ExpectedBindingRevision {
		return ErrManagedAgentRevisionConflict
	}
	if update.LeaseOwner == "" || binding.DispatchOwner != update.LeaseOwner || binding.DispatchLeaseUntil == nil ||
		!binding.DispatchLeaseUntil.After(time.Now()) {
		return ErrManagedAgentLeaseHeld
	}
	return nil
}

func managedAgentUpdateIsNoop(operation *models.ManagedAgentOperation, update models.ManagedAgentOperationUpdate) bool {
	return operation.State == update.State && (update.RemoteRunID == "" || update.RemoteRunID == operation.RemoteRunID) &&
		(update.PreSubmitRunID == "" || update.PreSubmitRunID == operation.PreSubmitRunID) &&
		update.SanitizedError == operation.SanitizedError && (update.ResultSnapshot == nil || *update.ResultSnapshot == operation.ResultSnapshot) &&
		(update.CompletionPending == nil || *update.CompletionPending == operation.CompletionPending)
}

func applyManagedAgentOperationUpdate(operation *models.ManagedAgentOperation, update models.ManagedAgentOperationUpdate, now time.Time) {
	operation.State = update.State
	if update.RemoteRunID != "" {
		operation.RemoteRunID = update.RemoteRunID
	}
	if update.PreSubmitRunID != "" {
		operation.PreSubmitRunID = update.PreSubmitRunID
	}
	operation.SanitizedError = update.SanitizedError
	if update.ResultSnapshot != nil {
		operation.ResultSnapshot = *update.ResultSnapshot
	}
	if update.CompletionPending != nil {
		operation.CompletionPending = *update.CompletionPending
	}
	operation.Revision++
	operation.UpdatedAt = now
	if update.State == models.ManagedAgentSubmissionAccepted && operation.AcceptedAt == nil {
		operation.AcceptedAt = &now
	}
	if update.State == models.ManagedAgentSubmissionSubmitting && operation.DispatchStartedAt == nil {
		operation.DispatchStartedAt = &now
	}
	if !managedAgentOperationActive(update.State) && operation.SettledAt == nil {
		operation.SettledAt = &now
	}
}

func persistManagedAgentOperationUpdate(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	operation *models.ManagedAgentOperation,
	expectedRevision int64,
) error {
	resultSnapshot, err := json.Marshal(operation.ResultSnapshot)
	if err != nil {
		return fmt.Errorf("encode managed agent result snapshot: %w", err)
	}
	result, err := tx.ExecContext(ctx, db.Rebind(`
		UPDATE managed_agent_operations SET submission_state = ?, remote_run_id = ?, pre_submit_run_id = ?, revision = ?,
			dispatch_started_at = ?, accepted_at = ?, settled_at = ?, updated_at = ?, sanitized_error = ?, result_snapshot = ?, completion_pending = ?
		WHERE id = ? AND revision = ?
	`), operation.State, operation.RemoteRunID, operation.PreSubmitRunID, operation.Revision, operation.DispatchStartedAt,
		operation.AcceptedAt, operation.SettledAt, operation.UpdatedAt, operation.SanitizedError, string(resultSnapshot), managedAgentBool(operation.CompletionPending),
		operation.ID, expectedRevision)
	if err != nil {
		return fmt.Errorf("update managed agent operation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm managed agent operation update: %w", err)
	}
	if rows != 1 {
		return ErrManagedAgentRevisionConflict
	}
	return nil
}

func releaseManagedAgentDispatchLeaseTx(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, bindingID string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, db.Rebind(`
		UPDATE managed_agent_bindings SET revision = revision + 1, dispatch_owner = '',
			dispatch_lease_until = NULL, updated_at = ? WHERE id = ?
	`), now, bindingID); err != nil {
		return fmt.Errorf("release managed agent dispatch lease: %w", err)
	}
	return nil
}
