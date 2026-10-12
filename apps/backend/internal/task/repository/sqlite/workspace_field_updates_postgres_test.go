package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
)

type postgresSettingsResult struct {
	row *models.Workspace
	err error
}

func waitSettingsPostgresWriter(t *testing.T, ctx context.Context, observer *sqlx.DB, holderPID, writerPID int) {
	t.Helper()
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	for {
		var blocked bool
		err := observer.QueryRowContext(ctx, `SELECT $1 = ANY(pg_blocking_pids($2)) AND EXISTS (SELECT 1 FROM pg_locks WHERE pid=$2 AND locktype='transactionid' AND NOT granted)`, holderPID, writerPID).Scan(&blocked)
		require.NoError(t, err)
		if blocked {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("workspace writer never waited on the actual holder backend", ctx.Err())
		case <-poll.C:
		}
	}
}

func lockedSettingsWrite(t *testing.T, repo *tasksqlite.Repository, competing *tasksqlite.Repository, observer *sqlx.DB, request *service.UpdateWorkspaceRequest, holderWrite func(context.Context, *sql.Tx) error) postgresSettingsResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	holderPID, writerPID := hierarchyBackendPID(t, repo.DB()), hierarchyBackendPID(t, competing.DB())
	require.NotEqual(t, holderPID, writerPID)
	tx, err := repo.DB().BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		cancel()
		require.NoError(t, err)
	}
	var workers sync.WaitGroup
	defer func() { _ = tx.Rollback(); cancel(); workers.Wait() }()
	_, err = tx.ExecContext(ctx, `SELECT id FROM workspaces WHERE id='pg-settings' FOR UPDATE`)
	require.NoError(t, err)
	api := workspaceSettingsAPI(t, competing)
	done := make(chan postgresSettingsResult, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		row, err := api.UpdateWorkspace(ctx, "pg-settings", request)
		done <- postgresSettingsResult{row: row, err: err}
	}()
	waitSettingsPostgresWriter(t, ctx, observer, holderPID, writerPID)
	t.Logf("workspace PostgreSQL holder PID=%d competing PID=%d physically blocked", holderPID, writerPID)
	require.NoError(t, holderWrite(ctx, tx))
	require.NoError(t, tx.Commit())
	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		t.Fatal("PostgreSQL workspace writer did not settle", ctx.Err())
		return postgresSettingsResult{}
	}
}

// @covers AC-WORKSPACES-SETTINGS-UPDATES-001.1, AC-WORKSPACES-SETTINGS-UPDATES-001.7
func TestPostgresWorkspaceFieldUpdatesPhysicalConcurrency(t *testing.T) {
	for held := range 2 {
		t.Run(fmt.Sprintf("held_%d", held), func(t *testing.T) {
			a, b, observer := workflowPostgresPair(t)
			require.NoError(t, a.CreateWorkspace(t.Context(), &models.Workspace{ID: "pg-settings", Name: "Before", Description: "Keep", ACPIdleTimeoutMinutes: 120}))
			name, enabled, timeout := "PostgreSQL saved", true, 45
			requests := [2]*service.UpdateWorkspaceRequest{{Name: &name}, {ACPIdleSuspensionEnabled: &enabled, ACPIdleTimeoutMinutes: &timeout}}
			result := lockedSettingsWrite(t, a, b, observer, requests[held], func(ctx context.Context, tx *sql.Tx) error {
				if held == 0 {
					_, err := tx.ExecContext(ctx, `UPDATE workspaces SET acp_idle_suspension_enabled=$1,acp_idle_timeout_minutes=$2,updated_at=$3 WHERE id='pg-settings'`, enabled, timeout, time.Now().UTC())
					return err
				}
				_, err := tx.ExecContext(ctx, `UPDATE workspaces SET name=$1,updated_at=$2 WHERE id='pg-settings'`, name, time.Now().UTC())
				return err
			})
			require.NoError(t, result.err)
			stored, err := a.GetWorkspace(t.Context(), "pg-settings")
			require.NoError(t, err)
			require.Equal(t, name, stored.Name)
			require.True(t, stored.ACPIdleSuspensionEnabled)
			require.Equal(t, timeout, stored.ACPIdleTimeoutMinutes)
			require.Equal(t, "Keep", stored.Description)
			require.Equal(t, stored, result.row, "returned row observes the earlier committed disjoint edit")
		})
	}
}

// @covers AC-WORKSPACES-SETTINGS-UPDATES-001.2, AC-WORKSPACES-SETTINGS-UPDATES-001.3, AC-WORKSPACES-SETTINGS-UPDATES-001.5, AC-WORKSPACES-SETTINGS-UPDATES-001.6, AC-WORKSPACES-SETTINGS-UPDATES-001.8
func TestPostgresWorkspaceFieldUpdatesCompatibility(t *testing.T) {
	t.Run("presence_complete_writers_and_rollback", func(t *testing.T) {
		a, b, _ := workflowPostgresPair(t)
		api := workspaceSettingsAPI(t, b)
		ctx := t.Context()
		executor, environment, agent, config := "executor", "environment", "agent", "config"
		require.NoError(t, a.CreateWorkspace(ctx, &models.Workspace{
			ID: "pg-settings", Name: "Before", Description: "Before description",
			DefaultExecutorID: &executor, DefaultEnvironmentID: &environment, DefaultAgentProfileID: &agent, DefaultConfigAgentProfileID: &config,
			ACPIdleSuspensionEnabled: true,
		}))
		before, err := a.GetWorkspace(ctx, "pg-settings")
		require.NoError(t, err)
		require.True(t, before.ACPIdleSuspensionEnabled)
		require.Equal(t, 120, before.ACPIdleTimeoutMinutes)
		empty, blank, trimmed, disabled, minutes := "", " \t", " selected ", false, 23
		row, err := api.UpdateWorkspace(ctx, before.ID, &service.UpdateWorkspaceRequest{
			Name: &empty, Description: &empty, DefaultExecutorID: &empty, DefaultEnvironmentID: &blank,
			DefaultAgentProfileID: &trimmed, DefaultConfigAgentProfileID: &blank,
			ACPIdleSuspensionEnabled: &disabled, ACPIdleTimeoutMinutes: &minutes,
		})
		require.NoError(t, err)
		require.Empty(t, row.Name)
		require.Empty(t, row.Description)
		require.Nil(t, row.DefaultExecutorID)
		require.Nil(t, row.DefaultEnvironmentID)
		require.Equal(t, "selected", *row.DefaultAgentProfileID)
		require.Nil(t, row.DefaultConfigAgentProfileID)
		require.False(t, row.ACPIdleSuspensionEnabled)
		require.Equal(t, minutes, row.ACPIdleTimeoutMinutes)
		row, err = api.UpdateWorkspace(ctx, before.ID, &service.UpdateWorkspaceRequest{DefaultAgentProfileID: &empty})
		require.NoError(t, err)
		var nulls bool
		require.NoError(t, a.DB().QueryRowContext(ctx, `SELECT default_executor_id IS NULL AND default_environment_id IS NULL AND default_agent_profile_id IS NULL AND default_config_agent_profile_id IS NULL FROM workspaces WHERE id='pg-settings'`).Scan(&nulls))
		require.True(t, nulls)
		exactVersion, exactName := row.UpdatedAt, "Exact saved"
		row, err = api.UpdateWorkspace(ctx, row.ID, &service.UpdateWorkspaceRequest{Name: &exactName, ExpectedUpdatedAt: &exactVersion})
		require.NoError(t, err)
		rejected := "Rejected stale"
		_, err = api.UpdateWorkspace(ctx, row.ID, &service.UpdateWorkspaceRequest{Name: &rejected, ExpectedUpdatedAt: &exactVersion})
		require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict)
		stored, err := a.GetWorkspace(ctx, row.ID)
		require.NoError(t, err)
		require.Equal(t, row, stored)
		for _, next := range []string{"First saved", "Later saved"} {
			row, err = api.UpdateWorkspace(ctx, row.ID, &service.UpdateWorkspaceRequest{Name: &next})
			require.NoError(t, err)
		}
		require.Equal(t, "Later saved", row.Name)
		beforeEmpty := *row
		row, err = api.UpdateWorkspace(ctx, row.ID, &service.UpdateWorkspaceRequest{})
		require.NoError(t, err)
		require.True(t, row.UpdatedAt.After(beforeEmpty.UpdatedAt))
		comparison := *row
		comparison.UpdatedAt = beforeEmpty.UpdatedAt
		require.Equal(t, beforeEmpty, comparison)
		row.Description = "Complete writer description"
		require.NoError(t, a.UpdateWorkspaceIfUnchanged(ctx, row, row.UpdatedAt))
		require.ErrorIs(t, b.UpdateWorkspaceIfUnchanged(ctx, before, before.UpdatedAt), repoerrors.ErrTaskVersionConflict)
		complete, err := a.GetWorkspace(ctx, row.ID)
		require.NoError(t, err)
		require.Equal(t, "Complete writer description", complete.Description)
		_, err = a.DB().ExecContext(ctx, `ALTER TABLE workspaces ADD CONSTRAINT reject_settings_name CHECK (name <> 'constraint rejected')`)
		require.NoError(t, err)
		badName, enabled := "constraint rejected", true
		_, err = api.UpdateWorkspace(ctx, row.ID, &service.UpdateWorkspaceRequest{Name: &badName, ACPIdleSuspensionEnabled: &enabled})
		require.Error(t, err)
		stored, err = b.GetWorkspace(ctx, row.ID)
		require.NoError(t, err)
		require.Equal(t, complete, stored, "constraint abort rolls back all fields and timestamp")
		_, err = api.UpdateWorkspace(ctx, "missing-settings", &service.UpdateWorkspaceRequest{Name: &exactName})
		require.ErrorIs(t, err, repoerrors.ErrWorkspaceNotFound)
		require.ErrorIs(t, b.UpdateWorkspaceIfUnchanged(ctx, &models.Workspace{ID: "missing-settings"}, complete.UpdatedAt), repoerrors.ErrTaskVersionConflict)
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		_, err = api.UpdateWorkspace(cancelled, complete.ID, &service.UpdateWorkspaceRequest{Name: &exactName})
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("exact_fence_after_actual_row_wait", func(t *testing.T) {
		a, b, observer := workflowPostgresPair(t)
		require.NoError(t, a.CreateWorkspace(t.Context(), &models.Workspace{ID: "pg-settings", Name: "Before"}))
		before, err := b.GetWorkspace(t.Context(), "pg-settings")
		require.NoError(t, err)
		name, description := "Rejected exact", "Intervening committed"
		result := lockedSettingsWrite(t, a, b, observer, &service.UpdateWorkspaceRequest{Name: &name, ExpectedUpdatedAt: &before.UpdatedAt}, func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `UPDATE workspaces SET description=$1,updated_at=$2 WHERE id='pg-settings'`, description, time.Now().UTC())
			return err
		})
		require.ErrorIs(t, result.err, repoerrors.ErrTaskVersionConflict)
		stored, err := a.GetWorkspace(t.Context(), before.ID)
		require.NoError(t, err)
		require.Equal(t, before.Name, stored.Name)
		require.Equal(t, description, stored.Description)
		require.False(t, stored.UpdatedAt.Equal(before.UpdatedAt))
		comparison := *stored
		comparison.Description, comparison.UpdatedAt = before.Description, before.UpdatedAt
		require.Equal(t, *before, comparison)
	})
}
