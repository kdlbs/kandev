package promptsync

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestPollerWaitsOneTickThenSyncsAndStops(t *testing.T) {
	f := newSyncFixture(t)
	f.svc.now = time.Now
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	poller := NewPoller(f.svc, log)
	poller.interval = 20 * time.Millisecond
	poller.Start(context.Background())
	poller.Start(context.Background()) // idempotent
	assert.Zero(t, f.gitlab.lists, "no sync before the first tick")
	assert.Eventually(t, func() bool {
		cfg, err := f.svc.GetConfig(context.Background())
		return err == nil && cfg.LastSyncedAt != nil
	}, 2*time.Second, 10*time.Millisecond)
	poller.Stop()
	poller.Stop() // idempotent
}
