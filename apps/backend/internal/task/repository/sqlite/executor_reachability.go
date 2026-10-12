package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

const executorReachabilityColumns = `executor_id, state, reason, message, consecutive_failures, host, checked_at, last_success_at, updated_at`

func scanExecutorReachability(scanner interface{ Scan(...any) error }) (*models.ExecutorReachability, error) {
	r := &models.ExecutorReachability{}
	var state, reason string
	var checkedAt, lastSuccessAt sql.NullTime
	if err := scanner.Scan(
		&r.ExecutorID, &state, &reason, &r.Message, &r.ConsecutiveFailures, &r.Host,
		&checkedAt, &lastSuccessAt, &r.UpdatedAt,
	); err != nil {
		return nil, err
	}
	r.State = models.ExecutorReachabilityState(state)
	r.Reason = models.ExecutorReachabilityReason(reason)
	if checkedAt.Valid {
		v := checkedAt.Time
		r.CheckedAt = &v
	}
	if lastSuccessAt.Valid {
		v := lastSuccessAt.Time
		r.LastSuccessAt = &v
	}
	return r, nil
}

// ListSSHExecutorsForReachability returns every eligible SSH executor
// (type=ssh, not soft-deleted, status=active) ordered ascending by id, the
// primary key, never reused, so the order is total and needs no tiebreak.
func (r *Repository) ListSSHExecutorsForReachability(ctx context.Context) ([]*models.Executor, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT id, name, type, status, is_system, resumable, config, created_at, updated_at, deleted_at
		FROM executors
		WHERE type = 'ssh' AND deleted_at IS NULL AND status = 'active'
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.Executor
	for rows.Next() {
		executor, err := scanExecutorRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, executor)
	}
	return result, rows.Err()
}

// GetExecutorReachability returns the stored record for one executor.
// Returns models.ErrExecutorReachabilityNotFound if none exists.
func (r *Repository) GetExecutorReachability(ctx context.Context, executorID string) (*models.ExecutorReachability, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT `+executorReachabilityColumns+` FROM executor_reachability WHERE executor_id = ?
	`), executorID)
	rec, err := scanExecutorReachability(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, models.ErrExecutorReachabilityNotFound
		}
		return nil, err
	}
	return rec, nil
}

// ListExecutorReachability returns every stored reachability record.
func (r *Repository) ListExecutorReachability(ctx context.Context) ([]*models.ExecutorReachability, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT `+executorReachabilityColumns+` FROM executor_reachability ORDER BY executor_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.ExecutorReachability
	for rows.Next() {
		rec, err := scanExecutorReachability(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, rec)
	}
	return result, rows.Err()
}

// UpsertExecutorReachability records a single probe (or launch dial)
// observation. The write is a single statement: the consecutive-failure
// counter and the derived state for an existing row are computed by the
// statement itself from the row's own prior values, so no read-modify-write
// in Go can lose a streak to an interleaving. A write whose CheckedAt is not
// strictly later than the stored checked_at is discarded (last-write-wins),
// and the row is only touched at all when the executor is still the active
// SSH executor obs.SeenUpdatedAt was captured against — see the system
// design's Persistence section for the full contract.
func (r *Repository) UpsertExecutorReachability(ctx context.Context, obs models.ExecutorReachabilityObservation) error {
	now := time.Now().UTC()
	checkedAt := obs.CheckedAt.UTC()
	var lastSuccessAt any
	if obs.Reason == "" {
		lastSuccessAt = checkedAt
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO executor_reachability (
			executor_id, state, reason, message, consecutive_failures, host,
			checked_at, last_success_at, updated_at
		)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?
		 WHERE EXISTS (SELECT 1 FROM executors e
				WHERE e.id = ? AND e.deleted_at IS NULL
				  AND e.status = 'active' AND e.updated_at = ?)
		ON CONFLICT (executor_id) DO UPDATE SET
			consecutive_failures = CASE WHEN excluded.reason = '' THEN 0
										ELSE executor_reachability.consecutive_failures + 1 END,
			state = CASE WHEN excluded.reason = '' THEN 'reachable'
						 WHEN excluded.reason IN ('config','host_key') THEN 'unreachable'
						 WHEN executor_reachability.consecutive_failures + 1 >= ?
							  THEN 'unreachable'
						 ELSE executor_reachability.state END,
			reason = excluded.reason, message = excluded.message, host = excluded.host,
			checked_at = excluded.checked_at,
			last_success_at = CASE WHEN excluded.reason = '' THEN excluded.checked_at
								   ELSE executor_reachability.last_success_at END,
			updated_at = excluded.updated_at
		WHERE executor_reachability.checked_at IS NULL
		   OR excluded.checked_at > executor_reachability.checked_at
	`),
		obs.ExecutorID, string(obs.InitialState), string(obs.Reason), obs.Message, obs.InitialFailures, obs.Host,
		checkedAt, lastSuccessAt, now,
		obs.ExecutorID, obs.SeenUpdatedAt.UTC(),
		obs.FailureThreshold,
	)
	return err
}

// ResetExecutorReachability invalidates the stored record after a
// connection-configuration save. It is its own statement, not a branch of
// the upsert above: the upsert's state CASE has no unknown branch and its
// WHERE admits only a strictly later checked_at, which a reset does not
// carry. The executor updated_at predicate couples the reset to the save
// version, so a delayed callback cannot reset a newer configuration. A
// no-op when the executor isn't an active SSH executor or the version is
// stale: AC-EXECUTORS-SSH-REACHABILITY-001.17 keeps a disabled executor's
// record untouched, so eligibility wins over the reset trigger.
func (r *Repository) ResetExecutorReachability(ctx context.Context, executorID, host string, seenUpdatedAt time.Time) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO executor_reachability (
			executor_id, state, reason, message, consecutive_failures,
			checked_at, host, last_success_at, updated_at
		)
		SELECT ?, 'unknown', '', '', 0, NULL, ?, NULL, ?
			 WHERE EXISTS (SELECT 1 FROM executors e
				WHERE e.id = ? AND e.type = 'ssh'
				  AND e.deleted_at IS NULL AND e.status = 'active'
				  AND e.updated_at = ?)
		ON CONFLICT (executor_id) DO UPDATE SET
			state = 'unknown', reason = '', message = '', consecutive_failures = 0,
			host = excluded.host, checked_at = NULL, last_success_at = NULL,
			updated_at = excluded.updated_at
	`), executorID, host, now, executorID, seenUpdatedAt.UTC())
	return err
}

// DeleteExecutorReachability removes the stored record for one executor.
func (r *Repository) DeleteExecutorReachability(ctx context.Context, executorID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM executor_reachability WHERE executor_id = ?
	`), executorID)
	return err
}
