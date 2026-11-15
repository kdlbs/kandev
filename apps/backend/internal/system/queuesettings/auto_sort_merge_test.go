package queuesettings

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.1
func TestAutoSortMergeSettingsPersistenceAndRevision(t *testing.T) {
	raw := &fakeRawStore{raw: []byte(`{"max_per_session":6}`), found: true}
	service := NewService(NewStore(raw), &fakeTarget{}, func() Environment { return Environment{} }, testLogger(t))
	ctx := context.Background()
	response, err := service.Get(ctx)
	require.NoError(t, err)
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	var decoded map[string]map[string]any
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, false, decoded["settings"]["auto_sort_merge_enabled"], "legacy settings default sort OFF")
	for _, value := range []bool{true, true, false} {
		var patch SettingsPatch
		data, marshalErr := json.Marshal(map[string]bool{"auto_sort_merge_enabled": value})
		require.NoError(t, marshalErr)
		require.NoError(t, json.Unmarshal(data, &patch))
		_, err = service.Update(ctx, patch)
		require.NoError(t, err)
		var stored map[string]any
		require.NoError(t, json.Unmarshal(raw.raw, &stored))
		require.Equal(t, value, stored["auto_sort_merge_enabled"])
		wantRevision := float64(1)
		if !value {
			wantRevision = 2
		}
		require.Equal(t, wantRevision, stored["auto_sort_merge_revision"])
	}
}

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.13
func TestAutoSortMergeCombinedLoaderKeepsOneSettingsEnvelope(t *testing.T) {
	raw := &compareAndSwapRawStore{}
	writer := NewService(NewStore(raw), &fakeTarget{}, func() Environment { return Environment{} }, testLogger(t))
	target := &policyLoaderTarget{}
	reader := &pausedSortSettingsStore{compareAndSwapRawStore: raw, captured: make(chan struct{}), release: make(chan struct{})}
	_ = NewService(NewStore(reader), target, func() Environment { return Environment{} }, testLogger(t))
	done := make(chan messagequeue.GlobalQueuePolicy, 1)
	go func() {
		policy, err := target.load(context.Background())
		if err != nil {
			done <- messagequeue.GlobalQueuePolicy{}
			return
		}
		done <- policy
	}()
	<-reader.captured
	_, err := writer.Update(context.Background(), SettingsPatch{AutoMergeEnabled: new(false), AutoSortMergeEnabled: new(true)})
	close(reader.release)
	old := <-done
	require.True(t, old.AutoMerge.Enabled)
	require.False(t, old.AutoSortMerge.Enabled)
	require.NoError(t, err)
	policy, err := target.load(context.Background())
	require.NoError(t, err)
	require.False(t, policy.AutoMerge.Enabled)
	require.Equal(t, int64(1), policy.AutoMerge.Revision)
	require.True(t, policy.AutoSortMerge.Enabled)
	require.Equal(t, int64(1), policy.AutoSortMerge.Revision)
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
