package messagequeue

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

type sortReadFailureRepository struct{ *memoryRepository }

func (r *sortReadFailureRepository) GetAutoSortMergeOverride(context.Context, QueueSessionIdentity) (*AutoSortMergeOverride, error) {
	return nil, errors.New("sort unavailable")
}

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.14
func TestAutoSortMergeReadFailureKeepsOrdinaryMerge(t *testing.T) {
	base := NewMemoryRepository().(*memoryRepository)
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, base, identity)
	service := NewService(&sortReadFailureRepository{base}, 10, logger.Default())
	service.SetGlobalQueuePolicy(GlobalQueuePolicy{AutoMerge: AutoMergePolicy{Enabled: true}, AutoSortMerge: AutoSortMergePolicy{Enabled: true}})
	ctx := context.Background()
	for _, row := range []*QueuedMessage{sortTestEntry("A1", "A"), sortTestEntry("B1", "B")} {
		require.NoError(t, base.InsertForSession(ctx, identity, row, 0))
	}
	_, err := service.QueueMessageWithMetadataForSession(ctx, identity, "A2", "", QueuedByAgent, false, nil, map[string]any{MetadataSenderTaskID: "A"})
	require.NoError(t, err)
	_, err = service.QueueMessageWithMetadataForSession(ctx, identity, "A3", "", QueuedByAgent, false, nil, map[string]any{MetadataSenderTaskID: "A"})
	require.NoError(t, err)
	after, err := base.ListBySession(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, []string{"A1", "B1", "A2\n\nA3"}, sortTestContents(after))
}

type pausingSortReadRepository struct {
	Repository
	autoMergeAdmissionRepository
	once              sync.Once
	captured, release chan struct{}
}

func (r *pausingSortReadRepository) GetAutoSortMergeOverride(ctx context.Context, identity QueueSessionIdentity) (*AutoSortMergeOverride, error) {
	value, err := r.Repository.GetAutoSortMergeOverride(ctx, identity)
	r.once.Do(func() { close(r.captured); <-r.release })
	return value, err
}

func TestAutoSortMergeAdmissionRetriesChangedSortOverride(t *testing.T) {
	base := NewMemoryRepository().(*memoryRepository)
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, base, identity)
	ctx := context.Background()
	for _, row := range []*QueuedMessage{sortTestEntry("A1", "A"), sortTestEntry("B1", "B")} {
		require.NoError(t, base.InsertForSession(ctx, identity, row, 0))
	}
	repo := &pausingSortReadRepository{Repository: base, autoMergeAdmissionRepository: base, captured: make(chan struct{}), release: make(chan struct{})}
	service := NewService(repo, 10, logger.Default())
	service.SetGlobalQueuePolicy(GlobalQueuePolicy{AutoMerge: AutoMergePolicy{Enabled: true}, AutoSortMerge: AutoSortMergePolicy{Enabled: true}})
	done := make(chan error, 1)
	go func() {
		_, err := service.QueueMessageWithMetadataForSession(ctx, identity, "A2", "", QueuedByAgent, false, nil, map[string]any{MetadataSenderTaskID: "A"})
		done <- err
	}()
	<-repo.captured
	_, err := base.SetAutoSortMergeOverride(ctx, identity, false)
	close(repo.release)
	require.NoError(t, err)
	require.NoError(t, <-done)
	after, err := base.ListBySession(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, []string{"A1", "B1", "A2"}, sortTestContents(after))
}
