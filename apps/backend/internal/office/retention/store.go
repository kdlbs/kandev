package retention

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

// queryer is the subset of *sqlx.DB / *sqlx.Conn this package needs to run
// a sweep or a census. The sweep runs every statement for its whole
// duration through one queryer: the writer pool directly on SQLite, or one
// dedicated connection on PostgreSQL (see lock.go) — so the advisory lock
// and the batches it protects can never diverge onto different sessions.
type queryer interface {
	db.Rebinder
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryxContext(ctx context.Context, query string, args ...any) (*sqlx.Rows, error)
	QueryRowxContext(ctx context.Context, query string, args ...any) *sqlx.Row
	GetContext(ctx context.Context, dest any, query string, args ...any) error
	SelectContext(ctx context.Context, dest any, query string, args ...any) error
	BeginTxx(ctx context.Context, opts *sql.TxOptions) (*sqlx.Tx, error)
}

// Store is the raw SQL access layer for retention: the eligibility counts
// (used by both the preview and the backlog check), the batch deletes, and
// the status census. It holds no state of its own.
type Store struct {
	pool *db.Pool
}

// NewStore wraps the database pool office_routine_runs, runs, and their
// satellites live in.
func NewStore(pool *db.Pool) *Store {
	return &Store{pool: pool}
}

// IsPostgres reports whether the writer pool is PostgreSQL. On SQLite one
// backend process owns the database file, so the in-process sweeping guard
// is sufficient and no advisory lock is taken.
func (s *Store) IsPostgres() bool {
	return dialect.IsPostgres(s.pool.Writer().DriverName())
}

func routineRunEligibleSubquery() string {
	return `
		SELECT id,
		       COALESCE(completed_at, created_at) AS completion_time,
		       ROW_NUMBER() OVER (
		         PARTITION BY routine_id
		         ORDER BY COALESCE(completed_at, created_at) DESC, id DESC
		       ) AS rn
		FROM office_routine_runs
		WHERE status IN (?)`
}

func runEligibleSubquery() string {
	return `
		SELECT id,
		       COALESCE(finished_at, requested_at) AS completion_time,
		       ROW_NUMBER() OVER (
		         PARTITION BY agent_profile_id
		         ORDER BY COALESCE(finished_at, requested_at) DESC, id DESC
		       ) AS rn
		FROM runs
		WHERE status IN (?)`
}

// CountEligibleRoutineRuns is the office_routine_runs eligibility count,
// uncapped by any batch limit: used for both the preview's WouldDelete
// (AC-OFFICE-RUN-HISTORY-RETENTION-003.9) and the backlog determination
// (AC-OFFICE-RUN-HISTORY-RETENTION-002.3) so the two can never disagree
// about what "eligible" means.
func (s *Store) CountEligibleRoutineRuns(ctx context.Context, q queryer, cutoff time.Time, floor int) (int64, error) {
	query := `SELECT COUNT(*) FROM (` + routineRunEligibleSubquery() + `) ranked WHERE rn > ? AND completion_time < ?`
	return countEligible(ctx, q, query, RoutineRunHistoryStatuses, floor, cutoff)
}

// CountEligibleRuns is the runs table's equivalent of CountEligibleRoutineRuns.
func (s *Store) CountEligibleRuns(ctx context.Context, q queryer, cutoff time.Time, floor int) (int64, error) {
	query := `SELECT COUNT(*) FROM (` + runEligibleSubquery() + `) ranked WHERE rn > ? AND completion_time < ?`
	return countEligible(ctx, q, query, RunHistoryStatuses, floor, cutoff)
}

func countEligible(ctx context.Context, q queryer, query string, statuses []string, floor int, cutoff time.Time) (int64, error) {
	bound, args, err := db.Bind(q, query, statuses, floor, cutoff)
	if err != nil {
		return 0, err
	}
	var count int64
	if err := q.QueryRowxContext(ctx, bound, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// DeleteRoutineRunsBatch deletes at most batchLimit eligible
// office_routine_runs rows, oldest first, re-asserting status, age and the
// per-owner floor in the same statement
// (AC-OFFICE-RUN-HISTORY-RETENTION-002.3, -002.4). One statement is
// already atomic, satisfying -002.5 without an explicit transaction.
func (s *Store) DeleteRoutineRunsBatch(ctx context.Context, q queryer, cutoff time.Time, floor, batchLimit int) (int64, error) {
	query := `
		DELETE FROM office_routine_runs
		WHERE id IN (
			SELECT id FROM (` + routineRunEligibleSubquery() + `) ranked
			WHERE rn > ? AND completion_time < ?
			ORDER BY completion_time ASC, id ASC
			LIMIT ?
		)
		AND status IN (?)
		AND COALESCE(completed_at, created_at) < ?`
	bound, args, err := db.Bind(q, query,
		RoutineRunHistoryStatuses, floor, cutoff, batchLimit,
		RoutineRunHistoryStatuses, cutoff,
	)
	if err != nil {
		return 0, err
	}
	res, err := q.ExecContext(ctx, bound, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RunBatchResult reports what one runs batch (and its satellites) actually
// deleted. Abandoned is true only when the batch was rolled back twice in a
// row and gave up — every count is then zero, matching what the rollback
// left committed (AC-OFFICE-RUN-HISTORY-RETENTION-002.7).
type RunBatchResult struct {
	RunsDeleted          int64
	RunEventsDeleted     int64
	RouteAttemptsDeleted int64
	RunSkillsDeleted     int64
	Abandoned            bool
}

// DeleteRunBatch selects up to batchLimit eligible runs, deletes each
// selected run's satellites and then the run itself in one transaction,
// re-asserting the whole eligibility predicate (status, age, and floor) at
// delete time. If a concurrent ScheduleRetry resurrects a row between
// selection and delete, step 5's affected-row count falls short of the
// selected id count; the whole transaction is rolled back and retried once
// with a fresh selection. A second mismatch abandons the batch
// (AC-OFFICE-RUN-HISTORY-RETENTION-002.4, -002.7).
func (s *Store) DeleteRunBatch(ctx context.Context, q queryer, cutoff time.Time, floor, batchLimit int) (RunBatchResult, error) {
	for attempt := 0; attempt < 2; attempt++ {
		if testBeforeSelectEligibleRunIDs != nil {
			testBeforeSelectEligibleRunIDs(attempt)
		}
		ids, err := s.selectEligibleRunIDs(ctx, q, cutoff, floor, batchLimit)
		if err != nil {
			return RunBatchResult{}, err
		}
		if len(ids) == 0 {
			return RunBatchResult{}, nil
		}
		if testAfterSelectEligibleRunIDs != nil {
			testAfterSelectEligibleRunIDs(attempt, ids)
		}
		result, matched, err := s.deleteRunBatchOnce(ctx, q, ids, cutoff, floor)
		if err != nil {
			return RunBatchResult{}, err
		}
		if matched {
			return result, nil
		}
	}
	return RunBatchResult{Abandoned: true}, nil
}

// testBeforeSelectEligibleRunIDs and testAfterSelectEligibleRunIDs, when
// set, bracket each of DeleteRunBatch's (at most two) selections — a
// deterministic seam this package's own tests use to exercise the
// selection-to-delete resurrection race, including the two-consecutive-
// mismatches abandon path, without depending on cross-connection goroutine
// timing. Never set outside tests.
var (
	testBeforeSelectEligibleRunIDs func(attempt int)
	testAfterSelectEligibleRunIDs  func(attempt int, ids []string)
)

func (s *Store) selectEligibleRunIDs(ctx context.Context, q queryer, cutoff time.Time, floor, batchLimit int) ([]string, error) {
	query := `
		SELECT id FROM (` + runEligibleSubquery() + `) ranked
		WHERE rn > ? AND completion_time < ?
		ORDER BY completion_time ASC, id ASC
		LIMIT ?`
	bound, args, err := db.Bind(q, query, RunHistoryStatuses, floor, cutoff, batchLimit)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryxContext(ctx, bound, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) deleteRunBatchOnce(ctx context.Context, q queryer, ids []string, cutoff time.Time, floor int) (RunBatchResult, bool, error) {
	tx, err := q.BeginTxx(ctx, nil)
	if err != nil {
		return RunBatchResult{}, false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	runEventsDeleted, err := deleteByRunIDs(ctx, tx, "run_events", ids)
	if err != nil {
		return RunBatchResult{}, false, err
	}
	routeAttemptsDeleted, err := deleteByRunIDs(ctx, tx, "office_run_route_attempts", ids)
	if err != nil {
		return RunBatchResult{}, false, err
	}
	runSkillsDeleted, err := deleteByRunIDs(ctx, tx, "office_run_skills", ids)
	if err != nil {
		return RunBatchResult{}, false, err
	}

	runsDeleted, err := deleteRunsByIDs(ctx, tx, ids, cutoff, floor)
	if err != nil {
		return RunBatchResult{}, false, err
	}
	if runsDeleted != int64(len(ids)) {
		return RunBatchResult{}, false, nil
	}

	if err := tx.Commit(); err != nil {
		return RunBatchResult{}, false, err
	}
	committed = true
	return RunBatchResult{
		RunsDeleted:          runsDeleted,
		RunEventsDeleted:     runEventsDeleted,
		RouteAttemptsDeleted: routeAttemptsDeleted,
		RunSkillsDeleted:     runSkillsDeleted,
	}, true, nil
}

// retentionMaxHostParams caps id-list placeholders per statement.
// batch_limit's documented range (AC-OFFICE-RUN-HISTORY-RETENTION-004.3)
// permits up to 100,000, which would otherwise bind that many ids in one IN
// clause and can overflow SQLite's compiled variable-count limit or
// PostgreSQL's wire-protocol parameter cap. Matches the bound this repo
// already uses for the same reason (internal/task/repository/sqlite's
// sqliteMaxHostParams).
const retentionMaxHostParams = 500

// chunkIDs splits ids into sub-slices of at most size entries so an
// IN-clause query built from them stays under retentionMaxHostParams
// regardless of batch_limit. An empty input returns nil rather than one
// empty chunk, since an empty IN () clause is a SQL syntax error.
func chunkIDs(ids []string, size int) [][]string {
	if len(ids) == 0 {
		return nil
	}
	if size <= 0 || len(ids) <= size {
		return [][]string{ids}
	}
	chunks := make([][]string, 0, (len(ids)+size-1)/size)
	for i := 0; i < len(ids); i += size {
		end := i + size
		if end > len(ids) {
			end = len(ids)
		}
		chunks = append(chunks, ids[i:end])
	}
	return chunks
}

// deleteRunsByIDs deletes ids from runs, in chunks of at most
// retentionMaxHostParams. Each chunk's outer WHERE re-asserts status and age
// directly, in addition to the floor-checking subquery: PostgreSQL's
// EvalPlanQual recheck of a concurrently updated row re-evaluates a direct
// column predicate against the row's fresh values, but does not rebuild an
// uncorrelated id-membership subquery, so the subquery alone is not enough
// to exclude a row resurrected between selection and delete
// (AC-OFFICE-RUN-HISTORY-RETENTION-002.4).
func deleteRunsByIDs(ctx context.Context, tx *sqlx.Tx, ids []string, cutoff time.Time, floor int) (int64, error) {
	var total int64
	for _, chunk := range chunkIDs(ids, retentionMaxHostParams) {
		query := `
			DELETE FROM runs
			WHERE id IN (?)
			AND id IN (
				SELECT id FROM (` + runEligibleSubquery() + `) ranked
				WHERE rn > ? AND completion_time < ?
			)
			AND status IN (?)
			AND COALESCE(finished_at, requested_at) < ?`
		bound, args, err := db.Bind(tx, query, chunk, RunHistoryStatuses, floor, cutoff, RunHistoryStatuses, cutoff)
		if err != nil {
			return 0, err
		}
		res, err := tx.ExecContext(ctx, bound, args...)
		if err != nil {
			return 0, err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		total += affected
	}
	return total, nil
}

func deleteByRunIDs(ctx context.Context, tx *sqlx.Tx, table string, ids []string) (int64, error) {
	var total int64
	for _, chunk := range chunkIDs(ids, retentionMaxHostParams) {
		query := fmt.Sprintf(`DELETE FROM %s WHERE run_id IN (?)`, table)
		bound, args, err := db.Bind(tx, query, chunk)
		if err != nil {
			return 0, err
		}
		res, err := tx.ExecContext(ctx, bound, args...)
		if err != nil {
			return 0, err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		total += affected
	}
	return total, nil
}

// CountRunEvents is run_events' plain retained count: it has no status
// column, so it keeps a bare COUNT(*) rather than a census.
func (s *Store) CountRunEvents(ctx context.Context, q queryer) (int64, error) {
	var count int64
	if err := q.GetContext(ctx, &count, `SELECT COUNT(*) FROM run_events`); err != nil {
		return 0, err
	}
	return count, nil
}

// CensusRoutineRuns issues office_routine_runs' status census
// (AC-OFFICE-RUN-HISTORY-RETENTION-001.10, -003.5, -003.11). The
// unknown-status detector needs every distinct status value, which the
// top-routine attribution's aggregation does not carry, so it stays a
// separate GROUP BY status scan. The retained total and the top-routine
// attribution, by contrast, must agree with each other by construction —
// AC-003.5's share is defined as one routine's retained rows as a
// proportion of the table's retained count — so routineRunCensusTotals
// reads both from one statement rather than two, which a concurrent write
// between them could otherwise make disagree.
func (s *Store) CensusRoutineRuns(ctx context.Context, q queryer, now time.Time) (TableCensus, error) {
	statusCounts, err := statusCensus(ctx, q, "office_routine_runs")
	if err != nil {
		return TableCensus{}, err
	}
	_, unknown := summarizeStatusCensus(statusCounts, RoutineRunHistoryStatuses, RoutineRunLiveStatuses)

	if testBetweenRoutineRunCensusReads != nil {
		testBetweenRoutineRunCensusReads(q)
	}

	retained, topID, topCount, err := routineRunCensusTotals(ctx, q)
	if err != nil {
		return TableCensus{}, err
	}
	census := TableCensus{RetainedCount: retained, UnknownStatuses: unknown, AsOf: now}
	if retained > 0 {
		census.TopRoutineID = topID
		census.TopRoutineShare = float64(topCount) / float64(retained)
	}
	return census, nil
}

// testBetweenRoutineRunCensusReads, when set, runs right after the
// unknown-status scan and right before routineRunCensusTotals' single-
// statement read — a deterministic seam for proving a concurrent write
// landing there cannot desynchronize the retained total from the
// top-routine attribution, since both now come from that one statement.
// Never set outside tests.
var testBetweenRoutineRunCensusReads func(q queryer)

// CensusRuns issues runs' status census, the runs-table equivalent of
// CensusRoutineRuns without the routine attribution AC-003.5 is specific
// to office_routine_runs.
func (s *Store) CensusRuns(ctx context.Context, q queryer, now time.Time) (TableCensus, error) {
	statusCounts, err := statusCensus(ctx, q, "runs")
	if err != nil {
		return TableCensus{}, err
	}
	retained, unknown := summarizeStatusCensus(statusCounts, RunHistoryStatuses, RunLiveStatuses)
	return TableCensus{RetainedCount: retained, UnknownStatuses: unknown, AsOf: now}, nil
}

// CensusRunEvents is run_events' census: a plain count, since the table
// has no status column and therefore no unknown-status detection.
func (s *Store) CensusRunEvents(ctx context.Context, q queryer, now time.Time) (TableCensus, error) {
	count, err := s.CountRunEvents(ctx, q)
	if err != nil {
		return TableCensus{}, err
	}
	return TableCensus{RetainedCount: count, AsOf: now}, nil
}

func statusCensus(ctx context.Context, q queryer, table string) (map[string]int64, error) {
	query := fmt.Sprintf(`SELECT status, COUNT(*) AS count FROM %s GROUP BY status`, table)
	rows, err := q.QueryxContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	counts := map[string]int64{}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

// routineRunCensusTotals reads office_routine_runs' table-wide retained
// total and the routine holding the largest share of it from one
// statement: a single GROUP BY routine_id pass, with the table total taken
// as a window sum over that same grouping, so the two numbers reflect
// exactly one snapshot and a share computed from them can never exceed 1.0.
// An empty table produces no groups at all; that is the legitimate zero
// state, not a failure, so sql.ErrNoRows is not propagated.
func routineRunCensusTotals(ctx context.Context, q queryer) (retained int64, topRoutineID string, topRoutineCount int64, err error) {
	var row struct {
		RoutineID string `db:"routine_id"`
		Retained  int64  `db:"retained"`
		Total     int64  `db:"total"`
	}
	query := `
		SELECT routine_id, COUNT(*) AS retained, SUM(COUNT(*)) OVER () AS total
		FROM office_routine_runs
		GROUP BY routine_id
		ORDER BY retained DESC, routine_id ASC
		LIMIT 1`
	if err := q.GetContext(ctx, &row, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", 0, nil
		}
		return 0, "", 0, err
	}
	return row.Total, row.RoutineID, row.Retained, nil
}
