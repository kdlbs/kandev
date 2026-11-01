package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

const (
	usageProjectionProviderOptionsPhaseClear    = "clear"
	usageProjectionProviderOptionsPhaseBuild    = "build"
	usageProjectionProviderOptionsPhaseComplete = "complete"
)

type usageProjectionProviderOptionWork struct {
	Scope         string `db:"scope_id"`
	Generation    string `db:"generation"`
	Work          string `db:"work_key"`
	AppliedBuild  string `db:"applied_build_id"`
	OptionsBuild  string `db:"provider_options_build_id"`
	Cursor        string `db:"provider_options_cursor"`
	Phase         string `db:"provider_options_phase"`
	Version       int    `db:"provider_options_version"`
	LifetimeFacts int64  `db:"lifetime_count"`
}

// Each transaction clears or derives at most one bounded page for one work.
// Callers hold the scope publication lock across this operation.
func (r *Repository) advanceUsageProjectionProviderOptionsTx(ctx context.Context, tx *sqlx.Tx, scope, generation, active string, limit int) (bool, error) {
	var work usageProjectionProviderOptionWork
	err := tx.GetContext(ctx, &work, r.db.Rebind(`SELECT h.scope_id,h.generation,h.work_key,a.build_id AS applied_build_id,
		h.provider_options_build_id,h.provider_options_cursor,h.provider_options_phase,h.provider_options_version,h.lifetime_count
		FROM usage_projection_history h JOIN usage_projection_applied a
		ON a.scope_id=h.scope_id AND a.generation=h.generation AND a.work_key=h.work_key
		WHERE h.scope_id=? AND h.generation=? AND (h.provider_options_ready=0 OR h.provider_options_version=0)
		ORDER BY h.work_key LIMIT 1`), scope, generation)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if usageProjectionProviderOptionsNeedReset(work) {
		if err := r.resetUsageProjectionProviderOptionsTx(ctx, tx, work); err != nil {
			return true, err
		}
		work.OptionsBuild, work.Version, work.Cursor, work.Phase = work.AppliedBuild, 1, "", usageProjectionProviderOptionsPhaseClear
	}
	if work.Phase == usageProjectionProviderOptionsPhaseBuild {
		completed, err := r.advanceUsageProjectionProviderOptionsBuildTx(ctx, tx, work, limit)
		if err != nil || !completed || generation != active {
			return true, err
		}
		return true, r.invalidateUsageProjectionProviderOptionsSnapshotTx(ctx, tx, scope, generation)
	}
	return true, r.advanceUsageProjectionProviderOptionsClearTx(ctx, tx, work, limit)
}

func (r *Repository) invalidateUsageProjectionProviderOptionsSnapshotTx(ctx context.Context, tx *sqlx.Tx, scope, generation string) error {
	var pending bool
	if err := tx.GetContext(ctx, &pending, r.db.Rebind(`SELECT EXISTS(SELECT 1 FROM usage_projection_history
		WHERE scope_id=? AND generation=? AND (provider_options_ready=0 OR provider_options_version=0))`), scope, generation); err != nil {
		return err
	}
	if pending {
		return nil
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_scopes SET revision=revision+1
		WHERE scope_id=? AND active_generation=?`), scope, generation)
	return err
}

func usageProjectionProviderOptionsNeedReset(work usageProjectionProviderOptionWork) bool {
	return work.OptionsBuild != work.AppliedBuild || work.Version == 0 || work.Phase == "" || work.Phase == usageProjectionProviderOptionsPhaseComplete
}

func (r *Repository) resetUsageProjectionProviderOptionsTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionProviderOptionWork) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_history SET provider_options_build_id=?,provider_options_version=1,
		provider_options_cursor='',provider_options_phase='clear',provider_options_ready=0
		WHERE scope_id=? AND generation=? AND work_key=?`), work.AppliedBuild, work.Scope, work.Generation, work.Work)
	return err
}

func (r *Repository) advanceUsageProjectionProviderOptionsClearTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionProviderOptionWork, limit int) error {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_projection_provider_options
		WHERE (scope_id,generation,work_key,model,source,provider) IN (
			SELECT scope_id,generation,work_key,model,source,provider FROM usage_projection_provider_options
			WHERE scope_id=? AND generation=? AND work_key=? ORDER BY model,source,provider LIMIT ?)`),
		work.Scope, work.Generation, work.Work, limit)
	if err != nil {
		return err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if deleted >= int64(limit) {
		return nil
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_history SET provider_options_phase='build',provider_options_cursor=''
		WHERE scope_id=? AND generation=? AND work_key=?`), work.Scope, work.Generation, work.Work)
	return err
}

func (r *Repository) advanceUsageProjectionProviderOptionsBuildTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionProviderOptionWork, limit int) (bool, error) {
	var factKeys []string
	if err := tx.SelectContext(ctx, &factKeys, r.db.Rebind(`SELECT fact_key FROM usage_canonical_facts
		WHERE scope_id=? AND generation=? AND build_id=? AND fact_key>?
		ORDER BY fact_key LIMIT ?`), work.Scope, work.Generation, work.AppliedBuild, work.Cursor, limit); err != nil {
		return false, err
	}
	if len(factKeys) == 0 {
		return true, r.markUsageProjectionProviderOptionsReadyTx(ctx, tx, work)
	}
	if err := r.insertUsageProjectionProviderOptionsPageTx(ctx, tx, work, factKeys); err != nil {
		return false, err
	}
	complete := len(factKeys) < limit
	ready, phase := 0, usageProjectionProviderOptionsPhaseBuild
	if complete {
		ready, phase = 1, usageProjectionProviderOptionsPhaseComplete
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_history SET provider_options_cursor=?,provider_options_ready=?,provider_options_phase=?
		WHERE scope_id=? AND generation=? AND work_key=? AND provider_options_build_id=?`),
		factKeys[len(factKeys)-1], ready, phase, work.Scope, work.Generation, work.Work, work.AppliedBuild)
	return complete, err
}

func (r *Repository) markUsageProjectionProviderOptionsReadyTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionProviderOptionWork) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_history SET provider_options_ready=1,provider_options_phase='complete'
		WHERE scope_id=? AND generation=? AND work_key=?`), work.Scope, work.Generation, work.Work)
	return err
}

func (r *Repository) insertUsageProjectionProviderOptionsPageTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionProviderOptionWork, factKeys []string) error {
	query, arguments := usageProjectionProviderOptionsSelect(work, factKeys)
	insert := `INSERT INTO usage_projection_provider_options(scope_id,generation,work_key,model,source,provider) ` + query
	_, err := tx.ExecContext(ctx, r.db.Rebind(insert+` ON CONFLICT(scope_id,generation,work_key,model,source,provider) DO NOTHING`), arguments...)
	return err
}

func usageProjectionProviderOptionsSelect(work usageProjectionProviderOptionWork, factKeys []string) (string, []any) {
	// AppliedBuild is read under the scope lock, so the contribution primary key can seek by fact_key.
	arguments := []any{work.Scope, work.Generation, work.AppliedBuild}
	arguments = append(arguments, stringArgs(factKeys)...)
	query := `SELECT DISTINCT c.scope_id,c.generation,c.work_key,c.model,c.source,c.provider
		FROM usage_projection_contributions c
		WHERE c.scope_id=? AND c.generation=? AND c.build_id=? AND c.fact_key IN (` + placeholders(len(factKeys)) + `)`
	if work.LifetimeFacts > 0 {
		query = `SELECT DISTINCT c.scope_id,c.generation,c.work_key,c.model,c.source,c.provider
			FROM usage_projection_contributions c
			JOIN usage_canonical_facts f ON f.scope_id=c.scope_id AND f.generation=c.generation AND f.work_key=c.work_key AND f.build_id=c.build_id AND f.fact_key=c.fact_key
			WHERE c.scope_id=? AND c.generation=? AND c.build_id=? AND c.fact_key IN (` + placeholders(len(factKeys)) + `) AND (
				f.coverage_kind='lifetime' OR NOT EXISTS (
					SELECT 1 FROM usage_canonical_facts l
					WHERE l.scope_id=f.scope_id AND l.generation=f.generation AND l.work_key=f.work_key
					AND l.build_id=f.build_id
					AND (l.bundle_key<>f.bundle_key OR l.segment_key=f.segment_key) AND l.coverage_kind='lifetime'
					AND ((c.metric_kind='tokens' AND ` + usageLifetimeTokenAuthority("l") + `)
						OR (c.metric_kind='cost' AND ` + usageLifetimeCostMaskAuthority("l", "c.cost_subcents IS NULL") + `))))`
	}
	return query, arguments
}
