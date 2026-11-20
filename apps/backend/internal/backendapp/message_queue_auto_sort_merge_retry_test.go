package backendapp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/system/queuesettings"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	"github.com/stretchr/testify/require"
)

type policyAdmissionQueueRepository interface {
	messagequeue.Repository
	InsertForSessionWithPolicy(context.Context, messagequeue.QueueSessionIdentity, *messagequeue.QueuedMessage, *messagequeue.QueueAttachmentClaim, int, messagequeue.AdmissionPolicy) error
	AutoMergeCandidateIntoAboveForSessionWithPolicy(context.Context, messagequeue.QueueSessionIdentity, *messagequeue.QueuedMessage, *messagequeue.QueueAttachmentClaim, messagequeue.AdmissionPolicy) (*messagequeue.QueuedMessage, bool, error)
	AutoMergeIntoAboveForSessionWithPolicy(context.Context, messagequeue.QueueSessionIdentity, string, messagequeue.AdmissionPolicy) (*messagequeue.QueuedMessage, bool, error)
}

type changingSortPolicyRepository struct {
	policyAdmissionQueueRepository
	once sync.Once
}

func (r *changingSortPolicyRepository) GetAutoMergeOverride(ctx context.Context, identity messagequeue.QueueSessionIdentity) (*messagequeue.AutoMergeOverride, error) {
	value, err := r.policyAdmissionQueueRepository.GetAutoMergeOverride(ctx, identity)
	if err != nil {
		return nil, err
	}
	r.once.Do(func() {
		_, err = r.SetSessionPolicyAndCompact(ctx, identity, messagequeue.SessionPolicyChange{Enabled: false}, messagequeue.AutoMergePolicy{Enabled: true, Source: messagequeue.AutoMergeSourceGlobal}, nil)
	})
	return value, err
}

func TestAutoMergePolicyRetryWithSharedSingleConnectionSettingsLoader(t *testing.T) {
	pool := newMessageQueueSettingsTestPool(t)
	database := pool.Writer()
	_, err := database.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY,archived_at TIMESTAMP,updated_at TIMESTAMP);
 CREATE TABLE task_sessions (id TEXT PRIMARY KEY,task_id TEXT,queue_incarnation_id TEXT);
 INSERT INTO tasks (id,updated_at) VALUES ('task',CURRENT_TIMESTAMP);
 INSERT INTO task_sessions VALUES ('session','task','inc');`)
	require.NoError(t, err)
	repo, err := messagequeue.NewSQLiteRepository(database, database)
	require.NoError(t, err)
	wrapped := &changingSortPolicyRepository{policyAdmissionQueueRepository: repo.(policyAdmissionQueueRepository)}
	queue := messagequeue.NewService(wrapped, 10, testLogger(t))
	raw, err := systemsettings.NewStore(pool)
	require.NoError(t, err)
	settings := queuesettings.NewService(queuesettings.NewStore(raw), queue, func() queuesettings.Environment { return queuesettings.Environment{} }, testLogger(t))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = settings.Update(ctx, queuesettings.SettingsPatch{AutoMergeEnabled: new(true)})
	require.NoError(t, err)
	identity := messagequeue.QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	for _, item := range []struct{ content, sender string }{{"A1", "A"}, {"B1", "B"}} {
		require.NoError(t, repo.InsertForSession(ctx, identity, &messagequeue.QueuedMessage{TaskID: "task", SessionID: "session", Content: item.content, QueuedBy: messagequeue.QueuedByAgent, Metadata: map[string]any{messagequeue.MetadataSenderTaskID: item.sender}}, 0))
	}
	_, err = queue.QueueMessageWithMetadataForSession(ctx, identity, "A2", "", messagequeue.QueuedByAgent, false, nil, map[string]any{messagequeue.MetadataSenderTaskID: "A"})
	require.NoError(t, err)
	entries, err := repo.ListBySession(ctx, "session")
	require.NoError(t, err)
	contents := make([]string, len(entries))
	for index, entry := range entries {
		contents[index] = entry.Content
	}
	require.Equal(t, []string{"A1", "B1", "A2"}, contents)
	status, err := queue.Snapshot(ctx, identity)
	require.NoError(t, err)
	require.Equal(t, new(false), status.AutoMergeEnabled)
}
