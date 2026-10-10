package automation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareRetryRecoveryRunKeepsOutboxOnOperationLookupError(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	automation := &Automation{ID: "recovery-reader-automation", WorkspaceID: "recovery-reader-workspace", Name: "recovery reader", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, automation))
	group := &RetryGroup{ID: "recovery-reader-group", AutomationID: automation.ID, Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{ID: "recovery-reader-run", AutomationID: automation.ID,
		TriggerType: TriggerTypeManual, Status: RunStatusTriggered,
		RetryGroupID: group.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
	require.NoError(t, store.CreateRun(ctx, run))
	eventID := "recovery-reader-event"
	require.NoError(t, store.CreateRetryOutbox(ctx, &RetryOutbox{
		EventID: eventID, RunID: run.ID, SnapshotVersion: 1, State: retryOutboxPending,
	}))
	_, err := store.db.Exec(`DROP TABLE automation_run_operations`)
	require.NoError(t, err)

	_, skip, err := (&Service{store: store}).prepareRetryRecoveryRun(ctx, RetryOutbox{
		EventID: eventID, RunID: run.ID, SnapshotVersion: 1, State: retryOutboxPending,
	})
	require.Error(t, err)
	require.False(t, skip)

	var outboxState string
	require.NoError(t, store.db.Get(&outboxState,
		`SELECT state FROM automation_retry_outbox WHERE event_id = ?`, eventID))
	require.Equal(t, retryOutboxPending, outboxState)
}
