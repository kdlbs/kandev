package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
)

const (
	passInterval      = 10 * time.Minute
	dailyInterval     = 24 * time.Hour
	unfinishedAfter   = 24 * time.Hour
	passWindow        = 24 * time.Hour
	turnRetention     = 400 * 24 * time.Hour
	snapshotRetention = 90 * 24 * time.Hour
	batchSize         = 500
)

func (l *Ledger) runJobs() {
	defer l.wg.Done()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-l.stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	l.RunDaily(ctx)
	l.RunPass(ctx)
	pass, daily := time.NewTicker(passInterval), time.NewTicker(dailyInterval)
	defer pass.Stop()
	defer daily.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-pass.C:
			l.RunPass(ctx)
		case <-daily.C:
			l.RunDaily(ctx)
		}
	}
}

// RunPass is the 10-minute pass over rows finished in the last 24 hours: it
// corrects a ceiling or pause outcome first recorded from the session state,
// and fills a model usage reported after completion. The two are independent
// statements, so a row with its model already set is still corrected.
func (l *Ledger) RunPass(ctx context.Context) {
	l.safe(StageSettle, func() error { return l.correctOutcomes(ctx) })
	l.safe(StageModel, func() error { return l.fillMissingModels(ctx) })
}

func (l *Ledger) correctOutcomes(ctx context.Context) error {
	db := l.deps.DB
	_, err := db.ExecContext(ctx, db.Rebind(`
		UPDATE coordinator_turns SET verdict = ?, outcome = (
			SELECT u.outcome FROM coordinator_unattended_turns u WHERE u.ledger_turn_id = coordinator_turns.id)
		WHERE finished_at >= ? AND outcome NOT IN ('stopped_at_ceiling', 'stopped_by_pause')
		  AND EXISTS (SELECT 1 FROM coordinator_unattended_turns u
		              WHERE u.ledger_turn_id = coordinator_turns.id AND u.outcome IN ('stopped_at_ceiling', 'stopped_by_pause'))`),
		VerdictBlocked, l.now().Add(-passWindow).UTC())
	if err != nil {
		return fmt.Errorf("correct outcomes: %w", err)
	}
	return nil
}

func (l *Ledger) fillMissingModels(ctx context.Context) error {
	var rows []struct {
		ID            string `db:"id"`
		SessionTurnID string `db:"session_turn_id"`
	}
	db := l.deps.DB
	if err := db.SelectContext(ctx, &rows, db.Rebind(
		`SELECT id, session_turn_id FROM coordinator_turns WHERE finished_at >= ? AND model = '' ORDER BY finished_at, id`),
		l.now().Add(-passWindow).UTC()); err != nil {
		return fmt.Errorf("list rows without a model: %w", err)
	}
	for _, r := range rows {
		if err := l.fillModel(ctx, r.ID, r.SessionTurnID); err != nil {
			return err
		}
	}
	return nil
}

// RunDaily settles stuck rows, then applies retention. A failing batch is
// counted and ends its job; the next run resumes because deletion is keyed by
// age.
func (l *Ledger) RunDaily(ctx context.Context) {
	l.safe(StageSettle, func() error { return l.settleStuck(ctx) })
	l.safe(StageRetention, func() error { return l.deleteOldTurns(ctx) })
	l.safe(StageRetention, func() error { return l.deleteOldSnapshots(ctx) })
}

type stuckRow struct {
	ID            string `db:"id"`
	SessionID     string `db:"session_id"`
	SessionTurnID string `db:"session_turn_id"`
}

// settleStuck closes rows unfinished for more than 24 hours.
func (l *Ledger) settleStuck(ctx context.Context) error {
	db := l.deps.DB
	cutoff := l.now().Add(-unfinishedAfter).UTC()
	for {
		var batch []stuckRow
		if err := db.SelectContext(ctx, &batch, db.Rebind(
			`SELECT id, session_id, session_turn_id FROM coordinator_turns WHERE finished_at IS NULL AND started_at < ? ORDER BY started_at, id LIMIT ?`),
			cutoff, batchSize); err != nil {
			return fmt.Errorf("list stuck turns: %w", err)
		}
		if len(batch) == 0 {
			return nil
		}
		ids := make([]string, len(batch))
		for i, r := range batch {
			ids[i] = r.ID
		}
		query, args, err := sqlx.In(`UPDATE coordinator_turns SET finished_at = started_at, outcome = 'interrupted', verdict = 'blocked'
			WHERE finished_at IS NULL AND id IN (?)`, ids)
		if err != nil {
			return fmt.Errorf("settle stuck turns: %w", err)
		}
		if _, err := db.ExecContext(ctx, db.Rebind(query), args...); err != nil {
			return fmt.Errorf("settle stuck turns: %w", err)
		}
		for _, r := range batch {
			l.dropActive(r.SessionID, r.ID, r.SessionTurnID)
		}
		if len(batch) < batchSize {
			return nil
		}
	}
}

// deleteOldTurns removes rows older than 400 days with their call rows, one
// transaction per batch of 500.
func (l *Ledger) deleteOldTurns(ctx context.Context) error {
	cutoff := l.now().Add(-turnRetention).UTC()
	for {
		n, err := l.deleteTurnBatch(ctx, cutoff)
		if err != nil || n < batchSize {
			return err
		}
	}
}

func (l *Ledger) deleteTurnBatch(ctx context.Context, cutoff time.Time) (int, error) {
	db := l.deps.DB
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin retention batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var ids []string
	if err := tx.SelectContext(ctx, &ids, tx.Rebind(
		`SELECT id FROM coordinator_turns WHERE started_at < ? ORDER BY started_at, id LIMIT ?`), cutoff, batchSize); err != nil {
		return 0, fmt.Errorf("list old turns: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	for _, table := range []struct{ name, column string }{{"coordinator_turn_calls", "turn_id"}, {"coordinator_turns", "id"}} {
		query, args, err := sqlx.In(`DELETE FROM `+table.name+` WHERE `+table.column+` IN (?)`, ids)
		if err != nil {
			return 0, fmt.Errorf("delete old turns: %w", err)
		}
		if _, err := tx.ExecContext(ctx, tx.Rebind(query), args...); err != nil {
			return 0, fmt.Errorf("delete old turns: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit retention batch: %w", err)
	}
	return len(ids), nil
}

// deleteOldSnapshots removes snapshots no ledger row started within the last
// 90 days references. The delete re-checks the reference inside its own
// transaction, so a snapshot a turn just started on is never removed.
func (l *Ledger) deleteOldSnapshots(ctx context.Context) error {
	cutoff := l.now().Add(-snapshotRetention).UTC()
	for {
		n, err := l.deleteSnapshotBatch(ctx, cutoff)
		if err != nil || n < batchSize {
			return err
		}
	}
}

func (l *Ledger) deleteSnapshotBatch(ctx context.Context, cutoff time.Time) (int, error) {
	db := l.deps.DB
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin snapshot batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := l.lockSnapshots(ctx, tx); err != nil {
		return 0, err
	}
	const unreferenced = `NOT EXISTS (SELECT 1 FROM coordinator_turns t WHERE t.snapshot_hash = coordinator_turn_snapshots.hash AND t.started_at >= ?)`
	var hashes []string
	if err := tx.SelectContext(ctx, &hashes, tx.Rebind(
		`SELECT hash FROM coordinator_turn_snapshots WHERE `+unreferenced+` ORDER BY hash LIMIT ?`), cutoff, batchSize); err != nil {
		return 0, fmt.Errorf("list old snapshots: %w", err)
	}
	if len(hashes) == 0 {
		return 0, nil
	}
	query, args, err := sqlx.In(`DELETE FROM coordinator_turn_snapshots WHERE hash IN (?) AND `+unreferenced, hashes, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete old snapshots: %w", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(query), args...); err != nil {
		return 0, fmt.Errorf("delete old snapshots: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit snapshot batch: %w", err)
	}
	return len(hashes), nil
}

// lockSnapshots serialises snapshot publication against snapshot retention on
// PostgreSQL, where a not-yet-committed turn row is invisible to the retention
// re-check. SQLite's single writer already serialises them.
func (l *Ledger) lockSnapshots(ctx context.Context, tx *sqlx.Tx) error {
	if !dialect.IsPostgres(l.deps.DB.DriverName()) {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('coordinator_turn_snapshots', 0))`); err != nil {
		return fmt.Errorf("lock snapshots: %w", err)
	}
	return nil
}
