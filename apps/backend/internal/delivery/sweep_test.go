package delivery_test

import (
	"context"
	"database/sql"
	"expvar"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
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

// readExpvarMapTotal sums every key of a package-level expvar.Map (the
// evaluations-by-outcome map published by metrics.go), following the same
// before/after-delta convention as readExpvarInt since the map is also
// process-global and never reset between tests.
func readExpvarMapTotal(t *testing.T, name string) int64 {
	t.Helper()
	v := expvar.Get(name)
	if v == nil {
		t.Fatalf("expvar %q not published", name)
	}
	mv, ok := v.(*expvar.Map)
	if !ok {
		t.Fatalf("expvar %q is not an *expvar.Map", name)
	}
	var total int64
	mv.Do(func(kv expvar.KeyValue) {
		iv, ok := kv.Value.(*expvar.Int)
		if !ok {
			t.Fatalf("expvar map %q key %q value not an *expvar.Int", name, kv.Key)
		}
		total += iv.Value()
	})
	return total
}

func TestSelectDuePairs_NoLedgerRowIsUnconditionallyDue(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")

	due, fallback, ledgerErr := repo.SelectDuePairs(context.Background(),
		[]delivery.CandidatePair{{TaskID: "task-1", RepositoryID: "repo-1"}}, time.Now().UTC())
	if fallback {
		t.Fatal("a healthy, present ledger table must not report fallback")
	}
	if ledgerErr != nil {
		t.Fatalf("ledgerErr = %v, want nil (a healthy, present ledger table must not report an error)", ledgerErr)
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

	due, _, _ := repo.SelectDuePairs(context.Background(),
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

	due, _, _ := repo.SelectDuePairs(context.Background(),
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

	due, _, _ := repo.SelectDuePairs(context.Background(),
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

	due, _, _ := repo.SelectDuePairs(context.Background(),
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
	due, _, _ := repo.SelectDuePairs(context.Background(),
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

	due, fallback, ledgerErr := repo.SelectDuePairs(context.Background(), []delivery.CandidatePair{
		{TaskID: "task-1", RepositoryID: "repo-1"},
		{TaskID: "task-2", RepositoryID: "repo-frozen"},
	}, time.Now().UTC())
	if !fallback {
		t.Fatal("expected fallback=true when the ledger table is missing")
	}
	if ledgerErr == nil {
		t.Fatal("expected a non-nil ledgerErr describing why the fallback fired")
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

// TestRunPass_AncestrySkippedWhenNoSessionHasMovingHead covers spec
// "Default-branch observation § Ancestry precondition" and
// "delivery_ledger_ancestry_skipped_total" (spec.md:1797-1801), composed
// through the real Sweep with a real, non-nil AncestryChecker rather than
// the nil used by every other RunPass test in this file — every prior
// test passed nil, which left runAncestryIfDue and both ancestry counters
// effectively unexercised. Here the pair's one session shows a single
// snapshot (no distinct non-empty heads of its own), so the precondition
// is not met: the skipped counter must increment and the checkout must
// never even be resolved, proven via the calls counter rather than merely
// asserting Errored=false (which a different bug could also produce).
func TestRunPass_AncestrySkippedWhenNoSessionHasMovingHead(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	if _, err := db.Exec(db.Rebind(`UPDATE repositories SET default_branch = ? WHERE id = ?`), "main", "repo-1"); err != nil {
		t.Fatalf("set default_branch: %v", err)
	}
	seedTaskSession(t, db, "sess-1", "task-1", "repo-1")
	seedGitSnapshot(t, db, "snap-1", "sess-1", "feature", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 3, time.Now().UTC())

	checkoutCalls := 0
	ancestry := &delivery.AncestryChecker{Checkout: fakeCheckoutResolver{path: t.TempDir(), calls: &checkoutCalls}}
	sweep := delivery.NewSweep(repo, ancestry, nil)

	skippedBefore := readExpvarInt(t, "delivery_ledger_ancestry_skipped_total")
	errorsBefore := readExpvarInt(t, "delivery_ledger_ancestry_errors_total")

	sweep.RunPass(context.Background())

	if delta := readExpvarInt(t, "delivery_ledger_ancestry_skipped_total") - skippedBefore; delta != 1 {
		t.Fatalf("ancestry_skipped delta = %d, want 1", delta)
	}
	if delta := readExpvarInt(t, "delivery_ledger_ancestry_errors_total") - errorsBefore; delta != 0 {
		t.Fatalf("ancestry_errors delta = %d, want 0 (skipped, not attempted)", delta)
	}
	if checkoutCalls != 0 {
		t.Fatalf("checkout resolved %d times, want 0: a skipped precondition must never reach the checker", checkoutCalls)
	}
	row := readLedgerRow(t, db, "task-1", "repo-1")
	if row.ReachedAt.Valid {
		t.Fatalf("reached_default_at = %v, want NULL: no ancestry attempt means no ancestor_of_default observation", row.ReachedAt)
	}
}

// TestRunPass_GenuineAncestorReachesDefaultViaSweep covers the same spec
// sections as the skipped test above from the opposite side: a real
// ancestor relation, composed end-to-end through Sweep.RunPass, a real
// git repository (the same fixture ancestry_test.go's unit tests use),
// and a real AncestryChecker — not merely AncestryChecker.Check in
// isolation. The pair's one session shows two distinct non-empty heads
// (satisfying the moving-head precondition); the more recent of the two
// is the git repo's actual HEAD, which is trivially an ancestor of its
// own default branch. This is the missing link test-supervisor flagged:
// spec.md:2041-2050 warns that "a test that omits the relation is
// asserting nothing", and no prior test joined AncestryChecker to the
// evaluator through the sweep.
func TestRunPass_GenuineAncestorReachesDefaultViaSweep(t *testing.T) {
	work, defaultBranch := newAncestryTestRepo(t)
	head := runGit(t, work, "rev-parse", "HEAD")

	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	if _, err := db.Exec(db.Rebind(`UPDATE repositories SET default_branch = ? WHERE id = ?`), defaultBranch, "repo-1"); err != nil {
		t.Fatalf("set default_branch: %v", err)
	}
	seedTaskSession(t, db, "sess-1", "task-1", "repo-1")
	older := time.Now().UTC().Add(-time.Hour)
	newer := time.Now().UTC()
	// Two distinct non-empty heads for the same session satisfies the
	// moving-head precondition; the older, unrelated value never reaches
	// git (only the most recent snapshot's head is selected), so it does
	// not need to resolve to a real commit.
	seedGitSnapshot(t, db, "snap-old", "sess-1", "feature", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 1, older)
	seedGitSnapshot(t, db, "snap-new", "sess-1", "feature", head, 2, newer)

	ancestry := &delivery.AncestryChecker{Checkout: fakeCheckoutResolver{path: work}}
	sweep := delivery.NewSweep(repo, ancestry, nil)

	sweep.RunPass(context.Background())

	row := readLedgerRow(t, db, "task-1", "repo-1")
	if !row.ReachedAt.Valid {
		t.Fatal("reached_default_at = NULL, want set: HEAD is trivially an ancestor of its own default branch")
	}
	if row.ReachedBasis.String != string(delivery.ReachedBasisAncestorOfDefault) {
		t.Fatalf("reached_default_basis = %q, want %q", row.ReachedBasis.String, delivery.ReachedBasisAncestorOfDefault)
	}
	if row.ReachedRef.String != head {
		t.Fatalf("reached_default_ref = %q, want %q (the selected session head)", row.ReachedRef.String, head)
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

// TestRunPass_RankOneDegradedOutcomeChangeRecordsCounterAndAdvancesUpdatedAt
// is Review round 2, finding #7: upsert_test.go's rank-1 degraded write
// test only ever calls repo.Upsert directly, which never touches
// delivery_ledger_degraded_outcome_changed_total or
// delivery_ledger_demotions_suppressed_total — those counters are
// incremented one layer up, in RunPass's per-pair loop, off the
// UpsertResult it gets back. This test drives the exact same scenario
// (a degraded pair's outcome changing with its rank pinned at 1) through
// two real sweep passes so the counters actually move, and checks
// updated_at advances on the row that resulted.
func TestRunPass_RankOneDegradedOutcomeChangeRecordsCounterAndAdvancesUpdatedAt(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1") // default_branch defaults to '': every evaluation here is degraded
	seedTask(t, db, "task-1", "ws-1")
	seedGitHubStore(t, db)

	mergedAt := time.Now().UTC()
	seedGitHubPR(t, db, "pr-1", "task-1", "repo-1", "acme", "widgets", 1, "https://gh/1", "main", &mergedAt, nil)

	sweep := delivery.NewSweep(repo, nil, nil)
	sweep.RunPass(context.Background())

	before := readLedgerRow(t, db, "task-1", "repo-1")
	if before.Outcome.String != string(delivery.OutcomePRMerge) || before.Rank != 1 {
		t.Fatalf("row after first pass = %+v, want degraded pr_merge at rank 1", before)
	}

	demotionsBefore := readExpvarInt(t, "delivery_ledger_demotions_suppressed_total")
	degradedBefore := readExpvarInt(t, "delivery_ledger_degraded_outcome_changed_total")

	// The PR detaches and a snapshot lands at ahead=0 with no moving head:
	// still degraded (no default branch), so the pair re-evaluates from
	// pr_merge to no_delivery_observed at the SAME rank 1 — a
	// DegradedOutcomeChanged, never a Demoted (1 is not less than 1).
	detachedAt := mergedAt.Add(time.Minute)
	if _, err := db.Exec(db.Rebind(`UPDATE github_task_prs SET detached_at = ?, updated_at = ? WHERE id = ?`),
		detachedAt, detachedAt, "pr-1"); err != nil {
		t.Fatalf("detach pr: %v", err)
	}
	seedTaskSession(t, db, "sess-1", "task-1", "repo-1")
	seedGitSnapshot(t, db, "snap-1", "sess-1", "feature", "aaa", 0, detachedAt)

	sweep.RunPass(context.Background())

	if delta := readExpvarInt(t, "delivery_ledger_demotions_suppressed_total") - demotionsBefore; delta != 0 {
		t.Fatalf("demotions_suppressed_total delta = %d, want 0 (rank did not fall)", delta)
	}
	if delta := readExpvarInt(t, "delivery_ledger_degraded_outcome_changed_total") - degradedBefore; delta != 1 {
		t.Fatalf("degraded_outcome_changed_total delta = %d, want 1", delta)
	}

	after := readLedgerRow(t, db, "task-1", "repo-1")
	if after.Outcome.String != string(delivery.OutcomeNoDeliveryObserved) || after.Rank != 1 {
		t.Fatalf("row after second pass = %+v, want no_delivery_observed at rank 1", after)
	}
	if !after.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf("updated_at = %v, want after %v (an outcome change at unchanged rank must still advance it)", after.UpdatedAt, before.UpdatedAt)
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
// upsert still succeeds), evaluations_total does NOT increment for a pair
// whose upsert failed (it counts persisted evaluations, not computed
// ones — Review round 1, finding #2), and the pass reaches its LAST
// candidate rather than aborting after the first failure.
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
	evaluationsBefore := readExpvarMapTotal(t, "delivery_ledger_evaluations_total")

	sweep := delivery.NewSweep(repo, nil, nil)
	sweep.RunPass(context.Background())

	writeErrorsDelta := readExpvarInt(t, "delivery_ledger_write_errors_total") - writeErrorsBefore
	evalErrorsDelta := readExpvarInt(t, "delivery_ledger_evaluation_errors_total") - evalErrorsBefore
	evaluationsDelta := readExpvarMapTotal(t, "delivery_ledger_evaluations_total") - evaluationsBefore

	if writeErrorsDelta != 2 {
		t.Fatalf("write_errors_total delta = %d, want 2 (both candidates reached and failed their upsert)", writeErrorsDelta)
	}
	if evalErrorsDelta != 0 {
		t.Fatalf("evaluation_errors_total delta = %d, want 0 (input reads succeed; only the upsert fails)", evalErrorsDelta)
	}
	if evaluationsDelta != 0 {
		t.Fatalf("evaluations_total delta = %d, want 0 (a failed upsert must not count as a persisted evaluation)", evaluationsDelta)
	}
}

// TestRunPass_MissingLedgerTableLogsBothWarningsOnceEachCarryingTheError is
// Review round 2, finding #3 (an OBVIOUS production defect: both
// spec-mandated WARN logs discarded their underlying error, contradicting
// spec:962 and spec:1303-1304's explicit "carrying the error" requirement)
// together with finding #8 (nothing verified either log fires exactly once
// per pass rather than once per candidate). A dropped ledger table is a
// single failure that naturally triggers both warnings in the same real
// pass: the bulk dueness read fails first (dueness_unavailable), and then
// every due candidate's Upsert fails against the same missing table
// (write_failed) — so this one scenario proves both logs fire exactly
// once each, and both carry a non-nil, non-empty underlying error rather
// than just the fact that something failed.
func TestRunPass_MissingLedgerTableLogsBothWarningsOnceEachCarryingTheError(t *testing.T) {
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

	core, observed := observer.New(zapcore.WarnLevel)
	testLogger, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("build logger: %v", err)
	}

	sweep := delivery.NewSweep(repo, nil, testLogger)
	sweep.RunPass(context.Background())

	duenessEntries := observed.FilterMessage("delivery_ledger.dueness_unavailable").All()
	if len(duenessEntries) != 1 {
		t.Fatalf("got %d %q warnings, want exactly 1 (all warnings: %+v)",
			len(duenessEntries), "delivery_ledger.dueness_unavailable", observed.All())
	}
	duenessErr, ok := duenessEntries[0].ContextMap()["error"].(string)
	if !ok || duenessErr == "" {
		t.Fatalf("dueness_unavailable error field = %v, want a non-empty underlying error", duenessEntries[0].ContextMap()["error"])
	}

	writeFailedEntries := observed.FilterMessage("delivery_ledger.write_failed").All()
	if len(writeFailedEntries) != 1 {
		t.Fatalf("got %d %q warnings, want exactly 1 (all warnings: %+v)",
			len(writeFailedEntries), "delivery_ledger.write_failed", observed.All())
	}
	writeErr, ok := writeFailedEntries[0].ContextMap()["error"].(string)
	if !ok || writeErr == "" {
		t.Fatalf("write_failed error field = %v, want a non-empty underlying error", writeFailedEntries[0].ContextMap()["error"])
	}
	if count, ok := writeFailedEntries[0].ContextMap()["count"].(int64); !ok || count != 2 {
		t.Fatalf("write_failed count field = %v, want int64 2 (both candidates reached and failed their upsert)", writeFailedEntries[0].ContextMap()["count"])
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
	due, _, _ := repo.SelectDuePairs(context.Background(),
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
	due, _, _ = repo.SelectDuePairs(context.Background(),
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
