package delivery_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/delivery"
)

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
