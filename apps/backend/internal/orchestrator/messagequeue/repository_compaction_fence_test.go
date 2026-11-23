package messagequeue

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func TestCompactionKeepsMessagesFromDifferentManagementClaimsSeparate(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	identity := QueueSessionIdentity{TaskID: "exact-task", SessionID: "exact-session", SessionIncarnationID: "exact-incarnation"}
	seedQueueSessionIdentity(t, repo, identity)
	prepareWorkflowEntryAuthority(t, repo, identity)
	_, err := repo.db.Exec(`CREATE TABLE task_management_claims (
				task_id TEXT PRIMARY KEY, owner_kind TEXT NOT NULL, installation_id TEXT NOT NULL,
				instance_key TEXT NOT NULL, generation INTEGER NOT NULL
			)`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`INSERT INTO task_management_claims (task_id, owner_kind, installation_id, instance_key, generation)
				VALUES (?, 'plugin', 'manager', 'instance-1', 4)`, identity.TaskID)
	require.NoError(t, err)

	queue := NewService(repo, 10, logger.Default())
	_, err = queue.SetSessionAutoMerge(ctx, identity, false)
	require.NoError(t, err)

	admit := func(clientID, content, instance string, generation int64) *QueuedMessage {
		entry := currentWorkflowEntry(t, repo, identity)
		entry.EnforceTaskManagementClaim = true
		entry.ManagementInstallationID = "manager"
		entry.ManagementInstanceKey = instance
		entry.ExpectedClaimGeneration = generation
		queued, _, err := queue.QueueMessageWithMetadataForSessionWithClientQueueIDAtWorkflowEntry(
			ctx, identity, entry, clientID, content, "", QueuedByUser, false, nil, nil,
		)
		require.NoError(t, err)
		return queued
	}
	stale := admit("old-message", "old instruction", "instance-1", 4)
	_, err = repo.db.Exec(`UPDATE task_management_claims SET instance_key = 'instance-2', generation = 5 WHERE task_id = ?`, identity.TaskID)
	require.NoError(t, err)
	current := admit("new-message", "new instruction", "instance-2", 5)

	_, err = queue.SetSessionAutoMerge(ctx, identity, true)
	require.NoError(t, err)

	rows, err := repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Equal(t, []string{"old instruction", "new instruction"}, sortTestContents(rows))
	_, found, err := queue.TakeQueuedEntryForSession(ctx, identity, stale.ID)
	require.NoError(t, err)
	require.False(t, found, "the obsolete claim's instruction is discarded")
	taken, found, err := queue.TakeQueuedEntryForSession(ctx, identity, current.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "new instruction", taken.Content)
}
