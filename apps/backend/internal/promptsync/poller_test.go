package promptsync

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
)

func startTestPoller(t *testing.T, f *syncFixture, interval time.Duration) *Poller {
	t.Helper()
	f.svc.now = time.Now
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	poller := NewPoller(f.svc, log)
	poller.interval = interval
	poller.Start(context.Background())
	t.Cleanup(poller.Stop)
	return poller
}

func lastSynced(t *testing.T, f *syncFixture) *time.Time {
	t.Helper()
	cfg, err := f.svc.GetConfig(context.Background())
	require.NoError(t, err)
	return cfg.LastSyncedAt
}

func TestPollerDoesNotSyncBeforeTheFirstTick(t *testing.T) {
	f := newSyncFixture(t)
	poller := startTestPoller(t, f, time.Hour)
	poller.Start(context.Background()) // idempotent
	time.Sleep(50 * time.Millisecond)
	assert.Nil(t, lastSynced(t, f), "the first check waits one full tick")
	poller.Stop()
	poller.Stop() // idempotent
}

func TestPollerSyncsOnTick(t *testing.T) {
	f := newSyncFixture(t)
	startTestPoller(t, f, 20*time.Millisecond)
	assert.Eventually(t, func() bool { return lastSynced(t, f) != nil }, 2*time.Second, 10*time.Millisecond)
}

func TestRunExclusiveWaitsForTheServiceLock(t *testing.T) {
	f := newSyncFixture(t)
	f.svc.mu.Lock()
	var mu sync.Mutex
	ran := false
	done := make(chan struct{})
	go func() {
		_ = f.svc.RunExclusive(func() error {
			mu.Lock()
			ran = true
			mu.Unlock()
			return nil
		})
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	assert.False(t, ran, "must not run while a sync holds the lock")
	mu.Unlock()
	f.svc.mu.Unlock()
	<-done
	assert.True(t, ran)
}
