package linear

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// defaultAuthPollInterval is how often the auth-health poller probes each
// configured workspace.
const defaultAuthPollInterval = 90 * time.Second

// Poller probes the stored Linear credentials of every configured workspace on
// a fixed cadence and persists the result on the LinearConfig row.
type Poller struct {
	service  *Service
	logger   *logger.Logger
	interval time.Duration

	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
}

// NewPoller returns a poller that uses the default 90s cadence.
func NewPoller(svc *Service, log *logger.Logger) *Poller {
	return &Poller{service: svc, logger: log, interval: defaultAuthPollInterval}
}

// Start launches the background loop. Calling Start more than once without
// Stop is a no-op.
func (p *Poller) Start(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started || p.service == nil {
		return
	}
	p.started = true
	ctx, p.cancel = context.WithCancel(ctx)
	p.wg.Add(1)
	go p.loop(ctx)
	p.logger.Info("Linear auth poller started")
}

// Stop cancels the loop and waits for it to drain.
func (p *Poller) Stop() {
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
	p.logger.Info("Linear auth poller stopped")
}

func (p *Poller) loop(ctx context.Context) {
	defer p.wg.Done()
	p.probeAll(ctx)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.probeAll(ctx)
		}
	}
}

func (p *Poller) probeAll(ctx context.Context) {
	ids, err := p.service.Store().ListConfiguredWorkspaces(ctx)
	if err != nil {
		p.logger.Warn("linear poller: list workspaces failed", zap.Error(err))
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		p.service.RecordAuthHealth(ctx, id)
	}
}
