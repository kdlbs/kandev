package lifecycle

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
)

// OwnershipClaimer is the narrow surface the renewal loop drives, satisfied
// by *agentctl.ControlClient. Narrowed to one method so tests can drive the
// loop without a real HTTP round trip.
type OwnershipClaimer interface {
	ClaimOwnership(ctx context.Context) error
}

// OwnershipRenewer periodically issues the ownership-claim operation
// (AC-EXECUTORS-CONTROL-OWNERSHIP-003.8) so the adopted or freshly spawned
// control server never begins an unowned shutdown while this backend is
// running (AC-EXECUTORS-CONTROL-OWNERSHIP-003.2). It does not claim
// immediately on Start: the bootstrap handshake (fresh spawn) or a
// successful credential rotation (adoption) already counted as the first
// renewal at the moment this loop is started, so an immediate claim here
// would be a redundant fourth renewing operation not listed by AC-003.8.
//
// Follows the goroutine-ownership convention (explicit Start/Stop,
// WaitGroup-registered, idempotent both ends, selects on a stop signal
// rather than sleeping) established by internal/integrations/healthpoll.
type OwnershipRenewer struct {
	claimer  OwnershipClaimer
	interval time.Duration
	logger   *logger.Logger

	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
}

// NewOwnershipRenewer builds a renewer that issues ClaimOwnership on the
// given interval, which callers must derive via
// ownershipperiod.RenewalInterval so it stays strictly inside a third of
// the resolved unowned period.
func NewOwnershipRenewer(claimer OwnershipClaimer, interval time.Duration, log *logger.Logger) *OwnershipRenewer {
	return &OwnershipRenewer{claimer: claimer, interval: interval, logger: log}
}

// Start launches the background loop. Calling Start more than once without
// Stop is a no-op.
func (r *OwnershipRenewer) Start(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started || r.claimer == nil || r.interval <= 0 {
		return
	}
	r.started = true
	ctx, r.cancel = context.WithCancel(ctx)
	r.wg.Add(1)
	go r.loop(ctx)
}

// Stop cancels the loop and waits for it to drain. Safe to call even if
// Start was never called, and safe to call more than once.
func (r *OwnershipRenewer) Stop() {
	r.mu.Lock()
	if !r.started {
		r.mu.Unlock()
		return
	}
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	r.wg.Wait()
	r.mu.Lock()
	r.started = false
	r.mu.Unlock()
}

func (r *OwnershipRenewer) loop(ctx context.Context) {
	defer r.wg.Done()
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.claimer.ClaimOwnership(ctx); err != nil {
				if errors.Is(err, agentctl.ErrOwnershipCredentialSuperseded) {
					// AC-EXECUTORS-CONTROL-OWNERSHIP-002.3: a superseded
					// credential means some other party rotated it out from
					// under this backend (most likely a second backend that
					// adopted the server). Retrying would keep presenting a
					// credential that can never succeed again, and this loop
					// must not mint or fetch a replacement on its own -- only
					// a fresh adoption may do that. Stop instead of looping
					// back to the ticker.
					r.logger.Error("ownership renewal claim rejected: credential superseded; stopping renewal loop without retry",
						zap.Error(err))
					r.stopSelf()
					return
				}
				r.logger.Warn("ownership renewal claim failed; will retry on the next interval",
					zap.Error(err))
			}
		}
	}
}

// stopSelf marks the renewer stopped from within its own loop goroutine.
// Unlike Stop, it must not call wg.Wait(): the loop goroutine invoking this
// is exactly what wg is waiting on, so waiting here would deadlock. A
// concurrent explicit Stop() call remains safe: it will see r.started
// already false and return immediately, or it will call the (idempotent)
// cancel and then wg.Wait(), which unblocks as soon as this goroutine
// returns.
func (r *OwnershipRenewer) stopSelf() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
	}
	r.started = false
}
