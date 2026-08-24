package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// usageEventTransientBackoffs is AC-32's fixed exponential backoff between
// transient-error retry attempts: 50 ms then 100 ms, base 50 ms, factor 2, no
// jitter. A single serial writer (AC-34) has no fleet of peers to
// desynchronize from, so jitter would only add variance.
var usageEventTransientBackoffs = [2]time.Duration{50 * time.Millisecond, 100 * time.Millisecond}

// maxUsageEventTransientAttempts is AC-32's cap on transient-classified
// insert attempts (the whole AC-11 transaction, not the INSERT statement
// alone) before giving up.
const maxUsageEventTransientAttempts = 3

// CreateTaskUsageEvent inserts one task_usage_events row and atomically
// increments the owning session's task_sessions rollup columns in the same
// transaction (AC-11), classifying and reacting to insert failures per
// AC-32:
//
//   - a unique violation on usage_event_id is reported as
//     ErrDuplicateUsageEvent, with no retry and no rollup increment (AC-14,
//     AC-32) - this is the one insert failure that is an expected outcome,
//     not a fault;
//   - a foreign-key violation is retried exactly once with session_id
//     forced to "" (no rollup increment on that retry, since
//     IncrementTaskSessionUsageTx no-ops on an empty session id); a second
//     failure of any kind is returned as-is;
//   - a transient failure (serialization failure, deadlock, SQLITE_BUSY /
//     SQLITE_LOCKED) is retried in a fresh transaction, waiting per
//     usageEventTransientBackoffs between attempts, up to
//     maxUsageEventTransientAttempts total attempts;
//   - anything else is returned as-is, uncounted against either retry
//     budget.
//
// Before the first attempt, if event.SessionID is set, CreateTaskUsageEvent
// verifies the session actually belongs to event.TaskID (AC-33): a
// same-task session proceeds unchanged; a session belonging to a different
// task is recorded under event.TaskID with session_id cleared and no rollup
// increment - deliberately the same end state as the foreign-key retry
// above, so the two share one reaction even though only the FK case is a
// genuine driver-level retry. A session lookup that fails for any reason
// other than "no such session" is returned immediately, uncounted against
// either retry budget: it means the writer could not determine ownership at
// all, not that a mismatch was found.
func (r *Repository) CreateTaskUsageEvent(ctx context.Context, event *models.TaskUsageEvent) error {
	working := *event

	if err := r.clearMismatchedUsageEventSession(ctx, &working); err != nil {
		return err
	}

	fkRetried := false
	transientAttempts := 0

	for {
		err := r.insertUsageEventAndRollup(ctx, &working)
		if err == nil {
			return nil
		}

		switch {
		case isUsageEventUniqueViolation(err):
			return ErrDuplicateUsageEvent
		case internaldb.IsForeignKeyViolation(err) && !fkRetried:
			fkRetried = true
			working.SessionID = ""
			continue
		case internaldb.IsTransientError(err):
			transientAttempts++
			if transientAttempts >= maxUsageEventTransientAttempts {
				return err
			}
			if sleepErr := sleepOrDone(ctx, usageEventTransientBackoffs[transientAttempts-1]); sleepErr != nil {
				return sleepErr
			}
			continue
		default:
			return err
		}
	}
}

// clearMismatchedUsageEventSession implements AC-33's pre-insert ownership
// check: if event.SessionID is set, it must belong to event.TaskID. A
// same-task session (or a session that does not exist at all - the real
// insert's foreign-key violation handles that case) is left unchanged; a
// session belonging to a different task has its SessionID cleared, deliberately
// the same terminal handling as the foreign-key retry in CreateTaskUsageEvent's
// loop. A lookup error other than "no such session" is returned immediately,
// uncounted against any retry budget: it means ownership could not be
// determined at all, not that a mismatch was found.
func (r *Repository) clearMismatchedUsageEventSession(ctx context.Context, event *models.TaskUsageEvent) error {
	if event.SessionID == "" {
		return nil
	}
	ownerTaskID, err := r.taskIDForSession(ctx, event.SessionID)
	switch {
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("verify usage event session ownership: %w", err)
	case err == nil && ownerTaskID != event.TaskID:
		if r.log != nil {
			r.log.Warn("usage event session does not belong to its task; recording without session attribution",
				zap.String("usage_event_id", event.UsageEventID),
				zap.String("task_id", event.TaskID),
				zap.String("session_id", event.SessionID),
				zap.String("session_owner_task_id", ownerTaskID))
		}
		event.SessionID = ""
	}
	return nil
}

// sleepOrDone waits d, returning ctx.Err() early if ctx is cancelled first,
// so a shutdown drain deadline (AC-34) can interrupt a queued backoff
// instead of blocking the writer's worker goroutine past it.
func sleepOrDone(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// taskIDForSession returns the task_id owning sessionID, or sql.ErrNoRows if
// no such session exists.
func (r *Repository) taskIDForSession(ctx context.Context, sessionID string) (string, error) {
	var taskID string
	err := r.db.QueryRowxContext(ctx, r.db.Rebind(
		`SELECT task_id FROM task_sessions WHERE id = ?`), sessionID).Scan(&taskID)
	return taskID, err
}

// insertUsageEventAndRollup is AC-11's atomic unit: the ledger insert and
// the task_sessions rollup increment, applied in a single transaction so no
// committed state exists in which one landed and the other did not. Returns
// the raw, unclassified driver error on failure - classification and
// reaction belong to CreateTaskUsageEvent.
func (r *Repository) insertUsageEventAndRollup(ctx context.Context, event *models.TaskUsageEvent) error {
	if r.failUsageEventAttempts > 0 {
		r.failUsageEventAttempts--
		return r.failUsageEventErr
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := r.insertUsageEventRowTx(ctx, tx, event); err != nil {
		return err
	}

	if r.failUsageEventRollupAttempts > 0 {
		r.failUsageEventRollupAttempts--
		return r.failUsageEventRollupErr
	}

	if r.usageEventPreRollupHook != nil {
		r.usageEventPreRollupHook()
	}

	if event.SessionID != "" {
		cachedIn := coalesceInt64(event.TokensCachedRead) + coalesceInt64(event.TokensCachedWrite)
		if err := r.IncrementTaskSessionUsageTx(ctx, tx, event.SessionID,
			event.TokensIn, cachedIn, coalesceInt64(event.TokensOut), event.CostSubcents); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// coalesceInt64 treats a nil *int64 as zero (AC-12): a not-recorded token
// count contributes zero to the rollup rather than nulling out the whole
// sum.
func coalesceInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func (r *Repository) insertUsageEventRowTx(ctx context.Context, tx *sqlx.Tx, event *models.TaskUsageEvent) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_usage_events (
			usage_event_id, task_id, session_id, turn_id,
			agent_profile_id, agent_type, model, provider,
			tokens_in, tokens_cached_read, tokens_cached_write, tokens_out, tokens_thought,
			tokens_total, cost_subcents, cost_source, estimated,
			rate_input_per_million, rate_cached_read_per_million,
			rate_cached_write_per_million, rate_output_per_million,
			pricing_catalog_version, contract_version, occurred_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), event.UsageEventID, event.TaskID, nullableString(event.SessionID), nullableString(event.TurnID),
		event.AgentProfileID, event.AgentType, event.Model, event.Provider,
		event.TokensIn, event.TokensCachedRead, event.TokensCachedWrite, event.TokensOut, event.TokensThought,
		event.TokensTotal, event.CostSubcents, event.CostSource, dialect.BoolToInt(event.Estimated),
		event.RateInputPerMillion, event.RateCachedReadPerMillion,
		event.RateCachedWritePerMillion, event.RateOutputPerMillion,
		nullableString(event.PricingCatalogVersion), event.ContractVersion, event.OccurredAt, event.CreatedAt)
	return err
}

// ListTaskUsageEvents returns taskID's ledger rows ordered by
// (occurred_at, id) ascending (AC-16), which is a total order over the
// table: no two rows compare equal, so repeated reads return an identical
// sequence. limit <= 0 means no limit. An unknown task and a task with no
// rows both return an empty, non-nil slice and a nil error.
func (r *Repository) ListTaskUsageEvents(ctx context.Context, taskID string, limit int) ([]*models.TaskUsageEvent, error) {
	query := `
		SELECT id, usage_event_id, task_id, session_id, turn_id,
		       agent_profile_id, agent_type, model, provider,
		       tokens_in, tokens_cached_read, tokens_cached_write, tokens_out, tokens_thought,
		       tokens_total, cost_subcents, cost_source, estimated,
		       rate_input_per_million, rate_cached_read_per_million,
		       rate_cached_write_per_million, rate_output_per_million,
		       pricing_catalog_version, contract_version, occurred_at, created_at
		  FROM task_usage_events
		 WHERE task_id = ?
		 ORDER BY occurred_at ASC, id ASC
	`
	args := []interface{}{taskID}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	events := make([]*models.TaskUsageEvent, 0)
	for rows.Next() {
		event, err := scanTaskUsageEventRow(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func scanTaskUsageEventRow(row *sqlx.Rows) (*models.TaskUsageEvent, error) {
	var (
		event                                                         models.TaskUsageEvent
		sessionID, turnID, pricingCatalogVersion                      sql.NullString
		tokensCachedRead, tokensCachedWrite, tokensOut, tokensThought sql.NullInt64
		rateInput, rateCachedRead, rateCachedWrite, rateOutput        sql.NullInt64
	)
	if err := row.Scan(
		&event.ID, &event.UsageEventID, &event.TaskID, &sessionID, &turnID,
		&event.AgentProfileID, &event.AgentType, &event.Model, &event.Provider,
		&event.TokensIn, &tokensCachedRead, &tokensCachedWrite, &tokensOut, &tokensThought,
		&event.TokensTotal, &event.CostSubcents, &event.CostSource, &event.Estimated,
		&rateInput, &rateCachedRead, &rateCachedWrite, &rateOutput,
		&pricingCatalogVersion, &event.ContractVersion, &event.OccurredAt, &event.CreatedAt,
	); err != nil {
		return nil, err
	}

	event.SessionID = sessionID.String
	event.TurnID = turnID.String
	event.PricingCatalogVersion = pricingCatalogVersion.String
	event.TokensCachedRead = nullInt64ToPtr(tokensCachedRead)
	event.TokensCachedWrite = nullInt64ToPtr(tokensCachedWrite)
	event.TokensOut = nullInt64ToPtr(tokensOut)
	event.TokensThought = nullInt64ToPtr(tokensThought)
	event.RateInputPerMillion = nullInt64ToPtr(rateInput)
	event.RateCachedReadPerMillion = nullInt64ToPtr(rateCachedRead)
	event.RateCachedWritePerMillion = nullInt64ToPtr(rateCachedWrite)
	event.RateOutputPerMillion = nullInt64ToPtr(rateOutput)

	return &event, nil
}

func nullInt64ToPtr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	value := v.Int64
	return &value
}
