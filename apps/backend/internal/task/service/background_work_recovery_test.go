package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

func backgroundSettlementFixture(t *testing.T, postgres bool) (*Service, *sqliterepo.Repository, string) {
	t.Helper()
	svc, _, repo := createTestService(t)
	if postgres {
		db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
		_, err := db.Exec(`CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`)
		require.NoError(t, err)
		repo, err = sqliterepo.NewWithDB(db, db, nil)
		require.NoError(t, err)
		svc.backgroundWork = repo
	}
	ctx := context.Background()
	wsID, taskID, sessionID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "Settlement"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: wsID, Title: "Settlement"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}))
	require.NoError(t, svc.RecordBackgroundWorkloadObservation(ctx, streams.WorkloadRunObservation{
		WorkID: "work", RunID: "run", State: streams.RunStateRunning,
	}, taskID, sessionID))
	return svc, repo, sessionID
}

func TestBackgroundSettlementRollsBackRunWriteFailure(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		name := "sqlite"
		if postgres {
			name = "postgres"
		}
		t.Run(name, func(t *testing.T) {
			svc, repo, sessionID := backgroundSettlementFixture(t, postgres)
			ctx := context.Background()
			trigger := `CREATE TRIGGER reject_background_run BEFORE UPDATE ON task_session_background_runs BEGIN SELECT RAISE(ABORT, 'injected run write failure'); END`
			if postgres {
				_, err := repo.DB().Exec(`CREATE FUNCTION reject_background_run() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected run write failure'; END $$`)
				require.NoError(t, err)
				trigger = `CREATE TRIGGER reject_background_run BEFORE UPDATE ON task_session_background_runs FOR EACH ROW EXECUTE FUNCTION reject_background_run()`
			}
			_, err := repo.DB().Exec(trigger)
			require.NoError(t, err)
			updates, err := svc.SettleOpenBackgroundWorkloads(ctx, sessionID)
			require.ErrorContains(t, err, "injected run write failure")
			require.Empty(t, updates)
			work, err := repo.GetBackgroundWorkload(ctx, sessionID, "work")
			require.NoError(t, err)
			require.Equal(t, "running", work.State, "failed settlement must remain eligible after restart")
			open, err := svc.ListSessionIDsWithOpenBackgroundWork(ctx)
			require.NoError(t, err)
			require.Contains(t, open, sessionID)
			drop := `DROP TRIGGER reject_background_run`
			if postgres {
				drop += ` ON task_session_background_runs`
			}
			_, err = repo.DB().Exec(drop)
			require.NoError(t, err)
			updates, err = svc.SettleOpenBackgroundWorkloads(ctx, sessionID)
			require.NoError(t, err)
			require.Len(t, updates, 1, "successful retry must publish the committed parent update")
			run, err := repo.GetBackgroundRun(ctx, sessionID, "run")
			require.NoError(t, err)
			require.Equal(t, "ended", run.State)
			require.NotNil(t, run.FinishedAt)
			require.WithinDuration(t, *updates[0].FinishedAt, *run.FinishedAt, time.Microsecond)
		})
	}
}

func TestBackgroundSettlementRejectsLateNonterminalRuns(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		name := "sqlite"
		if postgres {
			name = "postgres"
		}
		t.Run(name, func(t *testing.T) {
			svc, repo, sessionID := backgroundSettlementFixture(t, postgres)
			ctx := context.Background()
			_, err := svc.SettleOpenBackgroundWorkloads(ctx, sessionID)
			require.NoError(t, err)
			for _, id := range []string{"run", "late-new-run"} {
				require.NoError(t, svc.RecordBackgroundWorkloadObservation(ctx, streams.WorkloadRunObservation{
					WorkID: "work", RunID: id, State: streams.RunStateRunning,
				}, "task", sessionID))
				run, err := repo.GetBackgroundRun(ctx, sessionID, id)
				require.NoError(t, err)
				require.True(t, run == nil || streams.RunState(run.State).IsTerminal(), "late observation reopened run %s", id)
			}
			exit := 0
			require.NoError(t, svc.RecordBackgroundWorkloadObservation(ctx, streams.WorkloadRunObservation{
				WorkID: "work", RunID: "run", State: streams.RunStateCompleted, ExitCode: &exit,
			}, "task", sessionID))
			run, err := repo.GetBackgroundRun(ctx, sessionID, "run")
			require.NoError(t, err)
			require.Equal(t, "completed", run.State)
			require.Equal(t, &exit, run.ExitCode)
		})
	}
}

func TestBackgroundSettlementRepairsLegacyPartialWrite(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		name := "sqlite"
		if postgres {
			name = "postgres"
		}
		t.Run(name, func(t *testing.T) {
			svc, repo, sessionID := backgroundSettlementFixture(t, postgres)
			ctx := context.Background()
			query := `UPDATE task_session_background_work SET state='ended', revision=revision+1 WHERE session_id=?`
			if postgres {
				query = `UPDATE task_session_background_work SET state='ended', revision=revision+1 WHERE session_id=$1`
			}
			_, err := repo.DB().Exec(query, sessionID)
			require.NoError(t, err)
			open, err := svc.ListSessionIDsWithOpenBackgroundWork(ctx)
			require.NoError(t, err)
			require.Contains(t, open, sessionID)
			updates, err := svc.SettleOpenBackgroundWorkloads(ctx, sessionID)
			require.NoError(t, err)
			require.Len(t, updates, 1)
			run, err := repo.GetBackgroundRun(ctx, sessionID, "run")
			require.NoError(t, err)
			require.Equal(t, "ended", run.State)
			require.NotNil(t, run.FinishedAt)
			open, err = svc.ListSessionIDsWithOpenBackgroundWork(ctx)
			require.NoError(t, err)
			require.NotContains(t, open, sessionID)
		})
	}
}

func TestBackgroundSettlementFencesStaleOutputProjection(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		name := "sqlite"
		if postgres {
			name = "postgres"
		}
		t.Run(name, func(t *testing.T) {
			svc, repo, sessionID := backgroundSettlementFixture(t, postgres)
			ctx := context.Background()
			stale, err := repo.GetBackgroundWorkload(ctx, sessionID, "work")
			require.NoError(t, err)
			_, err = svc.SettleOpenBackgroundWorkloads(ctx, sessionID)
			require.NoError(t, err)
			stale.Revision++
			stale.Output = "trailing output"
			require.NoError(t, repo.UpsertBackgroundWorkload(ctx, stale))
			work, err := repo.GetBackgroundWorkload(ctx, sessionID, "work")
			require.NoError(t, err)
			require.Equal(t, "ended", work.State)
			require.NotNil(t, work.FinishedAt)
			require.Equal(t, "trailing output", work.Output)
		})
	}
}
