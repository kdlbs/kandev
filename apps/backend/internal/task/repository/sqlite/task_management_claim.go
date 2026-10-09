package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

//nolint:cyclop,funlen,gocognit // The transaction serializes task version, claim owner, generation, and audit history.
func (r *Repository) ChangeTaskManagementClaim(ctx context.Context, change models.TaskManagementClaimChange) (*models.TaskManagementClaim, error) {
	if err := validateTaskManagementClaimChange(change); err != nil {
		return nil, err
	}
	expectedTaskVersion, err := time.Parse(time.RFC3339Nano, change.ExpectedTaskResourceVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid task resource version: %w", err)
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var workspaceID string
	var taskVersion time.Time
	query := `SELECT workspace_id, updated_at FROM tasks WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	if err := tx.QueryRowContext(ctx, r.db.Rebind(query), change.TaskID).Scan(&workspaceID, &taskVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", repoerrors.ErrTaskNotFound, change.TaskID)
		}
		return nil, err
	}
	if workspaceID != change.WorkspaceID || !taskVersion.Equal(expectedTaskVersion) {
		return nil, repoerrors.ErrTaskVersionConflict
	}
	// SQLite has no row locks. This no-op write establishes the same task-row
	// serialization boundary used by exact task mutations before claim state is
	// read or changed.
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET updated_at = updated_at
		WHERE id = ? AND workspace_id = ? AND updated_at = ?
	`), change.TaskID, change.WorkspaceID, taskVersion)
	if err != nil {
		return nil, err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return nil, err
		}
		return nil, repoerrors.ErrTaskVersionConflict
	}

	claim, exists, err := readTaskManagementClaimInTx(ctx, tx, r.db.Rebind, change.TaskID, dialect.IsPostgres(r.db.DriverName()))
	if err != nil {
		return nil, err
	}
	if exists {
		if claim.ResourceVersion != change.ExpectedClaimResourceVersion {
			return nil, repoerrors.ErrTaskManagementClaimConflict
		}
		if change.ActorInstallationID != "" &&
			(claim.OwnerKind != completionGateActorPlugin || change.ActorInstallationID != claim.InstallationID || change.ActorInstanceKey == "" || change.ActorInstanceKey != claim.InstanceKey) {
			return nil, repoerrors.ErrTaskManagementClaimConflict
		}
	} else if change.ExpectedClaimResourceVersion != "" {
		return nil, repoerrors.ErrTaskManagementClaimConflict
	}
	if !exists {
		claim = &models.TaskManagementClaim{TaskID: change.TaskID, WorkspaceID: change.WorkspaceID}
	}
	previousOwnerKind, previousOwnerActorID := claim.OwnerKind, claim.OwnerActorID
	previousInstallationID, previousInstanceKey := claim.InstallationID, claim.InstanceKey
	if previousOwnerKind != "" {
		if change.Action == models.TaskManagementClaimAcquire {
			return nil, repoerrors.ErrTaskManagementClaimOwned
		}
	} else if change.Action != models.TaskManagementClaimAcquire {
		return nil, repoerrors.ErrTaskManagementClaimConflict
	}

	now := r.nowUTC()
	resourceVersion := uuid.NewString()
	claimedInstallationID, claimedInstanceKey := change.InstallationID, change.InstanceKey
	ownerKind, ownerActorID := change.OwnerKind, change.OwnerActorID
	var acquiredAt *time.Time
	action := models.TaskManagementClaimAcquired
	switch change.Action {
	case models.TaskManagementClaimAcquire:
		ownerKind, ownerActorID = completionGateActorPlugin, ""
		claim.Generation++
		acquiredAt = &now
	case models.TaskManagementClaimTransfer:
		if ownerKind == "" {
			ownerKind = completionGateActorPlugin
		}
		if ownerKind == completionGateActorPlugin && (claimedInstallationID == previousInstallationID && claimedInstanceKey == previousInstanceKey) {
			return nil, fmt.Errorf("claim transfer target is already the owner")
		}
		switch ownerKind {
		case completionGateActorPlugin:
			if claimedInstallationID == "" || claimedInstanceKey == "" || ownerActorID != "" {
				return nil, errors.New("plugin claim transfer target is incomplete")
			}
		case "human":
			if ownerActorID == "" || ownerActorID != change.ActorID || claimedInstallationID != "" || claimedInstanceKey != "" {
				return nil, errors.New("human claim transfer target is invalid")
			}
		default:
			return nil, errors.New("unsupported task management claim owner kind")
		}
		claim.Generation++
		acquiredAt = &now
		action = models.TaskManagementClaimTransferred
	case models.TaskManagementClaimRelease:
		claimedInstallationID, claimedInstanceKey = "", ""
		ownerKind, ownerActorID = "", ""
		claim.Generation++
		action = models.TaskManagementClaimReleased
	default:
		return nil, fmt.Errorf("unsupported task management claim action %q", change.Action)
	}

	claim.WorkspaceID = change.WorkspaceID
	claim.OwnerKind = ownerKind
	claim.OwnerActorID = ownerActorID
	claim.InstallationID = claimedInstallationID
	claim.InstanceKey = claimedInstanceKey
	claim.ResourceVersion = resourceVersion
	claim.AcquiredAt = acquiredAt
	claim.UpdatedAt = now
	claim.UpdatedByActor = change.ActorID
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_management_claims (
			task_id, workspace_id, owner_kind, owner_actor_id, installation_id, instance_key, generation,
			resource_version, acquired_at, updated_at, updated_by_actor
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			workspace_id = excluded.workspace_id,
			owner_kind = excluded.owner_kind,
			owner_actor_id = excluded.owner_actor_id,
			installation_id = excluded.installation_id,
			instance_key = excluded.instance_key,
			generation = excluded.generation,
			resource_version = excluded.resource_version,
			acquired_at = excluded.acquired_at,
			updated_at = excluded.updated_at,
			updated_by_actor = excluded.updated_by_actor
	`), change.TaskID, change.WorkspaceID, ownerKind, ownerActorID, claimedInstallationID, claimedInstanceKey,
		claim.Generation, resourceVersion, acquiredAt, now, change.ActorID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_management_claim_history (
			id, task_id, workspace_id, action, previous_owner_kind, previous_owner_actor_id,
			previous_installation_id, previous_instance_key, installation_id, instance_key,
			owner_kind, owner_actor_id, generation,
			actor_id, reason, resource_version, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), uuid.NewString(), change.TaskID, change.WorkspaceID, action,
		previousOwnerKind, previousOwnerActorID, previousInstallationID, previousInstanceKey,
		claimedInstallationID, claimedInstanceKey, ownerKind, ownerActorID,
		claim.Generation, change.ActorID, strings.TrimSpace(change.Reason), resourceVersion, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return claim, nil
}

//nolint:cyclop // The claim validator checks every action against plugin and human ownership rules.
func validateTaskManagementClaimChange(change models.TaskManagementClaimChange) error {
	if change.TaskID == "" || change.WorkspaceID == "" || change.ExpectedTaskResourceVersion == "" || change.ActorID == "" {
		return errors.New("task management claim command identity is incomplete")
	}
	if change.ActorInstallationID != "" && change.ActorInstanceKey == "" {
		return errors.New("plugin claim actor identity is incomplete")
	}
	if len(change.Reason) > 500 {
		return errors.New("task management claim reason is too long")
	}
	switch change.Action {
	case models.TaskManagementClaimAcquire:
		if change.InstallationID == "" || change.InstanceKey == "" {
			return errors.New("claim owner identity is incomplete")
		}
	case models.TaskManagementClaimTransfer:
		switch change.OwnerKind {
		case "human":
			if change.OwnerActorID == "" || change.InstallationID != "" || change.InstanceKey != "" {
				return errors.New("human claim owner identity is incomplete")
			}
		case "", completionGateActorPlugin:
			if change.InstallationID == "" || change.InstanceKey == "" {
				return errors.New("claim owner identity is incomplete")
			}
		default:
			return errors.New("unsupported task management claim owner kind")
		}
	case models.TaskManagementClaimRelease:
	case "":
		return errors.New("task management claim action is required")
	default:
		return errors.New("unsupported task management claim action")
	}
	return nil
}

func readTaskManagementClaimInTx(ctx context.Context, tx *sqlx.Tx, rebind func(string) string, taskID string, lock bool) (*models.TaskManagementClaim, bool, error) {
	query := `SELECT task_id, workspace_id, owner_kind, owner_actor_id, installation_id, instance_key, generation, resource_version, acquired_at, updated_at, updated_by_actor FROM task_management_claims WHERE task_id = ?`
	if lock {
		query += forUpdateClause
	}
	var claim models.TaskManagementClaim
	var acquiredAt sql.NullTime
	err := tx.QueryRowContext(ctx, rebind(query), taskID).Scan(
		&claim.TaskID, &claim.WorkspaceID, &claim.OwnerKind, &claim.OwnerActorID,
		&claim.InstallationID, &claim.InstanceKey, &claim.Generation,
		&claim.ResourceVersion, &acquiredAt, &claim.UpdatedAt, &claim.UpdatedByActor,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if acquiredAt.Valid {
		claim.AcquiredAt = &acquiredAt.Time
	}
	return &claim, true, nil
}

func (r *Repository) GetTaskManagementClaim(ctx context.Context, taskID string) (*models.TaskManagementClaim, error) {
	var claim models.TaskManagementClaim
	var acquiredAt sql.NullTime
	err := r.db.QueryRowContext(ctx, r.db.Rebind(`
		SELECT task_id, workspace_id, owner_kind, owner_actor_id, installation_id, instance_key, generation,
			resource_version, acquired_at, updated_at, updated_by_actor
		FROM task_management_claims WHERE task_id = ?
	`), taskID).Scan(&claim.TaskID, &claim.WorkspaceID, &claim.OwnerKind, &claim.OwnerActorID,
		&claim.InstallationID, &claim.InstanceKey,
		&claim.Generation, &claim.ResourceVersion, &acquiredAt, &claim.UpdatedAt, &claim.UpdatedByActor)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if acquiredAt.Valid {
		claim.AcquiredAt = &acquiredAt.Time
	}
	return &claim, nil
}

func (r *Repository) ListTaskManagementClaimHistory(ctx context.Context, taskID string) ([]*models.TaskManagementClaimHistory, error) {
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(`
		SELECT id, task_id, workspace_id, action, previous_owner_kind, previous_owner_actor_id,
			previous_installation_id, previous_instance_key, installation_id, instance_key,
			owner_kind, owner_actor_id, generation,
			actor_id, reason, resource_version, created_at
		FROM task_management_claim_history
		WHERE task_id = ?
		ORDER BY created_at DESC, id DESC
		LIMIT 100
	`), taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	history := make([]*models.TaskManagementClaimHistory, 0)
	for rows.Next() {
		item := new(models.TaskManagementClaimHistory)
		if err := rows.StructScan(item); err != nil {
			return nil, err
		}
		history = append(history, item)
	}
	return history, rows.Err()
}

func (r *Repository) checkTaskManagementClaimFence(ctx context.Context, tx interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, taskID string, fence models.TaskManagementClaimFence) error {
	var ownerKind, installationID, instanceKey string
	var generation int64
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT owner_kind, installation_id, instance_key, generation
		FROM task_management_claims WHERE task_id = ?
	`), taskID).Scan(&ownerKind, &installationID, &instanceKey, &generation)
	if errors.Is(err, sql.ErrNoRows) {
		if fence.InstanceKey != "" || fence.Generation != 0 {
			return repoerrors.ErrTaskManagementClaimConflict
		}
		return nil
	}
	if err != nil {
		return err
	}
	if ownerKind == "" {
		if fence.InstanceKey != "" || fence.Generation != generation {
			return repoerrors.ErrTaskManagementClaimConflict
		}
		return nil
	}
	if ownerKind != completionGateActorPlugin || fence.InstallationID != installationID || fence.InstanceKey != instanceKey || fence.Generation != generation {
		return repoerrors.ErrTaskManagementClaimConflict
	}
	return nil
}
