package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

const (
	projectionScan    = "scan"
	projectionReplay  = "replay"
	projectionReady   = "ready"
	projectionRetired = "retired"
)

func projectionLimit(limit int) int {
	if limit < 1 || limit > 200 {
		return 200
	}
	return limit
}

// StartUsageHistoryRebuild is an internal maintenance operation. Its scope is
// captured accounting ownership, independent of the current live workspace org.
func (r *Repository) StartUsageHistoryRebuild(parent context.Context, scope string) (string, error) {
	ctx, release, err := r.beginAnalyticsOperation(parent)
	if err != nil {
		return "", err
	}
	defer release()
	generation, err := r.startUsageHistoryRebuild(ctx, scope)
	return generation, normalizeAnalyticsOperationError(parent, ctx, err)
}

func (r *Repository) startUsageHistoryRebuild(ctx context.Context, scope string) (string, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	sequence, _, err := r.lockUsageProjectionScopeTx(ctx, tx, scope)
	if err != nil {
		return "", err
	}
	var existing string
	err = tx.GetContext(ctx, &existing, r.db.Rebind(`SELECT generation FROM usage_projection_generations WHERE scope_id = ? AND state IN ('scan', 'replay') LIMIT 1`), scope)
	if err == nil {
		return existing, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	bounds, err := r.usageSourceBoundsTx(ctx, tx, scope)
	if err != nil {
		return "", err
	}
	generation, now := uuid.NewString(), time.Now().UTC()
	version, err := r.usageWorkVersionTx(ctx, tx, scope)
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_projection_generations
		(scope_id, generation, state, start_sequence, replay_sequence, created_at, updated_at, work_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`), scope, generation, projectionScan, sequence, sequence, now, now, version)
	if err != nil {
		return "", err
	}
	for table, upper := range bounds {
		_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_projection_checkpoints (scope_id, generation, source_table, upper_bound) VALUES (?, ?, ?, ?)`), scope, generation, table, upper)
		if err != nil {
			return "", err
		}
	}
	return generation, tx.Commit()
}

// AdvanceUsageHistoryProjection commits one bounded source, replay, or selector
// page. Only cutover changes which generation readers consider authoritative.
func (r *Repository) AdvanceUsageHistoryProjection(parent context.Context, scope, generation string, limit int) (bool, error) {
	ctx, release, err := r.beginAnalyticsOperation(parent)
	if err != nil {
		return false, err
	}
	defer release()
	done, err := r.advanceUsageProjection(ctx, scope, generation, projectionLimit(limit))
	return done, normalizeAnalyticsOperationError(parent, ctx, err)
}

func (r *Repository) advanceUsageProjection(ctx context.Context, scope, generation string, limit int) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	head, active, err := r.lockUsageProjectionScopeTx(ctx, tx, scope)
	if err != nil {
		return false, err
	}
	indexed, err := r.repairUsageProjectionSourcesTx(ctx, tx, scope, limit)
	if err != nil {
		return false, err
	}
	if indexed {
		return false, tx.Commit()
	}
	var state string
	var replay, version, required int64
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT g.state, g.replay_sequence, g.work_version, s.work_version FROM usage_projection_generations g JOIN usage_scopes s ON s.scope_id=g.scope_id WHERE g.scope_id = ? AND g.generation = ?`), scope, generation).Scan(&state, &replay, &version, &required)
	if err != nil {
		return false, ErrUsageNotVisible
	}
	if state == projectionRetired {
		return true, tx.Commit()
	}
	if version != required {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_generations SET state = ? WHERE scope_id = ? AND generation = ?`), projectionRetired, scope, generation); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	done := false
	switch {
	case state == projectionScan:
		err = r.scanUsageProjectionPageTx(ctx, tx, scope, generation, limit)
	case replay < head:
		err = r.replayUsageProjectionPageTx(ctx, tx, scope, generation, replay, head, limit)
	default:
		done, err = r.advanceUsageProjectionWorkOrCutoverTx(ctx, tx, scope, generation, active, limit)
	}
	if err != nil {
		return false, err
	}
	if err := r.pruneUsageProjectionLogTx(ctx, tx, scope, limit); err != nil {
		return false, err
	}
	return done, tx.Commit()
}

func (r *Repository) advanceUsageProjectionWorkOrCutoverTx(ctx context.Context, tx *sqlx.Tx, scope, generation, active string, limit int) (bool, error) {
	worked, err := r.advanceUsageProjectionWorkTx(ctx, tx, scope, generation, limit)
	if err != nil || worked {
		return false, err
	}
	err = r.cutoverUsageProjectionTx(ctx, tx, scope, generation, active)
	return err == nil, err
}

func (r *Repository) repairUsageProjectionSourcesTx(ctx context.Context, tx *sqlx.Tx, scope string, limit int) (bool, error) {
	worked, err := r.repairUsageContextPageTx(ctx, tx, scope, limit)
	if err != nil || worked {
		return worked, err
	}
	return r.repairUsageScopePageTx(ctx, tx, scope, limit)
}

func (r *Repository) scanUsageProjectionPageTx(ctx context.Context, tx *sqlx.Tx, scope, generation string, limit int) error {
	var table, upper, last string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT source_table, upper_bound, last_key FROM usage_projection_checkpoints
		WHERE scope_id = ? AND generation = ? AND complete = 0 ORDER BY source_table LIMIT 1`), scope, generation).Scan(&table, &upper, &last)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_generations SET state = ? WHERE scope_id = ? AND generation = ?`), projectionReplay, scope, generation)
		return err
	}
	if err != nil {
		return err
	}
	if table != usageMeasurementsTable && table != usageBucketsTable {
		return errors.New("invalid projection source checkpoint")
	}
	var ids []string
	query := `SELECT id FROM ` + table + ` WHERE scope_id = ? AND id > ? AND id <= ? ORDER BY id LIMIT ?`
	if err := tx.SelectContext(ctx, &ids, r.db.Rebind(query), scope, last, upper, limit); err != nil {
		return err
	}
	for _, id := range ids {
		if err := r.copyUsageProjectionSourceTx(ctx, tx, scope, generation, table, id); err != nil {
			return err
		}
		last = id
	}
	complete := 0
	if len(ids) < limit {
		complete = 1
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_checkpoints SET last_key = ?, complete = ? WHERE scope_id = ? AND generation = ? AND source_table = ?`), last, complete, scope, generation, table)
	return err
}

func (r *Repository) copyUsageProjectionSourceTx(ctx context.Context, tx *sqlx.Tx, scope, generation, table, id string) error {
	row, err := r.loadIndexedObservationTx(ctx, tx, table, id)
	if err != nil {
		return err
	}
	if row.WorkKey == "" {
		if err := r.indexRetainedSourceRowTx(ctx, tx, table, id); err != nil {
			return err
		}
		row, err = r.loadIndexedObservationTx(ctx, tx, table, id)
		if err != nil {
			return err
		}
	}
	var sequence int64
	err = tx.GetContext(ctx, &sequence, r.db.Rebind(`SELECT commit_sequence FROM usage_projection_source_versions WHERE scope_id = ? AND source_table = ? AND source_id = ?`), scope, table, id)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err = r.recordUsageMutationTx(ctx, tx, "index", nil, &row); err != nil {
			return err
		}
		err = tx.GetContext(ctx, &sequence, r.db.Rebind(`SELECT commit_sequence FROM usage_projection_source_versions WHERE scope_id = ? AND source_table = ? AND source_id = ?`), scope, table, id)
	}
	if err != nil {
		return err
	}
	if err := r.markUsageProjectionItemTx(ctx, tx, scope, generation, table, id, sequence, row.Retired); err != nil {
		return err
	}
	return r.enqueueUsageProjectionWorkTx(ctx, tx, generation, row.partition(), sequence)
}

func (r *Repository) markUsageProjectionItemTx(ctx context.Context, tx *sqlx.Tx, scope, generation, table, id string, sequence int64, tombstone int) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_projection_items (scope_id, generation, source_table, source_id, commit_sequence, tombstone)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(scope_id, generation, source_table, source_id) DO UPDATE
		SET commit_sequence = excluded.commit_sequence, tombstone = excluded.tombstone WHERE usage_projection_items.commit_sequence < excluded.commit_sequence
		OR (usage_projection_items.commit_sequence = excluded.commit_sequence AND excluded.tombstone = 0)`), scope, generation, table, id, sequence, tombstone)
	return err
}

func (r *Repository) replayUsageProjectionPageTx(ctx context.Context, tx *sqlx.Tx, scope, generation string, after, head int64, limit int) error {
	var entries []struct {
		Sequence int64  `db:"commit_sequence"`
		Payload  string `db:"payload"`
		Group    string `db:"publication_group"`
	}
	err := tx.SelectContext(ctx, &entries, r.db.Rebind(`SELECT commit_sequence, payload, publication_group FROM usage_projection_log WHERE scope_id = ? AND commit_sequence > ? AND commit_sequence <= ? ORDER BY commit_sequence LIMIT ?`), scope, after, head, limit)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Sequence != after+1 {
			return errors.New("usage projection replay has a sequence gap")
		}
		var replay usageProjectionReplay
		if err := decodeUsageProjection(entry.Payload, &replay); err != nil {
			return err
		}
		if replay.Version != 1 {
			return fmt.Errorf("unsupported usage replay version %d", replay.Version)
		}
		if err := r.applyUsageProjectionReplayTx(ctx, tx, scope, generation, entry.Sequence, replay); err != nil {
			return err
		}
		if err := r.linkUsageProjectionPublicationTx(ctx, tx, scope, generation, entry.Group, replay.Mutation); err != nil {
			return err
		}
		after = entry.Sequence
	}
	if len(entries) == 0 {
		return errors.New("usage projection replay payload is missing")
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_generations SET replay_sequence = ?, updated_at = ? WHERE scope_id = ? AND generation = ?`), after, time.Now().UTC(), scope, generation)
	return err
}

func (r *Repository) applyUsageProjectionReplayTx(ctx context.Context, tx *sqlx.Tx, scope, generation string, sequence int64, replay usageProjectionReplay) error {
	change := replay.Mutation
	for _, partition := range []*models.UsagePartition{change.OldPartition, change.NewPartition} {
		if partition != nil {
			if partition.ScopeID != scope {
				return errors.New("usage replay crosses retained scopes")
			}
			if err := r.enqueueUsageProjectionWorkTx(ctx, tx, generation, *partition, sequence); err != nil {
				return err
			}
		}
	}
	for _, item := range []struct {
		row       *models.SessionUsageMeasurement
		table     string
		tombstone int
	}{{change.Before, replay.BeforeTable, 1}, {change.After, replay.AfterTable, 0}} {
		if item.row != nil {
			if err := r.markUsageProjectionItemTx(ctx, tx, scope, generation, item.table, item.row.ID, sequence, item.tombstone); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Repository) enqueueUsageProjectionWorkTx(ctx context.Context, tx *sqlx.Tx, generation string, partition models.UsagePartition, sequence int64) error {
	var applied int64
	err := tx.GetContext(ctx, &applied, r.db.Rebind(`SELECT commit_sequence FROM usage_projection_applied WHERE scope_id = ? AND generation = ? AND work_key = ?`), partition.ScopeID, generation, partition.WorkKey)
	if err == nil && applied >= sequence {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_projection_work (scope_id, generation, work_key, subject_id, requested_sequence, publish_group) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope_id, generation, work_key) DO UPDATE SET requested_sequence = CASE WHEN excluded.requested_sequence > usage_projection_work.requested_sequence THEN excluded.requested_sequence ELSE usage_projection_work.requested_sequence END,
		state = CASE WHEN excluded.requested_sequence > usage_projection_work.selection_sequence THEN 'pending' ELSE usage_projection_work.state END`), partition.ScopeID, generation, partition.WorkKey, partition.SubjectID, sequence, partition.WorkKey)
	return err
}

func (r *Repository) cutoverUsageProjectionTx(ctx context.Context, tx *sqlx.Tx, scope, generation, active string) error {
	if active != generation {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_builds SET state = 'garbage' WHERE scope_id = ? AND generation = ?`), scope, active); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_generations SET state = ? WHERE scope_id = ? AND generation = ?`), projectionRetired, scope, active); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_scopes SET active_generation = ?, revision = revision + 1 WHERE scope_id = ?`), generation, scope); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_generations SET state = ?, updated_at = ? WHERE scope_id = ? AND generation = ?`), projectionReady, time.Now().UTC(), scope, generation); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_dirty_partitions WHERE scope_id = ?`), scope)
	return err
}

func (r *Repository) pruneUsageProjectionLogTx(ctx context.Context, tx *sqlx.Tx, scope string, limit int) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_projection_log WHERE scope_id = ? AND commit_sequence IN (
		SELECT commit_sequence FROM usage_projection_log WHERE scope_id = ? AND commit_sequence <= (
		SELECT MIN(replay_sequence) FROM usage_projection_generations WHERE scope_id = ? AND state <> 'retired') ORDER BY commit_sequence LIMIT ?)`), scope, scope, scope, limit)
	return err
}
