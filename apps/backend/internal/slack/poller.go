package slack

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/integrations/healthpoll"
)

// Poller probes the stored Slack credentials on the shared 90s auth-health
// cadence, mirroring Linear/Jira. The actual loop lives in
// internal/integrations/healthpoll; this wrapper exists so callers can write
// the familiar `slack.NewPoller(...).Start(ctx)` shape.
type Poller struct {
	inner *healthpoll.Poller
}

// NewPoller returns a poller using the default 90s cadence. Returns nil when
// svc is nil so the caller's nil-check still works.
func NewPoller(svc *Service, log *logger.Logger) *Poller {
	if svc == nil {
		return nil
	}
	return &Poller{inner: healthpoll.New("slack", svcProber{svc}, log)}
}

// Start launches the background loop.
func (p *Poller) Start(ctx context.Context) {
	if p == nil || p.inner == nil {
		return
	}
	p.inner.Start(ctx)
}

// Stop cancels the loop and waits for it to drain.
func (p *Poller) Stop() {
	if p == nil || p.inner == nil {
		return
	}
	p.inner.Stop()
}

// svcProber adapts *Service to healthpoll.Prober.
type svcProber struct{ svc *Service }

func (s svcProber) HasConfig(ctx context.Context) (bool, error) {
	return s.svc.Store().HasConfig(ctx)
}

func (s svcProber) RecordAuthHealth(ctx context.Context) {
	s.svc.RecordAuthHealth(ctx)
}
