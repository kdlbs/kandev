package delivery_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/delivery"
)

func TestComputeStallSignal_EmptyLedgerIsNotAnInfiniteStall(t *testing.T) {
	repo, _ := newTestRepo(t)
	sig := repo.ComputeStallSignal(context.Background())
	if sig.LastEvaluatedUnix != 0 || sig.StallSeconds != 0 {
		t.Fatalf("sig = %+v, want 0/0 for an empty table", sig)
	}
	if sig.LedgerErr != nil || sig.ComparandErr != nil {
		t.Fatalf("sig = %+v, want no errors", sig)
	}
}

func TestComputeStallSignal_LedgerRowsWithEmptyComparandIsNotAStall(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	if _, err := repo.Upsert(context.Background(), delivery.UpsertInput{
		TaskID: "task-1", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: delivery.Classification{Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisNoObservations, Rank: 2},
		EvaluatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// No session at all: the comparand set is empty.

	sig := repo.ComputeStallSignal(context.Background())
	if sig.LastEvaluatedUnix == 0 {
		t.Fatal("expected the true MAX(last_evaluated_at), not 0")
	}
	if sig.StallSeconds != 0 {
		t.Fatalf("stall_seconds = %d, want 0 (no work to be behind on)", sig.StallSeconds)
	}
}

// TestComputeStallSignal_DanglingRepositoryIDExcludedFromComparand is the
// R5-F4 regression test: a session whose repository_id is non-empty but
// matches no repositories row must not drive the comparand, or a broken
// reference publishes a permanent false stall on a healthy writer.
func TestComputeStallSignal_DanglingRepositoryIDExcludedFromComparand(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	if _, err := repo.Upsert(context.Background(), delivery.UpsertInput{
		TaskID: "task-1", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: delivery.Classification{Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisNoObservations, Rank: 2},
		EvaluatedAt:    time.Now().UTC().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// A session whose repository_id points at nothing.
	seedTaskSession(t, db, "sess-dangling", "task-1", "repo-does-not-exist")

	sig := repo.ComputeStallSignal(context.Background())
	if sig.StallSeconds != 0 {
		t.Fatalf("stall_seconds = %d, want 0 (dangling repository_id must not drive the comparand)", sig.StallSeconds)
	}
}

func TestComputeStallSignal_SoftDeletedRepositorySessionExcludedFromComparand(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	if _, err := repo.Upsert(context.Background(), delivery.UpsertInput{
		TaskID: "task-1", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: delivery.Classification{Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisNoObservations, Rank: 2},
		EvaluatedAt:    time.Now().UTC().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	seedRepository(t, db, "repo-deleted", "ws-1")
	if _, err := db.Exec(db.Rebind(`UPDATE repositories SET deleted_at = ? WHERE id = ?`), time.Now().UTC(), "repo-deleted"); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	seedTaskSession(t, db, "sess-deleted-repo", "task-1", "repo-deleted")

	sig := repo.ComputeStallSignal(context.Background())
	if sig.StallSeconds != 0 {
		t.Fatalf("stall_seconds = %d, want 0 (a soft-deleted repository's sessions are frozen by design)", sig.StallSeconds)
	}
}

func TestComputeStallSignal_UnattributedSessionExcludedFromComparand(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	if _, err := repo.Upsert(context.Background(), delivery.UpsertInput{
		TaskID: "task-1", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: delivery.Classification{Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisNoObservations, Rank: 2},
		EvaluatedAt:    time.Now().UTC().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	seedTaskSession(t, db, "sess-unattributed", "task-1", "")

	sig := repo.ComputeStallSignal(context.Background())
	if sig.StallSeconds != 0 {
		t.Fatalf("stall_seconds = %d, want 0 (an unattributed session contributes to no pair)", sig.StallSeconds)
	}
}

func TestCountUnattributedSessions_IsAGaugeNotSummedAcrossCalls(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	seedTaskSession(t, db, "s1", "task-1", "")
	seedTaskSession(t, db, "s2", "task-1", "")
	seedTaskSession(t, db, "s3", "task-1", "")

	ctx := context.Background()
	n1, err := repo.CountUnattributedSessions(ctx)
	if err != nil {
		t.Fatalf("count 1: %v", err)
	}
	n2, err := repo.CountUnattributedSessions(ctx)
	if err != nil {
		t.Fatalf("count 2: %v", err)
	}
	if n1 != 3 || n2 != 3 {
		t.Fatalf("n1=%d n2=%d, want 3 both times (gauge, not accumulated)", n1, n2)
	}
}
