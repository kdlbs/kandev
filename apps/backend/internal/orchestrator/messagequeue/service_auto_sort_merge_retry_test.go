package messagequeue

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

type pausingSortReadRepository struct {
	Repository
	autoMergeAdmissionRepository
	once              sync.Once
	captured, release chan struct{}
}

func (r *pausingSortReadRepository) GetAutoMergeOverride(ctx context.Context, identity QueueSessionIdentity) (*AutoMergeOverride, error) {
	value, err := r.Repository.GetAutoMergeOverride(ctx, identity)
	r.once.Do(func() { close(r.captured); <-r.release })
	return value, err
}

func TestAutoMergeAdmissionRetriesChangedOverride(t *testing.T) {
	base := NewMemoryRepository().(*memoryRepository)
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, base, identity)
	ctx := context.Background()
	for _, row := range []*QueuedMessage{sortTestEntry("A1", "A"), sortTestEntry("B1", "B")} {
		require.NoError(t, base.InsertForSession(ctx, identity, row, 0))
	}
	repo := &pausingSortReadRepository{Repository: base, autoMergeAdmissionRepository: base, captured: make(chan struct{}), release: make(chan struct{})}
	service := NewService(repo, 10, logger.Default())
	done := make(chan error, 1)
	go func() {
		_, err := service.QueueMessageWithMetadataForSession(ctx, identity, "A2", "", QueuedByAgent, false, nil, map[string]any{MetadataSenderTaskID: "A"})
		done <- err
	}()
	<-repo.captured
	_, err := setAutoMergeOverrideForTest(ctx, base, identity, false)
	close(repo.release)
	require.NoError(t, err)
	require.NoError(t, <-done)
	after, err := base.ListBySession(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, []string{"A1", "B1", "A2"}, sortTestContents(after))
}
