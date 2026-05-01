package linear

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/integrations/healthpoll"
)

// Poller probes the stored Linear credentials of every configured workspace on
// the shared 90s auth-health cadence. The actual loop lives in
// internal/integrations/healthpoll; this type exists so callers can keep the
// familiar `linear.NewPoller(...).Start(ctx)` shape without depending on the
// shared package directly.
type Poller struct {
	inner *healthpoll.Poller
}

// NewPoller returns a poller using the default 90s cadence. Returns nil when
// svc is nil so the caller's nil-check still works after the indirection.
func NewPoller(svc *Service, log *logger.Logger) *Poller {
	if svc == nil {
		return nil
	}
	return &Poller{inner: healthpoll.New("linear", svcProber{svc}, log)}
}

// Start launches the background loop. Calling Start more than once without
// Stop is a no-op.
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

// svcProber adapts *Service to healthpoll.Prober without leaking the shared
// interface into Service's public API.
type svcProber struct{ svc *Service }

func (s svcProber) ListConfiguredWorkspaces(ctx context.Context) ([]string, error) {
	return s.svc.Store().ListConfiguredWorkspaces(ctx)
}

func (s svcProber) RecordAuthHealth(ctx context.Context, workspaceID string) {
	s.svc.RecordAuthHealth(ctx, workspaceID)
}
