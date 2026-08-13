package delivery_test

import (
	"context"
	"database/sql"
	"expvar"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/delivery"
)

// readExpvarInt reads a package-level expvar.Int published by
// internal/delivery/metrics.go, following the same convention as
// internal/office/scheduler/metrics_vars_test.go. These are process-global
// and never reset between tests, so callers must compare a before/after
// delta rather than an absolute value.
func readExpvarInt(t *testing.T, name string) int64 {
	t.Helper()
	v := expvar.Get(name)
	if v == nil {
		t.Fatalf("expvar %q not published", name)
	}
	iv, ok := v.(*expvar.Int)
	if !ok {
		t.Fatalf("expvar %q is not an *expvar.Int", name)
	}
	return iv.Value()
}

func TestSelectDuePairs_NoLedgerRowIsUnconditionallyDue(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")

	due, fallback := repo.SelectDuePairs(context.Background(),
		[]delivery.CandidatePair{{TaskID: "task-1", RepositoryID: "repo-1"}}, time.Now().UTC())
	if fallback {
		t.Fatal("a healthy, present ledger table must not report fallback")
	}
	if len(due) != 1 {
		t.Fatalf("due = %+v, want the pair with no ledger row", due)
	}
}

func TestSelectDuePairs_AlreadyFreshIsNotSelectedAgain(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	now := time.Now().UTC()

	if _, err := repo.Upsert(context.Background(), delivery.UpsertInput{
		TaskID: "task-1", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: delivery.Classification{
			Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisNoObservations, Rank: 2,
			ReachedDefault: true, ReachedBasis: delivery.ReachedBasisAncestorOfDefault, ReachedRef: "aaa",
		},
		EvaluatedAt: now,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	due, _ := repo.SelectDuePairs(context.Background(),
		[]delivery.CandidatePair{{TaskID: "task-1", RepositoryID: "repo-1"}}, now.Add(time.Second))
	if len(due) != 0 {
		t.Fatalf("due = %+v, want none (nothing moved, reached_default_at already set)", due)
	}
}

func TestSelectDuePairs_InputMovementReselects(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	past := time.Now().UTC().Add(-time.Hour)

	if _, err := repo.Upsert(context.Background(), delivery.UpsertInput{
		TaskID: "task-1", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: delivery.Classification{
			Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisNoObservations, Rank: 2,
			ReachedDefault: true, ReachedBasis: delivery.ReachedBasisAncestorOfDefault, ReachedRef: "aaa",
		},
		EvaluatedAt: past,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	seedTaskSession(t, db, "sess-1", "task-1", "repo-1")
	seedGitSnapshot(t, db, "snap-1", "sess-1", "feature", "bbb", 3, time.Now().UTC())

	due, _ := repo.SelectDuePairs(context.Background(),
		[]delivery.CandidatePair{{TaskID: "task-1", RepositoryID: "repo-1"}}, time.Now().UTC())
	if len(due) != 1 {
		t.Fatalf("due = %+v, want the pair selected because a snapshot moved", due)
	}
}

func TestSelectDuePairs_StaleRefreshWhileReachedDefaultIsNull(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	old := time.Now().UTC().Add(-25 * time.Hour)

	if _, err := repo.Upsert(context.Background(), delivery.UpsertInput{
		TaskID: "task-1", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: delivery.Classification{Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisBranchCommitsUnmerged, Rank: 5},
		EvaluatedAt:    old,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	due, _ := repo.SelectDuePairs(context.Background(),
		[]delivery.CandidatePair{{TaskID: "task-1", RepositoryID: "repo-1"}}, time.Now().UTC())
	if len(due) != 1 {
		t.Fatalf("due = %+v, want selected by stale refresh (>24h, reached_default_at NULL)", due)
	}
}

func TestSelectDuePairs_ReachedDefaultSetNeverStaleRefreshed(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	old := time.Now().UTC().Add(-25 * time.Hour)
	// Backdate the repository/task rows themselves so condition 2 (input
	// movement) cannot spuriously fire from their own seed-time
	// updated_at — this test isolates condition 3 (stale refresh) alone.
	backdateRepoAndTask(t, db, "repo-1", "task-1", old.Add(-time.Hour))

	if _, err := repo.Upsert(context.Background(), delivery.UpsertInput{
		TaskID: "task-1", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: delivery.Classification{
			Outcome: delivery.OutcomePRMerge, Basis: delivery.BasisProviderPRMerged, Rank: 8,
			ReachedDefault: true, ReachedBasis: delivery.ReachedBasisProviderPRMerged, ReachedRef: "https://pr/1",
		},
		EvaluatedAt: old,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	due, _ := repo.SelectDuePairs(context.Background(),
		[]delivery.CandidatePair{{TaskID: "task-1", RepositoryID: "repo-1"}}, time.Now().UTC())
	if len(due) != 0 {
		t.Fatalf("due = %+v, want none (stale refresh applies only while reached_default_at is NULL)", due)
	}
}

func TestSelectDuePairs_FreezeOverridesEveryCondition(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	if _, err := db.Exec(db.Rebind(`UPDATE repositories SET deleted_at = ? WHERE id = ?`), time.Now().UTC(), "repo-1"); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	// No ledger row: would otherwise be unconditionally due.
	due, _ := repo.SelectDuePairs(context.Background(),
		[]delivery.CandidatePair{{TaskID: "task-1", RepositoryID: "repo-1"}}, time.Now().UTC())
	if len(due) != 0 {
		t.Fatalf("due = %+v, want none (freeze overrides the unconditionally-due rule)", due)
	}
}

func TestSelectDuePairs_MissingLedgerTableFallsBackToEveryNonFrozenCandidate(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedRepository(t, db, "repo-frozen", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	seedTask(t, db, "task-2", "ws-1")
	if _, err := db.Exec(db.Rebind(`UPDATE repositories SET deleted_at = ? WHERE id = ?`), time.Now().UTC(), "repo-frozen"); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	// Simulate a swallowed migration: the table existed (newTestRepo
	// created it) and is now gone.
	if _, err := db.Exec(`DROP TABLE task_delivery_ledger`); err != nil {
		t.Fatalf("drop ledger table: %v", err)
	}

	due, fallback := repo.SelectDuePairs(context.Background(), []delivery.CandidatePair{
		{TaskID: "task-1", RepositoryID: "repo-1"},
		{TaskID: "task-2", RepositoryID: "repo-frozen"},
	}, time.Now().UTC())
	if !fallback {
		t.Fatal("expected fallback=true when the ledger table is missing")
	}
	if len(due) != 1 || due[0].RepositoryID != "repo-1" {
		t.Fatalf("due = %+v, want only the non-frozen candidate (freeze still applies under the fallback)", due)
	}
}

func TestOrderPairs_ByTaskCreatedAtThenTaskIDThenRepositoryID(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-a", "ws-1")
	seedRepository(t, db, "repo-b", "ws-1")

	now := time.Now().UTC()
	seedTaskAt(t, db, "task-later", "ws-1", now.Add(time.Hour))
	seedTaskAt(t, db, "task-earlier", "ws-1", now)

	pairs := []delivery.CandidatePair{
		{TaskID: "task-later", RepositoryID: "repo-a"},
		{TaskID: "task-earlier", RepositoryID: "repo-b"},
		{TaskID: "task-earlier", RepositoryID: "repo-a"},
	}
	ordered := repo.OrderPairs(context.Background(), pairs)
	if len(ordered) != 3 {
		t.Fatalf("ordered = %+v, want 3", ordered)
	}
	if ordered[0].TaskID != "task-earlier" || ordered[0].RepositoryID != "repo-a" {
		t.Fatalf("ordered[0] = %+v, want task-earlier/repo-a (repository_id ascending within the same task)", ordered[0])
	}
	if ordered[1].TaskID != "task-earlier" || ordered[1].RepositoryID != "repo-b" {
		t.Fatalf("ordered[1] = %+v, want task-earlier/repo-b", ordered[1])
	}
	if ordered[2].TaskID != "task-later" {
		t.Fatalf("ordered[2] = %+v, want task-later last (created later)", ordered[2])
	}
}

func backdateRepoAndTask(t *testing.T, db *sqlx.DB, repositoryID, taskID string, at time.Time) {
	t.Helper()
	if _, err := db.Exec(db.Rebind(`UPDATE repositories SET updated_at = ? WHERE id = ?`), at, repositoryID); err != nil {
		t.Fatalf("backdate repository: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`UPDATE tasks SET updated_at = ? WHERE id = ?`), at, taskID); err != nil {
		t.Fatalf("backdate task: %v", err)
	}
}

func seedTaskAt(t *testing.T, db interface {
	Rebind(string) string
	Exec(string, ...interface{}) (sql.Result, error)
}, id, workspaceID string, createdAt time.Time) {
	t.Helper()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
	`), id, workspaceID, id, createdAt, createdAt); err != nil {
		t.Fatalf("seed task %s: %v", id, err)
	}
}

// TestRunPass_EndToEnd exercises the full pipeline once: candidacy,
// dueness, ordering, evaluation, upsert, and writer-health publication,
// against a pair with a real merged pull request.
func TestRunPass_EndToEnd(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	if _, err := db.Exec(db.Rebind(`UPDATE repositories SET default_branch = ? WHERE id = ?`), "main", "repo-1"); err != nil {
		t.Fatalf("set default_branch: %v", err)
	}
	seedGitHubStore(t, db)
	now := time.Now().UTC()
	seedGitHubPR(t, db, "pr-1", "task-1", "repo-1", "acme", "widgets", 1, "https://gh/1", "main", &now, nil)

	sweep := delivery.NewSweep(repo, nil, nil)
	sweep.RunPass(context.Background())

	row := readLedgerRow(t, db, "task-1", "repo-1")
	if row.Outcome.String != string(delivery.OutcomePRMerge) || row.Rank != 8 {
		t.Fatalf("row = %+v, want pr_merge at rank 8", row)
	}
	if row.EvaluationSeq != 1 {
		t.Fatalf("evaluation_seq = %d, want 1", row.EvaluationSeq)
	}

	// A second pass with nothing changed must not re-select the pair.
	sweep.RunPass(context.Background())
	after := readLedgerRow(t, db, "task-1", "repo-1")
	if after.EvaluationSeq != 1 {
		t.Fatalf("evaluation_seq after second pass = %d, want still 1 (not re-selected, nothing moved and reached_default_at unset only matters after 24h)", after.EvaluationSeq)
	}
}

// TestRunPass_OrphanedProviderRowMissingTaskExcludedFromUpsert covers spec
// "Candidate pairs" / "Scenarios": a github_task_prs row whose task_id
// matches no tasks row is excluded from candidacy before ever reaching the
// upsert — no ledger row is written for it, on this pass or a second one
// with nothing changed, which is what keeps
// delivery_ledger_write_errors_total at its documented healthy resting
// value of 0 rather than climbing forever on a foreign-key failure.
func TestRunPass_OrphanedProviderRowMissingTaskExcludedFromUpsert(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedGitHubStore(t, db)
	now := time.Now().UTC()
	seedGitHubPR(t, db, "pr-orphan", "task-ghost", "repo-1", "acme", "widgets",
		1, "https://gh/1", "main", &now, nil)

	sweep := delivery.NewSweep(repo, nil, nil)
	sweep.RunPass(context.Background())

	if countLedgerRows(t, db, "task-ghost", "repo-1") != 0 {
		t.Fatal("no ledger row must be written for a pair excluded by a missing task row")
	}

	// A second pass with nothing changed must not attempt the upsert either.
	sweep.RunPass(context.Background())
	if countLedgerRows(t, db, "task-ghost", "repo-1") != 0 {
		t.Fatal("second pass must not write a ledger row for the still-orphaned pair")
	}
}

// countLedgerRows returns the number of task_delivery_ledger rows for a
// pair — used where the pair is expected to have none, since readLedgerRow
// fails the test on no rows.
func countLedgerRows(t *testing.T, db *sqlx.DB, taskID, repositoryID string) int {
	t.Helper()
	var n int
	if err := db.QueryRowxContext(context.Background(), db.Rebind(`
		SELECT COUNT(*) FROM task_delivery_ledger WHERE task_id = ? AND repository_id = ?
	`), taskID, repositoryID).Scan(&n); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	return n
}

// TestRunPass_MissingLedgerTableCountsWriteErrorsAndReachesEveryCandidate
// covers spec "Scenarios § Idempotency, ordering and concurrency": with the
// ledger table itself missing, every candidate is treated as due (the
// SelectDuePairs fallback, already covered by
// TestSelectDuePairs_MissingLedgerTableFallsBackToEveryNonFrozenCandidate),
// and this test covers what happens next — each due pair's Upsert then
// fails against the missing table, write_errors_total increments once per
// pair (never evaluation_errors_total, since every input read before the
// upsert still succeeds), and the pass reaches its LAST candidate rather
// than aborting after the first failure.
func TestRunPass_MissingLedgerTableCountsWriteErrorsAndReachesEveryCandidate(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	seedTask(t, db, "task-2", "ws-1")
	seedTaskRepository(t, db, "tr-1", "task-1", "repo-1")
	seedTaskRepository(t, db, "tr-2", "task-2", "repo-1")

	if _, err := db.Exec(`DROP TABLE task_delivery_ledger`); err != nil {
		t.Fatalf("drop ledger table: %v", err)
	}

	writeErrorsBefore := readExpvarInt(t, "delivery_ledger_write_errors_total")
	evalErrorsBefore := readExpvarInt(t, "delivery_ledger_evaluation_errors_total")

	sweep := delivery.NewSweep(repo, nil, nil)
	sweep.RunPass(context.Background())

	writeErrorsDelta := readExpvarInt(t, "delivery_ledger_write_errors_total") - writeErrorsBefore
	evalErrorsDelta := readExpvarInt(t, "delivery_ledger_evaluation_errors_total") - evalErrorsBefore

	if writeErrorsDelta != 2 {
		t.Fatalf("write_errors_total delta = %d, want 2 (both candidates reached and failed their upsert)", writeErrorsDelta)
	}
	if evalErrorsDelta != 0 {
		t.Fatalf("evaluation_errors_total delta = %d, want 0 (input reads succeed; only the upsert fails)", evalErrorsDelta)
	}
}

// TestRunPass_SnapshotQueryErrorAbandonsEvaluationNoColumnWritten covers
// spec "Scenarios § Idempotency, ordering and concurrency": an input-query
// failure abandons the evaluation entirely — no column is written,
// including last_evaluated_at — evaluation_errors_total increments,
// evaluations_total does not, and the pair is still due on the next pass.
// SnapshotsForPair is the representative case: unlike ProvidersForPair, it
// applies no missing-table tolerance, so dropping its table is a clean,
// deterministic way to force the real (non-tolerated) query error
// evaluatePair's four early-return branches exist to handle.
func TestRunPass_SnapshotQueryErrorAbandonsEvaluationNoColumnWritten(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	seedTaskRepository(t, db, "tr-1", "task-1", "repo-1")

	if _, err := db.Exec(`DROP TABLE task_session_git_snapshots`); err != nil {
		t.Fatalf("drop task_session_git_snapshots: %v", err)
	}

	evalErrorsBefore := readExpvarInt(t, "delivery_ledger_evaluation_errors_total")

	sweep := delivery.NewSweep(repo, nil, nil)
	sweep.RunPass(context.Background())

	if delta := readExpvarInt(t, "delivery_ledger_evaluation_errors_total") - evalErrorsBefore; delta != 1 {
		t.Fatalf("evaluation_errors_total delta = %d, want 1", delta)
	}
	if countLedgerRows(t, db, "task-1", "repo-1") != 0 {
		t.Fatal("no ledger row must be written when the snapshot query fails")
	}
}

// TestSelectDuePairs_ArchivedTaskRemainsEligible covers spec "Persistence
// guarantees" / "Scenarios": archiving a task neither excludes it from
// candidacy nor freezes it — no predicate anywhere reads
// tasks.archived_at, so an archived task's pair is due under exactly the
// same three conditions as any other.
func TestSelectDuePairs_ArchivedTaskRemainsEligible(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-archived", "ws-1")
	if _, err := db.Exec(db.Rebind(`UPDATE tasks SET archived_at = ? WHERE id = ?`),
		time.Now().UTC(), "task-archived"); err != nil {
		t.Fatalf("archive task: %v", err)
	}

	// No ledger row yet: unconditionally due, same as an unarchived pair.
	due, _ := repo.SelectDuePairs(context.Background(),
		[]delivery.CandidatePair{{TaskID: "task-archived", RepositoryID: "repo-1"}}, time.Now().UTC())
	if len(due) != 1 {
		t.Fatalf("due = %+v, want the archived task's pair selected", due)
	}

	if _, err := repo.Upsert(context.Background(), delivery.UpsertInput{
		TaskID: "task-archived", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: delivery.Classification{Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisNoObservations, Rank: 2},
		EvaluatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// A new session on the archived task's pair moves an input: still due,
	// exactly like an unarchived pair.
	seedTaskSession(t, db, "sess-archived", "task-archived", "repo-1")
	seedGitSnapshot(t, db, "snap-archived", "sess-archived", "feature", "aaa", 3, time.Now().UTC())
	due, _ = repo.SelectDuePairs(context.Background(),
		[]delivery.CandidatePair{{TaskID: "task-archived", RepositoryID: "repo-1"}}, time.Now().UTC())
	if len(due) != 1 {
		t.Fatalf("due = %+v, want the archived task's pair re-selected on input movement", due)
	}
}

func TestSweep_StartStopLifecycle(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")

	sweep := delivery.NewSweep(repo, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sweep.Start(ctx)
	sweep.Start(ctx) // second Start must be a no-op, not a second goroutine
	sweep.Stop()
	sweep.Stop() // idempotent
}
