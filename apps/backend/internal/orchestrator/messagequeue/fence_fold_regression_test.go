package messagequeue

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// A task-management fenced entry is never a fold target. The sorted and the
// adjacent path both skip it, so its content keeps the claim it was admitted
// under.
func TestAutoFoldSkipsTaskManagementFencedTarget(t *testing.T) {
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	ctx := context.Background()
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, repo, identity)
	fenced := sortTestEntry("A1", "A")
	require.NoError(t, repo.InsertForSession(ctx, identity, fenced, 0))
	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO queue_task_management_fences (queue_id, task_id, installation_id, instance_key, generation)
		VALUES (?, ?, '', '', 0)
	`), fenced.ID, identity.TaskID)
	require.NoError(t, err)
	service := NewService(repo, 10, logger.Default())
	service.SetGlobalAutoMergePolicy(AutoMergePolicy{Enabled: true})

	_, err = service.QueueMessageWithMetadataForSession(ctx, identity, "A2", "", QueuedByAgent, false, nil,
		map[string]any{MetadataSenderTaskID: "A"})
	require.NoError(t, err)
	after, err := repo.ListBySession(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, []string{"A1", "A2"}, sortTestContents(after))
}
