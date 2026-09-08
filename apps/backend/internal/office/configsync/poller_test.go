package configsync

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/github"
)

func TestPoller_StartStopIdempotent(t *testing.T) {
	svc, _, _ := newTestService(t)
	p := NewPoller(svc, svc.logger)

	p.Start(context.Background())
	t.Cleanup(p.Stop)
	p.Start(context.Background()) // second start is a no-op
	p.Stop()
	p.Stop() // second stop is a no-op
}

func TestPoller_SyncsDueConfigsOnTick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, _, fg := newTestService(t)
		ctx := context.Background()
		_, err := svc.SetConfigForWorkspace(ctx, "ws-1", testSetConfigRequest("cfg"))
		require.NoError(t, err)
		fg.dirs["cfg"] = []github.RepoContentEntry{}

		p := NewPoller(svc, svc.logger)
		p.Start(context.Background())
		t.Cleanup(p.Stop)

		// No sync before the first tick: the loop waits a full interval so
		// boot doesn't hammer the configured provider.
		synctest.Wait()
		cfg, err := svc.GetConfigForWorkspace(ctx, "ws-1")
		require.NoError(t, err)
		assert.Nil(t, cfg.LastSyncedAt)

		time.Sleep(PollInterval + time.Second)
		synctest.Wait()
		p.Stop()

		cfg, err = svc.GetConfigForWorkspace(ctx, "ws-1")
		require.NoError(t, err)
		assert.True(t, cfg.LastOk)
		assert.NotNil(t, cfg.LastSyncedAt)
	})
}
