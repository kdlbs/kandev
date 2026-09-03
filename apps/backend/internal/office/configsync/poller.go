package configsync

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// PollInterval is the outer ticker cadence. Each tick only syncs workspaces
// whose own interval_seconds has elapsed (see Service.SyncDueConfigs).
const PollInterval = 60 * time.Second

// Poller periodically syncs config sync definitions for every configured
// workspace. Owns a single goroutine with Start/Stop lifecycle semantics,
// mirroring workflowsync.Poller (own goroutine, own lock map on the wrapped
// Service, no shared code).
type Poller struct {
	svc      *Service
	logger   *logger.Logger
	interval time.Duration

	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
}

// NewPoller creates a config sync poller at the default interval.
func NewPoller(svc *Service, log *logger.Logger) *Poller {
	return &Poller{
		svc:      svc,
		logger:   log.WithFields(zap.String("component", "office-configsync-poller")),
		interval: PollInterval,
	}
}

// Start launches the polling loop. Idempotent.
func (p *Poller) Start(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return
	}
	p.started = true
	ctx, p.cancel = context.WithCancel(ctx)
	p.wg.Add(1)
	go p.loop(ctx)
}

// Stop cancels the polling loop and waits for it to drain. Idempotent. The
// mutex is held through the wait so a concurrent Start cannot register a new
// loop on the WaitGroup mid-shutdown (the loop itself never takes the mutex,
// so holding it here cannot deadlock).
func (p *Poller) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.started {
		return
	}
	p.started = false
	p.cancel()
	p.wg.Wait()
}

// loop waits a full interval before the first sync so boot doesn't hammer
// the configured provider, then syncs due configs on every tick.
func (p *Poller) loop(ctx context.Context) {
	defer p.wg.Done()
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.svc.SyncDueConfigs(ctx)
		}
	}
}
