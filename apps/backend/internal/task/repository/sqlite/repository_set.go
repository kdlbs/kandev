package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// Repository sets are a named group of workspace repositories used to fill the
// task-creation picker in one action.
//
// Two contracts drive every query in this file:
//
//   - Membership order is authoritative and contiguous. Writes assign positions
//     from slice order; reads sort by position.
//   - Repositories are soft-deleted (repositories.deleted_at), so the foreign
//     key cascade never fires for them. Deleting a repository prunes its
//     membership rows (see repository_entity.go), and every read here *also*
//     filters soft-deleted and out-of-workspace repositories. Either mechanism
//     alone would do; both are present so a set can never surface a repository
//     the user cannot select.

const repositorySetColumns = `id, workspace_id, name, description, created_at, updated_at`

// CreateRepositorySet inserts a set and its ordered membership in one
// transaction. Positions come from set.Items order; item ids are generated when
// empty. The unique constraints on (workspace_id, name) and
// (repository_set_id, repository_id) mean a duplicate name or a repeated
// repository fails the whole insert, leaving no partial row behind.
func (r *Repository) CreateRepositorySet(ctx context.Context, set *models.RepositorySet) error {
	if set.ID == "" {
		set.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	set.CreatedAt = now
	set.UpdatedAt = now

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO repository_sets (`+repositorySetColumns+`)
		VALUES (?, ?, ?, ?, ?, ?)
	`), set.ID, set.WorkspaceID, set.Name, set.Description, set.CreatedAt, set.UpdatedAt)
	if err != nil {
		return err
	}
	if err := r.insertRepositorySetItems(ctx, tx, set.ID, set.Items, now); err != nil {
		return err
	}
	return tx.Commit()
}

// insertRepositorySetItems writes membership rows with contiguous positions,
// filling in each item's generated id, owner, position, and timestamps so the
// caller's model matches what the database now holds without a re-read.
func (r *Repository) insertRepositorySetItems(
	ctx context.Context,
	tx *sqlx.Tx,
	setID string,
	items []models.RepositorySetItem,
	now time.Time,
) error {
	for i := range items {
		if items[i].ID == "" {
			items[i].ID = uuid.New().String()
		}
		items[i].RepositorySetID = setID
		items[i].Position = i
		items[i].CreatedAt = now
		items[i].UpdatedAt = now
		_, err := tx.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO repository_set_items (
				id, repository_set_id, repository_id, position, base_branch, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`), items[i].ID, setID, items[i].RepositoryID, items[i].Position,
			items[i].BaseBranch, now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetRepositorySet loads one set with its resolved membership.
func (r *Repository) GetRepositorySet(ctx context.Context, id string) (*models.RepositorySet, error) {
	set := &models.RepositorySet{}
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT `+repositorySetColumns+` FROM repository_sets WHERE id = ?
	`), id).Scan(&set.ID, &set.WorkspaceID, &set.Name, &set.Description, &set.CreatedAt, &set.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repoerrors.ErrRepositorySetNotFound
	}
	if err != nil {
		return nil, err
	}
	itemsBySet, err := r.listRepositorySetItems(ctx, []string{set.ID})
	if err != nil {
		return nil, err
	}
	set.Items = itemsBySet[set.ID]
	return set, nil
}

// GetRepositorySetByName finds a set by workspace and name, comparing the name
// case-insensitively. Returns nil, nil when the name is unused: the caller
// decides whether that is a conflict or a miss.
func (r *Repository) GetRepositorySetByName(
	ctx context.Context,
	workspaceID string,
	name string,
) (*models.RepositorySet, error) {
	// LOWER() rather than a NOCASE collation: the same statement has to run on
	// Postgres, where NOCASE does not exist.
	set := &models.RepositorySet{}
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT `+repositorySetColumns+` FROM repository_sets
		WHERE workspace_id = ? AND LOWER(name) = ?
	`), workspaceID, strings.ToLower(strings.TrimSpace(name))).Scan(
		&set.ID, &set.WorkspaceID, &set.Name, &set.Description, &set.CreatedAt, &set.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	itemsBySet, err := r.listRepositorySetItems(ctx, []string{set.ID})
	if err != nil {
		return nil, err
	}
	set.Items = itemsBySet[set.ID]
	return set, nil
}

// ListRepositorySets returns a workspace's sets, newest name-ordered first, with
// membership resolved in one batched query rather than one query per set.
func (r *Repository) ListRepositorySets(ctx context.Context, workspaceID string) ([]*models.RepositorySet, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+repositorySetColumns+` FROM repository_sets
		WHERE workspace_id = ? ORDER BY LOWER(name)
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	sets := make([]*models.RepositorySet, 0)
	ids := make([]string, 0)
	for rows.Next() {
		set := &models.RepositorySet{}
		if err := rows.Scan(&set.ID, &set.WorkspaceID, &set.Name, &set.Description,
			&set.CreatedAt, &set.UpdatedAt); err != nil {
			return nil, err
		}
		sets = append(sets, set)
		ids = append(ids, set.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	itemsBySet, err := r.listRepositorySetItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, set := range sets {
		set.Items = itemsBySet[set.ID]
	}
	return sets, nil
}

// listRepositorySetItems resolves membership for several sets at once, excluding
// members whose repository is soft-deleted or has moved out of the set's
// workspace.
func (r *Repository) listRepositorySetItems(
	ctx context.Context,
	setIDs []string,
) (map[string][]models.RepositorySetItem, error) {
	itemsBySet := make(map[string][]models.RepositorySetItem, len(setIDs))
	if len(setIDs) == 0 {
		return itemsBySet, nil
	}
	query, args, err := sqlx.In(`
		SELECT i.id, i.repository_set_id, i.repository_id, i.position, i.base_branch,
			i.created_at, i.updated_at
		FROM repository_set_items i
		INNER JOIN repository_sets s ON s.id = i.repository_set_id
		INNER JOIN repositories rep ON rep.id = i.repository_id
		WHERE i.repository_set_id IN (?)
			AND rep.deleted_at IS NULL
			AND rep.workspace_id = s.workspace_id
		ORDER BY i.repository_set_id, i.position
	`, setIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		item := models.RepositorySetItem{}
		if err := rows.Scan(&item.ID, &item.RepositorySetID, &item.RepositoryID,
			&item.Position, &item.BaseBranch, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		itemsBySet[item.RepositorySetID] = append(itemsBySet[item.RepositorySetID], item)
	}
	return itemsBySet, rows.Err()
}

// ListRepositorySetIDsByRepository returns the sets that currently hold a
// repository. Callers use it to capture the affected sets *before* a repository
// deletion prunes their membership, so they can publish the post-delete shape.
func (r *Repository) ListRepositorySetIDsByRepository(
	ctx context.Context,
	repositoryID string,
) ([]string, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(
		`SELECT repository_set_id FROM repository_set_items WHERE repository_id = ?`), repositoryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// UpdateRepositorySet writes the set's own fields and, when repositoryItems is
// non-nil, replaces its whole membership in the SAME transaction.
//
// The two must commit together: a name change that lands while the membership
// replacement fails leaves the set renamed but still holding the old
// repositories, with the API reporting failure and publishing nothing. A nil
// repositoryItems leaves membership untouched, which is how an update that only
// changes name or description is expressed.
func (r *Repository) UpdateRepositorySet(
	ctx context.Context,
	set *models.RepositorySet,
	repositoryItems *[]models.RepositorySetItem,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	set.UpdatedAt = time.Now().UTC()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE repository_sets SET name = ?, description = ?, updated_at = ? WHERE id = ?
	`), set.Name, set.Description, set.UpdatedAt, set.ID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	// A set deleted between the service's read and this write must not be
	// resurrected, nor reported as updated.
	if affected == 0 {
		return repoerrors.ErrRepositorySetNotFound
	}

	if repositoryItems != nil {
		if err := r.replaceItemsTx(ctx, tx, set.ID, *repositoryItems, set.UpdatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// replaceItemsTx rewrites a set's whole membership inside an existing
// transaction. Positions come out contiguous from zero in the order supplied,
// which is also how reordering is expressed.
func (r *Repository) replaceItemsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	setID string,
	repositoryItems []models.RepositorySetItem,
	now time.Time,
) error {
	if _, err := tx.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM repository_set_items WHERE repository_set_id = ?`), setID); err != nil {
		return err
	}
	return r.insertRepositorySetItems(ctx, tx, setID, repositoryItems, now)
}

// DeleteRepositorySet removes a set and, by cascade, its membership rows. It
// reports whether a row was deleted so a repeat delete is not an error.
// Repositories themselves are never touched.
func (r *Repository) DeleteRepositorySet(ctx context.Context, id string) (bool, error) {
	// The items cascade is declared on the table, but a database opened without
	// foreign keys enforced would leave orphans; deleting explicitly in the same
	// transaction makes the outcome independent of that pragma.
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM repository_set_items WHERE repository_set_id = ?`), id); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM repository_sets WHERE id = ?`), id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return affected > 0, nil
}
