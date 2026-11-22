package promptsync

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// PollInterval is the outer tick on which the poller checks whether the
// configuration is due. The first check happens one full tick after Start.
const PollInterval = 60 * time.Second

// Poller runs automatic prompt syncs. It owns one goroutine, started by Start
// and drained by Stop.
type Poller struct {
	svc      *Service
	logger   *logger.Logger
	interval time.Duration

	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
}

// NewPoller creates a poller for svc.
func NewPoller(svc *Service, log *logger.Logger) *Poller {
	return &Poller{
		svc:      svc,
		logger:   log.WithFields(zap.String("component", "promptsync-poller")),
		interval: PollInterval,
	}
}

// Start begins polling. Calling Start on a running poller is a no-op.
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

// Stop cancels polling and waits for an in-flight sync to finish.
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

func (p *Poller) loop(ctx context.Context) {
	defer p.wg.Done()
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.svc.SyncIfDue(ctx)
		}
	}
}
