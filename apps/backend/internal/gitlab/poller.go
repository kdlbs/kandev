package gitlab

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/integrations/healthpoll"
)

const (
	defaultMRPollInterval     = 1 * time.Minute
	defaultReviewPollInterval = 5 * time.Minute
	defaultIssuePollInterval  = 5 * time.Minute
)

// Poller runs background loops for MR monitoring + review/issue queue checking.
type Poller struct {
	service  *Service
	eventBus bus.EventBus
	logger   *logger.Logger
	auth     *healthpoll.Poller

	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
}

// NewPoller creates a new background poller.
func NewPoller(svc *Service, eventBus bus.EventBus, log *logger.Logger) *Poller {
	if svc == nil {
		return nil
	}
	return &Poller{
		service: svc, eventBus: eventBus, logger: log,
		auth: healthpoll.New("gitlab", gitLabHealthProber{service: svc}, log),
	}
}

// Start kicks off the polling loops. Repeated Start calls are no-ops.
// Safe to call concurrently with Stop.
func (p *Poller) Start(ctx context.Context) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return
	}
	p.started = true
	ctx, p.cancel = context.WithCancel(ctx)
	p.auth.Start(ctx)

	p.wg.Add(3)
	go p.mrMonitorLoop(ctx)
	go p.reviewWatchLoop(ctx)
	go p.issueWatchLoop(ctx)
}

// Stop cancels all loops and waits for them to drain. Safe to call
// concurrently with Start.
func (p *Poller) Stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if !p.started || p.cancel == nil {
		p.mu.Unlock()
		return
	}
	cancel := p.cancel
	p.mu.Unlock()
	cancel()
	p.auth.Stop()
	p.wg.Wait()
	p.mu.Lock()
	p.started = false
	p.mu.Unlock()
}

type gitLabHealthProber struct {
	service *Service
}

func (p gitLabHealthProber) HasConfig(ctx context.Context) (bool, error) {
	p.service.mu.RLock()
	store := p.service.store
	p.service.mu.RUnlock()
	if store == nil {
		return false, nil
	}
	ids, err := store.ListConfigWorkspaceIDs(ctx)
	return len(ids) > 0, err
}

func (p gitLabHealthProber) RecordAuthHealth(ctx context.Context) {
	p.service.RecordWorkspaceAuthHealth(ctx)
}

// --- MR watcher loop ---

func (p *Poller) mrMonitorLoop(ctx context.Context) {
	defer p.wg.Done()
	ticker := time.NewTicker(defaultMRPollInterval)
	defer ticker.Stop()

	// Initial run.
	p.runMRMonitor(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.runMRMonitor(ctx)
		}
	}
}

func (p *Poller) runMRMonitor(ctx context.Context) {
	watches, err := p.service.ListActiveMRWatches(ctx)
	if err != nil {
		p.logger.Warn("gitlab poller: list MR watches", zap.Error(err))
		return
	}
	for _, w := range watches {
		if ctx.Err() != nil {
			return
		}
		if _, _, err := p.service.CheckMRWatch(ctx, w); err != nil {
			p.logger.Debug("gitlab poller: check MR watch",
				zap.String("watch_id", w.ID), zap.Error(err))
		}
	}
}

// --- Review watcher loop ---

func (p *Poller) reviewWatchLoop(ctx context.Context) {
	defer p.wg.Done()
	ticker := time.NewTicker(defaultReviewPollInterval)
	defer ticker.Stop()

	p.runReviewWatch(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.runReviewWatch(ctx)
		}
	}
}

func (p *Poller) runReviewWatch(ctx context.Context) {
	watches, err := p.service.ListAllReviewWatches(ctx)
	if err != nil {
		p.logger.Warn("gitlab poller: list review watches", zap.Error(err))
		return
	}
	now := time.Now().UTC()
	for _, w := range watches {
		if ctx.Err() != nil {
			return
		}
		if !w.Enabled || !shouldPollWatch(w.LastPolledAt, w.PollIntervalSeconds, now) {
			continue
		}
		newMRs, err := p.service.CheckReviewWatch(ctx, w)
		if err != nil {
			p.logger.Debug("gitlab poller: review watch check",
				zap.String("watch_id", w.ID), zap.Error(err))
			continue
		}
		for _, mr := range newMRs {
			p.service.publishNewReviewMREvent(ctx, w, mr)
		}
	}
}

// --- Issue watcher loop ---

func (p *Poller) issueWatchLoop(ctx context.Context) {
	defer p.wg.Done()
	ticker := time.NewTicker(defaultIssuePollInterval)
	defer ticker.Stop()

	p.runIssueWatch(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.runIssueWatch(ctx)
		}
	}
}

func (p *Poller) runIssueWatch(ctx context.Context) {
	watches, err := p.service.ListAllIssueWatches(ctx)
	if err != nil {
		p.logger.Warn("gitlab poller: list issue watches", zap.Error(err))
		return
	}
	now := time.Now().UTC()
	for _, w := range watches {
		if ctx.Err() != nil {
			return
		}
		if !w.Enabled || !shouldPollWatch(w.LastPolledAt, w.PollIntervalSeconds, now) {
			continue
		}
		newIssues, err := p.service.CheckIssueWatch(ctx, w)
		if err != nil {
			p.logger.Debug("gitlab poller: issue watch check",
				zap.String("watch_id", w.ID), zap.Error(err))
			continue
		}
		for _, issue := range newIssues {
			p.service.publishNewIssueEvent(ctx, w, issue)
		}
	}
}

func shouldPollWatch(last *time.Time, intervalSec int, now time.Time) bool {
	if last == nil {
		return true
	}
	if intervalSec <= 0 {
		intervalSec = defaultWatchPollIntervalSec
	}
	return now.Sub(*last) >= time.Duration(intervalSec)*time.Second
}
