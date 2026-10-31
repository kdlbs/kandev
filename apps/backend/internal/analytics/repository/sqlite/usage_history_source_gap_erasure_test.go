package sqlite

import (
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryErasureIncludesWithheldNativeSource(t *testing.T) {
	runErasureWithheldNativeSource(t, false)
}

func TestUsageHistoryPostgresErasureIncludesWithheldNativeSource(t *testing.T) {
	runErasureWithheldNativeSource(t, true)
}

func runErasureWithheldNativeSource(t *testing.T, postgres bool) {
	for _, backfill := range []bool{false, true} {
		name := "admission_gap"
		if backfill {
			name = "backfill_gap"
		}
		t.Run(name, func(t *testing.T) {
			repo, tasks := currentTargetDatabaseFixture(t, postgres)
			tasks.SetUsageHistoryRetainer(nil)
			createCurrentTargetSession(t, tasks, "legacy-task", "legacy-session", "legacy-transcript")
			if !backfill {
				tasks.SetUsageHistoryRetainer(repo)
			}
			at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
			old := &taskmodels.TaskUsageEvent{UsageEventID: "withheld-a", TaskID: "legacy-task", SessionID: "legacy-session", AgentType: "fixture",
				Model: "model", TokensIn: 12, TokensTotal: 12, ContractVersion: 1, OccurredAt: at, CreatedAt: at}
			require.NoError(t, tasks.CreateTaskUsageEvent(t.Context(), old))
			if backfill {
				finishFixtureRetention(t, repo)
				tasks.SetUsageHistoryRetainer(repo)
			}
			var retained, gaps int
			require.NoError(t, repo.db.Get(&retained, `SELECT COUNT(*) FROM session_usage_buckets WHERE source_record_id='withheld-a'`))
			require.Zero(t, retained, "unproven source ownership is an explicit gap, not retained attribution")
			require.NoError(t, repo.db.Get(&gaps, `SELECT COUNT(*) FROM usage_history_migration_gaps WHERE source_key='native' AND organization_id='org-a' AND task_id='legacy-task'`))
			require.Positive(t, gaps)
			_, err := repo.db.Exec(`UPDATE workspaces SET org_id='org-b' WHERE id='ws-1'`)
			require.NoError(t, err)
			fresh := *old
			fresh.UsageEventID, fresh.TokensIn, fresh.TokensTotal = "withheld-b", 77, 77
			require.NoError(t, tasks.CreateTaskUsageEvent(t.Context(), &fresh))
			require.NoError(t, repo.EraseOrganizationUsage(t.Context(), "org-a"))
			var ledger []struct {
				ID     string `db:"usage_event_id"`
				Tokens int64  `db:"tokens_total"`
			}
			require.NoError(t, repo.db.Select(&ledger, `SELECT usage_event_id,tokens_total FROM task_usage_events ORDER BY id`))
			require.Len(t, ledger, 1, "captured erasure must remove exactly the old withheld source")
			require.Equal(t, "withheld-b", ledger[0].ID)
			require.EqualValues(t, 77, ledger[0].Tokens)
			require.NoError(t, repo.ResetWorkspaceUsageHistory(t.Context(), "ws-1"))
			require.NoError(t, repo.db.Get(&retained, `SELECT COUNT(*) FROM task_usage_events`))
			require.Zero(t, retained, "workspace reset also clears withheld native source rows")
		})
	}
}
