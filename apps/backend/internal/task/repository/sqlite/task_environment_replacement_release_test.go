package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestReplacementReleaseKeepsStartedAndRenewedClaims(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		for _, scenario := range []string{"unused", "started", "renewed", "generation", "owner", "ready", "live_execution", "cleanup"} {
			t.Run(backend+"/"+scenario, func(t *testing.T) {
				var repo *Repository
				if backend == "postgres" {
					repo, _, _ = newTaskPostgresRepoPair(t)
				} else {
					repo = newRepoForEntityTests(t)
				}
				ctx := context.Background()
				seedMaterializationClaimTask(t, repo, "task-release")
				seedActiveReplacementReservation(t, repo, "task-release", "env-release", "owner-release",
					models.TaskSessionStateCompleted, 0, false, scenario == "live_execution")
				expected, err := repo.GetTaskEnvironment(ctx, "env-release")
				require.NoError(t, err)
				switch scenario {
				case "cleanup":
					reserveBarrier(t, repo, "task-release", "release-cleanup")
				case "started":
					require.NoError(t, repo.UpdateTaskSessionState(ctx, "owner-release", models.TaskSessionStateStarting, ""))
				case "renewed":
					_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE task_environments SET updated_at = ? WHERE id = ?`), time.Now().UTC().Add(time.Minute), expected.ID)
				case "generation":
					_, err = repo.db.ExecContext(ctx, `UPDATE task_environments SET ownership_generation = ownership_generation + 1 WHERE id = 'env-release'`)
				case "owner":
					_, err = repo.db.ExecContext(ctx, `UPDATE task_environments SET materialization_session_id = 'other-owner' WHERE id = 'env-release'`)
				case "ready":
					_, err = repo.db.ExecContext(ctx, `UPDATE task_environments SET status = 'ready', materialization_session_id = '' WHERE id = 'env-release'`)
				}
				require.NoError(t, err)
				before, err := repo.GetTaskEnvironment(ctx, expected.ID)
				require.NoError(t, err)
				released, err := repo.ReleaseTaskEnvironmentReplacement(ctx, expected, "owner-release")
				require.NoError(t, err)
				require.Equal(t, scenario == "unused" || scenario == "cleanup", released)
				after, err := repo.GetTaskEnvironment(ctx, expected.ID)
				require.NoError(t, err)
				if released {
					require.Equal(t, models.TaskEnvironmentStatusFailed, after.Status)
					require.Empty(t, after.MaterializationSessionID)
				} else {
					require.Equal(t, before, after)
				}
			})
		}
	}
}

func TestPostgresReplacementReleaseWaitsForSessionStart(t *testing.T) {
	repo, launcher, observer := newTaskPostgresRepoPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	seedMaterializationClaimTask(t, repo, "task-release-race")
	seedActiveReplacementReservation(t, repo, "task-release-race", "env-release-race", "owner-release-race",
		models.TaskSessionStateCompleted, 0, false, false)
	expected, err := repo.GetTaskEnvironment(ctx, "env-release-race")
	require.NoError(t, err)
	start, err := launcher.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = start.Rollback() }()
	var holderPID, releasePID int
	require.NoError(t, start.GetContext(ctx, &holderPID, `SELECT pg_backend_pid()`))
	require.NoError(t, repo.db.GetContext(ctx, &releasePID, `SELECT pg_backend_pid()`))
	_, err = start.ExecContext(ctx, `UPDATE task_sessions SET state = 'STARTING' WHERE id = 'owner-release-race'`)
	require.NoError(t, err)
	type result struct {
		released bool
		err      error
	}
	done := make(chan result, 1)
	go func() {
		released, err := repo.ReleaseTaskEnvironmentReplacement(ctx, expected, "owner-release-race")
		done <- result{released, err}
	}()
	returnedEarly := false
	require.Eventually(t, func() bool {
		select {
		case <-done:
			returnedEarly = true
			return true
		default:
		}
		var waiting bool
		err := observer.QueryRowContext(ctx, `SELECT $2 = ANY(pg_blocking_pids($1))`, releasePID, holderPID).Scan(&waiting)
		if err != nil {
			t.Errorf("observe replacement release lock: %v", err)
			return false
		}
		return waiting
	}, 5*time.Second, 10*time.Millisecond)
	require.False(t, returnedEarly, "release must wait for the in-flight session transition")
	require.NoError(t, start.Commit())
	outcome := <-done
	require.NoError(t, outcome.err)
	require.False(t, outcome.released, "a committed start must retain its materialization claim")
}
