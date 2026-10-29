package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

type usageProjectionWork struct {
	Scope      string `db:"scope_id"`
	Generation string `db:"generation"`
	Work       string `db:"work_key"`
	Subject    string `db:"subject_id"`
	Requested  int64  `db:"requested_sequence"`
	Selected   int64  `db:"selection_sequence"`
	Cursor     string `db:"cursor"`
	Build      string `db:"build_id"`
	State      string `db:"state"`
	Group      string `db:"publish_group"`
}

func (r *Repository) advanceUsageProjectionWorkTx(ctx context.Context, tx *sqlx.Tx, scope, generation string, limit int) (bool, error) {
	var work usageProjectionWork
	err := tx.GetContext(ctx, &work, r.db.Rebind(`SELECT * FROM usage_projection_work WHERE scope_id = ? AND generation = ? ORDER BY CASE WHEN state = 'complete' THEN 1 ELSE 0 END, work_key LIMIT 1`), scope, generation)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := r.prepareUsageProjectionWorkTx(ctx, tx, &work); err != nil {
		return true, err
	}
	if work.State == "complete" {
		return true, r.publishUsageProjectionGroupTx(ctx, tx, work)
	}
	if work.State == "resolve" {
		return true, r.resolveUsageProjectionPageTx(ctx, tx, work, limit)
	}
	query := models.UsageSelectionQuery{Partition: models.UsagePartition{ScopeID: scope, WorkKey: work.Work}, Cursor: work.Cursor, Limit: limit}
	page, err := r.selectUsageWorkTx(ctx, tx, query, scope, work.Work, false)
	if err != nil {
		return true, err
	}
	for _, fact := range page.Facts {
		if err := r.stageUsageProjectionFactTx(ctx, tx, work, fact); err != nil {
			return true, err
		}
	}
	if page.Preparing {
		if page.NextCursor != "" {
			work.Cursor = page.NextCursor
		}
		_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_work SET state = 'resolve', cursor = ? WHERE scope_id = ? AND generation = ? AND work_key = ?`), work.Cursor, scope, generation, work.Work)
		return true, err
	}
	if page.HasMore {
		if page.NextCursor == "" {
			return true, errors.New("usage selector did not advance its cursor")
		}
		_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_work SET cursor = ? WHERE scope_id = ? AND generation = ? AND work_key = ?`), page.NextCursor, scope, generation, work.Work)
		return true, err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_work SET state = 'complete' WHERE scope_id = ? AND generation = ? AND work_key = ?`), work.Scope, work.Generation, work.Work); err != nil {
		return true, err
	}
	return true, r.publishUsageProjectionGroupTx(ctx, tx, work)
}

func (r *Repository) prepareUsageProjectionWorkTx(ctx context.Context, tx *sqlx.Tx, work *usageProjectionWork) error {
	if work.Group == "" {
		work.Group = work.Work
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_work SET publish_group = ? WHERE scope_id = ? AND generation = ? AND work_key = ?`), work.Group, work.Scope, work.Generation, work.Work); err != nil {
			return err
		}
	}
	var current int64
	err := tx.GetContext(ctx, &current, r.db.Rebind(`SELECT commit_sequence FROM usage_projection_partitions WHERE scope_id = ? AND work_key = ?`), work.Scope, work.Work)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if work.Build != "" && work.Selected == current {
		return nil
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_builds SET state = 'garbage' WHERE scope_id = ? AND generation = ? AND build_id = ?`), work.Scope, work.Generation, work.Build); err != nil {
		return err
	}
	work.Build, work.Selected, work.Cursor, work.State = uuid.NewString(), current, "", "selecting"
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_projection_builds (scope_id, generation, work_key, build_id, state) VALUES (?, ?, ?, ?, 'staging')`), work.Scope, work.Generation, work.Work, work.Build); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_projection_resolutions WHERE scope_id = ? AND generation = ? AND work_key = ?`), work.Scope, work.Generation, work.Work); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_work SET build_id = ?, selection_sequence = ?, cursor = '', state = ? WHERE scope_id = ? AND generation = ? AND work_key = ?`), work.Build, current, work.State, work.Scope, work.Generation, work.Work)
	return err
}

func (r *Repository) stageUsageProjectionFactTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork, fact models.UsageCanonicalFact) error {
	fact.NativeAccountingVersion = normalizeNativeCanonicalAccounting(&fact, fact.NativeAccountingVersion)
	if fact.CoverageKind == usageCoveragePoint && fact.Measurement.CoverageStart != nil {
		end := fact.Measurement.CoverageStart.Add(time.Nanosecond)
		fact.Measurement.CoverageEnd = &end
	}
	payload, err := encodeUsageProjection(fact)
	if err != nil {
		return err
	}
	m := fact.Measurement
	tokenSource, costSource := "", ""
	if fact.TokenSource != nil {
		tokenSource = fact.TokenSource.Source
	}
	if fact.CostSource != nil {
		costSource = fact.CostSource.Source
	}
	start, end := usageProjectionFactBounds(fact)
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_canonical_facts
		(scope_id, generation, build_id, fact_key, work_key, subject_id, task_id, session_id, coverage_kind, coverage_start, coverage_end,
		source_date, calendar, model, provider, token_source, cost_source, payload, collected_at, segment_key, bundle_key) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope_id, generation, build_id, fact_key) DO UPDATE SET payload = excluded.payload, collected_at = excluded.collected_at`), work.Scope, work.Generation, work.Build, fact.Key, work.Work, m.SubjectID, m.TaskID, m.SessionID, fact.CoverageKind, start, end, m.SourceDate, m.SourceTimezone, m.Model, m.Provider, tokenSource, costSource, payload, m.CollectedAt, fact.SegmentKey, fact.BundleKey)
	if err != nil {
		return err
	}
	return r.stageUsageProjectionContributionsTx(ctx, tx, work, fact)
}

func (r *Repository) publishUsageProjectionWorkTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork) error {
	args := []any{work.Scope, work.Generation, work.Work, work.Build}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_projection_summary_queue (scope_id, generation, summary_key)
		SELECT DISTINCT scope_id, generation, summary_key FROM usage_projection_contributions c
		WHERE scope_id = ? AND generation = ? AND work_key = ? AND (build_id = ? OR build_id IN (
		SELECT build_id FROM usage_projection_applied a WHERE a.scope_id = c.scope_id AND a.generation = c.generation AND a.work_key = c.work_key))
		ON CONFLICT(scope_id, generation, summary_key) DO NOTHING`), args...)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_builds SET state = CASE WHEN build_id = ? THEN 'live' ELSE 'garbage' END WHERE scope_id = ? AND generation = ? AND work_key = ?`), work.Build, work.Scope, work.Generation, work.Work); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_projection_applied (scope_id, generation, work_key, build_id, commit_sequence) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(scope_id, generation, work_key) DO UPDATE SET build_id = excluded.build_id, commit_sequence = excluded.commit_sequence`), work.Scope, work.Generation, work.Work, work.Build, work.Selected)
	if err != nil {
		return err
	}
	if err := r.rebuildUsageProjectionSummariesTx(ctx, tx, work.Scope, work.Generation); err != nil {
		return err
	}
	if err := r.updateUsageProjectionHistoryTx(ctx, tx, work); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_generations SET revision = revision + 1, updated_at = ? WHERE scope_id = ? AND generation = ?`), time.Now().UTC(), work.Scope, work.Generation); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_dirty_partitions WHERE scope_id = ? AND (partition_key = ? OR partition_key LIKE ?)
		AND EXISTS(SELECT 1 FROM usage_projection_scopes WHERE scope_id = ? AND active_generation = ?)`), work.Scope, work.Work, work.Work+":%", work.Scope, work.Generation); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_projection_work WHERE scope_id = ? AND generation = ? AND work_key = ? AND selection_sequence = ?`), work.Scope, work.Generation, work.Work, work.Selected)
	return err
}
