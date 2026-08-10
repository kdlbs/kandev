package forgejo

import (
	"context"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

const defaultIssueWatchPollInterval = time.Minute

// Poller runs configured issue watches without requiring an HTTP request.
type Poller struct {
	service *Service
	logger  *logger.Logger

	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
}

func NewPoller(service *Service, log *logger.Logger) *Poller {
	if service == nil {
		return nil
	}
	return &Poller{service: service, logger: log}
}

func (p *Poller) Start(ctx context.Context) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return
	}
	ctx, p.cancel = context.WithCancel(ctx)
	p.started = true
	p.wg.Add(1)
	go p.loop(ctx)
}

func (p *Poller) Stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return
	}
	cancel := p.cancel
	p.mu.Unlock()
	cancel()
	p.wg.Wait()
	p.mu.Lock()
	p.started = false
	p.cancel = nil
	p.mu.Unlock()
}

func (p *Poller) loop(ctx context.Context) {
	defer p.wg.Done()
	ticker := time.NewTicker(defaultIssueWatchPollInterval)
	defer ticker.Stop()
	p.runIssueWatches(ctx, time.Now().UTC())
	p.runReviewWatches(ctx, time.Now().UTC())
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			p.runIssueWatches(ctx, now.UTC())
			p.runReviewWatches(ctx, now.UTC())
		}
	}
}

func (p *Poller) runReviewWatches(ctx context.Context, now time.Time) {
	watches, err := p.service.ListAllReviewWatches(ctx)
	if err != nil {
		p.logger.Warn("Forgejo poller: list review watches", zap.Error(err))
		return
	}
	for _, watch := range watches {
		if ctx.Err() != nil {
			return
		}
		if !watch.Enabled || !shouldPollIssueWatch(watch.LastPolledAt, watch.PollIntervalSeconds, now) {
			continue
		}
		if _, err := p.service.PollReviewWatch(ctx, watch); err != nil {
			p.logger.Debug("Forgejo poller: poll review watch", zap.String("watch_id", watch.ID), zap.Error(err))
		}
	}
}

func (p *Poller) runIssueWatches(ctx context.Context, now time.Time) {
	watches, err := p.service.ListAllIssueWatches(ctx)
	if err != nil {
		p.logger.Warn("Forgejo poller: list issue watches", zap.Error(err))
		return
	}
	for _, watch := range watches {
		if ctx.Err() != nil {
			return
		}
		if !watch.Enabled || !shouldPollIssueWatch(watch.LastPolledAt, watch.PollIntervalSeconds, now) {
			continue
		}
		if _, err := p.service.PollIssueWatch(ctx, watch.WorkspaceID, watch.ID); err != nil {
			p.logger.Debug("Forgejo poller: poll issue watch", zap.String("watch_id", watch.ID), zap.Error(err))
		}
	}
}

func shouldPollIssueWatch(last *time.Time, intervalSeconds int, now time.Time) bool {
	if last == nil {
		return true
	}
	if intervalSeconds <= 0 {
		intervalSeconds = int(defaultIssueWatchPollInterval.Seconds())
	}
	return now.Sub(*last) >= time.Duration(intervalSeconds)*time.Second
}
