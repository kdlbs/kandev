package messagequeue

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.15
func TestAutoMergeDelayedLoaderFailureKeepsEarlierGeneration(t *testing.T) {
	repo := NewMemoryRepository()
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, repo, identity)
	service := NewService(repo, 10, logger.Default())
	captured, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	service.SetGlobalAutoMergePolicyLoader(func(context.Context) (AutoMergePolicy, error) {
		if calls.Add(1) == 1 {
			close(captured)
			<-release
			return AutoMergePolicy{}, errors.New("settings unavailable")
		}
		return AutoMergePolicy{Enabled: true, Revision: 2}, nil
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
	require.False(t, older.AutoMergeAvailable)
	require.Equal(t, new(true), newer.AutoMergeEnabled)
}
