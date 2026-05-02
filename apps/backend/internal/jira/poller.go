package jira

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/integrations/healthpoll"
)

// defaultIssuePollTickInterval is how often the issue-watch loop wakes up to
// look at every enabled watcher. The actual JQL is only re-run for a watcher
// when its per-watch `PollIntervalSeconds` has elapsed since `LastPolledAt`,
// so this is the *minimum* granularity, not the *actual* cadence. 60 seconds
// matches the smallest interval the UI accepts; the gating check then rate-
// limits each individual watcher.
const defaultIssuePollTickInterval = 60 * time.Second

// Poller drives two background loops sharing a single Service:
//   - auth health: probes stored credentials so the UI can show connect status.
//     Delegated to internal/integrations/healthpoll for the loop semantics.
//   - issue watches: runs each enabled watcher's JQL and emits NewJiraIssueEvent
//     for every matching ticket the orchestrator hasn't yet seen. Jira-specific
//     and stays local.
//
// Both loops are cancelled together via Stop.
type Poller struct {
	service       *Service
	logger        *logger.Logger
	auth          *healthpoll.Poller
	issueInterval time.Duration
	issueTickHook func() // tests use this to observe each issue-watch tick.

	// mu guards started/cancel/wg against concurrent Start/Stop calls.
	mu              sync.Mutex
	cancelIssueLoop context.CancelFunc
	wg              sync.WaitGroup
	started         bool
}

// NewPoller returns a poller using the default cadences.
func NewPoller(svc *Service, log *logger.Logger) *Poller {
	if svc == nil {
		return nil
	}
	return &Poller{
		service:       svc,
		logger:        log,
		auth:          healthpoll.New("jira", svcProber{svc}, log),
		issueInterval: defaultIssuePollTickInterval,
	}
}

// SetIssueTickHook installs a callback fired at the end of each issue-watch
// tick. Production code never sets this; tests use it to wait for a tick
// without sleep-polling.
func (p *Poller) SetIssueTickHook(fn func()) {
	p.mu.Lock()
	p.issueTickHook = fn
	p.mu.Unlock()
}

// Start launches both background loops. Calling Start more than once without
// Stop is a no-op. A nil receiver is also a no-op so callers don't have to
// nil-check around `NewPoller(nil, log).Start(ctx)` patterns.
func (p *Poller) Start(ctx context.Context) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started || p.service == nil {
		return
	}
	p.started = true
	issueCtx, cancel := context.WithCancel(ctx)
	p.cancelIssueLoop = cancel
	p.auth.Start(ctx)
	p.wg.Add(1)
	go p.issueWatchLoop(issueCtx)
}

// Stop cancels both loops and waits for them to drain. A nil receiver is a
// no-op so callers using the `defer p.Stop()` pattern alongside a possibly-
// nil `NewPoller(svc, log)` don't have to nil-check.
func (p *Poller) Stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return
	}
	cancel := p.cancelIssueLoop
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	p.auth.Stop()
	p.wg.Wait()
	p.mu.Lock()
	p.started = false
	p.mu.Unlock()
}

// issueWatchLoop drives the periodic JQL-poll → publish-event flow. Unlike
// the auth loop, this one waits a full interval before its first tick so the
// backend doesn't hammer JIRA the moment it starts.
func (p *Poller) issueWatchLoop(ctx context.Context) {
	defer p.wg.Done()
	ticker := time.NewTicker(p.issueInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.checkIssueWatches(ctx)
			p.fireIssueTickHook()
		}
	}
}

func (p *Poller) checkIssueWatches(ctx context.Context) {
	watches, err := p.service.Store().ListEnabledIssueWatches(ctx)
	if err != nil {
		p.logger.Warn("jira poller: list enabled issue watches failed", zap.Error(err))
		return
	}
	if len(watches) == 0 {
		return
	}
	for _, w := range watches {
		if ctx.Err() != nil {
			return
		}
		if !isIssueWatchDue(w, time.Now()) {
			continue
		}
		newTickets, err := p.service.CheckIssueWatch(ctx, w)
		if err != nil {
			p.logger.Debug("jira poller: check issue watch failed",
				zap.String("watch_id", w.ID), zap.Error(err))
			continue
		}
		for _, t := range newTickets {
			p.logger.Info("new jira issue found for watch",
				zap.String("watch_id", w.ID),
				zap.String("issue_key", t.Key),
				zap.String("summary", t.Summary))
			p.service.publishNewJiraIssueEvent(ctx, w, t)
		}
	}
}

// isIssueWatchDue reports whether enough time has passed since the watch was
// last polled to re-run its JQL on this tick. A watch with no LastPolledAt
// (never polled, or DB row freshly created) is always due. PollIntervalSeconds
// <= 0 falls back to the default — the same normalisation the store applies on
// write — so a corrupt row never blocks polling forever.
func isIssueWatchDue(w *IssueWatch, now time.Time) bool {
	if w.LastPolledAt == nil {
		return true
	}
	interval := w.PollIntervalSeconds
	if interval <= 0 {
		interval = DefaultIssueWatchPollInterval
	}
	return now.Sub(*w.LastPolledAt) >= time.Duration(interval)*time.Second
}

func (p *Poller) fireIssueTickHook() {
	p.mu.Lock()
	hook := p.issueTickHook
	p.mu.Unlock()
	if hook != nil {
		hook()
	}
}

// svcProber adapts *Service to healthpoll.Prober without leaking the shared
// interface into Service's public API.
type svcProber struct{ svc *Service }

func (s svcProber) HasConfig(ctx context.Context) (bool, error) {
	return s.svc.Store().HasConfig(ctx)
}

func (s svcProber) RecordAuthHealth(ctx context.Context) {
	s.svc.RecordAuthHealth(ctx)
}
