package delivery_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/delivery"
	"github.com/kandev/kandev/internal/persistence"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresDeliveryLedgerMigration_FreshAndReplay is the PostgreSQL
// twin ADR 0027 requires for every schema-touching change: fresh init and
// replay both succeed, the table and its foreign keys land as declared,
// and the activation key is written. The task schema (tasks,
// repositories — the ledger's FK targets) must exist before
// delivery.NewWithDB runs: unlike SQLite, PostgreSQL requires FK target
// tables to exist at CREATE TABLE time. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresDeliveryLedgerMigration_FreshAndReplay(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, _, err := taskrepo.Provide(db, db, nil); err != nil {
		t.Fatalf("init task schema: %v", err)
	}

	if _, err := delivery.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init delivery schema: %v", err)
	}
	// Replay against the same database.
	if _, err := delivery.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay delivery schema: %v", err)
	}

	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = current_schema() AND table_name = 'task_delivery_ledger'
		)
	`).Scan(&exists)
	if err != nil {
		t.Fatalf("inspect task_delivery_ledger: %v", err)
	}
	if !exists {
		t.Fatal("task_delivery_ledger table missing after init")
	}

	var fkCount int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.table_constraints
		WHERE table_schema = current_schema()
		  AND table_name = 'task_delivery_ledger'
		  AND constraint_type = 'FOREIGN KEY'
	`).Scan(&fkCount)
	if err != nil {
		t.Fatalf("inspect foreign keys: %v", err)
	}
	if fkCount != 2 {
		t.Fatalf("foreign key count = %d, want 2 (task_id, repository_id)", fkCount)
	}

	val, err := persistence.ReadMetaKey(db, "telemetry.delivery_ledger.activated_at")
	if err != nil {
		t.Fatalf("read activation key: %v", err)
	}
	if val == "" {
		t.Fatal("expected activation key to be written on postgres")
	}
}

// TestPostgresUpsert_RankGuardedBehavior is Review round 2, finding #6:
// Repository.Upsert's dialect-sensitive rank-guard/high-water/write-once
// SQL (upsert.go's CASE expressions, chosen specifically because SQLite's
// two-arg max() and Postgres's GREATEST handle NULL differently — see
// highWaterObservedExpr's comment) had only a schema-existence Postgres
// test, never a behavioral one, despite ADR 0027 requiring a dialect-gated
// behavior test for every dialect-sensitive method. This drives the same
// scenarios as the SQLite-only tests in upsert_test.go — fresh insert,
// demotion suppression, equal-rank ref reselection, and the high-water
// NULL floor in both directions — against a real Postgres instance so a
// SQLite-only regression (e.g. IS DISTINCT FROM or the CASE expressions
// behaving differently under Postgres's stricter typing) cannot hide
// behind SQLite-only coverage. Skips unless KANDEV_TEST_POSTGRES_DSN is
// set.
func TestPostgresUpsert_RankGuardedBehavior(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, _, err := taskrepo.Provide(db, db, nil); err != nil {
		t.Fatalf("init task schema: %v", err)
	}
	repo, err := delivery.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init delivery schema: %v", err)
	}

	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-a", "ws-1")
	seedTask(t, db, "task-b", "ws-1")
	ctx := context.Background()
	t0 := time.Now().UTC().Truncate(time.Second)

	// Fresh insert.
	prA := "https://pr/A"
	result, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: "task-a", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: delivery.Classification{
			Outcome: delivery.OutcomePRMerge, Basis: delivery.BasisProviderPRMerged, Ref: &prA, Rank: 8,
			ObservedBranchCommits: intp(12),
		},
		EvaluatedAt: t0,
	})
	if err != nil {
		t.Fatalf("fresh insert: %v", err)
	}
	if !result.RowChanged {
		t.Fatal("fresh insert must report RowChanged")
	}
	row := readLedgerRow(t, db, "task-a", "repo-1")
	if row.Outcome.String != string(delivery.OutcomePRMerge) || row.Rank != 8 || row.Ref.String != prA {
		t.Fatalf("row after fresh insert = %+v", row)
	}

	// Demotion suppressed: a lower-rank re-evaluation, and its NULL
	// observed_branch_commits, must not move the stored higher-rank row —
	// both directions of the rank guard and the high-water floor at once.
	result, err = repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: "task-a", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: delivery.Classification{
			Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisBranchCommitsUnmerged, Rank: 5,
			ObservedBranchCommits: nil,
		},
		EvaluatedAt: t0.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("demotion upsert: %v", err)
	}
	if !result.Demoted || result.RowChanged {
		t.Fatalf("result = %+v, want Demoted=true RowChanged=false", result)
	}
	row = readLedgerRow(t, db, "task-a", "repo-1")
	if row.Outcome.String != string(delivery.OutcomePRMerge) || row.Rank != 8 || row.Ref.String != prA {
		t.Fatalf("row after suppressed demotion = %+v, want unchanged pr_merge at rank 8", row)
	}
	if !row.ObservedBranchCommits.Valid || row.ObservedBranchCommits.Int64 != 12 {
		t.Fatalf("observed_branch_commits = %v, want unchanged 12 (NULL incoming must not lower a stored value)", row.ObservedBranchCommits)
	}

	// Equal-rank ref reselection: another rank-8 evaluation with a
	// different ref must overwrite, never guaranteed-unchanged.
	prB := "https://pr/B"
	result, err = repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: "task-a", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: delivery.Classification{Outcome: delivery.OutcomePRMerge, Basis: delivery.BasisProviderPRMerged, Ref: &prB, Rank: 8},
		EvaluatedAt:    t0.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("equal-rank upsert: %v", err)
	}
	if !result.RowChanged {
		t.Fatal("a ref change at equal rank must report RowChanged")
	}
	row = readLedgerRow(t, db, "task-a", "repo-1")
	if row.Ref.String != prB {
		t.Fatalf("ref = %q, want prB after equal-rank reselection", row.Ref.String)
	}

	// High-water floor, other direction: a NULL stored observed_branch_commits
	// must accept a later real value rather than swallowing it.
	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: "task-b", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: delivery.Classification{Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisNoObservations, Rank: 2, ObservedBranchCommits: nil},
		EvaluatedAt:    t0,
	}); err != nil {
		t.Fatalf("task-b fresh insert: %v", err)
	}
	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: "task-b", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: delivery.Classification{Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisBranchCommitsUnmerged, Rank: 5, ObservedBranchCommits: intp(7)},
		EvaluatedAt:    t0.Add(time.Minute),
	}); err != nil {
		t.Fatalf("task-b second upsert: %v", err)
	}
	row = readLedgerRow(t, db, "task-b", "repo-1")
	if !row.ObservedBranchCommits.Valid || row.ObservedBranchCommits.Int64 != 7 {
		t.Fatalf("observed_branch_commits = %v, want 7 (a NULL stored value must not swallow an incoming real value)", row.ObservedBranchCommits)
	}
}
