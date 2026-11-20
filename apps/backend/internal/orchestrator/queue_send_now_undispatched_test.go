package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/stretchr/testify/require"
)

func planCommentSendNowService(t *testing.T, promptErr error) (*Service, *mockMessageCreator) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-1", models.TaskSessionStateWaitingForInput, ""))
	agentMgr := &mockAgentManager{isAgentRunning: true, promptErr: promptErr, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = withTestCapabilities(executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{}))
	database := sqlx.NewDb(repo.DB(), "sqlite3")
	_, err := messagequeue.NewSQLiteRepository(database, database)
	require.NoError(t, err)
	for _, column := range []struct {
		table, name, definition string
	}{
		{"task_sessions", "task_id", "TEXT NOT NULL DEFAULT ''"},
		{"task_sessions", "queue_incarnation_id", "TEXT NOT NULL DEFAULT ''"},
		{"queued_messages", "task_id", "TEXT NOT NULL DEFAULT ''"},
	} {
		exists, err := internaldb.ColumnExists(database, column.table, column.name)
		require.NoError(t, err)
		if !exists {
			_, err = database.Exec(`ALTER TABLE ` + column.table + ` ADD COLUMN ` + column.name + ` ` + column.definition)
			require.NoError(t, err)
		}
	}
	_, err = database.Exec(`UPDATE task_sessions SET task_id = ? WHERE id = ?`, "task-1", "session-1")
	require.NoError(t, err)
	queueRepo, err := messagequeue.NewSQLiteRepository(database, database)
	require.NoError(t, err)
	svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "session-1", false))
	for _, id := range []string{"comment-a", "comment-b"} {
		_, err := svc.messageQueue.QueueMessageWithMetadata(ctx, "session-1", "task-1", "feedback "+id, "",
			messagequeue.QueuedByUser, false, nil, map[string]interface{}{
				plancomments.MetadataClientQueueID:      id,
				plancomments.MetadataRequestFingerprint: "fingerprint-" + id,
			})
		require.NoError(t, err)
	}
	return svc, messages
}

func runPlanCommentSendNow(t *testing.T, svc *Service) {
	t.Helper()
	ctx := context.Background()
	sources := svc.messageQueue.GetStatus(ctx, "session-1").Entries
	require.Len(t, sources, 2)
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "task-1", "session-1")
	require.NoError(t, err)
	claimed, err := svc.messageQueue.ClaimSendNowForSession(ctx, identity, sources)
	require.NoError(t, err)
	svc.markQueuedDispatchInFlight("session-1", claimed.Dispatch.ID)
	// Keep restored entries pending so the test observes the restore itself.
	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "session-1", false))
	svc.executeSendNowClaim(claimed)
}

// @covers AC-TASKS-PLAN-COMMENTS-002.9
func TestSendNowPlanCommentsFailingBeforeTransportAreRestored(t *testing.T) {
	svc, messages := planCommentSendNowService(t,
		agentruntime.MarkPromptNotDispatched(errors.New("prompt admission persistence failed")))

	runPlanCommentSendNow(t, svc)

	entries := svc.messageQueue.GetStatus(context.Background(), "session-1").Entries
	require.Len(t, entries, 2, "undispatched Send Now sources must return to the queue")
	for _, entry := range entries {
		require.False(t, entry.IsDeliveryAttempted(), "entry %s kept the delivery marker", entry.ID)
		require.True(t, entry.IsDurablePlanComment(), "entry %s lost its replay identity", entry.ID)
	}
	require.Len(t, messages.userMessages, 1, "the combined transcript is written once")
}

// @covers AC-TASKS-PLAN-COMMENTS-002.9
func TestSendNowPlanCommentsClearDeliveryMarkersAfterWorkerCancellation(t *testing.T) {
	ctx := context.Background()
	svc, _ := planCommentSendNowService(t, nil)
	sources := svc.messageQueue.GetStatus(ctx, "session-1").Entries
	require.Len(t, sources, 2)
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "task-1", "session-1")
	require.NoError(t, err)
	claim, err := svc.messageQueue.ClaimSendNowForSession(ctx, identity, sources)
	require.NoError(t, err)
	require.Equal(t, "task-1", claim.Identity.TaskID)
	receipts := planCommentSendNowReceipts(claim)
	require.NoError(t, svc.messageQueue.MarkDeliveryAttemptedForSession(ctx, claim.Identity, receipts))
	for i := range claim.Sources {
		claim.Sources[i].Metadata[messagequeue.MetadataDeliveryAttempted] = true
	}

	workerCtx, cancel := context.WithCancel(ctx)
	cancel()
	dispatchErr := agentruntime.MarkPromptNotDispatched(errors.New("prompt admission persistence failed"))
	require.True(t, svc.revokeUndispatchedSendNow(workerCtx, claim, dispatchErr))

	entries := svc.messageQueue.GetStatus(ctx, "session-1").Entries
	require.Len(t, entries, 2, "cancelled undispatched sources must be restored from SQLite")
	for _, entry := range entries {
		require.False(t, entry.IsDeliveryAttempted(), "entry %s kept its at-most-once marker", entry.ID)
		require.True(t, entry.IsDurablePlanComment(), "entry %s lost its replay identity", entry.ID)
	}
}

// @covers AC-TASKS-PLAN-COMMENTS-002.9
func TestSendNowPlanCommentsPossiblyDeliveredAreSettled(t *testing.T) {
	svc, _ := planCommentSendNowService(t, errors.New("agentctl transport closed mid-request"))

	runPlanCommentSendNow(t, svc)

	require.Empty(t, svc.messageQueue.GetStatus(context.Background(), "session-1").Entries,
		"a failure after possible receipt keeps at-most-once delivery")
}
