package repository

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-WORKFLOW-START-SELECTION-001.1, AC-TASKS-WORKFLOW-START-SELECTION-001.2, AC-TASKS-WORKFLOW-START-SELECTION-001.4, AC-TASKS-WORKFLOW-START-SELECTION-001.6
func TestWorkflowStartSelectionPostgresIntent(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	for _, initial := range []string{"a", "b"} {
		t.Run(initial, func(t *testing.T) {
			repo := setupPostgresDecisionTestRepo(t, dsn, 2)
			seedSelectionSteps(t, repo, initial)
			ctx := context.Background()
			observed, err := repo.GetStep(ctx, "a")
			require.NoError(t, err)
			selected := "a"
			if initial == "a" {
				selected = "b"
			}
			promoted, err := repo.GetStep(ctx, selected)
			require.NoError(t, err)
			promoted.IsStartStep = true
			require.NoError(t, repo.UpdateStep(ctx, promoted))
			observed.Name = "Renamed on PostgreSQL"
			demoted, err := repo.UpdateStepWithDemotedStartStepsIntent(ctx, observed, nil)
			require.NoError(t, err)
			require.Empty(t, demoted)
			require.Equal(t, selected == "a", observed.IsStartStep)
			assertSelection(t, repo, selected)
		})
	}
	t.Run("rollback", func(t *testing.T) {
		repo := setupPostgresDecisionTestRepo(t, dsn, 2)
		runSelectionRollback(t, repo, `CREATE FUNCTION reject_selection() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.id='b' THEN RAISE EXCEPTION 'injected target failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_selection BEFORE UPDATE OF name ON workflow_steps FOR EACH ROW EXECUTE FUNCTION reject_selection()`)
	})
	t.Run("legacy and exact", func(t *testing.T) { runSelectionLegacyAndExact(t, setupPostgresDecisionTestRepo(t, dsn, 2)) })
}

// @covers AC-TASKS-WORKFLOW-START-SELECTION-001.1, AC-TASKS-WORKFLOW-START-SELECTION-001.3
func TestWorkflowStartSelectionPostgresWaitsForCurrentFlag(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	for _, initial := range []string{"a", "b"} {
		t.Run(initial, func(t *testing.T) {
			repo := setupPostgresDecisionTestRepo(t, dsn, 3)
			seedSelectionSteps(t, repo, initial)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			observed, err := repo.GetStep(ctx, "a")
			require.NoError(t, err)
			observed.Name = "Renamed after row lock"
			blocker, err := repo.db.BeginTxx(ctx, nil)
			require.NoError(t, err)
			var workers sync.WaitGroup
			t.Cleanup(func() { _ = blocker.Rollback(); cancel(); workers.Wait() })
			_, err = blocker.ExecContext(ctx, `SELECT id FROM workflow_steps WHERE id='a' FOR UPDATE`)
			require.NoError(t, err)
			var blockerPID int
			require.NoError(t, blocker.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID))
			result := make(chan error, 1)
			workers.Add(1)
			go func() {
				defer workers.Done()
				_, writeErr := repo.UpdateStepWithDemotedStartStepsIntent(ctx, observed, nil)
				result <- writeErr
			}()
			for {
				var waiting bool
				err = blocker.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_locks WHERE NOT granted AND $1=ANY(pg_blocking_pids(pid)))`, blockerPID).Scan(&waiting)
				require.NoError(t, err)
				if waiting {
					t.Logf("actual PostgreSQL lock wait observed behind blocker pid=%d", blockerPID)
					break
				}
				select {
				case writeErr := <-result:
					t.Fatalf("update bypassed held target lock: %v", writeErr)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				default:
					runtime.Gosched()
				}
			}
			selected := "a"
			if initial == "a" {
				selected = "b"
			}
			_, err = blocker.ExecContext(ctx, `UPDATE workflow_steps SET is_start_step=0 WHERE workflow_id='wf-test'`)
			require.NoError(t, err)
			_, err = blocker.ExecContext(ctx, `UPDATE workflow_steps SET is_start_step=1 WHERE id=$1`, selected)
			require.NoError(t, err)
			require.NoError(t, blocker.Commit())
			require.NoError(t, <-result)
			workers.Wait()
			require.Equal(t, selected == "a", observed.IsStartStep)
			assertSelection(t, repo, selected)
		})
	}
}
