package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// Wakeup request status constants — kept in sync with the spec's
// "queued" | "claimed" | "coalesced" | "skipped" | "failed" enum.
// Terminal states all stamp finished_at; only "queued" is visible to the
// dispatcher's claim path.
const (
	WakeupStatusQueued    = "queued"
	WakeupStatusClaimed   = "claimed"
	WakeupStatusCoalesced = "coalesced"
	WakeupStatusSkipped   = "skipped"
	WakeupStatusFailed    = "failed"
)

// ErrWakeupIdempotencyConflict is returned by CreateWakeupRequest when
// a row with the same idempotency_key already exists. The dispatcher /
// cron handler treats this as "another caller already enqueued this
// fire; silently advance and move on" — it is not an error condition.
var ErrWakeupIdempotencyConflict = errors.New("wakeup request idempotency conflict")

// WakeupRequest is one row in agent_wakeup_requests — the unifying queue
// for "wake this agent up" requests across heartbeat / comment /
// agent-error / routine / self / user sources.
//
// IdempotencyKey uses sql.NullString so the partial UNIQUE index on
// (idempotency_key) WHERE idempotency_key != ” can distinguish
// "no key" (NULL) from "empty key" (empty string is treated as no key
// by the index expression). RunID stays a regular string — empty
// means "not yet attached to a run" rather than NULL semantics.
type WakeupRequest struct {
	ID             string         `db:"id"`
	AgentProfileID string         `db:"agent_profile_id"`
	Source         string         `db:"source"`
	Reason         string         `db:"reason"`
	Payload        string         `db:"payload"`
	Status         string         `db:"status"`
	CoalescedCount int            `db:"coalesced_count"`
	IdempotencyKey sql.NullString `db:"idempotency_key"`
	RunID          string         `db:"run_id"`
	RequestedAt    time.Time      `db:"requested_at"`
	ClaimedAt      sql.NullTime   `db:"claimed_at"`
	FinishedAt     sql.NullTime   `db:"finished_at"`
}

// CreateWakeupRequest inserts a new wakeup-request row. When
// IdempotencyKey is set and the partial UNIQUE index is violated, the
// function returns ErrWakeupIdempotencyConflict (wrapped) so the cron
// handler / dispatcher can short-circuit cleanly without inspecting
// driver-specific error strings.
//
// Defaults: Status defaults to WakeupStatusQueued; CoalescedCount to 1;
// Payload to "{}"; RequestedAt to time.Now().UTC() when zero.
func (r *Repository) CreateWakeupRequest(ctx context.Context, req *WakeupRequest) error {
	if req == nil {
		return errors.New("nil wakeup request")
	}
	if req.AgentProfileID == "" {
		return errors.New("wakeup request: agent_profile_id required")
	}
	if req.Source == "" {
		return errors.New("wakeup request: source required")
	}
	if req.Status == "" {
		req.Status = WakeupStatusQueued
	}
	if req.CoalescedCount <= 0 {
		req.CoalescedCount = 1
	}
	if strings.TrimSpace(req.Payload) == "" {
		req.Payload = "{}"
	}
	if req.RequestedAt.IsZero() {
		req.RequestedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO agent_wakeup_requests (
			id, agent_profile_id, source, reason, payload, status,
			coalesced_count, idempotency_key, run_id,
			requested_at, claimed_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		req.ID, req.AgentProfileID, req.Source, req.Reason, req.Payload, req.Status,
		req.CoalescedCount, req.IdempotencyKey, req.RunID,
		req.RequestedAt, req.ClaimedAt, req.FinishedAt,
	)
	if err != nil && isUniqueConstraintErr(err) {
		return fmt.Errorf("%w: %v", ErrWakeupIdempotencyConflict, err)
	}
	return err
}

// GetWakeupRequest returns the row for the given id. Returns
// sql.ErrNoRows wrapped when no such row exists.
func (r *Repository) GetWakeupRequest(ctx context.Context, id string) (*WakeupRequest, error) {
	var row WakeupRequest
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT id, agent_profile_id, source, reason, payload, status,
		       coalesced_count, idempotency_key, run_id,
		       requested_at, claimed_at, finished_at
		FROM agent_wakeup_requests
		WHERE id = ?
	`), id).StructScan(&row)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ListQueuedWakeupRequestsForAgent returns all wakeup requests in
// status="queued" for the given agent, ordered oldest first. Used by
// the dispatcher to drain the agent's queue per tick.
func (r *Repository) ListQueuedWakeupRequestsForAgent(
	ctx context.Context, agentProfileID string,
) ([]*WakeupRequest, error) {
	var rows []*WakeupRequest
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT id, agent_profile_id, source, reason, payload, status,
		       coalesced_count, idempotency_key, run_id,
		       requested_at, claimed_at, finished_at
		FROM agent_wakeup_requests
		WHERE agent_profile_id = ? AND status = ?
		ORDER BY requested_at ASC
	`), agentProfileID, WakeupStatusQueued)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []*WakeupRequest{}
	}
	return rows, nil
}

// MarkWakeupRequestClaimed transitions a request from queued → claimed
// and attaches the run_id of the freshly-created run. Stamps both
// claimed_at and finished_at to "now": claimed is the dispatcher's
// terminal state for this row (the run takes over from here).
func (r *Repository) MarkWakeupRequestClaimed(
	ctx context.Context, id, runID string,
) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_wakeup_requests
		SET status = ?, run_id = ?, claimed_at = ?, finished_at = ?
		WHERE id = ?
	`), WakeupStatusClaimed, runID, now, now, id)
	return err
}

// MarkWakeupRequestCoalesced transitions a request to status="coalesced",
// attaches the existing run id it was merged into, and bumps that run's
// coalesced_count. The wakeup request's payload is also merged into the
// run's context_snapshot via JSON-merge: top-level keys from this
// request's payload overwrite the prior snapshot.
//
// All four updates run on the writer in sequence. They are not wrapped
// in a transaction because SQLite serialises writes anyway and the
// dispatcher tolerates partial visibility — the worst case is a request
// marked "coalesced" but the run's count not incremented, which only
// affects telemetry.
func (r *Repository) MarkWakeupRequestCoalesced(
	ctx context.Context, id, intoRunID string,
) error {
	now := time.Now().UTC()
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_wakeup_requests
		SET status = ?, run_id = ?, claimed_at = ?, finished_at = ?
		WHERE id = ?
	`), WakeupStatusCoalesced, intoRunID, now, now, id); err != nil {
		return err
	}
	if intoRunID == "" {
		return nil
	}
	if err := r.bumpRunCoalescedCount(ctx, intoRunID); err != nil {
		return err
	}
	return r.mergeWakeupPayloadIntoRunSnapshot(ctx, id, intoRunID)
}

// PromoteRunAndCoalesceWakeupIfQueued atomically promotes a queued run,
// attaches the wakeup request, increments the coalesced count, and merges the
// request payload. The first run update holds the writer lock until commit, so
// the scheduler cannot claim the run with only part of the coalesced state.
// It returns false when the scheduler claimed the run before this transaction.
func (r *Repository) PromoteRunAndCoalesceWakeupIfQueued(
	ctx context.Context, requestID, runID, reason string,
) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE runs SET reason = ? WHERE id = ? AND status = 'queued'
	`), reason, runID)
	if err != nil {
		return false, err
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if updated == 0 {
		return false, nil
	}

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE agent_wakeup_requests
		SET status = ?, run_id = ?, claimed_at = ?, finished_at = ?
		WHERE id = ?
	`), WakeupStatusCoalesced, runID, now, now, requestID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE runs SET coalesced_count = coalesced_count + 1 WHERE id = ?
	`), runID); err != nil {
		return false, err
	}
	if err := mergeWakeupPayloadIntoRunSnapshotWith(ctx, tx, requestID, runID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// bumpRunCoalescedCount increments coalesced_count on the runs row by 1.
// Used by MarkWakeupRequestCoalesced when a wakeup-request lands on top
// of an in-flight run for the same agent.
func (r *Repository) bumpRunCoalescedCount(ctx context.Context, runID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs SET coalesced_count = coalesced_count + 1 WHERE id = ?
	`), runID)
	return err
}

// mergeWakeupPayloadIntoRunSnapshot merges the wakeup request's payload
// into the target run's context_snapshot. Delegates to
// mergeWakeupPayloadIntoRunSnapshotWith on r.db — the non-transactional
// merge site; PromoteRunAndCoalesceWakeupIfQueued calls the same helper on
// its own tx instead of duplicating the SQL, so there is exactly one
// json_patch call site for both merge entry points.
func (r *Repository) mergeWakeupPayloadIntoRunSnapshot(
	ctx context.Context, requestID, runID string,
) error {
	return mergeWakeupPayloadIntoRunSnapshotWith(ctx, r.db, requestID, runID)
}

// mergeWakeupPayloadIntoRunSnapshotWith merges the wakeup request's
// payload into the target run's context_snapshot via SQLite's json_patch
// (top-level merge: keys from the request payload overwrite same-named
// keys already on the snapshot). When the snapshot is empty / NULL it
// initialises to "{}" first so the patch lands on a valid object.
//
// The three routine catch-up gap-summary keys are stripped from the
// incoming payload before the patch via json_remove: a coalesced or
// promoted wakeup request never adds, changes, or removes the gap
// statement an existing run's agent already carries
// (AC-OFFICE-ROUTINE-CATCHUP-002.10) — each run's gap belongs only to the
// claim that created it. json_remove of an absent key is a no-op in
// SQLite, so this is byte-for-byte safe for every non-routine wakeup
// source, none of which writes these keys.
//
// exec is r.db for the non-transactional caller or a *sqlx.Tx for the
// transactional one — sqlx.ExtContext (which both satisfy) already
// includes Rebind via its embedded binder interface.
func mergeWakeupPayloadIntoRunSnapshotWith(
	ctx context.Context, exec sqlx.ExtContext, requestID, runID string,
) error {
	_, err := exec.ExecContext(ctx, exec.Rebind(`
		UPDATE runs
		SET context_snapshot = json_patch(
			COALESCE(NULLIF(context_snapshot, ''), '{}'),
			json_remove(
				(SELECT payload FROM agent_wakeup_requests WHERE id = ?),
				'$.missed_ticks', '$.missed_since', '$.missed_truncated'
			)
		)
		WHERE id = ?
	`), requestID, runID)
	return err
}

// MarkWakeupRequestSkipped transitions a request to status="skipped"
// when the agent's concurrency policy decides not to process it. The
// reason is stored verbatim in the request's reason column for
// telemetry; an empty reason leaves the original reason intact.
func (r *Repository) MarkWakeupRequestSkipped(
	ctx context.Context, id, reason string,
) error {
	now := time.Now().UTC()
	if reason == "" {
		_, err := r.db.ExecContext(ctx, r.db.Rebind(`
			UPDATE agent_wakeup_requests
			SET status = ?, finished_at = ?
			WHERE id = ?
		`), WakeupStatusSkipped, now, id)
		return err
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_wakeup_requests
		SET status = ?, reason = ?, finished_at = ?
		WHERE id = ?
	`), WakeupStatusSkipped, reason, now, id)
	return err
}

// MarkWakeupRequestFailed transitions a request to a terminal failed state
// when direct dispatch cannot claim it. This prevents a queued row from
// appearing healthy when no background poller can retry it.
func (r *Repository) MarkWakeupRequestFailed(ctx context.Context, id, reason string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_wakeup_requests
		SET status = ?, reason = ?, finished_at = ?
		WHERE id = ? AND status = ?
	`), WakeupStatusFailed, reason, now, id, WakeupStatusQueued)
	return err
}

// isUniqueConstraintErr returns true when err looks like a SQLite
// UNIQUE constraint violation. Driver-specific error inspection would
// be cleaner but the go-sqlite3 driver's typed error doesn't surface
// outside the package; matching on the prefix is the documented work-
// around used elsewhere in the codebase.
func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed: UNIQUE")
}
