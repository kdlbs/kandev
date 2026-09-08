package delivery

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/db/dialect"
)

// The rank-guarded classification columns. Repeated verbatim in both the
// SET clause (the actual write) and the updated_at "did anything change"
// predicate, because SQL's UPSERT gives no other way to compare the
// would-be new value against the pre-update stored value from inside the
// same atomic statement (spec "Concurrency": expressed inside the
// statement's SET clause, never a Go read-then-write).
const (
	rankGuardOutcome = `CASE WHEN excluded.evidence_rank >= task_delivery_ledger.evidence_rank ` +
		`THEN excluded.delivery_outcome ELSE task_delivery_ledger.delivery_outcome END`
	rankGuardBasis = `CASE WHEN excluded.evidence_rank >= task_delivery_ledger.evidence_rank ` +
		`THEN excluded.delivery_basis ELSE task_delivery_ledger.delivery_basis END`
	rankGuardRef = `CASE WHEN excluded.evidence_rank >= task_delivery_ledger.evidence_rank ` +
		`THEN excluded.delivery_ref ELSE task_delivery_ledger.delivery_ref END`
	rankGuardRank = `CASE WHEN excluded.evidence_rank >= task_delivery_ledger.evidence_rank ` +
		`THEN excluded.evidence_rank ELSE task_delivery_ledger.evidence_rank END`

	writeOnceAt = `CASE WHEN task_delivery_ledger.reached_default_at IS NULL ` +
		`THEN excluded.reached_default_at ELSE task_delivery_ledger.reached_default_at END`
	writeOnceBasis = `CASE WHEN task_delivery_ledger.reached_default_at IS NULL ` +
		`THEN excluded.reached_default_basis ELSE task_delivery_ledger.reached_default_basis END`
	writeOnceRef = `CASE WHEN task_delivery_ledger.reached_default_at IS NULL ` +
		`THEN excluded.reached_default_ref ELSE task_delivery_ledger.reached_default_ref END`

	firstClassifiedAtExpr = `CASE WHEN task_delivery_ledger.first_classified_at IS NULL ` +
		`THEN excluded.first_classified_at ELSE task_delivery_ledger.first_classified_at END`

	// highWaterObservedExpr avoids SQLite's two-arg max() (NULL-poisons on
	// either side being NULL) and Postgres's differently-behaved GREATEST,
	// per spec "High-water columns": NULL handling is made explicit with a
	// portable CASE rather than either dialect's native function.
	highWaterObservedExpr = `CASE
		WHEN excluded.observed_branch_commits IS NULL THEN task_delivery_ledger.observed_branch_commits
		WHEN task_delivery_ledger.observed_branch_commits IS NULL THEN excluded.observed_branch_commits
		WHEN task_delivery_ledger.observed_branch_commits >= excluded.observed_branch_commits
			THEN task_delivery_ledger.observed_branch_commits
		ELSE excluded.observed_branch_commits
	END`

	highWaterEvaluatedAtExpr = `CASE
		WHEN task_delivery_ledger.last_evaluated_at >= excluded.last_evaluated_at
			THEN task_delivery_ledger.last_evaluated_at
		ELSE excluded.last_evaluated_at
	END`

	// updatedAtExpr advances updated_at only when a classification or
	// observation column actually changes value (spec "Idempotency": a
	// re-evaluation with unchanged inputs must not advance updated_at).
	// IS DISTINCT FROM is NULL-safe on both dialects (SQLite since 3.39,
	// bundled by the go-sqlite3 driver version this module pins).
	updatedAtExpr = `CASE WHEN
			` + rankGuardOutcome + ` IS DISTINCT FROM task_delivery_ledger.delivery_outcome
		OR	` + rankGuardBasis + ` IS DISTINCT FROM task_delivery_ledger.delivery_basis
		OR	` + rankGuardRef + ` IS DISTINCT FROM task_delivery_ledger.delivery_ref
		OR	` + highWaterObservedExpr + ` IS DISTINCT FROM task_delivery_ledger.observed_branch_commits
		OR	` + writeOnceAt + ` IS DISTINCT FROM task_delivery_ledger.reached_default_at
		THEN excluded.updated_at ELSE task_delivery_ledger.updated_at END`
)

const upsertSQL = `
INSERT INTO task_delivery_ledger (
	id, task_id, repository_id, workspace_id,
	delivery_outcome, delivery_basis, delivery_ref, evidence_rank,
	reached_default_at, reached_default_basis, reached_default_ref,
	observed_branch_commits, first_classified_at,
	last_evaluated_at, evaluation_seq, created_at, updated_at
)
SELECT
	?, ?, repositories.id, ?,
	?, ?, ?, ?,
	?, ?, ?,
	?, ?,
	?, ?, ?, ?
FROM repositories
WHERE repositories.id = ? AND repositories.deleted_at IS NULL
ON CONFLICT (task_id, repository_id) DO UPDATE SET
	workspace_id = excluded.workspace_id,
	delivery_outcome = ` + rankGuardOutcome + `,
	delivery_basis = ` + rankGuardBasis + `,
	delivery_ref = ` + rankGuardRef + `,
	evidence_rank = ` + rankGuardRank + `,
	reached_default_at = ` + writeOnceAt + `,
	reached_default_basis = ` + writeOnceBasis + `,
	reached_default_ref = ` + writeOnceRef + `,
	observed_branch_commits = ` + highWaterObservedExpr + `,
	first_classified_at = ` + firstClassifiedAtExpr + `,
	last_evaluated_at = ` + highWaterEvaluatedAtExpr + `,
	evaluation_seq = task_delivery_ledger.evaluation_seq + 1,
	updated_at = ` + updatedAtExpr + `
RETURNING evidence_rank, delivery_outcome, delivery_basis, delivery_ref,
	observed_branch_commits, reached_default_at, updated_at, evaluation_seq
`

// errRepositoryNotLive means the repository disappeared or became soft
// deleted before the ledger write. It is a no-op for a sweep evaluation, not
// a database failure, so the caller must leave the pair due without advancing
// its ledger row.
var errRepositoryNotLive = errors.New("delivery ledger repository is not live")

// UpsertInput is everything one persisted evaluation writes for a pair.
// EvaluatedAt is T0, the instant this evaluation began reading its
// inputs — never the commit instant (spec "Sweep selection predicate",
// "Which instant last_evaluated_at records"). It is also used as the
// evaluation's own clock reading for reached_default_at and
// first_classified_at when this evaluation is the one that sets them
// (Build decision R5-F6: same instant source as last_evaluated_at, for
// the same comparability reason the spec argues for that column).
type UpsertInput struct {
	TaskID       string
	RepositoryID string
	WorkspaceID  string
	Classification
	EvaluatedAt time.Time
}

// UpsertResult reports what one Upsert call actually did, for the
// writer-health counters. These three signals are derived from a pre-
// upsert read compared against the post-upsert RETURNING row rather than
// computed atomically inside the statement, unlike every persisted
// column above. That is a deliberate, narrower exception than the
// NULL-handling one the spec calls out for high-water columns: it can
// only ever be inexact under a genuine concurrent write to the very same
// pair, which is unreachable in this deployment because the sweep never
// overlaps itself and no event trigger is wired (Build decision R5-F3).
// The counters are documented as a convenience for a live /debug/vars
// reader, not the ground truth — last_evaluated_at in the database is.
type UpsertResult struct {
	RowChanged             bool
	Demoted                bool
	DegradedOutcomeChanged bool
}

type existingRow struct {
	EvidenceRank int
	Outcome      sql.NullString
}

// Upsert persists one evaluation via the single INSERT ... ON CONFLICT DO
// UPDATE statement described above. See package doc and spec "Ordering,
// idempotency, concurrency".
func (r *Repository) Upsert(ctx context.Context, in UpsertInput) (UpsertResult, error) {
	before, beforeErr := r.readExisting(ctx, in.TaskID, in.RepositoryID)
	if beforeErr != nil && beforeErr != sql.ErrNoRows {
		return UpsertResult{}, beforeErr
	}

	id := uuid.New().String()
	now := time.Now().UTC()

	var outcome sql.NullString
	if in.Outcome != "" {
		outcome = sql.NullString{String: string(in.Outcome), Valid: true}
	}
	var basis sql.NullString
	if in.Basis != "" {
		basis = sql.NullString{String: string(in.Basis), Valid: true}
	}

	var reachedAt *time.Time
	var reachedBasis, reachedRef *string
	if in.ReachedDefault {
		t := in.EvaluatedAt
		reachedAt = &t
		rb := string(in.ReachedBasis)
		reachedBasis = &rb
		// An empty ReachedRef (a merged, non-detached provider row with an
		// empty pr_url/mr_url/pull_request_url — all TEXT NOT NULL with no
		// DEFAULT) must store NULL, not '', in the write-once
		// reached_default_ref column (R6-F3).
		reachedRef = refPtr(in.ReachedRef)
	}

	queryText := upsertSQL
	if dialect.IsPostgres(r.db.DriverName()) {
		// PostgreSQL must lock the source row while the INSERT ... SELECT is
		// evaluated. This serializes the final liveness check with a concurrent
		// soft delete; SQLite serializes both writers when the INSERT starts.
		queryText = strings.Replace(queryText, "\nON CONFLICT", "\nFOR UPDATE\nON CONFLICT", 1)
	}
	query := r.db.Rebind(queryText)
	var after existingRow
	var afterBasis, afterRef sql.NullString
	var afterObserved sql.NullInt64
	var afterReachedAt sql.NullTime
	var afterUpdatedAt time.Time
	var afterSeq int
	err := r.db.QueryRowxContext(ctx, query,
		id, in.TaskID, in.WorkspaceID,
		outcome, basis, in.Ref, in.Rank,
		reachedAt, reachedBasis, reachedRef,
		in.ObservedBranchCommits, in.EvaluatedAt,
		in.EvaluatedAt, 1, now, now,
		in.RepositoryID,
	).Scan(
		&after.EvidenceRank, &after.Outcome, &afterBasis, &afterRef,
		&afterObserved, &afterReachedAt, &afterUpdatedAt, &afterSeq,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return UpsertResult{}, errRepositoryNotLive
	}
	if err != nil {
		return UpsertResult{}, err
	}

	result := UpsertResult{}
	if beforeErr == sql.ErrNoRows {
		result.RowChanged = true
	} else {
		result.RowChanged = !afterUpdatedAt.Equal(before.updatedAt)
		result.Demoted = in.Rank < before.EvidenceRank
		result.DegradedOutcomeChanged = in.Rank == 1 &&
			before.EvidenceRank == 1 &&
			before.Outcome.String != after.Outcome.String
	}
	return result, nil
}

type existingRowFull struct {
	existingRow
	updatedAt time.Time
}

func (r *Repository) readExisting(ctx context.Context, taskID, repositoryID string) (existingRowFull, error) {
	var row existingRowFull
	err := r.db.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT evidence_rank, delivery_outcome, updated_at
		FROM task_delivery_ledger
		WHERE task_id = ? AND repository_id = ?
	`), taskID, repositoryID).Scan(&row.EvidenceRank, &row.Outcome, &row.updatedAt)
	return row, err
}
