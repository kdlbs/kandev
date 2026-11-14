package service

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type projectionBlockingStore struct {
	started chan struct{}
	calls   atomic.Int64
}

func (s *projectionBlockingStore) RunUsageHistoryProjectionBatch(ctx context.Context, _ int) (bool, error) {
	s.calls.Add(1)
	s.started <- struct{}{}
	<-ctx.Done()
	return false, ctx.Err()
}

func TestUsageHistoryProjectionWorkerStopsAndRestarts(t *testing.T) {
	store := &projectionBlockingStore{started: make(chan struct{}, 2)}
	worker := NewUsageHistoryWorker(store, nil)
	t.Cleanup(func() { require.NoError(t, worker.Stop()) })
	worker.Start(context.Background())
	worker.Start(context.Background())
	<-store.started
	require.NoError(t, worker.Stop())
	require.EqualValues(t, 1, store.calls.Load(), "Start must be idempotent and Stop must join the admitted operation")
	worker.Start(context.Background())
	<-store.started
	require.NoError(t, worker.Stop())
	require.EqualValues(t, 2, store.calls.Load())
}
