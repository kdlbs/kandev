package delivery_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/delivery"
)

// ledgerRow is a raw read of every column, used to assert exact stored
// state without going through any repository accessor.
type ledgerRow struct {
	Outcome               sql.NullString
	Basis                 sql.NullString
	Ref                   sql.NullString
	Rank                  int
	ReachedAt             sql.NullTime
	ReachedBasis          sql.NullString
	ReachedRef            sql.NullString
	ObservedBranchCommits sql.NullInt64
	FirstClassifiedAt     sql.NullTime
	LastEvaluatedAt       time.Time
	EvaluationSeq         int
	WorkspaceID           string
	UpdatedAt             time.Time
}

func readLedgerRow(t *testing.T, db *sqlx.DB, taskID, repositoryID string) ledgerRow {
	t.Helper()
	var row ledgerRow
	err := db.QueryRowxContext(context.Background(), db.Rebind(`
		SELECT delivery_outcome, delivery_basis, delivery_ref, evidence_rank,
		       reached_default_at, reached_default_basis, reached_default_ref,
		       observed_branch_commits, first_classified_at,
		       last_evaluated_at, evaluation_seq, workspace_id, updated_at
		FROM task_delivery_ledger WHERE task_id = ? AND repository_id = ?
	`), taskID, repositoryID).Scan(
		&row.Outcome, &row.Basis, &row.Ref, &row.Rank,
		&row.ReachedAt, &row.ReachedBasis, &row.ReachedRef,
		&row.ObservedBranchCommits, &row.FirstClassifiedAt,
		&row.LastEvaluatedAt, &row.EvaluationSeq, &row.WorkspaceID, &row.UpdatedAt,
	)
	if err != nil {
		t.Fatalf("read ledger row: %v", err)
	}
	return row
}

func setupPair(t *testing.T) (*delivery.Repository, *sqlx.DB, string, string, string) {
	t.Helper()
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	return repo, db, "task-1", "repo-1", "ws-1"
}

func TestUpsert_FreshInsert(t *testing.T) {
	repo, db, taskID, repoID, wsID := setupPair(t)
	ctx := context.Background()
	t0 := time.Now().UTC().Truncate(time.Second)

	result, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{
			Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisNoObservations, Rank: 2,
		},
		EvaluatedAt: t0,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if !result.RowChanged {
		t.Fatal("first insert must report RowChanged")
	}

	row := readLedgerRow(t, db, taskID, repoID)
	if row.Outcome.String != string(delivery.OutcomeUnknown) || row.Rank != 2 {
		t.Fatalf("row = %+v", row)
	}
	if row.EvaluationSeq != 1 {
		t.Fatalf("evaluation_seq = %d, want 1 on insert", row.EvaluationSeq)
	}
	if !row.FirstClassifiedAt.Valid || !row.FirstClassifiedAt.Time.Equal(t0) {
		t.Fatalf("first_classified_at = %v, want %v", row.FirstClassifiedAt, t0)
	}
	if !row.LastEvaluatedAt.Equal(t0) {
		t.Fatalf("last_evaluated_at = %v, want %v", row.LastEvaluatedAt, t0)
	}
}

func TestUpsert_SoftDeletedRepositoryDoesNotWrite(t *testing.T) {
	repo, db, taskID, repoID, wsID := setupPair(t)
	if _, err := db.Exec(db.Rebind(`UPDATE repositories SET deleted_at = ? WHERE id = ?`),
		time.Now().UTC(), repoID); err != nil {
		t.Fatalf("soft delete repository: %v", err)
	}

	_, err := repo.Upsert(context.Background(), delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{
			Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisNoObservations, Rank: 2,
		},
		EvaluatedAt: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("upsert should report that a soft-deleted repository was not written")
	}

	var count int
	if err := db.QueryRow(db.Rebind(`
		SELECT COUNT(*) FROM task_delivery_ledger WHERE task_id = ? AND repository_id = ?
	`), taskID, repoID).Scan(&count); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("ledger rows = %d, want 0 for a soft-deleted repository", count)
	}
}

func TestUpsert_IdempotentReevaluationLeavesColumnsUnchanged(t *testing.T) {
	repo, db, taskID, repoID, wsID := setupPair(t)
	ctx := context.Background()
	t0 := time.Now().UTC().Truncate(time.Second)

	in := delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{
			Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisBranchCommitsUnmerged, Rank: 5,
			ObservedBranchCommits: intp(12),
		},
		EvaluatedAt: t0,
	}
	if _, err := repo.Upsert(ctx, in); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	before := readLedgerRow(t, db, taskID, repoID)

	t1 := t0.Add(time.Minute)
	in.EvaluatedAt = t1
	result, err := repo.Upsert(ctx, in)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if result.RowChanged {
		t.Fatal("re-evaluation with unchanged inputs must not report RowChanged")
	}

	after := readLedgerRow(t, db, taskID, repoID)
	if after.Outcome != before.Outcome || after.Basis != before.Basis || after.Ref != before.Ref ||
		after.Rank != before.Rank || after.ObservedBranchCommits != before.ObservedBranchCommits {
		t.Fatalf("classification columns changed: before=%+v after=%+v", before, after)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("updated_at advanced on a no-op evaluation: before=%v after=%v", before.UpdatedAt, after.UpdatedAt)
	}
	if after.EvaluationSeq != before.EvaluationSeq+1 {
		t.Fatalf("evaluation_seq = %d, want %d (advances every persisted evaluation)", after.EvaluationSeq, before.EvaluationSeq+1)
	}
	if !after.LastEvaluatedAt.Equal(t1) {
		t.Fatalf("last_evaluated_at = %v, want %v", after.LastEvaluatedAt, t1)
	}
}

func TestUpsert_PromotionAdvancesRankAndDoesNotSuppressDemotion(t *testing.T) {
	repo, db, taskID, repoID, wsID := setupPair(t)
	ctx := context.Background()
	t0 := time.Now().UTC()

	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{Outcome: delivery.OutcomeNoDeliveryObserved, Basis: delivery.BasisNoCommitsObserved, Rank: 4},
		EvaluatedAt:    t0,
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	result, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{
			Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisBranchCommitsUnmerged, Rank: 5, ObservedBranchCommits: intp(12),
		},
		EvaluatedAt: t0.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("promotion upsert: %v", err)
	}
	if !result.RowChanged || result.Demoted {
		t.Fatalf("result = %+v, want RowChanged=true Demoted=false", result)
	}

	row := readLedgerRow(t, db, taskID, repoID)
	if row.Outcome.String != string(delivery.OutcomeUnknown) || row.Rank != 5 {
		t.Fatalf("row = %+v, want promoted to rank 5", row)
	}
}

func TestUpsert_DemotionSuppressed(t *testing.T) {
	repo, db, taskID, repoID, wsID := setupPair(t)
	ctx := context.Background()
	t0 := time.Now().UTC()

	prURL := "https://pr/1"
	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{
			Outcome: delivery.OutcomePRMerge, Basis: delivery.BasisProviderPRMerged, Ref: &prURL, Rank: 8,
			ObservedBranchCommits: intp(12),
		},
		EvaluatedAt: t0,
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// PR later detached: re-evaluates to a lower rank.
	result, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisBranchCommitsUnmerged, Rank: 5},
		EvaluatedAt:    t0.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("demotion upsert: %v", err)
	}
	if !result.Demoted {
		t.Fatal("expected Demoted=true")
	}
	if result.RowChanged {
		t.Fatal("a suppressed demotion must not report RowChanged (classification columns are unchanged)")
	}

	row := readLedgerRow(t, db, taskID, repoID)
	if row.Outcome.String != string(delivery.OutcomePRMerge) || row.Rank != 8 || row.Ref.String != prURL {
		t.Fatalf("row = %+v, want unchanged pr_merge at rank 8", row)
	}
	if !row.ObservedBranchCommits.Valid || row.ObservedBranchCommits.Int64 != 12 {
		t.Fatalf("observed_branch_commits = %v, want unchanged 12", row.ObservedBranchCommits)
	}
}

func TestUpsert_EqualRankReselectsRef(t *testing.T) {
	repo, db, taskID, repoID, wsID := setupPair(t)
	ctx := context.Background()
	t0 := time.Now().UTC()

	prA := "https://pr/A"
	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{Outcome: delivery.OutcomePRMerge, Basis: delivery.BasisProviderPRMerged, Ref: &prA, Rank: 8},
		EvaluatedAt:    t0,
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	prB := "https://pr/B"
	result, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{Outcome: delivery.OutcomePRMerge, Basis: delivery.BasisProviderPRMerged, Ref: &prB, Rank: 8},
		EvaluatedAt:    t0.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if !result.RowChanged {
		t.Fatal("a ref change at equal rank must report RowChanged")
	}

	row := readLedgerRow(t, db, taskID, repoID)
	if row.Ref.String != prB {
		t.Fatalf("ref = %q, want prB (equal rank re-selects, never guaranteed unchanged)", row.Ref.String)
	}
}

func TestUpsert_EqualRankOneOutcomeChangeTracksInputsNotSilent(t *testing.T) {
	repo, db, taskID, repoID, wsID := setupPair(t)
	ctx := context.Background()
	t0 := time.Now().UTC()

	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{Outcome: delivery.OutcomePRMerge, Basis: delivery.BasisDefaultBranchUnknown, Rank: 1},
		EvaluatedAt:    t0,
	}); err != nil {
		t.Fatalf("first upsert (degraded pr_merge): %v", err)
	}
	before := readLedgerRow(t, db, taskID, repoID)

	// Detached, ahead=0 everywhere, still degraded: re-evaluates to
	// no_delivery_observed at the same rank 1.
	result, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{Outcome: delivery.OutcomeNoDeliveryObserved, Basis: delivery.BasisDefaultBranchUnknown, Rank: 1},
		EvaluatedAt:    t0.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if result.Demoted {
		t.Fatal("rank did not fall (1 -> 1); this must not count as a demotion")
	}
	if !result.DegradedOutcomeChanged {
		t.Fatal("expected DegradedOutcomeChanged=true")
	}
	if !result.RowChanged {
		t.Fatal("the write happens: outcome changed, so RowChanged must be true")
	}

	row := readLedgerRow(t, db, taskID, repoID)
	if row.Outcome.String != string(delivery.OutcomeNoDeliveryObserved) || row.Rank != 1 {
		t.Fatalf("row = %+v, want no_delivery_observed at rank 1", row)
	}
	// Review round 2, finding #7: the original version of this test never
	// checked updated_at, so a regression that left it frozen (the same
	// bug updatedAtExpr's IS DISTINCT FROM comparison exists to prevent)
	// would have gone unnoticed for exactly the scenario its own comment
	// calls out — an outcome change with no rank change.
	if !row.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf("updated_at = %v, want after %v (an outcome change at unchanged rank must still advance it)", row.UpdatedAt, before.UpdatedAt)
	}
}

func TestUpsert_WriteOnceTripleNeverOverwritten(t *testing.T) {
	repo, db, taskID, repoID, wsID := setupPair(t)
	ctx := context.Background()
	t0 := time.Now().UTC()

	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{
			Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisBranchCommitsUnmerged, Rank: 5,
			ReachedDefault: true, ReachedBasis: delivery.ReachedBasisAncestorOfDefault, ReachedRef: "commit-a",
		},
		EvaluatedAt: t0,
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	first := readLedgerRow(t, db, taskID, repoID)
	if !first.ReachedAt.Valid || first.ReachedRef.String != "commit-a" {
		t.Fatalf("first observation not stored: %+v", first)
	}

	// A later evaluation observes the default branch again through a
	// different basis: must not move the write-once triple.
	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{
			Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisReachedDefaultUnattributed, Rank: 6,
			ReachedDefault: true, ReachedBasis: delivery.ReachedBasisProviderPRMerged, ReachedRef: "https://pr/2",
		},
		EvaluatedAt: t0.Add(time.Minute),
	}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	second := readLedgerRow(t, db, taskID, repoID)
	if !second.ReachedAt.Time.Equal(first.ReachedAt.Time) {
		t.Fatalf("reached_default_at changed: %v -> %v", first.ReachedAt.Time, second.ReachedAt.Time)
	}
	if second.ReachedBasis.String != string(delivery.ReachedBasisAncestorOfDefault) || second.ReachedRef.String != "commit-a" {
		t.Fatalf("write-once triple overwritten: %+v", second)
	}
}

func TestUpsert_ObservedBranchCommitsHighWaterNeverDecreases(t *testing.T) {
	repo, db, taskID, repoID, wsID := setupPair(t)
	ctx := context.Background()
	t0 := time.Now().UTC()

	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{Outcome: delivery.OutcomePRMerge, Basis: delivery.BasisProviderPRMerged, Rank: 8, ObservedBranchCommits: intp(12)},
		EvaluatedAt:    t0,
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Snapshots deleted with their session: incoming value is nil.
	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisBranchCommitsUnmerged, Rank: 5, ObservedBranchCommits: nil},
		EvaluatedAt:    t0.Add(time.Minute),
	}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	row := readLedgerRow(t, db, taskID, repoID)
	if !row.ObservedBranchCommits.Valid || row.ObservedBranchCommits.Int64 != 12 {
		t.Fatalf("observed_branch_commits = %v, want still 12 (a NULL stored value is lower than any incoming value, and this direction must not decrease it)", row.ObservedBranchCommits)
	}
}

func TestUpsert_ObservedBranchCommitsNullStoredAcceptsIncoming(t *testing.T) {
	repo, db, taskID, repoID, wsID := setupPair(t)
	ctx := context.Background()
	t0 := time.Now().UTC()

	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisNoObservations, Rank: 2, ObservedBranchCommits: nil},
		EvaluatedAt:    t0,
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisBranchCommitsUnmerged, Rank: 5, ObservedBranchCommits: intp(7)},
		EvaluatedAt:    t0.Add(time.Minute),
	}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	row := readLedgerRow(t, db, taskID, repoID)
	if !row.ObservedBranchCommits.Valid || row.ObservedBranchCommits.Int64 != 7 {
		t.Fatalf("observed_branch_commits = %v, want 7 (NULL stored must not swallow an incoming value)", row.ObservedBranchCommits)
	}
}

func TestUpsert_LastEvaluatedAtIsHighWaterNeverRewound(t *testing.T) {
	repo, db, taskID, repoID, wsID := setupPair(t)
	ctx := context.Background()
	t3 := time.Now().UTC()
	t0 := t3.Add(-time.Minute)

	// The faster evaluation (began at T3) commits first.
	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisNoObservations, Rank: 2},
		EvaluatedAt:    t3,
	}); err != nil {
		t.Fatalf("fast upsert: %v", err)
	}

	// The slower evaluation, which began earlier at T0, commits second.
	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisNoObservations, Rank: 2},
		EvaluatedAt:    t0,
	}); err != nil {
		t.Fatalf("slow upsert: %v", err)
	}

	row := readLedgerRow(t, db, taskID, repoID)
	if !row.LastEvaluatedAt.Equal(t3) {
		t.Fatalf("last_evaluated_at = %v, want %v (a slow evaluation must not rewind the column)", row.LastEvaluatedAt, t3)
	}
}

// TestUpsert_ReachedDefaultWithEmptyRefStoresNull covers R6-F3: the
// provider tables' URL columns (pr_url / mr_url / pull_request_url) are
// TEXT NOT NULL with no DEFAULT, so a merged, non-detached row can carry
// an empty string. computeDefaultBranchObservation propagates that
// empty string as ReachedRef, and reached_default_ref is a write-once
// column — persisting ” there instead of NULL would violate the
// document-wide "NULL, not empty string" rule in spec "What" and "Data
// model" the moment it is ever written, since the column can never be
// overwritten afterwards.
func TestUpsert_ReachedDefaultWithEmptyRefStoresNull(t *testing.T) {
	repo, db, taskID, repoID, wsID := setupPair(t)
	ctx := context.Background()
	t0 := time.Now().UTC()

	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{
			Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisReachedDefaultUnattributed, Rank: 6,
			ReachedDefault: true, ReachedBasis: delivery.ReachedBasisProviderPRMerged, ReachedRef: "",
		},
		EvaluatedAt: t0,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	row := readLedgerRow(t, db, taskID, repoID)
	if !row.ReachedAt.Valid {
		t.Fatalf("reached_default_at not stored: %+v", row)
	}
	if row.ReachedRef.Valid {
		t.Fatalf("reached_default_ref = %q, want NULL not empty string", row.ReachedRef.String)
	}
}

func TestUpsert_WorkspaceIDRefreshedUnconditionallyEvenOnDemotion(t *testing.T) {
	repo, db, taskID, repoID, wsID := setupPair(t)
	ctx := context.Background()
	t0 := time.Now().UTC()

	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: wsID,
		Classification: delivery.Classification{Outcome: delivery.OutcomePRMerge, Basis: delivery.BasisProviderPRMerged, Rank: 8},
		EvaluatedAt:    t0,
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	seedWorkspace(t, db, "ws-2")
	if _, err := repo.Upsert(ctx, delivery.UpsertInput{
		TaskID: taskID, RepositoryID: repoID, WorkspaceID: "ws-2",
		Classification: delivery.Classification{Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisBranchCommitsUnmerged, Rank: 5},
		EvaluatedAt:    t0.Add(time.Minute),
	}); err != nil {
		t.Fatalf("demoted upsert with new workspace: %v", err)
	}

	row := readLedgerRow(t, db, taskID, repoID)
	if row.WorkspaceID != "ws-2" {
		t.Fatalf("workspace_id = %q, want ws-2 (refreshed unconditionally, even on a suppressed demotion)", row.WorkspaceID)
	}
	if row.Outcome.String != string(delivery.OutcomePRMerge) || row.Rank != 8 {
		t.Fatalf("classification must remain the higher-rank pr_merge: %+v", row)
	}
}
