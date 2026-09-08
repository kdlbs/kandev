package delivery

import (
	"context"
	"expvar"
	"fmt"
	"time"
)

// expvar maps/ints published at package init, exposed via the stdlib
// /debug/vars handler, following the subproc_* / routing_* precedent
// (apps/backend/internal/common/subproc/metrics.go,
// internal/office/scheduler/metrics_vars.go). See spec "Writer health".
var (
	evaluationsTotal            = expvar.NewMap("delivery_ledger_evaluations_total") // keyed by delivery_outcome
	rowsWrittenTotal            = expvar.NewInt("delivery_ledger_rows_written_total")
	demotionsSuppressedTotal    = expvar.NewInt("delivery_ledger_demotions_suppressed_total")
	evaluationErrorsTotal       = expvar.NewInt("delivery_ledger_evaluation_errors_total")
	ancestryErrorsTotal         = expvar.NewInt("delivery_ledger_ancestry_errors_total")
	ancestrySkippedTotal        = expvar.NewInt("delivery_ledger_ancestry_skipped_total")
	writeErrorsTotal            = expvar.NewInt("delivery_ledger_write_errors_total")
	degradedOutcomeChangedTotal = expvar.NewInt("delivery_ledger_degraded_outcome_changed_total")
	pairsMissingRepositoryGauge = expvar.NewInt("delivery_ledger_pairs_missing_repository_total")
	pairsMissingTaskGauge       = expvar.NewInt("delivery_ledger_pairs_missing_task_total")
	sessionsUnattributedGauge   = expvar.NewInt("delivery_ledger_sessions_unattributed_total")
	lastEvaluatedUnixVar        = expvar.NewInt("delivery_ledger_last_evaluated_unix")
	stallSecondsVar             = expvar.NewInt("delivery_ledger_stall_seconds")

	defaultBranchPersistErrorsTotal = expvar.NewInt("delivery_ledger_default_branch_persist_errors_total")
)

func recordEvaluation(outcome Outcome) { evaluationsTotal.Add(string(outcome), 1) }
func recordRowWritten()                { rowsWrittenTotal.Add(1) }
func recordDemotionSuppressed()        { demotionsSuppressedTotal.Add(1) }
func recordEvaluationError()           { evaluationErrorsTotal.Add(1) }
func recordAncestryError()             { ancestryErrorsTotal.Add(1) }
func recordAncestrySkipped()           { ancestrySkippedTotal.Add(1) }
func recordWriteError()                { writeErrorsTotal.Add(1) }
func recordDegradedOutcomeChanged()    { degradedOutcomeChangedTotal.Add(1) }

// RecordDefaultBranchPersistError increments
// delivery_ledger_default_branch_persist_errors_total. Exported for call
// sites outside this package (backendapp's review base-branch resolution)
// that fail to write a detected repositories.default_branch value: per spec
// "Degraded evaluation", that repository's pairs keep reading as
// default_branch_unknown until some future write succeeds, and Review round
// 3 finding #4 was exactly that this could happen silently, with no signal
// anywhere that it was happening.
func RecordDefaultBranchPersistError() { defaultBranchPersistErrorsTotal.Add(1) }

func setPairsMissingRepositoryGauge(n int) { pairsMissingRepositoryGauge.Set(int64(n)) }
func setPairsMissingTaskGauge(n int)       { pairsMissingTaskGauge.Set(int64(n)) }
func setSessionsUnattributedGauge(n int)   { sessionsUnattributedGauge.Set(int64(n)) }

// StallSignal is what the sweep publishes at the end of every pass (spec
// "Writer health § The data-side stall signal"). LedgerErr / ComparandErr
// are non-nil exactly when their respective query failed; the sentinel
// values (-1) are what LastEvaluatedUnix / StallSeconds hold in that case,
// but the caller logs the distinct WARN messages, not this type.
type StallSignal struct {
	LastEvaluatedUnix int64
	StallSeconds      int64
	LedgerErr         error
	ComparandErr      error
}

// ComputeStallSignal implements the five-row state table in spec "Writer
// health": the ledger query and the comparand query are independent and
// either can fail or return empty on its own, so the sentinels
// distinguish "unavailable" from "healthy and empty" — publishing 0/0 for
// a missing table would be indistinguishable from a fresh, healthy
// database.
func (r *Repository) ComputeStallSignal(ctx context.Context) StallSignal {
	maxEvaluated, hasRows, err := r.maxLastEvaluatedAt(ctx)
	if err != nil {
		return StallSignal{LastEvaluatedUnix: -1, StallSeconds: -1, LedgerErr: err}
	}
	if !hasRows {
		return StallSignal{LastEvaluatedUnix: 0, StallSeconds: 0}
	}

	sig := StallSignal{LastEvaluatedUnix: maxEvaluated.Unix()}
	maxSession, hasSessions, err := r.stallComparand(ctx)
	if err != nil {
		sig.StallSeconds = -1
		sig.ComparandErr = err
		return sig
	}
	if !hasSessions {
		sig.StallSeconds = 0
		return sig
	}
	diff := maxSession.Sub(maxEvaluated)
	if diff < 0 {
		diff = 0
	}
	sig.StallSeconds = int64(diff.Seconds())
	return sig
}

func publishStallSignal(sig StallSignal) {
	lastEvaluatedUnixVar.Set(sig.LastEvaluatedUnix)
	stallSecondsVar.Set(sig.StallSeconds)
}

func (r *Repository) maxLastEvaluatedAt(ctx context.Context) (time.Time, bool, error) {
	var raw interface{}
	err := r.ro.QueryRowxContext(ctx, `SELECT MAX(last_evaluated_at) FROM task_delivery_ledger`).Scan(&raw)
	if err != nil {
		return time.Time{}, false, err
	}
	return scanAggregateTime(raw)
}

// stallComparand computes MAX(task_sessions.updated_at) restricted to
// sessions that could have produced work for the writer (spec "Writer
// health"). The INNER JOIN against repositories — rather than a LEFT
// JOIN — is the R5-F2-adjacent Build decision for R5-F4: a session whose
// repository_id is non-empty but matches no repositories row must drop
// out of the comparand exactly like the two named filters, or a dangling
// repository_id would publish a permanent false stall on a healthy
// writer. deleted_at IS NULL then excludes soft-deleted repositories,
// whose pairs are frozen by design.
func (r *Repository) stallComparand(ctx context.Context) (time.Time, bool, error) {
	var raw interface{}
	err := r.ro.QueryRowxContext(ctx, `
		SELECT MAX(s.updated_at)
		FROM task_sessions s
		JOIN repositories rep ON rep.id = s.repository_id
		WHERE s.repository_id != '' AND rep.deleted_at IS NULL
	`).Scan(&raw)
	if err != nil {
		return time.Time{}, false, err
	}
	return scanAggregateTime(raw)
}

// sqliteAggregateTimeLayouts are the text formats an aggregate (MAX/MIN)
// over a TIMESTAMP column can come back as on SQLite: aggregate functions
// erase the column's declared type affinity that go-sqlite3 normally uses
// to auto-parse a scanned value into time.Time, so the driver hands back
// the raw stored text instead. This is the exact set of layouts
// mattn/go-sqlite3 itself writes when binding a time.Time parameter
// (SQLiteTimestampFormats), tried in the same order it does.
var sqliteAggregateTimeLayouts = []string{
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"2006-01-02",
}

// scanAggregateTime interprets the driver value of a MAX/MIN aggregate
// over a TIMESTAMP column, which on Postgres is already a time.Time
// (aggregates do not erase Postgres's wire-protocol type info) but on
// SQLite may be a plain string or []byte. A NULL aggregate (empty input
// set) reports ok=false, never a zero time mistaken for a real one.
func scanAggregateTime(raw interface{}) (time.Time, bool, error) {
	switch v := raw.(type) {
	case nil:
		return time.Time{}, false, nil
	case time.Time:
		return v, true, nil
	case string:
		t, err := parseAggregateTimeString(v)
		return t, err == nil, err
	case []byte:
		t, err := parseAggregateTimeString(string(v))
		return t, err == nil, err
	default:
		return time.Time{}, false, fmt.Errorf("delivery: unsupported aggregate time value type %T", raw)
	}
}

func parseAggregateTimeString(s string) (time.Time, error) {
	for _, layout := range sqliteAggregateTimeLayouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("delivery: cannot parse aggregate time %q", s)
}

// CountUnattributedSessions counts distinct sessions with an empty
// repository_id, for the sessions_unattributed_total gauge (set, not
// incremented, once per sweep pass).
func (r *Repository) CountUnattributedSessions(ctx context.Context) (int, error) {
	var n int
	err := r.ro.QueryRowxContext(ctx, `
		SELECT COUNT(*) FROM task_sessions WHERE repository_id = '' OR repository_id IS NULL
	`).Scan(&n)
	return n, err
}
