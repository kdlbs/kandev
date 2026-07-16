package workflowsync

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPoller_StartStopIdempotent(t *testing.T) {
	svc, _ := setupTestService(t, seededMockClient())
	p := NewPoller(svc, svc.logger)

	p.Start(context.Background())
	t.Cleanup(p.Stop)
	p.Start(context.Background()) // second start is a no-op
	p.Stop()
	p.Stop() // second stop is a no-op
}

func TestPoller_SyncsDueConfigsOnTick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, applier := setupTestService(t, seededMockClient())
		configureWorkspace(t, svc, "ws-1")

		p := NewPoller(svc, svc.logger)
		p.Start(context.Background())
		t.Cleanup(p.Stop)

		// No sync before the first tick: the loop waits a full interval so
		// boot doesn't hammer the GitHub API.
		synctest.Wait()
		assert.Empty(t, applier.calls)

		time.Sleep(PollInterval + time.Second)
		synctest.Wait()
		p.Stop()

		require.Len(t, applier.calls, 1)
		cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
		require.NoError(t, err)
		assert.True(t, cfg.LastOk)
		assert.NotNil(t, cfg.LastSyncedAt)
	})
}
