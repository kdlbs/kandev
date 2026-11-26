package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/system/queuesettings"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	"github.com/stretchr/testify/require"
)

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.3
func TestAutoMergeSharedSingleConnectionSettingsLoader(t *testing.T) {
	pool := newMessageQueueSettingsTestPool(t)
	database := pool.Writer()
	_, err := database.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY,archived_at TIMESTAMP,updated_at TIMESTAMP);
 CREATE TABLE task_sessions (id TEXT PRIMARY KEY,task_id TEXT,queue_incarnation_id TEXT);
 INSERT INTO tasks (id,updated_at) VALUES ('task',CURRENT_TIMESTAMP);
 INSERT INTO task_sessions VALUES ('session','task','inc');`)
	require.NoError(t, err)
	repo, err := messagequeue.NewSQLiteRepository(database, database)
	require.NoError(t, err)
	queue := messagequeue.NewService(repo, 10, testLogger(t))
	raw, err := systemsettings.NewStore(pool)
	require.NoError(t, err)
	settings := queuesettings.NewService(queuesettings.NewStore(raw), queue, func() queuesettings.Environment { return queuesettings.Environment{} }, testLogger(t))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = settings.Update(ctx, queuesettings.SettingsPatch{AutoMergeEnabled: new(true)})
	require.NoError(t, err)
	identity := messagequeue.QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	status, err := queue.Snapshot(ctx, identity)
	require.NoError(t, err)
	require.Equal(t, new(true), status.AutoMergeEnabled)
	result, err := queue.SetSessionAutoMerge(ctx, identity, false)
	require.NoError(t, err)
	require.False(t, result.AutoMerge.Enabled)
	for _, item := range []struct{ content, sender string }{{"A1", "A"}, {"B1", "B"}, {"A2", "A"}} {
		require.NoError(t, repo.InsertForSession(ctx, identity, &messagequeue.QueuedMessage{TaskID: "task", SessionID: "session", Content: item.content, QueuedBy: messagequeue.QueuedByAgent, Metadata: map[string]any{messagequeue.MetadataSenderTaskID: item.sender}}, 0))
	}
	result, err = queue.SetSessionAutoMerge(ctx, identity, true)
	require.NoError(t, err)
	require.Equal(t, 1, result.CompactedCount)
	result, err = queue.SetSessionAutoMerge(ctx, identity, false)
	require.NoError(t, err)
	require.False(t, result.AutoMerge.Enabled)
	require.NoError(t, repo.InsertForSession(ctx, identity, &messagequeue.QueuedMessage{TaskID: "task", SessionID: "session", Content: "A3", QueuedBy: messagequeue.QueuedByAgent, Metadata: map[string]any{messagequeue.MetadataSenderTaskID: "A"}}, 0))
	result, err = queue.SetSessionAutoMerge(ctx, identity, true)
	require.NoError(t, err)
	require.Equal(t, 1, result.CompactedCount)
	_, err = queue.QueueMessageWithMetadataForSession(ctx, identity, "prompt", "", messagequeue.QueuedByUser, false, nil, nil)
	require.NoError(t, err)
}
