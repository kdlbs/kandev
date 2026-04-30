package jira

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// defaultAuthPollInterval is how often the auth-health poller probes each
// configured workspace. 90s is a compromise: short enough to keep
// session-cookie auth warm (Atlassian idle-times sessions out after a few
// minutes) and to surface expirations promptly in the UI, long enough that we
// don't hammer Atlassian when many workspaces are configured.
const defaultAuthPollInterval = 90 * time.Second

// defaultIssuePollInterval is how often the issue-watch loop runs through every
// enabled watcher. Five minutes mirrors the GitHub issue watcher cadence and
// keeps API usage well below Atlassian's rate limits.
const defaultIssuePollInterval = 5 * time.Minute

// Poller drives two background loops sharing a single Service:
//   - auth health: probes stored credentials so the UI can show connect status.
//   - issue watches: runs each enabled watcher's JQL and emits NewJiraIssueEvent
//     for every matching ticket the orchestrator hasn't yet seen.
//
// Both loops are cancelled together via Stop.
type Poller struct {
	service       *Service
	logger        *logger.Logger
	authInterval  time.Duration
	issueInterval time.Duration
	issueTickHook func() // tests use this to observe each issue-watch tick.

	// mu guards started/cancel/wg against concurrent Start/Stop calls.
	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
}

// NewPoller returns a poller using the default cadences.
func NewPoller(svc *Service, log *logger.Logger) *Poller {
	return &Poller{
		service:       svc,
		logger:        log,
		authInterval:  defaultAuthPollInterval,
		issueInterval: defaultIssuePollInterval,
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
// Stop is a no-op.
func (p *Poller) Start(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started || p.service == nil {
		return
	}
	p.started = true
	ctx, p.cancel = context.WithCancel(ctx)
	p.wg.Add(2)
	go p.loop(ctx)
	go p.issueWatchLoop(ctx)
	p.logger.Info("Jira poller started")
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
	p.logger.Info("Jira poller stopped")
}

func (p *Poller) loop(ctx context.Context) {
	defer p.wg.Done()
	// Run an initial probe immediately so the UI gets a status without waiting
	// the full interval after backend startup.
	p.probeAll(ctx)
	ticker := time.NewTicker(p.authInterval)
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

func (p *Poller) fireIssueTickHook() {
	p.mu.Lock()
	hook := p.issueTickHook
	p.mu.Unlock()
	if hook != nil {
		hook()
	}
}

func (p *Poller) probeAll(ctx context.Context) {
	ids, err := p.service.Store().ListConfiguredWorkspaces(ctx)
	if err != nil {
		p.logger.Warn("jira poller: list workspaces failed", zap.Error(err))
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		p.service.RecordAuthHealth(ctx, id)
	}
}
