package automation

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/repository"
)

// TestListRuns_ReturnsRepositoryAndDedupReason_WithTasksTablePresent guards
// the runs-list read path against silently dropping repository_reason and
// dedup_reason. Wiring a real task repository onto the same DB connection
// makes ListRuns exercise listRunsWithTaskState — the query production and
// the runs-list UI actually read from — instead of the isolated-store
// listRunsRaw fallback, whose SELECT * would mask a missing column.
func TestListRuns_ReturnsRepositoryAndDedupReason_WithTasksTablePresent(t *testing.T) {
	root := t.TempDir()
	sqlDB, err := db.OpenSQLite(filepath.Join(root, "runs.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(sqlDB, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	_, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })

	store, err := NewStore(sqlxDB, sqlxDB)
	require.NoError(t, err)

	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-1", Name: "alert ingest", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))

	run := &AutomationRun{
		AutomationID:     a.ID,
		TriggerType:      TriggerTypeWebhook,
		Status:           RunStatusTriggered,
		RepositoryReason: "selector_no_match: acme/unconfigured-webapp",
		DedupReason:      "dedup_unresolved",
	}
	require.NoError(t, store.CreateRun(ctx, run))

	runs, err := store.ListRuns(ctx, a.ID, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, "selector_no_match: acme/unconfigured-webapp", runs[0].RepositoryReason)
	require.Equal(t, "dedup_unresolved", runs[0].DedupReason)
}
