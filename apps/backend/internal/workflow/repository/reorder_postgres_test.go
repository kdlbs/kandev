package repository

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/testutil"
)

// @covers AC-TASKS-WORKFLOW-STEP-ORDERING-001.1, AC-TASKS-WORKFLOW-STEP-ORDERING-001.4
func TestPostgresReorderStepsMembership(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	runReorderMembershipTests(t, func(t *testing.T) *Repository {
		return setupPostgresDecisionTestRepo(t, dsn, 2)
	})
}

// @covers AC-TASKS-WORKFLOW-STEP-ORDERING-001.3
func TestPostgresReorderStepsRollsBackSecondWriteFailure(t *testing.T) {
	repo := setupPostgresDecisionTestRepo(t, testutil.PostgresDSNFromEnv(t), 2)
	ctx := context.Background()
	seedReorderSteps(t, repo)
	before, err := repo.ListStepsByWorkflow(ctx, "wf-test")
	require.NoError(t, err)
	_, err = repo.db.Exec(`CREATE FUNCTION reject_reorder() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN IF NEW.id = 'a' THEN RAISE EXCEPTION 'injected second write failure'; END IF; RETURN NEW; END $$;
	CREATE TRIGGER reject_second_reorder BEFORE UPDATE OF position ON workflow_steps FOR EACH ROW EXECUTE FUNCTION reject_reorder()`)
	require.NoError(t, err)
	require.Error(t, repo.ReorderSteps(ctx, "wf-test", []string{"b", "a", "c"}))
	after, err := repo.ListStepsByWorkflow(ctx, "wf-test")
	require.NoError(t, err)
	require.Equal(t, before, after)
}

// @covers AC-TASKS-WORKFLOW-STEP-ORDERING-001.2, AC-TASKS-COMPLETION-001.4
func TestPostgresReorderStepsPreservesConcurrentEdit(t *testing.T) {
	repo := setupPostgresDecisionTestRepo(t, testutil.PostgresDSNFromEnv(t), 2)
	seedReorderSteps(t, repo)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	blocker, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = blocker.Rollback() })
	_, err = blocker.ExecContext(ctx, `SELECT id FROM workflow_steps WHERE id = 'b' FOR UPDATE`)
	require.NoError(t, err)
	waiter, err := repo.db.Connx(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = waiter.Close() })
	var pid int
	require.NoError(t, waiter.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
	require.NoError(t, waiter.Close())
	finished := make(chan error, 1)
	go func() { finished <- repo.ReorderSteps(ctx, "wf-test", []string{"b", "a", "c"}) }()
	for {
		var waiting bool
		require.NoError(t, blocker.QueryRowContext(ctx, `SELECT COALESCE(wait_event_type = 'Lock', false) FROM pg_stat_activity WHERE pid = $1`, pid).Scan(&waiting))
		if waiting {
			break
		}
		runtime.Gosched()
	}
	_, err = blocker.ExecContext(ctx, `UPDATE workflow_steps SET prompt = 'edited concurrently', agent_profile_id = 'edited-profile', complete_task_on_enter = 1 WHERE id = 'b'`)
	require.NoError(t, err)
	require.NoError(t, blocker.Commit())
	require.NoError(t, <-finished)
	stored, err := repo.GetStep(ctx, "b")
	require.NoError(t, err)
	require.Equal(t, "edited concurrently", stored.Prompt)
	require.Equal(t, "edited-profile", stored.AgentProfileID)
	require.True(t, stored.CompleteTaskOnEnter)
	assertReorderPositions(t, repo, []string{"b", "a", "c"})
}

// @covers AC-TASKS-WORKFLOW-STEP-ORDERING-001.1
func TestPostgresReorderStepsConcurrentOrdersRemainComplete(t *testing.T) {
	repo := setupPostgresDecisionTestRepo(t, testutil.PostgresDSNFromEnv(t), 4)
	seedReorderSteps(t, repo)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	orders := [][]string{{"a", "b", "c"}, {"b", "a", "c"}, {"c", "b", "a"}, {"b", "c", "a"}}
	start := make(chan struct{})
	results := make(chan error, len(orders))
	for _, ids := range orders {
		go func() {
			<-start
			results <- repo.ReorderSteps(ctx, "wf-test", ids)
		}()
	}
	close(start)
	for range orders {
		require.NoError(t, <-results)
	}
	steps, err := repo.ListStepsByWorkflow(ctx, "wf-test")
	require.NoError(t, err)
	ids := []string{steps[0].ID, steps[1].ID, steps[2].ID}
	require.Contains(t, orders, ids)
	assertReorderPositions(t, repo, ids)
}
