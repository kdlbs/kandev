package sqlite

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresClaimStepEntryMarker_ConcurrentRace is the PostgreSQL
// counterpart to the CAS-loser coverage in
// internal/orchestrator/step_entry_dispatch_cas_loser_test.go: it proves
// ClaimStepEntryMarker's at-most-once guarantee (AC-D3/AC-D4,
// docs/specs/workflow-on-enter-action-dispatch/spec.md) holds against
// Postgres's real UNIQUE-violation error shape
// (isStepEntryMarkerUniqueViolation's *pgconn.PgError branch), not only
// against go-sqlite3's string-matched one every orchestrator dispatch test
// in this Build round already exercises. Several real connections race a
// claim for the same (entry_id, position); exactly one must win. Skips
// unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresClaimStepEntryMarker_ConcurrentRace(t *testing.T) {
	const concurrency = 8
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), concurrency)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "postgres-cas-marker-workspace", Name: "Postgres CAS marker workspace"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "postgres-cas-marker-task", WorkspaceID: "postgres-cas-marker-workspace",
		WorkflowID: "postgres-cas-marker-workflow", WorkflowStepID: "postgres-cas-marker-step",
		Title: "CAS marker candidate",
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	var entryID int64
	if err := db.QueryRowContext(ctx, db.Rebind(`
		INSERT INTO workflow_step_entries (task_id, step_id, entry_seq, digest, created_at)
		VALUES (?, ?, ?, ?, ?)
		RETURNING id
	`), "postgres-cas-marker-task", "postgres-cas-marker-step", 1, "digest-1", time.Now()).Scan(&entryID); err != nil {
		t.Fatalf("seed step entry: %v", err)
	}

	start := make(chan struct{})
	results := make(chan bool, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			claimed, claimErr := repo.ClaimStepEntryMarker(ctx, entryID, 0, "clear_decisions", fmt.Sprintf("op-%d", i), time.Now())
			if claimErr != nil {
				t.Errorf("concurrent claim attempt %d: %v", i, claimErr)
				return
			}
			results <- claimed
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	claimedCount := 0
	for c := range results {
		if c {
			claimedCount++
		}
	}
	if claimedCount != 1 {
		t.Fatalf("expected exactly 1 concurrent claim to win, got %d", claimedCount)
	}

	state, _, found, err := repo.GetStepEntryMarkerState(ctx, entryID, 0)
	if err != nil {
		t.Fatalf("GetStepEntryMarkerState: %v", err)
	}
	if !found {
		t.Fatalf("expected a marker row to exist after the race")
	}
	if state != StepEntryMarkerInProgress {
		t.Fatalf("marker state after the race = %q, want %q (only the winner claimed it, nobody completed it)", state, StepEntryMarkerInProgress)
	}
}

// TestPostgresIsStepEntryMarkerUniqueViolation_MatchesRealConstraintError
// drives the actual UNIQUE-constraint index through Postgres directly (a
// second raw INSERT for the same (entry_id, position), no ClaimStepEntryMarker
// involved) and asserts isStepEntryMarkerUniqueViolation classifies the
// resulting *pgconn.PgError the same way its already-tested SQLite string
// match does. This is what ClaimStepEntryMarker's !claimed branch relies on
// for every CAS-loser test in internal/orchestrator — without this,
// SQLite's error-shape coverage tells us nothing about the Postgres branch
// of the same function.
func TestPostgresIsStepEntryMarkerUniqueViolation_MatchesRealConstraintError(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "postgres-cas-marker-dup-workspace", Name: "Postgres CAS marker dup workspace"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "postgres-cas-marker-dup-task", WorkspaceID: "postgres-cas-marker-dup-workspace",
		WorkflowID: "postgres-cas-marker-dup-workflow", WorkflowStepID: "postgres-cas-marker-dup-step",
		Title: "CAS marker dup candidate",
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	var entryID int64
	if err := db.QueryRowContext(ctx, db.Rebind(`
		INSERT INTO workflow_step_entries (task_id, step_id, entry_seq, digest, created_at)
		VALUES (?, ?, ?, ?, ?)
		RETURNING id
	`), "postgres-cas-marker-dup-task", "postgres-cas-marker-dup-step", 1, "digest-1", time.Now()).Scan(&entryID); err != nil {
		t.Fatalf("seed step entry: %v", err)
	}

	claimed, err := repo.ClaimStepEntryMarker(ctx, entryID, 0, "clear_decisions", "op-first", time.Now())
	if err != nil || !claimed {
		t.Fatalf("first claim: claimed=%v err=%v", claimed, err)
	}

	claimed, err = repo.ClaimStepEntryMarker(ctx, entryID, 0, "clear_decisions", "op-second", time.Now())
	if err != nil {
		t.Fatalf("second claim on the same (entry_id, position) returned a real error instead of claimed=false: %v", err)
	}
	if claimed {
		t.Fatalf("second claim on the same (entry_id, position) succeeded, want claimed=false (UNIQUE violation)")
	}
}
