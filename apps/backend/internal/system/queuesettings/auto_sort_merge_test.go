package queuesettings

import (
	"context"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.13
func TestAutoMergeLoaderKeepsOneSettingsEnvelope(t *testing.T) {
	raw := &compareAndSwapRawStore{}
	writer := NewService(NewStore(raw), &fakeTarget{}, func() Environment { return Environment{} }, testLogger(t))
	target := &policyLoaderTarget{}
	reader := &pausedSortSettingsStore{compareAndSwapRawStore: raw, captured: make(chan struct{}), release: make(chan struct{})}
	_ = NewService(NewStore(reader), target, func() Environment { return Environment{} }, testLogger(t))
	done := make(chan messagequeue.AutoMergePolicy, 1)
	go func() {
		policy, err := target.load(context.Background())
		if err != nil {
			done <- messagequeue.AutoMergePolicy{}
			return
		}
		done <- policy
	}()
	<-reader.captured
	_, err := writer.Update(context.Background(), SettingsPatch{AutoMergeEnabled: new(false)})
	close(reader.release)
	old := <-done
	require.True(t, old.Enabled)
	require.NoError(t, err)
	policy, err := target.load(context.Background())
	require.NoError(t, err)
	require.False(t, policy.Enabled)
	require.Equal(t, int64(1), policy.Revision)
}

type pausedSortSettingsStore struct {
	*compareAndSwapRawStore
	captured chan struct{}
	release  chan struct{}
}

func (s *pausedSortSettingsStore) GetConsistent(ctx context.Context, key string) ([]byte, bool, error) {
	raw, found, err := s.compareAndSwapRawStore.GetConsistent(ctx, key)
	select {
	case <-s.captured:
	default:
		close(s.captured)
		<-s.release
	}
	return raw, found, err
}
