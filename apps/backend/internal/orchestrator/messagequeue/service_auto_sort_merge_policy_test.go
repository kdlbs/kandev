package messagequeue

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.3
func TestAutoSortMergeStatusDefaultPreference(t *testing.T) {
	repo := NewMemoryRepository()
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, repo, identity)
	service := NewService(repo, 10, logger.Default())
	status, err := service.Snapshot(context.Background(), identity)
	require.NoError(t, err)
	raw, err := json.Marshal(status)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(raw, &fields))
	require.Equal(t, true, fields["auto_sort_merge_available"])
	require.Equal(t, false, fields["auto_sort_merge_enabled"])
	require.Equal(t, "global", fields["auto_sort_merge_source"])
	require.Equal(t, float64(0), fields["auto_sort_merge_revision"])
}

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.3
func TestAutoSortMergeSessionPreferenceSurvivesGlobalChanges(t *testing.T) {
	repo := NewMemoryRepository()
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, repo, identity)
	service := NewService(repo, 10, logger.Default())
	setter, ok := any(service).(interface {
		SetSessionAutoSortMerge(context.Context, QueueSessionIdentity, bool) (SessionPolicyResult, error)
	})
	require.True(t, ok, "session switch persists an explicit preference")
	result, err := setter.SetSessionAutoSortMerge(context.Background(), identity, true)
	require.NoError(t, err)
	require.Equal(t, AutoSortMergePolicy{Enabled: true, Source: AutoMergeSourceSession, Revision: 1}, result.AutoSortMerge)
	service.SetGlobalQueuePolicy(GlobalQueuePolicy{AutoMerge: AutoMergePolicy{Enabled: false}, AutoSortMerge: AutoSortMergePolicy{Enabled: false, Revision: 2}})
	status, err := service.Snapshot(context.Background(), identity)
	require.NoError(t, err)
	require.Equal(t, new(true), status.AutoSortMergeEnabled, "Auto-merge OFF must not erase the preference")
	require.Equal(t, AutoMergeSourceSession, status.AutoSortMergeSource)
}

type failingSortSnapshotRepository struct{ Repository }

func (r *failingSortSnapshotRepository) Snapshot(ctx context.Context, identity QueueSessionIdentity) (RepositorySnapshot, error) {
	snapshot, err := r.Repository.Snapshot(ctx, identity)
	snapshot.AutoSortMergeOverride = nil
	snapshot.AutoSortMergeError = errors.New("sort override unavailable")
	return snapshot, err
}

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.14
func TestAutoSortMergeUnavailableDoesNotHideAutoMerge(t *testing.T) {
	base := NewMemoryRepository()
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, base, identity)
	service := NewService(&failingSortSnapshotRepository{base}, 10, logger.Default())
	status, err := service.Snapshot(context.Background(), identity)
	require.NoError(t, err)
	require.True(t, status.AutoMergeAvailable)
	require.Equal(t, new(true), status.AutoMergeEnabled)
	require.False(t, status.AutoSortMergeAvailable)
	require.Nil(t, status.AutoSortMergeEnabled)
	require.Empty(t, status.AutoSortMergeSource)
	require.Nil(t, status.AutoSortMergeRevision)
}

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.15
func TestAutoSortMergeDelayedLoaderFailureKeepsEarlierGeneration(t *testing.T) {
	repo := NewMemoryRepository()
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, repo, identity)
	service := NewService(repo, 10, logger.Default())
	captured, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	service.SetGlobalQueuePolicyLoader(func(context.Context) (GlobalQueuePolicy, error) {
		if calls.Add(1) == 1 {
			close(captured)
			<-release
			return GlobalQueuePolicy{}, errors.New("settings unavailable")
		}
		return GlobalQueuePolicy{AutoMerge: AutoMergePolicy{Enabled: true}, AutoSortMerge: AutoSortMergePolicy{Enabled: true, Revision: 2}}, nil
	})
	done := make(chan *QueueStatus, 1)
	go func() {
		status, err := service.Snapshot(context.Background(), identity)
		if err != nil {
			done <- nil
			return
		}
		done <- status
	}()
	<-captured
	newer, err := service.Snapshot(context.Background(), identity)
	close(release)
	older := <-done
	require.NoError(t, err)
	require.NotNil(t, older)
	require.Less(t, older.StatusGeneration, newer.StatusGeneration)
	require.False(t, older.AutoSortMergeAvailable)
	require.Equal(t, new(true), newer.AutoSortMergeEnabled)
}
