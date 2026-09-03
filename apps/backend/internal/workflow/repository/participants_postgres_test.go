package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
	"github.com/kandev/kandev/internal/workflow/models"
)

// TestPostgresResolveCurrentRunner_FallsBackToLatestTaskRunner is the
// PostgreSQL counterpart to
// TestResolveCurrentRunner_LatestTaskRunnerOrdersByCreatedAtNotInsertionOrder.
// The third tier this exercises used to read `ORDER BY rowid DESC` —
// `rowid` is a SQLite-only pseudo-column with no Postgres equivalent, so the
// query would fail outright (`column "rowid" does not exist`) rather than
// silently pick the wrong row. Running the exact resolution path against a
// real Postgres connection is the only way to prove the fixed
// `ORDER BY created_at DESC, agent_profile_id ASC` actually executes there.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresResolveCurrentRunner_FallsBackToLatestTaskRunner(t *testing.T) {
	repo := setupPostgresDecisionTestRepo(t, testutil.PostgresDSNFromEnv(t), 1)
	ctx := context.Background()
	work := newPhase2TestStep(t, repo, "Work")
	review := newPhase2TestStep(t, repo, "Review")
	done := newPhase2TestStep(t, repo, "Done")

	older := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	newer := time.Now().UTC().Truncate(time.Millisecond)

	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at)
		VALUES (?, ?, ?, 'runner', ?, 0, 0, ?)
	`), "pg-runner-inserted-first", work.ID, "task-pg-reorder", "runner-newer-created-at", newer); err != nil {
		t.Fatalf("insert first runner row: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at)
		VALUES (?, ?, ?, 'runner', ?, 0, 0, ?)
	`), "pg-runner-inserted-second", review.ID, "task-pg-reorder", "runner-older-created-at", older); err != nil {
		t.Fatalf("insert second runner row: %v", err)
	}

	got, err := repo.ResolveCurrentRunner(ctx, done.ID, "task-pg-reorder")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "runner-newer-created-at" {
		t.Fatalf("expected the row with the newer created_at, got %q", got)
	}
}

// TestPostgresEnsureRoleSeat_ConcurrentEntriesConvergeOnOneSeat is the
// PostgreSQL counterpart to TestEnsureRoleSeat_ConcurrentEntriesConvergeOnOneSeat,
// proving EnsureRoleSeat's transactional check-then-insert plus retry-on-
// natural-key-violation genuinely closes the AC-OFFICE-REVIEW-SEATS-002.7
// race under real cross-connection concurrency, not SQLite's single-writer
// serialization. Uses setupPostgresDecisionTestRepo (same isolated-schema,
// multi-connection setup TestPostgresRecordStepDecision_ConcurrentSameDeciderRace
// uses) because genuine concurrency is the whole point. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresEnsureRoleSeat_ConcurrentEntriesConvergeOnOneSeat(t *testing.T) {
	const concurrency = 8
	repo := setupPostgresDecisionTestRepo(t, testutil.PostgresDSNFromEnv(t), concurrency)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")

	const taskID = "task-pg-seat-race"
	start := make(chan struct{})
	type result struct {
		seat *models.WorkflowStepParticipant
		err  error
	}
	results := make(chan result, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			seat, _, err := repo.EnsureRoleSeat(ctx, "wf-test", step.ID, taskID, "reviewer", "agent-race")
			results <- result{seat: seat, err: err}
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	var firstID string
	for res := range results {
		if res.err != nil {
			t.Fatalf("EnsureRoleSeat: %v", res.err)
		}
		if firstID == "" {
			firstID = res.seat.ID
		} else if res.seat.ID != firstID {
			t.Fatalf("expected every concurrent caller to converge on one seat id, got %q and %q", firstID, res.seat.ID)
		}
	}

	rows, err := repo.ListParticipantsForTaskWorkflow(ctx, taskID, "wf-test")
	if err != nil {
		t.Fatalf("list workflow participants: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly one seat to survive the race, got %d: %+v", len(rows), rows)
	}
}
