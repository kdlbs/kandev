package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func assertPostgresBackgroundWorkSettlement(t *testing.T, svc *taskservice.Service, repo *sqliterepo.Repository) {
	t.Helper()
	ctx := context.Background()
	workspaceID, taskID, sessionID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Background work boot"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Background work boot"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}))
	require.NoError(t, svc.RecordBackgroundWorkloadObservation(ctx, streams.WorkloadRunObservation{
		WorkID: "work", RunID: "run", Kind: streams.WorkloadKindShell, State: streams.RunStateRunning,
	}, taskID, sessionID))
	stale, err := repo.GetBackgroundWorkload(ctx, sessionID, "work")
	require.NoError(t, err)
	require.NotNil(t, stale, "production service must persist through its configured repository")
	assertPostgresSettlementRollback(t, svc, repo, sessionID)
	updates, err := svc.SettleOpenBackgroundWorkloads(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, updates, 1)
	run, err := repo.GetBackgroundRun(ctx, sessionID, "run")
	require.NoError(t, err)
	require.Equal(t, "ended", run.State)
	require.NotNil(t, run.FinishedAt)
	require.WithinDuration(t, *updates[0].FinishedAt, *run.FinishedAt, time.Microsecond)
	stale.Revision++
	stale.Output = "trailing output"
	require.NoError(t, repo.UpsertBackgroundWorkload(ctx, stale))
	stored, err := repo.GetBackgroundWorkload(ctx, sessionID, "work")
	require.NoError(t, err)
	require.Equal(t, "ended", stored.State)
	require.NotNil(t, stored.FinishedAt)
	require.Equal(t, "trailing output", stored.Output)
	assertPostgresLateBackgroundRuns(t, svc, repo, taskID, sessionID)
	assertPostgresLegacyBackgroundSettlement(t, svc, repo, taskID, sessionID)
}

func assertPostgresSettlementRollback(t *testing.T, svc *taskservice.Service, repo *sqliterepo.Repository, sessionID string) {
	t.Helper()
	ctx := context.Background()
	_, err := repo.DB().Exec(`CREATE FUNCTION reject_boot_background_run() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected boot run failure'; END $$`)
	require.NoError(t, err)
	_, err = repo.DB().Exec(`CREATE TRIGGER reject_boot_background_run BEFORE UPDATE ON task_session_background_runs FOR EACH ROW EXECUTE FUNCTION reject_boot_background_run()`)
	require.NoError(t, err)
	updates, err := svc.SettleOpenBackgroundWorkloads(ctx, sessionID)
	require.ErrorContains(t, err, "injected boot run failure")
	require.Empty(t, updates)
	parent, err := repo.GetBackgroundWorkload(ctx, sessionID, "work")
	require.NoError(t, err)
	require.Equal(t, "running", parent.State)
	open, err := svc.ListSessionIDsWithOpenBackgroundWork(ctx)
	require.NoError(t, err)
	require.Contains(t, open, sessionID)
	_, err = repo.DB().Exec(`DROP TRIGGER reject_boot_background_run ON task_session_background_runs`)
	require.NoError(t, err)
}

func assertPostgresLateBackgroundRuns(t *testing.T, svc *taskservice.Service, repo *sqliterepo.Repository, taskID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	for _, runID := range []string{"run", "late-run"} {
		require.NoError(t, svc.RecordBackgroundWorkloadObservation(ctx, streams.WorkloadRunObservation{
			WorkID: "work", RunID: runID, State: streams.RunStateWaiting,
		}, taskID, sessionID))
		run, err := repo.GetBackgroundRun(ctx, sessionID, runID)
		require.NoError(t, err)
		require.Equal(t, "ended", run.State)
	}
	code := 3
	require.NoError(t, svc.RecordBackgroundWorkloadObservation(ctx, streams.WorkloadRunObservation{
		WorkID: "work", RunID: "run", State: streams.RunStateFailed, ExitCode: &code,
	}, taskID, sessionID))
	run, err := repo.GetBackgroundRun(ctx, sessionID, "run")
	require.NoError(t, err)
	require.Equal(t, "failed", run.State)
	require.Equal(t, &code, run.ExitCode)
}

func assertPostgresLegacyBackgroundSettlement(t *testing.T, svc *taskservice.Service, repo *sqliterepo.Repository, taskID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, svc.RecordBackgroundWorkloadObservation(ctx, streams.WorkloadRunObservation{
		WorkID: "legacy", RunID: "legacy-run", State: streams.RunStateRunning,
	}, taskID, sessionID))
	_, err := repo.DB().Exec(`UPDATE task_session_background_work SET state='ended',revision=revision+1 WHERE session_id=$1 AND id='legacy'`, sessionID)
	require.NoError(t, err)
	open, err := svc.ListSessionIDsWithOpenBackgroundWork(ctx)
	require.NoError(t, err)
	require.Contains(t, open, sessionID)
	updates, err := svc.SettleOpenBackgroundWorkloads(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, updates, 1)
	require.Equal(t, "legacy", updates[0].WorkID)
	run, err := repo.GetBackgroundRun(ctx, sessionID, "legacy-run")
	require.NoError(t, err)
	require.Equal(t, "ended", run.State)
	open, err = svc.ListSessionIDsWithOpenBackgroundWork(ctx)
	require.NoError(t, err)
	require.NotContains(t, open, sessionID)
}
