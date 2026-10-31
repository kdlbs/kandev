package backendapp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type usageProjectionStartupStore struct {
	started chan struct{}
	joined  chan struct{}
}

func (s *usageProjectionStartupStore) RunUsageHistoryProjectionBatch(ctx context.Context, _ int) (bool, error) {
	close(s.started)
	defer close(s.joined)
	<-ctx.Done()
	return false, ctx.Err()
}

func TestUsageHistoryProjectionStartupShutdownJoinsWorker(t *testing.T) {
	store := &usageProjectionStartupStore{started: make(chan struct{}), joined: make(chan struct{})}
	stop := startUsageHistoryProjection(context.Background(), store, nil)
	t.Cleanup(func() { require.NoError(t, stop()) })
	<-store.started
	require.NoError(t, quiesceForRestore(nil, nil, nil, nil, []func() error{stop}))
	select {
	case <-store.joined:
	default:
		t.Fatal("database restore cleanup returned while a projection writer was still running")
	}
	require.NoError(t, stop())
}
