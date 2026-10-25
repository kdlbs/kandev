// Package physicalclaim serializes physical-worktree inventory ownership.
package physicalclaim

import (
	"context"
	"sort"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// Slot is an inventory identity, independent from the sessions that use it.
type Slot struct {
	ID            string
	EnvironmentID string
	RepositoryID  string
	BranchSlug    string
}

// Lock serializes physical claims before a separate READ COMMITTED observation.
// SQLite's immediate writer transaction already provides this serialization.
func Lock(ctx context.Context, tx *sqlx.Tx, ids ...string) error {
	if !dialect.IsPostgres(tx.DriverName()) {
		return nil
	}
	ordered := append([]string(nil), ids...)
	sort.Strings(ordered)
	previous := ""
	for _, id := range ordered {
		if id == "" || id == previous {
			continue
		}
		previous = id
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "physical-worktree:"+id); err != nil {
			return err
		}
	}
	return nil
}

// Resolve returns one live owner, or one historical slot when no live owner exists.
// The caller holds Lock for id until its transaction ends.
func Resolve(ctx context.Context, tx *sqlx.Tx, id string) (*Slot, error) {
	if id == "" {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, tx.Rebind(`SELECT id,task_environment_id,repository_id,COALESCE(branch_slug,''),
 CASE WHEN deleted_at IS NULL AND status <> 'deleted' THEN 1 ELSE 0 END
 FROM task_environment_repos WHERE worktree_id = ?`), id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var live, history []*Slot
	for rows.Next() {
		slot := &Slot{}
		var active int
		if err := rows.Scan(&slot.ID, &slot.EnvironmentID, &slot.RepositoryID, &slot.BranchSlug, &active); err != nil {
			return nil, err
		}
		history = append(history, slot)
		if active == 1 {
			live = append(live, slot)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	selected := live
	if len(selected) == 0 {
		selected = history
	}
	if len(selected) > 1 {
		return nil, &models.WorktreeIdentityAmbiguousError{WorktreeID: id}
	}
	if len(selected) == 0 {
		return nil, nil
	}
	return selected[0], nil
}

// Check permits only the current slot to retain or replace a physical claim.
// Empty physical identities remain valid inventory-only preparation rows.
func Check(ctx context.Context, tx *sqlx.Tx, id string, slot Slot) error {
	owner, err := Resolve(ctx, tx, id)
	if err != nil || owner == nil {
		return err
	}
	if slot.ID != "" && owner.ID == slot.ID {
		return nil
	}
	if slot.ID == "" && owner.EnvironmentID == slot.EnvironmentID && owner.RepositoryID == slot.RepositoryID && owner.BranchSlug == slot.BranchSlug {
		return nil
	}
	return &models.WorktreeIdentityAmbiguousError{WorktreeID: id}
}
