// Package recoveryclaim owns the durable authority used by automatic task
// environment recovery. It deliberately has no filesystem or runtime code.
package recoveryclaim

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

var (
	// ErrBusy means another runtime, session, cleanup, or recovery operation
	// currently owns the environment authority.
	ErrBusy = errors.New("task environment recovery is busy")
	// ErrClaimNotFound means the requested durable claim no longer exists.
	ErrClaimNotFound = errors.New("task environment recovery claim not found")
	// ErrClaimMismatch means a caller presented stale operation or generation
	// identity and cannot release or use the claim.
	ErrClaimMismatch = errors.New("task environment recovery claim mismatch")
)

type claimContextKey struct{}

// WithClaim attaches a recovery claim to an internal operation context. The
// marker lets nested lifecycle calls reuse the same authority without trying
// to acquire or release it a second time.
func WithClaim(ctx context.Context, claim *models.TaskEnvironmentRecoveryClaim) context.Context {
	if claim == nil {
		return ctx
	}
	return context.WithValue(ctx, claimContextKey{}, claim)
}

// WithoutClaim preserves the operation context while preventing a detached
// asynchronous phase from using an authority that its caller has released.
func WithoutClaim(ctx context.Context) context.Context {
	if ClaimFromContext(ctx) == nil {
		return ctx
	}
	var noClaim *models.TaskEnvironmentRecoveryClaim
	return context.WithValue(ctx, claimContextKey{}, noClaim)
}

// ClaimFromContext returns the recovery claim carried by ctx, if any.
func ClaimFromContext(ctx context.Context) *models.TaskEnvironmentRecoveryClaim {
	if ctx == nil {
		return nil
	}
	claim, _ := ctx.Value(claimContextKey{}).(*models.TaskEnvironmentRecoveryClaim)
	return claim
}

// Acquire obtains environment-scoped recovery authority in one transaction.
// The owner task and environment rows are serialized before the claim is
// inserted, so a stale owner or active lifecycle consumer cannot publish a
// replacement after this method returns.
//
//nolint:cyclop // Claim acquisition keeps identity, replay, liveness, and insert checks atomic.
func Acquire(ctx context.Context, db *sqlx.DB, req models.TaskEnvironmentRecoveryClaimRequest) (*models.TaskEnvironmentRecoveryClaim, error) {
	if db == nil {
		return nil, errors.New("task environment recovery claim: database is required")
	}
	if req.TaskEnvironmentID == "" || req.OwnerTaskID == "" || req.SessionID == "" || req.OperationID == "" || req.ExecutorType == "" {
		return nil, errors.New("task environment recovery claim: environment, owner, session, operation, and executor are required")
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if err := lockTask(ctx, db, tx, req.OwnerTaskID); err != nil {
		return nil, err
	}
	if err := ensureCleanupAbsent(ctx, db, tx, req.OwnerTaskID); err != nil {
		return nil, err
	}

	ownerTaskID, generation, executorType, err := loadEnvironmentIdentity(ctx, db, tx, req.TaskEnvironmentID)
	if err != nil {
		return nil, err
	}
	if ownerTaskID != req.OwnerTaskID || generation != req.OwnershipGeneration {
		return nil, fmt.Errorf("%w: environment %s is owned by %s at generation %d", repoerrors.ErrTaskEnvironmentOwnershipChanged, req.TaskEnvironmentID, ownerTaskID, generation)
	}
	if executorType != req.ExecutorType {
		return nil, fmt.Errorf("%w: environment %s uses executor %q, request selected %q", ErrClaimMismatch, req.TaskEnvironmentID, executorType, req.ExecutorType)
	}

	claim, err := loadClaim(ctx, db, tx, req.TaskEnvironmentID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if claim != nil {
		if claim.OwnerTaskID == req.OwnerTaskID && claim.OwnershipGeneration == req.OwnershipGeneration &&
			claim.SessionID == req.SessionID && claim.OperationID == req.OperationID && claim.ExecutorType == req.ExecutorType {
			return claim, tx.Commit()
		}
		return nil, fmt.Errorf("%w: environment %s is claimed by operation %s", ErrBusy, req.TaskEnvironmentID, claim.OperationID)
	}

	if busy, err := environmentHasConsumers(ctx, db, tx, req.TaskEnvironmentID, req.SessionID); err != nil {
		return nil, err
	} else if busy {
		return nil, fmt.Errorf("%w: environment %s has a live session or runtime", ErrBusy, req.TaskEnvironmentID)
	}

	now := time.Now().UTC()
	claim = &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID: req.TaskEnvironmentID, OwnerTaskID: req.OwnerTaskID,
		OwnershipGeneration: req.OwnershipGeneration, SessionID: req.SessionID,
		OperationID: req.OperationID, ExecutorType: req.ExecutorType,
		CreatedAt: now, UpdatedAt: now,
	}
	if _, err := tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO task_environment_recovery_claims (
			task_environment_id, owner_task_id, ownership_generation, session_id,
			operation_id, executor_type, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), claim.TaskEnvironmentID, claim.OwnerTaskID, claim.OwnershipGeneration,
		claim.SessionID, claim.OperationID, claim.ExecutorType, claim.CreatedAt, claim.UpdatedAt); err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("%w: environment %s was claimed concurrently", ErrBusy, req.TaskEnvironmentID)
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return claim, nil
}

// Release removes exactly the operation and generation supplied by claim.
// A stale release cannot remove a later recovery operation.
func Release(ctx context.Context, db *sqlx.DB, claim *models.TaskEnvironmentRecoveryClaim) error {
	if db == nil || claim == nil || claim.TaskEnvironmentID == "" || claim.OperationID == "" {
		return errors.New("task environment recovery claim: complete identity is required")
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	current, err := loadClaim(ctx, db, tx, claim.TaskEnvironmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrClaimNotFound
	}
	if err != nil {
		return err
	}
	if !sameClaim(current, claim) {
		return fmt.Errorf("%w: operation %s is not current", ErrClaimMismatch, claim.OperationID)
	}
	result, err := tx.ExecContext(ctx, db.Rebind(`
		DELETE FROM task_environment_recovery_claims
		WHERE task_environment_id = ? AND owner_task_id = ? AND ownership_generation = ?
		  AND session_id = ? AND operation_id = ? AND executor_type = ?
	`), claim.TaskEnvironmentID, claim.OwnerTaskID, claim.OwnershipGeneration,
		claim.SessionID, claim.OperationID, claim.ExecutorType)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrClaimMismatch
	}
	return tx.Commit()
}

// EnsureAvailableTx rejects mutations that would overlap recovery. A caller
// carrying the exact claim may continue its own guarded operation.
func EnsureAvailableTx(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, environmentID string) error {
	if db == nil || tx == nil || environmentID == "" {
		return nil
	}
	// Use the same task-before-environment lock order as Acquire. A claim
	// acquisition must not pass its owner check while an environment mutation
	// is already in flight, and a mutation must not validate a claim and then
	// publish after ownership transfer commits.
	ownerTaskID, err := loadEnvironmentOwner(ctx, db, tx, environmentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if err := lockTask(ctx, db, tx, ownerTaskID); err != nil {
		return err
	}
	if claim := ClaimFromContext(ctx); claim != nil && claim.TaskEnvironmentID == environmentID {
		return ValidateTx(ctx, db, tx, claim)
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM task_environment_recovery_claims WHERE task_environment_id = ?
		)
	`), environmentID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%w: environment %s has an active recovery claim", ErrBusy, environmentID)
	}
	return nil
}

// EnsureTaskAvailableTx rejects creation of a task-scoped cleanup operation
// while any environment owned by that task is under recovery. The owner task
// lock gives this check the same ordering as Acquire, so cleanup creation and
// claim acquisition cannot pass each other on separate connections.
func EnsureTaskAvailableTx(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, taskID string) error {
	if db == nil || tx == nil || taskID == "" {
		return nil
	}
	if err := lockTask(ctx, db, tx, taskID); err != nil && !errors.Is(err, repoerrors.ErrTaskNotFound) {
		return err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT EXISTS (
			SELECT 1
			FROM task_environment_recovery_claims c
			JOIN task_environments e ON e.id = c.task_environment_id
			WHERE e.task_id = ?
		)
	`), taskID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%w: task %s has an active recovery claim", ErrBusy, taskID)
	}
	return nil
}

// ValidateTx verifies that claim still names the current owner, generation,
// executor, and durable claim row. It is used by publication and by writes
// made by the owner while the claim is carried in context.
func ValidateTx(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, claim *models.TaskEnvironmentRecoveryClaim) error {
	if db == nil || tx == nil || claim == nil {
		return ErrClaimMismatch
	}
	ownerTaskID, err := loadEnvironmentOwner(ctx, db, tx, claim.TaskEnvironmentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repoerrors.ErrTaskEnvironmentNotFound
		}
		return err
	}
	if err := lockTask(ctx, db, tx, ownerTaskID); err != nil {
		return err
	}
	currentOwnerTaskID, generation, executorType, err := loadEnvironmentIdentity(ctx, db, tx, claim.TaskEnvironmentID)
	if err != nil {
		return err
	}
	if currentOwnerTaskID != claim.OwnerTaskID || generation != claim.OwnershipGeneration || executorType != claim.ExecutorType {
		return fmt.Errorf("%w: environment identity changed", ErrClaimMismatch)
	}
	current, err := loadClaim(ctx, db, tx, claim.TaskEnvironmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrClaimNotFound
	}
	if err != nil {
		return err
	}
	if !sameClaim(current, claim) {
		return ErrClaimMismatch
	}
	return nil
}

func loadEnvironmentOwner(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, environmentID string) (string, error) {
	var ownerTaskID string
	err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT task_id FROM task_environments WHERE id = ?
	`), environmentID).Scan(&ownerTaskID)
	return ownerTaskID, err
}

//nolint:nestif // SQLite and PostgreSQL require different row-locking paths.
func lockTask(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, taskID string) error {
	//nolint:nestif // SQLite and PostgreSQL require different row-locking paths.
	query := `SELECT id FROM tasks WHERE id = ?`
	if dialect.IsPostgres(db.DriverName()) {
		query += ` FOR UPDATE`
	} else {
		// SQLite starts deferred transactions by default. Take its single-writer
		// reservation before reading the owner and environment rows so an
		// ownership transfer cannot commit between validation and claim insert.
		result, err := tx.ExecContext(ctx, db.Rebind(`
			UPDATE tasks SET updated_at = updated_at WHERE id = ?
		`), taskID)
		if err != nil {
			return fmt.Errorf("lock recovery owner task: %w", err)
		}
		if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
			if rowsErr != nil {
				return fmt.Errorf("lock recovery owner task: %w", rowsErr)
			}
			return fmt.Errorf("%w: %s", repoerrors.ErrTaskNotFound, taskID)
		}
		return nil
	}
	var lockedID string
	if err := tx.QueryRowContext(ctx, db.Rebind(query), taskID).Scan(&lockedID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", repoerrors.ErrTaskNotFound, taskID)
		}
		return fmt.Errorf("lock recovery owner task: %w", err)
	}
	return nil
}

func ensureCleanupAbsent(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, taskID string) error {
	var active bool
	if err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM task_resource_cleanup_jobs
			WHERE task_id = ? AND state IN (?, ?, ?, ?)
		)
	`), taskID,
		models.TaskResourceCleanupStatePrepared,
		models.TaskResourceCleanupStatePending,
		models.TaskResourceCleanupStateRunning,
		models.TaskResourceCleanupStateRetryWait,
	).Scan(&active); err != nil {
		return fmt.Errorf("check recovery owner cleanup barrier: %w", err)
	}
	if active {
		return fmt.Errorf("%w: %s", repoerrors.ErrTaskCleanupInProgress, taskID)
	}
	return nil
}

func loadEnvironmentIdentity(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, environmentID string) (string, int64, string, error) {
	query := `SELECT task_id, ownership_generation, executor_type FROM task_environments WHERE id = ?`
	if dialect.IsPostgres(db.DriverName()) {
		query += ` FOR UPDATE`
	}
	var ownerTaskID, executorType string
	var generation int64
	if err := tx.QueryRowContext(ctx, db.Rebind(query), environmentID).Scan(&ownerTaskID, &generation, &executorType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", 0, "", fmt.Errorf("%w: %s", repoerrors.ErrTaskEnvironmentNotFound, environmentID)
		}
		return "", 0, "", err
	}
	return ownerTaskID, generation, executorType, nil
}

func environmentHasConsumers(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, environmentID, requestingSessionID string) (bool, error) {
	var sessionExists, runtimeExists bool
	if err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM task_sessions
			WHERE task_environment_id = ? AND id <> ?
			  AND state NOT IN ('COMPLETED', 'FAILED', 'CANCELLED')
		)
	`), environmentID, requestingSessionID).Scan(&sessionExists); err != nil {
		return false, err
	}
	if err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM executors_running er
			JOIN task_sessions ts ON ts.id = er.session_id
			WHERE ts.task_environment_id = ?
		)
	`), environmentID).Scan(&runtimeExists); err != nil {
		return false, err
	}
	return sessionExists || runtimeExists, nil
}

func loadClaim(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, environmentID string) (*models.TaskEnvironmentRecoveryClaim, error) {
	claim := &models.TaskEnvironmentRecoveryClaim{}
	err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT task_environment_id, owner_task_id, ownership_generation, session_id,
			operation_id, executor_type, created_at, updated_at
		FROM task_environment_recovery_claims WHERE task_environment_id = ?
	`), environmentID).Scan(
		&claim.TaskEnvironmentID, &claim.OwnerTaskID, &claim.OwnershipGeneration,
		&claim.SessionID, &claim.OperationID, &claim.ExecutorType,
		&claim.CreatedAt, &claim.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return claim, nil
}

func sameClaim(left, right *models.TaskEnvironmentRecoveryClaim) bool {
	return left != nil && right != nil && left.TaskEnvironmentID == right.TaskEnvironmentID &&
		left.OwnerTaskID == right.OwnerTaskID && left.OwnershipGeneration == right.OwnershipGeneration &&
		left.SessionID == right.SessionID && left.OperationID == right.OperationID && left.ExecutorType == right.ExecutorType
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate")
}
