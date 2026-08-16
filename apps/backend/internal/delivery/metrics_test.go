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

// TestComputeStallSignal_ExceedsThresholdWithValidComparand covers the
// firing state Review round 1 finding #4 flagged as untested: every
// existing TestComputeStallSignal_* case keeps StallSeconds at 0 (an
// empty/dangling/deleted/unattributed comparand), so nothing before this
// proved the >900s branch (spec.md:2179-2183) actually computes a
// positive value once a valid, live session drives the comparand ahead
// of the ledger's last_evaluated_at.
func TestComputeStallSignal_ExceedsThresholdWithValidComparand(t *testing.T) {
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
	// A live session on a real, non-deleted repository: it drives the
	// comparand to ~now, an hour ahead of the ledger's last_evaluated_at.
	seedTaskSession(t, db, "sess-live", "task-1", "repo-1")

	sig := repo.ComputeStallSignal(context.Background())
	if sig.LedgerErr != nil || sig.ComparandErr != nil {
		t.Fatalf("sig = %+v, want no errors", sig)
	}
	const stallThresholdSeconds = 900
	if sig.StallSeconds <= stallThresholdSeconds {
		t.Fatalf("stall_seconds = %d, want > %d (session is ~1h ahead of last_evaluated_at)", sig.StallSeconds, stallThresholdSeconds)
	}
}

// TestComputeStallSignal_ArchivedTaskSessionStillDrivesComparand is
// Review round 2, finding #9 (spec.md:2213-2217): archived tasks are still
// evaluated, so a stalled writer must still surface as stalled when the
// only live session belongs to an archived task. Unlike the soft-deleted
// repository and unattributed-session cases above, task archival must NOT
// be a comparand exclusion — stallComparand has no join against tasks at
// all, and this test guards against ever adding one.
func TestComputeStallSignal_ArchivedTaskSessionStillDrivesComparand(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedRepository(t, db, "repo-1", "ws-1")
	seedTask(t, db, "task-archived", "ws-1")
	if _, err := db.Exec(db.Rebind(`UPDATE tasks SET archived_at = ? WHERE id = ?`), time.Now().UTC(), "task-archived"); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	if _, err := repo.Upsert(context.Background(), delivery.UpsertInput{
		TaskID: "task-archived", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: delivery.Classification{Outcome: delivery.OutcomeUnknown, Basis: delivery.BasisNoObservations, Rank: 2},
		EvaluatedAt:    time.Now().UTC().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// The writer has stopped: this session updated only 20 minutes ago,
	// well inside the writer's own hour-old last_evaluated_at, but it is
	// the only thing driving the comparand and must not be excluded for
	// belonging to an archived task.
	seedTaskSession(t, db, "sess-archived-task", "task-archived", "repo-1")
	if _, err := db.Exec(db.Rebind(`UPDATE task_sessions SET updated_at = ? WHERE id = ?`), time.Now().UTC().Add(-20*time.Minute), "sess-archived-task"); err != nil {
		t.Fatalf("backdate session: %v", err)
	}

	sig := repo.ComputeStallSignal(context.Background())
	if sig.LedgerErr != nil || sig.ComparandErr != nil {
		t.Fatalf("sig = %+v, want no errors", sig)
	}
	const stallThresholdSeconds = 900
	if sig.StallSeconds <= stallThresholdSeconds {
		t.Fatalf("stall_seconds = %d, want > %d (archived task's session must still drive the comparand)", sig.StallSeconds, stallThresholdSeconds)
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

// TestComputeStallSignal_MissingLedgerTableReportsUnavailable covers spec
// "Writer health": a genuinely broken ledger (its migration swallowed at
// WARN) must report -1/-1, never 0/0 — the healthy-and-empty state — or a
// broken writer would be indistinguishable from a fresh, working one.
func TestComputeStallSignal_MissingLedgerTableReportsUnavailable(t *testing.T) {
	repo, db := newTestRepo(t)
	if _, err := db.Exec(`DROP TABLE task_delivery_ledger`); err != nil {
		t.Fatalf("drop ledger table: %v", err)
	}

	sig := repo.ComputeStallSignal(context.Background())
	if sig.LastEvaluatedUnix != -1 || sig.StallSeconds != -1 {
		t.Fatalf("sig = %+v, want -1/-1 for a missing ledger table", sig)
	}
	if sig.LedgerErr == nil {
		t.Fatal("expected LedgerErr, got nil")
	}
	if sig.ComparandErr != nil {
		t.Fatalf("ComparandErr = %v, want nil (never reached)", sig.ComparandErr)
	}
}

// TestComputeStallSignal_ComparandQueryErrorReportsUnknownStall covers spec
// "Writer health": when the ledger query succeeds but the comparand query
// errors, the true last-evaluated instant is still published — it did not
// fail — but the stall figure is unknown (-1) rather than 0, so a
// repeatedly failing comparand read can never report a healthy writer.
func TestComputeStallSignal_ComparandQueryErrorReportsUnknownStall(t *testing.T) {
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
	if _, err := db.Exec(`DROP TABLE task_sessions`); err != nil {
		t.Fatalf("drop task_sessions: %v", err)
	}

	sig := repo.ComputeStallSignal(context.Background())
	if sig.LastEvaluatedUnix == 0 || sig.LastEvaluatedUnix == -1 {
		t.Fatalf("last_evaluated_unix = %d, want the true MAX from the still-healthy ledger query", sig.LastEvaluatedUnix)
	}
	if sig.StallSeconds != -1 {
		t.Fatalf("stall_seconds = %d, want -1 (unknown, not 0)", sig.StallSeconds)
	}
	if sig.LedgerErr != nil {
		t.Fatalf("LedgerErr = %v, want nil", sig.LedgerErr)
	}
	if sig.ComparandErr == nil {
		t.Fatal("expected ComparandErr, got nil")
	}
}

// TestSessionsUnattributedGauge_PublishesLatestNotAccumulated replaces a
// prior tautological version of this test (Review round 1 recommended
// item) that only asserted CountUnattributedSessions returns the same
// number when called twice in a row — true of any pure query and never
// touching setSessionsUnattributedGauge (metrics.go:53) at all. This
// drives the gauge through two real Sweep.RunPass calls with a shrinking
// unattributed-session count and asserts the published expvar reflects
// only the latest pass (a Set, not an Add).
func TestSessionsUnattributedGauge_PublishesLatestNotAccumulated(t *testing.T) {
	repo, db := newTestRepo(t)
	seedWorkspace(t, db, "ws-1")
	seedTask(t, db, "task-1", "ws-1")
	seedTaskSession(t, db, "s1", "task-1", "")
	seedTaskSession(t, db, "s2", "task-1", "")
	seedTaskSession(t, db, "s3", "task-1", "")

	sweep := delivery.NewSweep(repo, nil, nil)
	sweep.RunPass(context.Background())
	if got := readExpvarInt(t, "delivery_ledger_sessions_unattributed_total"); got != 3 {
		t.Fatalf("gauge after first pass = %d, want 3", got)
	}

	if _, err := db.Exec(db.Rebind(`DELETE FROM task_sessions WHERE id IN (?, ?)`), "s2", "s3"); err != nil {
		t.Fatalf("delete sessions: %v", err)
	}

	sweep.RunPass(context.Background())
	if got := readExpvarInt(t, "delivery_ledger_sessions_unattributed_total"); got != 1 {
		t.Fatalf("gauge after second pass = %d, want 1 (a gauge reflects the latest count, not 3+1 accumulated)", got)
	}
}
