package conversationsearch

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type workerStore struct {
	err     error
	mu      sync.Mutex
	calls   []bool
	started chan struct{}
}

func (s *workerStore) ConversationSearchReady(context.Context) bool { return true }
func (s *workerStore) BackfillConversationSearch(context.Context, int) (bool, error) {
	return true, nil
}
func (s *workerStore) ProcessConversationSearchBatch(ctx context.Context, historical bool) (int, error) {
	s.mu.Lock()
	s.calls = append(s.calls, historical)
	n := len(s.calls)
	s.mu.Unlock()
	if n == 4 {
		close(s.started)
		<-ctx.Done()
		return 0, ctx.Err()
	}
	return 1, s.err
}
func TestConversationSearchWorkerFairnessAndStop(t *testing.T) {
	store := &workerStore{started: make(chan struct{})}
	stop := start(t.Context(), store, time.Millisecond, nil)
	select {
	case <-store.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not progress")
	}
	stop()
	stop()
	store.mu.Lock()
	defer store.mu.Unlock()
	require.Equal(t, []bool{false, false, false, true}, store.calls)
}

func TestConversationSearchErrorLogBudget(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	reporter := indexErrorReporter{log: log}
	now := time.Now()
	failure := errors.New("private conversation content")
	reporter.report(t.Context(), nil, "live_batch_failed", now)
	require.Zero(t, logs.Len())
	reporter.report(t.Context(), failure, "live_batch_failed", now)
	reporter.report(t.Context(), failure, "backfill_batch_failed", now.Add(59*time.Second))
	require.Equal(t, 1, logs.Len())
	reporter.report(t.Context(), failure, "backfill_enqueue_failed", now.Add(time.Minute))
	require.Equal(t, 2, logs.Len())
	require.Equal(t, map[string]any{"reason": "live_batch_failed"}, logs.All()[0].ContextMap())
	require.NotContains(t, logs.All()[0].Message, failure.Error())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	reporter.report(ctx, failure, "live_batch_failed", now.Add(2*time.Minute))
	require.Equal(t, 2, logs.Len())
}

func TestConversationSearchWorkerReportsFailure(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	store := &workerStore{started: make(chan struct{}), err: errors.New("private database details")}
	stop := start(t.Context(), store, time.Millisecond, log)
	defer stop()
	select {
	case <-store.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not retry after a database error")
	}
	stop()
	require.Equal(t, 1, logs.Len())
	require.Equal(t, map[string]any{"reason": "live_batch_failed"}, logs.All()[0].ContextMap())
}
