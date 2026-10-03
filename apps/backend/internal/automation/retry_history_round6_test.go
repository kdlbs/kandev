package automation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListRetryHistoryReturnsCompleteAttemptTimelineAndCursor(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{ID: "history-automation", WorkspaceID: "history-workspace", Name: "history", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "history-group", AutomationID: a.ID, TriggerID: "trigger-a", TriggerIDsJSON: `["trigger-a","trigger-b"]`, Generation: 1, State: RetryGroupCompleted}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	for i, id := range []string{"attempt-1", "attempt-2"} {
		require.NoError(t, store.CreateRun(ctx, &AutomationRun{
			ID: id, AutomationID: a.ID, TriggerID: "trigger-a", TriggerType: TriggerTypeManual,
			RetryGroupID: group.ID, RetryGroupGeneration: 1, AttemptNumber: int64(i + 1),
			RetryState: RetryStateCompleted, Status: RunStatusSucceeded,
			TriggerData: json.RawMessage(`{"projection_version":1}`),
		}))
	}
	page, err := store.ListRetryHistory(ctx, a.ID, "", 1)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, []string{"trigger-a", "trigger-b"}, page.Items[0].TriggerIDs)
	require.Len(t, page.Items[0].Attempts, 2)
	require.NotEmpty(t, page.HighWaterMark)
}
func TestListRetryHistoryReturnsNewestGroupsFirst(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	automation := &Automation{
		ID: "history-order-automation", WorkspaceID: "history-order-workspace",
		Name: "history order", Enabled: true,
	}
	require.NoError(t, store.CreateAutomation(ctx, automation))
	for _, group := range []*RetryGroup{
		{ID: "history-order-old", AutomationID: automation.ID, TriggerID: "trigger-old", Generation: 1, State: RetryGroupCompleted},
		{ID: "history-order-new", AutomationID: automation.ID, TriggerID: "trigger-new", Generation: 1, State: RetryGroupCompleted},
	} {
		require.NoError(t, store.CreateRetryGroup(ctx, group))
	}
	_, err := store.db.ExecContext(ctx,
		`UPDATE automation_retry_groups SET created_at = ? WHERE id = ?`,
		"2024-01-01T00:00:00Z", "history-order-old")
	require.NoError(t, err)
	_, err = store.db.ExecContext(ctx,
		`UPDATE automation_retry_groups SET created_at = ? WHERE id = ?`,
		"2024-01-02T00:00:00Z", "history-order-new")
	require.NoError(t, err)

	first, err := store.ListRetryHistory(ctx, automation.ID, "", 1)
	require.NoError(t, err)
	require.Len(t, first.Items, 1)
	require.Equal(t, "history-order-new", first.Items[0].RetryGroupID)
	require.NotEmpty(t, first.NextCursor)

	second, err := store.ListRetryHistory(ctx, automation.ID, first.NextCursor, 1)
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	require.Equal(t, "history-order-old", second.Items[0].RetryGroupID)
	require.Empty(t, second.NextCursor)
}

func TestCancelledRetryStateProjectsCancelledStatus(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	automation := &Automation{
		ID: "cancelled-history-automation", WorkspaceID: "cancelled-history-workspace",
		Name: "cancelled history", Enabled: true,
	}
	require.NoError(t, store.CreateAutomation(ctx, automation))
	group := &RetryGroup{
		ID: "cancelled-history-group", AutomationID: automation.ID,
		TriggerID: "trigger-a", Generation: 1, State: RetryGroupCancelled,
	}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{
		ID: "cancelled-history-run", AutomationID: automation.ID,
		TriggerID: "trigger-a", TriggerType: TriggerTypeManual,
		Status: RunStatusFailed, RetryGroupID: group.ID,
		RetryGroupGeneration: 1, AttemptNumber: 1,
		RetryState: RetryStateCancelled,
	}
	require.NoError(t, store.CreateRun(ctx, run))

	runs, err := store.ListRuns(ctx, automation.ID, 50)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, RunStatusCancelled, runs[0].Status)

	history, err := store.ListRetryHistory(ctx, automation.ID, "", 50)
	require.NoError(t, err)
	require.Len(t, history.Items, 1)
	require.Len(t, history.Items[0].Attempts, 1)
	require.Equal(t, RunStatusCancelled, history.Items[0].Attempts[0].Status)
}

func TestListRetryHistoryProjectsTaskStateAndRunSummary(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()
	automation := &Automation{
		ID: "history-projection-automation", WorkspaceID: "history-projection-workspace",
		Name: "history projection", Enabled: true,
	}
	require.NoError(t, store.CreateAutomation(ctx, automation))
	group := &RetryGroup{
		ID: "history-projection-group", AutomationID: automation.ID,
		Generation: 1, State: RetryGroupCompleted,
	}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	insertTask(t, store, "history-projection-task", true)
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO task_session_messages
			(id, task_id, turn_id, author_type, content, type)
		VALUES (?, ?, ?, 'agent', ?, 'message')`,
		"history-projection-message", "history-projection-task", "history-projection-turn",
		"archived retry summary")
	require.NoError(t, err)
	require.NoError(t, store.CreateRun(ctx, &AutomationRun{
		ID: "history-projection-run", AutomationID: automation.ID,
		TriggerID: "history-projection-trigger", TriggerType: TriggerTypeManual,
		TaskID: "history-projection-task", TurnID: "history-projection-turn",
		Status: RunStatusTaskCreated, RetryGroupID: group.ID,
		RetryGroupGeneration: 1, AttemptNumber: 1, RetryState: RetryStateCompleted,
	}))

	page, err := store.ListRetryHistory(ctx, automation.ID, "", 10)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Len(t, page.Items[0].Attempts, 1)
	attempt := page.Items[0].Attempts[0]
	require.Equal(t, RunStatusArchived, attempt.Status)
	require.Equal(t, "history-projection-task", attempt.TaskID)
	require.Equal(t, "archived retry summary", attempt.Summary)
}

func TestDeleteAllRunsRemovesRetryHistoryLedger(t *testing.T) {
	svc := newTestService(t)
	store := svc.store
	ctx := context.Background()
	automation := &Automation{
		ID: "delete-history-automation", WorkspaceID: "delete-history-workspace",
		Name: "delete history", Enabled: true,
	}
	require.NoError(t, store.CreateAutomation(ctx, automation))
	group := &RetryGroup{
		ID: "delete-history-group", AutomationID: automation.ID,
		TriggerID: "trigger-a", Generation: 1, State: RetryGroupCompleted,
	}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{
		ID: "delete-history-run", AutomationID: automation.ID,
		TriggerID: "trigger-a", TriggerType: TriggerTypeManual,
		Status: RunStatusSucceeded, RetryGroupID: group.ID,
		RetryGroupGeneration: 1, AttemptNumber: 1,
		RetryState: RetryStateCompleted,
	}
	require.NoError(t, store.CreateRun(ctx, run))
	intent := &RetryTaskIntent{
		ID: "delete-history-intent", RunID: run.ID,
		GroupGeneration: 1, State: retryIntentCreated, TaskID: "task-history",
	}
	require.NoError(t, store.CreateRetryIntent(ctx, intent))
	require.NoError(t, store.CreateRetryOperation(ctx, &RetryOperation{
		ID: "delete-history-operation", IntentID: intent.ID, RunID: run.ID,
		GroupGeneration: 1, Kind: retryTaskOperationKind,
		State: retryOperationCommitted, ExternalTaskID: "task-history",
	}))
	require.NoError(t, store.CreateRetryOutbox(ctx, &RetryOutbox{
		EventID: "delete-history-event", RunID: run.ID, SnapshotVersion: 1,
		State: retryOutboxRevoked,
	}))

	require.NoError(t, svc.DeleteAllRuns(ctx, automation.ID))

	history, err := store.ListRetryHistory(ctx, automation.ID, "", 50)
	require.NoError(t, err)
	require.Empty(t, history.Items)
	var groups, intents, operations, outbox int
	require.NoError(t, store.db.Get(&groups,
		`SELECT COUNT(*) FROM automation_retry_groups WHERE automation_id = ?`, automation.ID))
	require.NoError(t, store.db.Get(&intents,
		`SELECT COUNT(*) FROM automation_run_task_intents WHERE run_id = ?`, run.ID))
	require.NoError(t, store.db.Get(&operations,
		`SELECT COUNT(*) FROM automation_run_operations WHERE run_id = ?`, run.ID))
	require.NoError(t, store.db.Get(&outbox,
		`SELECT COUNT(*) FROM automation_retry_outbox WHERE event_id = ?`, "delete-history-event"))
	require.Zero(t, groups)
	require.Zero(t, intents)
	require.Zero(t, operations)
	require.Zero(t, outbox)
}
