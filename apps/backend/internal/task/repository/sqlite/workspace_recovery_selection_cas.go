package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) readWorkspaceRecoveryInventoryForUpdate(
	ctx context.Context,
	tx *sqlx.Tx,
	environmentID string,
) ([]models.WorkspaceRecoveryInventorySlot, error) {
	slotCount, repositoryIDs, err := r.lockWorkspaceRecoverySlots(ctx, tx, environmentID)
	if err != nil {
		return nil, err
	}
	if err := r.lockWorkspaceRecoveryRepositories(ctx, tx, repositoryIDs); err != nil {
		return nil, err
	}
	return r.readWorkspaceRecoveryInventory(ctx, tx, environmentID, slotCount)
}

func (r *Repository) lockWorkspaceRecoverySlots(
	ctx context.Context,
	tx *sqlx.Tx,
	environmentID string,
) (int, []string, error) {
	lockSlotsQuery := `
		SELECT id, COALESCE(repository_id, '')
		FROM task_environment_repos
		WHERE task_environment_id = ? AND (COALESCE(status, '') = '' OR status = 'active') AND deleted_at IS NULL
		ORDER BY id`
	if dialect.IsPostgres(r.db.DriverName()) {
		lockSlotsQuery += ` FOR UPDATE`
	}
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(lockSlotsQuery), environmentID)
	if err != nil {
		return 0, nil, err
	}
	var slotCount int
	repositoryIDs := make(map[string]struct{})
	for rows.Next() {
		var slotID, repositoryID string
		if err := rows.Scan(&slotID, &repositoryID); err != nil {
			_ = rows.Close()
			return 0, nil, err
		}
		slotCount++
		if repositoryID != "" {
			repositoryIDs[repositoryID] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, nil, err
	}
	if err := rows.Close(); err != nil {
		return 0, nil, err
	}
	orderedRepositoryIDs := make([]string, 0, len(repositoryIDs))
	for id := range repositoryIDs {
		orderedRepositoryIDs = append(orderedRepositoryIDs, id)
	}
	sort.Strings(orderedRepositoryIDs)
	return slotCount, orderedRepositoryIDs, nil
}
func (r *Repository) lockWorkspaceRecoveryRepositories(
	ctx context.Context,
	tx *sqlx.Tx,
	repositoryIDs []string,
) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}
	for _, repositoryID := range repositoryIDs {
		var lockedID string
		err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT id FROM repositories WHERE id = ? FOR UPDATE`), repositoryID).Scan(&lockedID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

func (r *Repository) readWorkspaceRecoveryInventory(
	ctx context.Context,
	tx *sqlx.Tx,
	environmentID string,
	slotCount int,
) ([]models.WorkspaceRecoveryInventorySlot, error) {
	query := `
		SELECT
			ter.id, ter.repository_id, COALESCE(ter.branch_slug, ''),
			COALESCE(ter.worktree_id, ''), COALESCE(ter.worktree_path, ''),
			COALESCE(ter.worktree_branch, ''), COALESCE(ter.worktree_branch_owner, 'unknown'),
			COALESCE(ter.worktree_integration_ref, ''),
			COALESCE(ter.worktree_recovery_head_sha, ''),
			COALESCE(ter.worktree_source_clone_path, ''),
			COALESCE(ter.worktree_source_common_dir, ''),
			COALESCE(ter.position, 0), COALESCE(ter.status, ''),
			CASE WHEN r.id IS NULL THEN 0 ELSE 1 END,
			CASE WHEN r.deleted_at IS NULL THEN 0 ELSE 1 END,
			COALESCE(r.workspace_id, ''), COALESCE(r.name, ''), COALESCE(r.source_type, ''),
			COALESCE(r.local_path, ''), COALESCE(r.provider, ''),
			COALESCE(r.provider_repo_id, ''), COALESCE(r.provider_host, ''),
			COALESCE(r.provider_scope, ''), COALESCE(r.provider_owner, ''),
			COALESCE(r.provider_name, ''), COALESCE(r.remote_url, '')
		FROM task_environment_repos ter
		LEFT JOIN repositories r ON r.id = ter.repository_id
		WHERE ter.task_environment_id = ? AND (COALESCE(ter.status, '') = '' OR ter.status = 'active') AND ter.deleted_at IS NULL
		ORDER BY ter.id`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE OF ter`
	}
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(query), environmentID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	slots := make([]models.WorkspaceRecoveryInventorySlot, 0, slotCount)
	for rows.Next() {
		var slot models.WorkspaceRecoveryInventorySlot
		var repositoryPresent, repositoryDeleted int
		if err := rows.Scan(
			&slot.EnvironmentRepoID, &slot.RepositoryID, &slot.BranchSlug,
			&slot.WorktreeID, &slot.WorktreePath, &slot.WorktreeBranch,
			&slot.WorktreeBranchOwner, &slot.WorktreeIntegrationRef,
			&slot.WorktreeRecoveryHeadSHA, &slot.WorktreeSourceClonePath,
			&slot.WorktreeSourceCommonDir, &slot.Position, &slot.Status,
			&repositoryPresent, &repositoryDeleted, &slot.RepositoryWorkspaceID,
			&slot.RepositoryName, &slot.RepositorySourceType, &slot.RepositoryLocalPath, &slot.RepositoryProvider,
			&slot.RepositoryProviderRepoID, &slot.RepositoryProviderHost,
			&slot.RepositoryProviderScope, &slot.RepositoryProviderOwner,
			&slot.RepositoryProviderName, &slot.RepositoryRemoteURL,
		); err != nil {
			return nil, err
		}
		if slot.Status == "" {
			slot.Status = worktreeRepoStatusActive
		}
		slot.RepositoryPresent = repositoryPresent == 1
		slot.RepositoryDeleted = repositoryDeleted == 1
		slots = append(slots, slot)
	}
	return slots, rows.Err()
}

func workspaceRecoverySnapshotMatchesObservation(
	snapshot models.WorkspaceRecoverySelectionSnapshot,
	observation models.WorkspaceRecoveryErrorObservation,
) bool {
	return snapshot.Complete() && snapshot.SessionPersisted && snapshot.TaskID == observation.TaskID &&
		snapshot.SessionID == observation.SessionID &&
		snapshot.TaskEnvironmentID == observation.TaskEnvironmentID &&
		snapshot.SessionEnvironmentMatchesSelected() &&
		snapshot.EnvironmentOwnerTaskID == observation.EnvironmentOwnerTaskID &&
		snapshot.OwnershipGeneration == observation.OwnershipGeneration
}
