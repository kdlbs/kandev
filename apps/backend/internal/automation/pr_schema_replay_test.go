package automation

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// Constructing the store again on an existing database keeps per-PR data and
// adds the per-PR columns, with their defaults, to rows from before them.
func TestPRFollowUpSchemaReplayAndUpgrade(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewStore(db, db)
	require.NoError(t, err)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-1", Name: "legacy", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	require.NoError(t, store.UpsertPRContinuation(ctx, &PRContinuation{
		AutomationID: a.ID, PRSubject: "acme/widget#1", RepoOwner: "acme", RepoName: "widget", PRNumber: 1, TaskID: "task-1",
	}))

	// Simulate a database from before the per-PR columns and the tracked
	// events column existed.
	for _, column := range []string{"follow_up_prompt", "pr_continuation_step_id", "pr_merged_action", "pr_closed_action"} {
		_, err := db.Exec(`ALTER TABLE automations DROP COLUMN ` + column)
		require.NoError(t, err)
	}
	_, err = db.Exec(`ALTER TABLE automation_pr_observations DROP COLUMN tracked_events`)
	require.NoError(t, err)

	replayed, err := NewStore(db, db)
	require.NoError(t, err)
	_, err = NewStore(db, db)
	require.NoError(t, err, "a second replay is a no-op")

	got, err := replayed.GetAutomation(ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, PREndActionKeep, got.PRMergedAction)
	require.Equal(t, PREndActionKeep, got.PRClosedAction)
	require.Empty(t, got.FollowUpPrompt)
	taskID, err := replayed.GetPRContinuationTaskID(ctx, a.ID, "acme/widget#1")
	require.NoError(t, err)
	require.Equal(t, "task-1", taskID)
	var tracked string
	require.NoError(t, db.Get(&tracked, `SELECT COUNT(*) FROM pragma_table_info('automation_pr_observations') WHERE name = 'tracked_events'`))
	require.Equal(t, "1", tracked)
}
