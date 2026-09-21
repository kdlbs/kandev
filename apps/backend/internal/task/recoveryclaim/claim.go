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

	kandevdb "github.com/kandev/kandev/internal/db"
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

const forUpdateClause = ` FOR UPDATE`

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
	if req.TaskEnvironmentID == "" || req.OwnerTaskID == "" || req.SessionID == "" || req.SessionIncarnationID == "" || req.OperationID == "" || req.ExecutorType == "" {
		return nil, errors.New("task environment recovery claim: environment, owner, session, session incarnation, operation, and executor are required")
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
		if err := validateRequesterIdentity(ctx, db, tx, req); err != nil {
			return nil, err
		}
		if claim.OwnerTaskID == req.OwnerTaskID && claim.OwnershipGeneration == req.OwnershipGeneration &&
			claim.SessionID == req.SessionID && claim.SessionIncarnationID == req.SessionIncarnationID &&
			claim.OperationID == req.OperationID && claim.ExecutorType == req.ExecutorType {
			return claim, tx.Commit()
		}
		return nil, fmt.Errorf("%w: environment %s is claimed by operation %s", ErrBusy, req.TaskEnvironmentID, claim.OperationID)
	}
	if err := validateRequesterIdentity(ctx, db, tx, req); err != nil {
		return nil, err
	}

	snapshot, err := loadAdmissionSnapshot(ctx, db, tx, req.TaskEnvironmentID, req.SessionID)
	if err != nil {
		return nil, err
	}
	if ClassifyAdmission(snapshot) == models.TaskEnvironmentAdmissionLiveBlocker {
		return nil, fmt.Errorf("%w: environment %s has a live session or runtime", ErrBusy, req.TaskEnvironmentID)
	}

	now := time.Now().UTC()
	claim = &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID: req.TaskEnvironmentID, OwnerTaskID: req.OwnerTaskID,
		OwnershipGeneration: req.OwnershipGeneration, SessionID: req.SessionID,
		SessionIncarnationID: req.SessionIncarnationID,
		OperationID:          req.OperationID, ExecutorType: req.ExecutorType,
		CreatedAt: now, UpdatedAt: now,
	}
	if _, err := tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO task_environment_recovery_claims (
			task_environment_id, owner_task_id, ownership_generation, session_id, session_incarnation_id,
			operation_id, executor_type, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), claim.TaskEnvironmentID, claim.OwnerTaskID, claim.OwnershipGeneration,
		claim.SessionID, claim.SessionIncarnationID, claim.OperationID, claim.ExecutorType, claim.CreatedAt, claim.UpdatedAt); err != nil {
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

func validateRequesterIdentity(
	ctx context.Context,
	db *sqlx.DB,
	tx *sqlx.Tx,
	req models.TaskEnvironmentRecoveryClaimRequest,
) error {
	query := `
		SELECT task_id, task_environment_id, queue_incarnation_id
		FROM task_sessions
		WHERE id = ?`
	if dialect.IsPostgres(db.DriverName()) {
		query += forUpdateClause
	}
	var taskID, incarnationID string
	var environmentID sql.NullString
	if err := tx.QueryRowContext(ctx, db.Rebind(query), req.SessionID).Scan(
		&taskID, &environmentID, &incarnationID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: requesting session is not current", ErrClaimMismatch)
		}
		return err
	}
	if !environmentID.Valid || environmentID.String != req.TaskEnvironmentID || incarnationID == "" || incarnationID != req.SessionIncarnationID {
		return fmt.Errorf("%w: requesting session authority changed", ErrClaimMismatch)
	}
	authorized, err := requesterTaskCanUseEnvironment(ctx, db, tx, req, taskID)
	if err != nil {
		return err
	}
	if !authorized {
		return fmt.Errorf("%w: requesting session authority changed", ErrClaimMismatch)
	}
	return nil
}

func requesterTaskCanUseEnvironment(
	ctx context.Context,
	db *sqlx.DB,
	tx *sqlx.Tx,
	req models.TaskEnvironmentRecoveryClaimRequest,
	requesterTaskID string,
) (bool, error) {
	ownerWorkspaceID, err := taskWorkspaceID(ctx, db, tx, req.OwnerTaskID)
	if err != nil {
		return false, err
	}
	for taskID := requesterTaskID; taskID != ""; {
		workspaceID, parentID, mode, taskErr := taskInheritance(ctx, db, tx, taskID)
		if errors.Is(taskErr, sql.ErrNoRows) {
			return false, nil
		}
		if taskErr != nil || workspaceID != ownerWorkspaceID {
			return false, taskErr
		}
		if taskID == req.OwnerTaskID {
			return true, nil
		}
		if mode != "inherit_parent" {
			break
		}
		taskID = parentID
	}
	return requesterTaskSharesEnvironment(ctx, db, tx, req, requesterTaskID, ownerWorkspaceID)
}

func taskWorkspaceID(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, taskID string) (string, error) {
	var workspaceID string
	query := `SELECT workspace_id FROM tasks WHERE id = ?`
	if dialect.IsPostgres(db.DriverName()) {
		query += forUpdateClause
	}
	err := tx.QueryRowContext(ctx, db.Rebind(query), taskID).Scan(&workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: environment owner is not current", ErrClaimMismatch)
	}
	return workspaceID, err
}

func taskInheritance(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, taskID string) (string, string, string, error) {
	var workspaceID, parentID, mode string
	query := fmt.Sprintf(`
		SELECT workspace_id, COALESCE(parent_id, ''), COALESCE(%s, '')
		FROM tasks WHERE id = ?`, dialect.JSONExtractPath(db.DriverName(), "metadata", "workspace", "mode"))
	if dialect.IsPostgres(db.DriverName()) {
		query += forUpdateClause
	}
	err := tx.QueryRowContext(ctx, db.Rebind(query), taskID).Scan(&workspaceID, &parentID, &mode)
	return workspaceID, parentID, mode, err
}

func requesterTaskSharesEnvironment(
	ctx context.Context,
	db *sqlx.DB,
	tx *sqlx.Tx,
	req models.TaskEnvironmentRecoveryClaimRequest,
	requesterTaskID, ownerWorkspaceID string,
) (bool, error) {
	var matched int
	err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT 1
		FROM task_workspace_groups groups
		JOIN task_workspace_group_members member
		  ON member.workspace_group_id = groups.id
		JOIN tasks requester ON requester.id = member.task_id
		WHERE groups.owner_task_id = ?
		  AND groups.materialized_environment_id = ?
		  AND member.task_id = ?
		  AND member.released_at IS NULL
		  AND requester.workspace_id = ?
		LIMIT 1
	`), req.OwnerTaskID, req.TaskEnvironmentID, requesterTaskID, ownerWorkspaceID).Scan(&matched)
	if errors.Is(err, sql.ErrNoRows) || kandevdb.IsMissingTableError(err) {
		return false, nil
	}
	return err == nil, err
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
		  AND session_id = ? AND session_incarnation_id = ? AND operation_id = ? AND executor_type = ?
	`), claim.TaskEnvironmentID, claim.OwnerTaskID, claim.OwnershipGeneration,
		claim.SessionID, claim.SessionIncarnationID, claim.OperationID, claim.ExecutorType)
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
	if err := validateClaimRequesterIdentity(ctx, db, tx, claim); err != nil {
		return err
	}
	return nil
}

func validateClaimRequesterIdentity(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, claim *models.TaskEnvironmentRecoveryClaim) error {
	return validateRequesterIdentity(ctx, db, tx, models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID:    claim.TaskEnvironmentID,
		OwnerTaskID:          claim.OwnerTaskID,
		SessionID:            claim.SessionID,
		SessionIncarnationID: claim.SessionIncarnationID,
	})
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
		query += forUpdateClause
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
		query += forUpdateClause
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

func loadAdmissionSnapshot(
	ctx context.Context,
	db *sqlx.DB,
	tx *sqlx.Tx,
	environmentID string,
	requestingSessionID string,
) (models.TaskEnvironmentAdmissionSnapshot, error) {
	snapshot := models.TaskEnvironmentAdmissionSnapshot{}
	if err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT COALESCE(materialization_session_id, '')
		FROM task_environments
		WHERE id = ?
	`), environmentID).Scan(&snapshot.MaterializationSessionID); err != nil {
		return snapshot, err
	}

	rows, err := tx.QueryContext(ctx, db.Rebind(`
		SELECT ts.id, ts.state,
			EXISTS (
				SELECT 1 FROM task_session_turns turn
				WHERE turn.task_session_id = ts.id AND turn.completed_at IS NULL
			),
			CASE WHEN er.session_id IS NULL THEN FALSE ELSE TRUE END,
			COALESCE(er.status, '')
		FROM task_sessions ts
		LEFT JOIN executors_running er ON er.session_id = ts.id
		WHERE ts.task_environment_id = ? AND ts.id <> ?
		ORDER BY ts.id
	`), environmentID, requestingSessionID)
	if err != nil {
		return snapshot, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		consumer := models.TaskEnvironmentAdmissionConsumer{}
		if err := rows.Scan(
			&consumer.SessionID,
			&consumer.SessionState,
			&consumer.HasActiveTurn,
			&consumer.HasExecutor,
			&consumer.ExecutorStatus,
		); err != nil {
			return snapshot, err
		}
		snapshot.Consumers = append(snapshot.Consumers, consumer)
	}
	return snapshot, rows.Err()
}

func loadClaim(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, environmentID string) (*models.TaskEnvironmentRecoveryClaim, error) {
	claim := &models.TaskEnvironmentRecoveryClaim{}
	err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT task_environment_id, owner_task_id, ownership_generation, session_id, session_incarnation_id,
			operation_id, executor_type, created_at, updated_at
		FROM task_environment_recovery_claims WHERE task_environment_id = ?
	`+claimForUpdateClause(db.DriverName())), environmentID).Scan(
		&claim.TaskEnvironmentID, &claim.OwnerTaskID, &claim.OwnershipGeneration,
		&claim.SessionID, &claim.SessionIncarnationID, &claim.OperationID, &claim.ExecutorType,
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
		left.SessionID == right.SessionID && left.SessionIncarnationID == right.SessionIncarnationID &&
		left.OperationID == right.OperationID && left.ExecutorType == right.ExecutorType
}

func claimForUpdateClause(driverName string) string {
	if dialect.IsPostgres(driverName) {
		return forUpdateClause
	}
	return ""
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate")
}
