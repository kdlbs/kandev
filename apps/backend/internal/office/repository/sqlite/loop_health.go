package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
)

// TriggerHealthRow is one overdue or stranded cron trigger
// (REQ-OFFICE-LOOP-LIVENESS-004). Condition names which predicate
// matched so a single evidence list can carry both kinds (AC-004.7).
type TriggerHealthRow struct {
	TriggerID   string     `db:"id"`
	RoutineID   string     `db:"routine_id"`
	RoutineName string     `db:"routine_name"`
	NextRunAt   *time.Time `db:"next_run_at"`
	LastFiredAt *time.Time `db:"last_fired_at"`
	Condition   string     `db:"-"`
}

// CountEligibleTriggers reports how many enabled cron triggers exist
// for the workspace, regardless of due state — the input to the
// not_armed verdict (AC-004.6): zero means no trigger is scheduled at
// all, not that none happen to be due right now.
func (r *Repository) CountEligibleTriggers(ctx context.Context, workspaceID string) (int, error) {
	var count int
	err := r.ro.GetContext(ctx, &count, r.ro.Rebind(`
		SELECT COUNT(*) FROM office_routine_triggers t
		JOIN office_routines rt ON rt.id = t.routine_id
		WHERE t.kind = 'cron' AND t.enabled = 1 AND rt.workspace_id = ?
	`), workspaceID)
	return count, err
}

// ListOverdueOrStrandedTriggers returns eligible cron triggers that are
// overdue (armed but past grace) or stranded (claimed and never
// re-armed past grace), capped at limit, with the untruncated total
// from the same statement (AC-004.9, AC-004.12). A trigger claimed
// less than strandedGrace ago is a claiming trigger — excluded here
// rather than given a third label (AC-004.15).
func (r *Repository) ListOverdueOrStrandedTriggers(
	ctx context.Context, workspaceID string, now time.Time,
	overdueGrace, strandedGrace time.Duration, limit int,
) ([]TriggerHealthRow, int, error) {
	overdueBefore := now.Add(-overdueGrace)
	strandedBefore := now.Add(-strandedGrace)

	type row struct {
		TriggerHealthRow
		Total int `db:"total"`
	}
	var rows []row
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT t.id AS id, t.routine_id AS routine_id, rt.name AS routine_name,
		       t.next_run_at AS next_run_at, t.last_fired_at AS last_fired_at,
		       COUNT(*) OVER () AS total
		FROM office_routine_triggers t
		JOIN office_routines rt ON rt.id = t.routine_id
		WHERE t.kind = 'cron' AND t.enabled = 1 AND rt.workspace_id = ?
		  AND (
		    (t.next_run_at IS NOT NULL AND t.next_run_at <= ?)
		    OR (t.next_run_at IS NULL AND (t.last_fired_at IS NULL OR t.last_fired_at <= ?))
		  )
		ORDER BY (t.next_run_at IS NULL) DESC, t.next_run_at ASC, t.id ASC
		LIMIT ?
	`), workspaceID, overdueBefore, strandedBefore, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]TriggerHealthRow, 0, len(rows))
	total := 0
	for _, rr := range rows {
		hr := rr.TriggerHealthRow
		if hr.NextRunAt == nil {
			hr.Condition = "stranded"
		} else {
			hr.Condition = "overdue"
		}
		out = append(out, hr)
		total = rr.Total
	}
	return out, total, nil
}

// StuckRunRow is one queued or claimed run the scheduler should have
// progressed but has not (REQ-OFFICE-LOOP-LIVENESS-004). StuckSince is
// the clock the row aged from: claimed_at for a claimed run,
// COALESCE(scheduled_retry_at, requested_at) for a queued one.
type StuckRunRow struct {
	RunID          string     `db:"id"`
	AgentProfileID string     `db:"agent_profile_id"`
	Status         string     `db:"status"`
	StuckSince     *time.Time `db:"stuck_since"`
	Condition      string     `db:"-"`
}

// stuckRunsUnionSQL is the shared claimed+queued candidate set behind
// ListStuckRuns' stuck_since column, unchanged by dialect: both source
// expressions (`claimed_at`, `COALESCE(scheduled_retry_at, requested_at)`)
// are plain TIMESTAMP-typed columns on both engines.
const stuckRunsUnionSQL = `
	SELECT r.id, r.agent_profile_id, r.status, r.claimed_at AS stuck_since
	FROM runs r
	JOIN agent_profiles a ON a.id = r.agent_profile_id
	WHERE a.workspace_id = ? AND r.status = 'claimed'
	  AND (r.claimed_at IS NULL OR r.claimed_at <= ?)
	UNION ALL
	SELECT r.id, r.agent_profile_id, r.status,
	       COALESCE(r.scheduled_retry_at, r.requested_at) AS stuck_since
	FROM runs r
	JOIN agent_profiles a ON a.id = r.agent_profile_id
	WHERE a.workspace_id = ? AND r.status = 'queued'
	  AND r.current_route_attempt_seq = 0
	  AND r.routing_blocked_status IS NULL
	  AND COALESCE(r.scheduled_retry_at, r.requested_at) <= ?
	  AND NOT EXISTS (
	    SELECT 1 FROM runs s
	    WHERE s.agent_profile_id = r.agent_profile_id AND s.status = 'claimed'
	  )
`

// stuckSinceColumnExpr and stuckSinceLayout keep stuck_since portable
// across dialects. stuck_since is a computed UNION ALL column, so it
// carries no declared type for the driver to auto-parse into
// time.Time (unlike a direct table column read): on SQLite, strftime()
// normalizes it to a fixed ISO-8601 UTC string that the scan target
// (sql.NullString) parses by hand below. Postgres has no strftime;
// selecting the raw TIMESTAMP column there returns a native time.Time
// that database/sql's Scan already formats into an RFC3339Nano string
// on assignment to *sql.NullString, so one scan type and one dialect
// switch cover both engines without a second query or row type.
func stuckSinceColumnExpr(driver string) string {
	if dialect.IsPostgres(driver) {
		return "w.stuck_since"
	}
	return "strftime('%Y-%m-%dT%H:%M:%fZ', w.stuck_since)"
}

func stuckSinceLayout(driver string) string {
	if dialect.IsPostgres(driver) {
		return time.RFC3339Nano
	}
	return "2006-01-02T15:04:05.000Z"
}

// ListStuckRuns returns claimed-stuck and queued-stuck runs in one
// list, each row naming which condition it matched, ordered claimed
// first then by stuck instant (NULL first), capped with the same-
// statement untruncated total (AC-004.9, AC-004.12). Neither predicate
// is windowed (AC-004.5): a run stuck longer than
// officeLoopEvaluationWindow is the one most worth surfacing.
//
// The queued half mirrors ClaimNextEligibleRun's own filter
// (AC-004.17) as an accepted limitation: it restricts to
// current_route_attempt_seq = 0, so a routed run that has dispatched
// at least once and requeued reads as silent rather than stuck. That
// gap is intentional (OPERATOR DECISION F29/F30) — fixing it here
// would let this read disagree with what the scheduler would actually
// claim next.
func (r *Repository) ListStuckRuns(
	ctx context.Context, workspaceID string, now time.Time,
	queuedGrace, claimedGrace time.Duration, limit int,
) ([]StuckRunRow, int, error) {
	claimedBefore := now.Add(-claimedGrace)
	queuedBefore := now.Add(-queuedGrace)
	driver := r.ro.DriverName()

	type row struct {
		ID             string         `db:"id"`
		AgentProfileID string         `db:"agent_profile_id"`
		Status         string         `db:"status"`
		StuckSince     sql.NullString `db:"stuck_since"`
		Total          int            `db:"total"`
	}
	var rows []row
	query := fmt.Sprintf(`
		SELECT w.id AS id, w.agent_profile_id AS agent_profile_id, w.status AS status,
		       %s AS stuck_since,
		       COUNT(*) OVER () AS total
		FROM (%s) w
		ORDER BY (w.status = 'claimed') DESC, (w.stuck_since IS NULL) DESC, w.stuck_since ASC, w.id ASC
		LIMIT ?
	`, stuckSinceColumnExpr(driver), stuckRunsUnionSQL)
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(query),
		workspaceID, claimedBefore, workspaceID, queuedBefore, limit)
	if err != nil {
		return nil, 0, err
	}
	layout := stuckSinceLayout(driver)
	out := make([]StuckRunRow, 0, len(rows))
	total := 0
	for _, rr := range rows {
		hr := StuckRunRow{RunID: rr.ID, AgentProfileID: rr.AgentProfileID, Status: rr.Status}
		if rr.StuckSince.Valid {
			parsed, perr := time.Parse(layout, rr.StuckSince.String)
			if perr != nil {
				return nil, 0, fmt.Errorf("parse stuck_since %q: %w", rr.StuckSince.String, perr)
			}
			hr.StuckSince = &parsed
		}
		if hr.Status == "claimed" {
			hr.Condition = "claimed_stuck"
		} else {
			hr.Condition = "queued_stuck"
		}
		out = append(out, hr)
		total = rr.Total
	}
	return out, total, nil
}

// SilentSuccessRow is one finished run that asserted success but never
// named a session (AC-005.4 made an evidence row).
type SilentSuccessRow struct {
	RunID       string     `db:"id"`
	RequestedAt time.Time  `db:"requested_at"`
	FinishedAt  *time.Time `db:"finished_at"`
}

// ListSilentSuccesses returns finished/processed/sessionless runs whose
// COALESCE(finished_at, requested_at) falls in [windowStart, now),
// newest first, capped with the same-statement untruncated total
// (AC-004.7, AC-004.9, AC-004.16).
func (r *Repository) ListSilentSuccesses(
	ctx context.Context, workspaceID string, windowStart time.Time, limit int,
) ([]SilentSuccessRow, int, error) {
	type row struct {
		SilentSuccessRow
		Total int `db:"total"`
	}
	var rows []row
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT r.id AS id, r.requested_at AS requested_at, r.finished_at AS finished_at,
		       COUNT(*) OVER () AS total
		FROM runs r
		JOIN agent_profiles a ON a.id = r.agent_profile_id
		WHERE a.workspace_id = ? AND r.status = 'finished' AND r.outcome = 'processed'
		  AND (r.session_id IS NULL OR r.session_id = '')
		  AND COALESCE(r.finished_at, r.requested_at) >= ?
		ORDER BY COALESCE(r.finished_at, r.requested_at) DESC, r.id ASC
		LIMIT ?
	`), workspaceID, windowStart, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]SilentSuccessRow, 0, len(rows))
	total := 0
	for _, rr := range rows {
		out = append(out, rr.SilentSuccessRow)
		total = rr.Total
	}
	return out, total, nil
}

// TerminalRunShapeInputRow carries exactly the columns
// service.ClassifyTerminalRun needs, for every terminal run (not
// queued, not claimed) in the evaluation window (AC-004.16). The
// caller classifies and buckets; this method reads no free text
// (AC-005.7) and is side-effect free.
type TerminalRunShapeInputRow struct {
	Status      string     `db:"status"`
	Outcome     *string    `db:"outcome"`
	SessionID   string     `db:"session_id"`
	RequestedAt time.Time  `db:"requested_at"`
	FinishedAt  *time.Time `db:"finished_at"`
}

// ListTerminalRunsInWindow returns every terminal run for the
// workspace whose COALESCE(finished_at, requested_at) falls in
// [windowStart, now). Unbounded by officeLoopEvidenceCap: this feeds
// the terminal-shape counts, not an evidence list.
func (r *Repository) ListTerminalRunsInWindow(
	ctx context.Context, workspaceID string, windowStart time.Time,
) ([]TerminalRunShapeInputRow, error) {
	var rows []TerminalRunShapeInputRow
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT r.status AS status, r.outcome AS outcome, r.session_id AS session_id,
		       r.requested_at AS requested_at, r.finished_at AS finished_at
		FROM runs r
		JOIN agent_profiles a ON a.id = r.agent_profile_id
		WHERE a.workspace_id = ? AND r.status NOT IN ('queued', 'claimed')
		  AND COALESCE(r.finished_at, r.requested_at) >= ?
	`), workspaceID, windowStart)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []TerminalRunShapeInputRow{}
	}
	return rows, nil
}
