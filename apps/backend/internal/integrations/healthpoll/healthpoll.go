// Package healthpoll provides a reusable background loop that probes the
// stored credentials of every configured workspace for an integration on a
// fixed cadence.
//
// Integrations supply a Prober (list workspaces + record auth health for one)
// and the package owns the Start/Stop lifecycle, the immediate-probe-on-Start
// convention, and the ticker. The default cadence is 90s — short enough to
// keep session-cookie auth warm and to surface expirations promptly in the UI,
// long enough that we don't hammer the upstream API when many workspaces are
// configured.
package healthpoll

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// DefaultInterval is the cadence used by Poller when no Interval is set.
const DefaultInterval = 90 * time.Second

// Prober is the integration-specific behaviour the loop drives. Both methods
// must be safe to call concurrently with themselves and each other; the loop
// itself is single-threaded but Start may be invoked while a previous Stop is
// still draining.
type Prober interface {
	// ListConfiguredWorkspaces returns the IDs of every workspace that has
	// credentials stored for this integration.
	ListConfiguredWorkspaces(ctx context.Context) ([]string, error)

	// RecordAuthHealth probes one workspace's credentials and persists the
	// result on the integration's config row. Errors are intentionally not
	// returned: this is a best-effort health signal, never the source of
	// truth for callers, so the implementation handles its own logging.
	RecordAuthHealth(ctx context.Context, workspaceID string)
}

// Poller drives Prober.RecordAuthHealth on a fixed cadence.
type Poller struct {
	prober   Prober
	logger   *logger.Logger
	name     string // appears in log lines, e.g. "jira", "linear"
	interval time.Duration

	// mu guards started/cancel/wg against concurrent Start/Stop calls.
	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
}

// New returns a poller using the default 90s cadence.
func New(name string, prober Prober, log *logger.Logger) *Poller {
	return &Poller{prober: prober, logger: log, name: name, interval: DefaultInterval}
}

// SetInterval overrides the default cadence. Must be called before Start.
func (p *Poller) SetInterval(d time.Duration) {
	p.mu.Lock()
	p.interval = d
	p.mu.Unlock()
}

// Start launches the background loop. Calling Start more than once without
// Stop is a no-op.
func (p *Poller) Start(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started || p.prober == nil {
		return
	}
	p.started = true
	ctx, p.cancel = context.WithCancel(ctx)
	p.wg.Add(1)
	go p.loop(ctx)
	p.logger.Info(p.name + " auth poller started")
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
	p.logger.Info(p.name + " auth poller stopped")
}

func (p *Poller) loop(ctx context.Context) {
	defer p.wg.Done()
	// Run an initial probe immediately so the UI gets a status without
	// waiting the full interval after backend startup.
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

// probeAll runs one probe pass across every configured workspace. Used by the
// loop and by in-package tests that want to drive a probe deterministically.
func (p *Poller) probeAll(ctx context.Context) {
	ids, err := p.prober.ListConfiguredWorkspaces(ctx)
	if err != nil {
		p.logger.Warn(p.name+" poller: list workspaces failed", zap.Error(err))
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		p.prober.RecordAuthHealth(ctx, id)
	}
}
