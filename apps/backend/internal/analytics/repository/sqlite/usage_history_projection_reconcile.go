package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jmoiron/sqlx"
)

// RunUsageHistoryProjectionBatch discovers work through durable sequence and
// generation indexes. It does not scan accepted observation history on idle ticks.
func (r *Repository) RunUsageHistoryProjectionBatch(parent context.Context, limit int) (bool, error) {
	repaired, err := r.repairUsageWorkBatch(parent, projectionLimit(limit))
	if err != nil || !repaired {
		return false, err
	}
	ctx, release, err := r.beginAnalyticsOperation(parent)
	if err != nil {
		return false, err
	}
	// New scopes and imported legacy rows can appear after startup backfill.
	// Verify one bounded source page before advancing publication work.
	accounted, err := r.repairNativeUsageAccountingBatch(ctx, projectionLimit(limit))
	if err != nil || !accounted {
		release()
		return false, normalizeAnalyticsOperationError(parent, ctx, err)
	}
	registered, err := r.registerUsageProjectionScopes(ctx, projectionLimit(limit))
	if err != nil || registered {
		release()
		return false, normalizeAnalyticsOperationError(parent, ctx, err)
	}
	scope, generation, err := r.nextUsageProjection(ctx)
	release()
	if err != nil {
		return false, err
	}
	if scope == "" {
		return r.collectUsageProjectionGarbage(parent, projectionLimit(limit))
	}
	if generation == "" {
		_, err = r.StartUsageHistoryRebuild(parent, scope)
		return false, err
	}
	_, err = r.AdvanceUsageHistoryProjection(parent, scope, generation, limit)
	return false, err
}

func (r *Repository) registerUsageProjectionScopes(ctx context.Context, limit int) (bool, error) {
	var scopes []string
	if err := r.ro.SelectContext(ctx, &scopes, r.ro.Rebind(`SELECT scope_id FROM usage_scopes WHERE projection_registered = 0 ORDER BY scope_id LIMIT ?`), limit); err != nil {
		return false, err
	}
	for _, scope := range scopes {
		tx, err := r.db.BeginTxx(ctx, nil)
		if err != nil {
			return false, err
		}
		_, _, err = r.lockUsageProjectionScopeTx(ctx, tx, scope)
		if err != nil {
			_ = tx.Rollback()
			return false, err
		}
		if err := tx.Commit(); err != nil {
			return false, err
		}
	}
	return len(scopes) > 0, nil
}

func (r *Repository) nextUsageProjection(ctx context.Context) (string, string, error) {
	queries := []string{
		`SELECT s.scope_id, '' FROM usage_projection_scopes s JOIN usage_scopes u ON u.scope_id=s.scope_id JOIN usage_projection_generations g
		 ON g.scope_id=s.scope_id AND g.generation=s.active_generation WHERE g.work_version <> u.work_version AND NOT EXISTS (
		 SELECT 1 FROM usage_projection_generations b WHERE b.scope_id=s.scope_id AND b.state IN ('scan','replay')) ORDER BY s.updated_at LIMIT 1`,
		`SELECT s.scope_id, s.active_generation FROM usage_projection_scopes s JOIN usage_projection_generations g
		 ON g.scope_id = s.scope_id AND g.generation = s.active_generation WHERE s.commit_sequence > g.replay_sequence
		 OR EXISTS(SELECT 1 FROM usage_projection_work w WHERE w.scope_id = s.scope_id AND w.generation = s.active_generation)
		 OR EXISTS(SELECT 1 FROM usage_projection_history h JOIN usage_projection_applied a
			ON a.scope_id=h.scope_id AND a.generation=h.generation AND a.work_key=h.work_key
			WHERE h.scope_id=s.scope_id AND h.generation=s.active_generation AND (h.provider_options_ready=0 OR h.provider_options_version=0))
		 ORDER BY s.updated_at LIMIT 1`,
		`SELECT scope_id, generation FROM usage_projection_generations WHERE state IN ('scan', 'replay') ORDER BY updated_at LIMIT 1`,
		`SELECT scope_id, active_generation FROM usage_projection_scopes s WHERE active_generation = '' AND NOT EXISTS (
		 SELECT 1 FROM usage_projection_generations g WHERE g.scope_id = s.scope_id AND g.state <> 'retired') ORDER BY updated_at LIMIT 1`,
	}
	for _, query := range queries {
		var scope, generation string
		err := r.ro.QueryRowxContext(ctx, query).Scan(&scope, &generation)
		if err == nil {
			return scope, generation, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", "", err
		}
	}
	return "", "", nil
}

func (r *Repository) collectUsageProjectionGarbage(parent context.Context, limit int) (bool, error) {
	ctx, release, err := r.beginAnalyticsOperation(parent)
	if err != nil {
		return false, err
	}
	defer release()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var scope, generation, build string
	err = tx.QueryRowxContext(ctx, `SELECT scope_id, generation, build_id FROM usage_projection_builds WHERE state = 'garbage' ORDER BY scope_id, generation, build_id LIMIT 1`).Scan(&scope, &generation, &build)
	if errors.Is(err, sql.ErrNoRows) {
		return r.collectRetiredUsageProjectionTx(ctx, tx, limit)
	}
	if err != nil {
		return false, err
	}
	if _, _, err := r.lockUsageProjectionScopeTx(ctx, tx, scope); err != nil {
		return false, err
	}
	for _, table := range []string{"usage_projection_contributions", "usage_canonical_facts"} {
		_, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM `+table+` WHERE scope_id = ? AND generation = ? AND build_id = ? AND fact_key IN (
			SELECT fact_key FROM `+table+` WHERE scope_id = ? AND generation = ? AND build_id = ? LIMIT ?)`), scope, generation, build, scope, generation, build, limit)
		if err != nil {
			return false, err
		}
		var remains bool
		if err := tx.GetContext(ctx, &remains, r.db.Rebind(`SELECT EXISTS(SELECT 1 FROM `+table+` WHERE scope_id = ? AND generation = ? AND build_id = ?)`), scope, generation, build); err != nil {
			return false, err
		}
		if remains {
			return false, tx.Commit()
		}
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_projection_builds WHERE scope_id = ? AND generation = ? AND build_id = ? AND state = 'garbage'`), scope, generation, build)
	if err != nil {
		return false, err
	}
	return false, tx.Commit()
}

func (r *Repository) collectRetiredUsageProjectionTx(ctx context.Context, tx *sqlx.Tx, limit int) (bool, error) {
	var scope, generation string
	err := tx.QueryRowxContext(ctx, `SELECT scope_id, generation FROM usage_projection_generations WHERE state = 'retired' AND NOT EXISTS (
		SELECT 1 FROM usage_projection_builds b WHERE b.scope_id = usage_projection_generations.scope_id AND b.generation = usage_projection_generations.generation) LIMIT 1`).Scan(&scope, &generation)
	if errors.Is(err, sql.ErrNoRows) {
		return true, tx.Commit()
	}
	if err != nil {
		return false, err
	}
	if _, _, err := r.lockUsageProjectionScopeTx(ctx, tx, scope); err != nil {
		return false, err
	}
	for _, entry := range usageProjectionTableKeys {
		if entry.table == "usage_projection_generations" || !strings.Contains(entry.key, "generation") {
			continue
		}
		query := `DELETE FROM ` + entry.table + ` WHERE (` + entry.key + `) IN (SELECT ` + entry.key + ` FROM ` + entry.table + ` WHERE scope_id = ? AND generation = ? LIMIT ?)`
		result, err := tx.ExecContext(ctx, r.db.Rebind(query), scope, generation, limit)
		if err != nil {
			return false, err
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if deleted == int64(limit) {
			return false, tx.Commit()
		}
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_projection_generations WHERE scope_id = ? AND generation = ? AND state = 'retired'`), scope, generation)
	if err != nil {
		return false, err
	}
	return false, tx.Commit()
}
