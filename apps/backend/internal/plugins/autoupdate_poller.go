package plugins

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// AutoUpdateInterval is the cadence at which the auto-update poller re-checks
// the marketplace for newer versions of opted-in plugins. Plugin releases are
// infrequent, so this sits well above the integration health cadence (90s);
// 90 minutes is responsive enough to pick up a release within a couple of
// hours without hammering the marketplace sources.
const AutoUpdateInterval = 90 * time.Minute

// AutoUpdatePoller drives Service.RunAutoUpdatePass on a fixed cadence,
// following the goroutine-ownership shape the repo standardizes on
// (WaitGroup + context cancel + idempotent Start/Stop, mirroring
// internal/integrations/healthpoll). Unlike the per-workspace integration
// pollers, plugin installs are instance-global, so this poller has no
// per-workspace fan-out — it runs a single sweep per tick.
type AutoUpdatePoller struct {
	svc      *Service
	log      *logger.Logger
	interval time.Duration

	// mu guards started/cancel/wg against concurrent Start/Stop calls.
	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
}

// NewAutoUpdatePoller returns a poller using the default 90-minute cadence.
func NewAutoUpdatePoller(svc *Service, log *logger.Logger) *AutoUpdatePoller {
	return &AutoUpdatePoller{svc: svc, log: log, interval: AutoUpdateInterval}
}

// Start launches the background loop. Calling Start more than once without an
// intervening Stop is a no-op.
func (p *AutoUpdatePoller) Start(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started || p.svc == nil {
		return
	}
	p.started = true
	ctx, p.cancel = context.WithCancel(ctx)
	p.wg.Add(1)
	go p.loop(ctx)
	p.log.Info("plugin auto-update poller started")
}

// Stop cancels the loop and waits for it to drain. Idempotent.
func (p *AutoUpdatePoller) Stop() {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return
	}
	cancel := p.cancel
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	p.wg.Wait()
	p.mu.Lock()
	p.started = false
	p.mu.Unlock()
	p.log.Info("plugin auto-update poller stopped")
}

func (p *AutoUpdatePoller) loop(ctx context.Context) {
	defer p.wg.Done()
	// Run one sweep shortly after start so updates that landed while the
	// backend was down are applied without waiting a full interval.
	p.runOnce(ctx)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.runOnce(ctx)
		}
	}
}

func (p *AutoUpdatePoller) runOnce(ctx context.Context) {
	outcome, err := p.svc.RunAutoUpdatePass(ctx)
	if err != nil {
		p.log.Warn("plugin auto-update pass failed", zap.Error(err))
		return
	}
	if len(outcome.Updated) > 0 || len(outcome.Failed) > 0 {
		p.log.Info("plugin auto-update pass complete",
			zap.Int("updated", len(outcome.Updated)),
			zap.Int("failed", len(outcome.Failed)))
	}
}
