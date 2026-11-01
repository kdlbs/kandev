package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

func (r *Repository) groupUsageProjectionMutationsTx(ctx context.Context, tx *sqlx.Tx, changes []models.UsageMutation) error {
	groups := make(map[string][]any)
	for _, change := range changes {
		partition := change.NewPartition
		if partition == nil {
			partition = change.OldPartition
		}
		if partition != nil {
			groups[partition.ScopeID] = append(groups[partition.ScopeID], change.CommitSequence)
		}
	}
	for scope, sequences := range groups {
		args := append([]any{uuid.NewString(), scope}, sequences...)
		_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_log SET publication_group = ? WHERE scope_id = ? AND commit_sequence IN (`+placeholders(len(sequences))+`)`), args...)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) linkUsageProjectionPublicationTx(ctx context.Context, tx *sqlx.Tx, scope, generation, group string, change models.UsageMutation) error {
	if group == "" {
		group = uuid.NewString()
	}
	for _, partition := range []*models.UsagePartition{change.OldPartition, change.NewPartition} {
		if partition == nil {
			continue
		}
		var current string
		err := tx.GetContext(ctx, &current, r.db.Rebind(`SELECT publish_group FROM usage_projection_work WHERE scope_id = ? AND generation = ? AND work_key = ?`), scope, generation, partition.WorkKey)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_projection_replay_groups (scope_id, generation, group_id, work_key) VALUES (?, ?, ?, ?)
			ON CONFLICT(scope_id, generation, group_id) DO NOTHING`), scope, generation, group, partition.WorkKey)
		if err != nil {
			return err
		}
		var anchor string
		err = tx.GetContext(ctx, &anchor, r.db.Rebind(`SELECT w.publish_group FROM usage_projection_work w JOIN usage_projection_replay_groups g
			ON g.scope_id = w.scope_id AND g.generation = w.generation AND g.work_key = w.work_key WHERE g.scope_id = ? AND g.generation = ? AND g.group_id = ?`), scope, generation, group)
		if err != nil {
			return err
		}
		if current != anchor {
			_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_work SET publish_group = ? WHERE scope_id = ? AND generation = ? AND publish_group IN (?, ?)`), min(current, anchor), scope, generation, current, anchor)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Related invalidations publish in one transaction, including supersession and
// complete-set retirement. The group contains work metadata; source candidates
// were resolved in separately committed bounded pages.
func (r *Repository) publishUsageProjectionGroupTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork) error {
	blocked, err := r.usageContextPublicationBlockedTx(ctx, tx, work)
	if err != nil || blocked {
		return err
	}
	var pending bool
	err = tx.GetContext(ctx, &pending, r.db.Rebind(`SELECT EXISTS(SELECT 1 FROM usage_projection_work WHERE scope_id = ? AND generation = ? AND publish_group = ? AND state <> 'complete')`), work.Scope, work.Generation, work.Group)
	if err != nil || pending {
		return err
	}
	var members []usageProjectionWork
	if err := tx.SelectContext(ctx, &members, r.db.Rebind(`SELECT * FROM usage_projection_work WHERE scope_id = ? AND generation = ? AND publish_group = ? ORDER BY work_key`), work.Scope, work.Generation, work.Group); err != nil {
		return err
	}
	for _, member := range members {
		if err := r.publishUsageProjectionWorkTx(ctx, tx, member); err != nil {
			return err
		}
	}
	if len(members) > 0 {
		if err := r.rebuildUsageProjectionSummariesTx(ctx, tx, work.Scope, work.Generation); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_projection_replay_groups WHERE scope_id = ? AND generation = ? AND NOT EXISTS (
		SELECT 1 FROM usage_projection_work w WHERE w.scope_id = usage_projection_replay_groups.scope_id AND w.generation = usage_projection_replay_groups.generation AND w.work_key = usage_projection_replay_groups.work_key)`), work.Scope, work.Generation)
	return err
}
