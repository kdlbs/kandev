package sqlite_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryInterruptedLegacyNativeScope(t *testing.T) {
	runInterruptedLegacyNativeScope(t, false)
}

func TestUsageHistoryPostgresInterruptedLegacyNativeScope(t *testing.T) {
	runInterruptedLegacyNativeScope(t, true)
}

func runInterruptedLegacyNativeScope(t *testing.T, postgres bool) {
	for _, erase := range []bool{false, true} {
		t.Run(fmt.Sprintf("erase=%t", erase), func(t *testing.T) {
			database, _ := projectionDatabases(t, postgres)
			tasks, repo := legacyScopeDatabaseFixture(t, database)
			tasks.SetUsageHistoryRetainer(nil)
			for i := range 3 {
				require.NoError(t, tasks.CreateTaskUsageEvent(t.Context(), &taskmodels.TaskUsageEvent{UsageEventID: fmt.Sprintf("legacy-%d", i), TaskID: "scope-task", SessionID: "scope-session", AgentType: "fixture", Model: "model", TokensIn: 10, TokensTotal: 10, ContractVersion: 1, OccurredAt: time.Date(2026, 1, 2, 12, i, 0, 0, time.UTC), CreatedAt: time.Date(2026, 1, 2, 12, i, 0, 0, time.UTC)}))
			}
			done, err := repo.BackfillUsageHistory(t.Context(), 1)
			require.NoError(t, err)
			require.False(t, done)
			var initial int
			require.NoError(t, database.Get(&initial, `SELECT COUNT(*) FROM session_usage_buckets WHERE source='native'`))
			require.Zero(t, initial, "native migration waits for source adoption to finish")
			done, err = repo.BackfillUsageHistory(t.Context(), 1)
			require.NoError(t, err)
			require.False(t, done)
			require.NoError(t, database.Get(&initial, `SELECT COUNT(*) FROM session_usage_buckets WHERE source='native'`))
			require.Equal(t, 1, initial, "the first native page uses captured session-creation ownership")
			_, err = database.Exec(`UPDATE workspaces SET org_id='scope-b' WHERE id='scope-ws'`)
			require.NoError(t, err)
			if erase {
				require.NoError(t, repo.EraseOrganizationUsage(t.Context(), "scope-a"))
			}
			restarted, err := analyticssqlite.NewWithDB(database, database)
			require.NoError(t, err)
			finishScopeBackfill(t, restarted)
			var foreign, retained int
			require.NoError(t, database.Get(&foreign, `SELECT COUNT(*) FROM session_usage_buckets b JOIN usage_scopes s ON s.scope_id=b.scope_id WHERE s.organization_id='scope-b'`))
			require.Zero(t, foreign, "a resumed source page must not adopt the new owner")
			require.NoError(t, database.Get(&retained, `SELECT COUNT(*) FROM session_usage_buckets WHERE source='native'`))
			if erase {
				require.Zero(t, retained)
				var tail int
				require.NoError(t, database.Get(&tail, `SELECT COUNT(*) FROM task_usage_events`))
				require.Zero(t, tail, "captured legacy tails must be erased before their adoption evidence")
			} else {
				require.Equal(t, 3, retained)
			}
		})
	}
}

func legacyScopeDatabaseFixture(t *testing.T, database *sqlx.DB) (*tasksqlite.Repository, *analyticssqlite.Repository) {
	t.Helper()
	tasks, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	repo, err := analyticssqlite.NewWithDB(database, database)
	require.NoError(t, err)
	tasks.SetUsageHistoryRetainer(repo)
	require.NoError(t, tasks.CreateWorkspace(t.Context(), &taskmodels.Workspace{ID: "scope-ws", Name: "Scope", OrgID: "scope-a"}))
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, tasks.CreateTask(t.Context(), &taskmodels.Task{ID: "scope-task", WorkspaceID: "scope-ws", Title: "Scope", CreatedAt: at, UpdatedAt: at}))
	require.NoError(t, tasks.CreateTaskSession(t.Context(), &taskmodels.TaskSession{ID: "scope-session", TaskID: "scope-task", StartedAt: at}))
	return tasks, repo
}

func finishScopeBackfill(t *testing.T, repo *analyticssqlite.Repository) {
	t.Helper()
	for range 1000 {
		done, err := repo.BackfillUsageHistory(t.Context(), 17)
		require.NoError(t, err)
		if done {
			return
		}
	}
	t.Fatal("bounded scope backfill did not finish")
}

func TestUsageHistoryMissingWireBindingUsesRetainedTurnScope(t *testing.T) {
	database, _ := projectionDatabases(t, false)
	tasks, repo := legacyScopeDatabaseFixture(t, database)
	at := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	turn := &taskmodels.Turn{ID: "scope-turn", TaskID: "scope-task", TaskSessionID: "scope-session", CreatedAt: at, StartedAt: at, UpdatedAt: at}
	require.NoError(t, tasks.CreateTurn(t.Context(), turn))
	require.NotEmpty(t, turn.UsageBindingID)
	_, err := database.Exec(`UPDATE workspaces SET org_id='scope-b' WHERE id='scope-ws'`)
	require.NoError(t, err)
	require.NoError(t, tasks.CreateTaskUsageEvent(t.Context(), &taskmodels.TaskUsageEvent{UsageEventID: "late-missing-wire-binding", TaskID: "scope-task", SessionID: "scope-session", TurnID: turn.ID, AgentType: "fixture", Model: "model", TokensIn: 10, TokensTotal: 10, ContractVersion: 1, OccurredAt: at, CreatedAt: at}))
	var org string
	require.NoError(t, database.Get(&org, `SELECT s.organization_id FROM session_usage_buckets b JOIN usage_scopes s ON s.scope_id=b.scope_id WHERE b.source_record_id='late-missing-wire-binding'`))
	require.Equal(t, "scope-a", org)
	finishScopeBackfill(t, repo)
	var rows int
	require.NoError(t, database.Get(&rows, `SELECT COUNT(*) FROM session_usage_buckets`))
	require.Equal(t, 1, rows)
	_, _, err = repo.ListRetainedCanonicalUsagePage(t.Context(), models.SessionUsageFilter{WorkspaceID: "scope-ws"})
	require.NoError(t, err)
}
