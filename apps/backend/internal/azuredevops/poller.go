package azuredevops

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/integrations/healthpoll"
)

const watcherPollTick = time.Minute

// Poller combines the shared auth-health poller with Azure watcher checks.
type Poller struct {
	service *Service
	logger  *logger.Logger
	auth    *healthpoll.Poller

	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
}

func NewPoller(service *Service, log *logger.Logger) *Poller {
	if service == nil {
		return nil
	}
	if log == nil {
		log = logger.Default()
	}
	return &Poller{service: service, logger: log, auth: healthpoll.New("azure devops", authProber{service: service}, log)}
}

func (p *Poller) Start(ctx context.Context) {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return
	}
	p.started = true
	watchCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.mu.Unlock()
	p.auth.Start(watchCtx)
	p.wg.Add(1)
	go p.watchLoop(watchCtx)
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
	p.started = false
	p.cancel = nil
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	p.auth.Stop()
	p.wg.Wait()
}

func (p *Poller) watchLoop(ctx context.Context) {
	defer p.wg.Done()
	p.runWatchChecks(ctx)
	ticker := time.NewTicker(watcherPollTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.runWatchChecks(ctx)
		}
	}
}

func (p *Poller) runWatchChecks(ctx context.Context) int {
	matched := 0
	now := time.Now().UTC()
	workWatches, err := p.service.store.ListEnabledWorkItemWatches(ctx)
	if err != nil {
		p.logger.Warn("azure devops watcher poll: list work-item watches", zap.Error(err))
	} else {
		for _, watch := range workWatches {
			if !watchDue(watch.LastPolledAt, watch.PollIntervalSeconds, now) {
				continue
			}
			result, checkErr := p.service.TriggerWorkItemWatch(ctx, watch.WorkspaceID, watch.ID)
			if checkErr != nil {
				p.logger.Warn("azure devops watcher poll: work-item check failed", zap.String("watch_id", watch.ID), zap.Error(checkErr))
				continue
			}
			matched += result.Matched
			if cleaned := p.service.cleanupWorkItemWatchForPoll(ctx, watch.ID); cleaned > 0 {
				p.logger.Info("azure devops watcher poll: cleaned terminal work-item tasks", zap.String("watch_id", watch.ID), zap.Int("tasks", cleaned))
			}
		}
	}
	prWatches, err := p.service.store.ListEnabledPullRequestWatches(ctx)
	if err != nil {
		p.logger.Warn("azure devops watcher poll: list pull-request watches", zap.Error(err))
	} else {
		for _, watch := range prWatches {
			if !watchDue(watch.LastPolledAt, watch.PollIntervalSeconds, now) {
				continue
			}
			result, checkErr := p.service.TriggerPullRequestWatch(ctx, watch.WorkspaceID, watch.ID)
			if checkErr != nil {
				p.logger.Warn("azure devops watcher poll: pull-request check failed", zap.String("watch_id", watch.ID), zap.Error(checkErr))
				continue
			}
			matched += result.Matched
			if cleaned := p.service.cleanupPullRequestWatchForPoll(ctx, watch.ID); cleaned > 0 {
				p.logger.Info("azure devops watcher poll: cleaned terminal pull-request tasks", zap.String("watch_id", watch.ID), zap.Int("tasks", cleaned))
			}
		}
	}
	return matched
}

func (s *Service) cleanupWorkItemWatchForPoll(ctx context.Context, watchID string) int {
	watch, err := s.store.GetWorkItemWatch(ctx, watchID)
	if err != nil || watch == nil || !watch.Enabled || watch.Deleting {
		return 0
	}
	client, err := s.clientForWorkspace(ctx, watch.WorkspaceID)
	if err != nil {
		return 0
	}
	return s.cleanupWorkItemWatchWithClient(ctx, watch, client)
}

func (s *Service) cleanupPullRequestWatchForPoll(ctx context.Context, watchID string) int {
	watch, err := s.store.GetPullRequestWatch(ctx, watchID)
	if err != nil || watch == nil || !watch.Enabled || watch.Deleting {
		return 0
	}
	client, err := s.clientForWorkspace(ctx, watch.WorkspaceID)
	if err != nil {
		return 0
	}
	return s.cleanupPullRequestWatchWithClient(ctx, watch, client)
}

func watchDue(lastPolledAt *time.Time, intervalSeconds int, now time.Time) bool {
	if lastPolledAt == nil {
		return true
	}
	interval := time.Duration(normalizePollInterval(intervalSeconds)) * time.Second
	return !lastPolledAt.Add(interval).After(now)
}

type authProber struct {
	service *Service
}

func (p authProber) HasConfig(ctx context.Context) (bool, error) {
	workspaceIDs, err := p.service.store.ListConfigWorkspaceIDs(ctx)
	return len(workspaceIDs) > 0, err
}

func (p authProber) RecordAuthHealth(ctx context.Context) {
	p.service.RecordAuthHealth(ctx)
}
