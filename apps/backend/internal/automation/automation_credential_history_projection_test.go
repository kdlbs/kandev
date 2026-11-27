package automation

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProtectedRunHistoryProjectionHidesTranscriptFromWorkspaceReaders(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := credentialIdentity("credential-creator")
	a := &Automation{
		WorkspaceID: "workspace-protected-history", Name: "Protected history",
		CreatedByUserID: "credential-creator",
	}
	require.NoError(t, store.CreateAutomation(ctx, a))
	storePersonalCredentialChoice(t, store, a)
	insertTask(t, store, "protected-history-task", false)
	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeScheduled, TaskID: "protected-history-task",
		SessionID: "private-session", TurnID: "private-turn", Status: RunStatusFailed,
		ErrorMessage: "secret-bearing provider failure", TriggerData: json.RawMessage(`{}`),
	}
	require.NoError(t, store.CreateRun(ctx, run))
	_, err := store.db.Exec(`INSERT INTO task_session_messages
		(id, task_id, turn_id, author_type, content, type, created_at)
		VALUES ('protected-history-message', ?, ?, 'agent', 'private transcript', 'message', ?)`,
		run.TaskID, run.TurnID, time.Now().UTC())
	require.NoError(t, err)
	listRows, _, err := store.ListHistoryRuns(ctx, HistoryRunFilter{WorkspaceID: a.WorkspaceID, Limit: 10})
	require.NoError(t, err)
	require.Len(t, listRows, 1)
	require.Equal(t, "credential-creator", listRows[0].ProtectedCreatorUserID)
	readerContext := credentialIdentity("workspace-reader")
	convertedList := historyRunFromRow(listRows[0], time.Now())
	listProjection := projectHistoryRun(readerContext, convertedList)
	assertHistoryTranscriptHidden(t, listProjection)

	storedRun, err := store.GetHistoryRun(ctx, a.WorkspaceID, run.ID)
	require.NoError(t, err)
	readerProjection := projectHistoryRun(readerContext, historyRunFromRow(storedRun, time.Now()))
	assertHistoryTranscriptHidden(t, readerProjection)
	creatorProjection := projectHistoryRun(credentialIdentity("credential-creator"), historyRunFromRow(storedRun, time.Now()))
	require.Equal(t, run.TaskID, creatorProjection.TaskID)
	require.Equal(t, run.SessionID, creatorProjection.SessionID)
	require.Equal(t, run.TurnID, creatorProjection.TurnID)
	require.Equal(t, "private transcript", creatorProjection.Summary)
	require.Equal(t, run.ErrorMessage, creatorProjection.ErrorMessage)
}

func assertHistoryTranscriptHidden(t *testing.T, run HistoryRun) {
	t.Helper()
	require.Equal(t, "credential-creator", run.ProtectedCreatorUserID)
	require.Empty(t, run.TaskID)
	require.Empty(t, run.SessionID)
	require.Empty(t, run.TurnID)
	require.Empty(t, run.ManagedConversationID)
	require.Empty(t, run.Summary)
	require.Equal(t, "Automation run failed.", run.ErrorMessage)
}
