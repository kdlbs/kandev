package coordinator

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// approvalSweepInterval is the sweep's cadence
// (proposal-recovery.md#recovery): once a minute, so a claim left by a
// still-running process (its approve request died after the claim) is
// recovered within a minute with no manager action.
const approvalSweepInterval = time.Minute

// approvalSweepStaleAfter is the sweep's cutoff window: a claim is stale
// once its claimed_at is more than two minutes before the tick time, the
// same window an approve request's own stale re-claim uses.
const approvalSweepStaleAfter = 2 * time.Minute

// StartApprovalSweep starts the once-a-minute background sweep that
// recovers "approving" proposals whose claim went stale while the claiming
// process kept running (proposal-recovery.md#recovery). Callers run this
// once, after StartupRecoveryPass returns (the startup pass covers every
// claim older than its own T0; the sweep covers claims that go stale
// afterwards). The loop stops when ctx is cancelled; calling this again
// before that is a no-op.
func (s *Service) StartApprovalSweep(ctx context.Context) {
	ticker := time.NewTicker(approvalSweepInterval)
	s.startApprovalSweep(ctx, ticker.C, ticker.Stop)
}

// startApprovalSweep is StartApprovalSweep's testable core: tick and
// stopTicker are injected so tests can drive the sweep with a fake tick
// channel and observe shutdown, never a real sleep. A tick channel with the
// standard *time.Ticker's capacity-1 buffer already drops a tick that
// arrives while the previous pass is still running, so this loop needs no
// extra bookkeeping to keep two passes from overlapping.
func (s *Service) startApprovalSweep(ctx context.Context, tick <-chan time.Time, stopTicker func()) {
	s.sweepMu.Lock()
	if s.sweepStarted {
		s.sweepMu.Unlock()
		return
	}
	s.sweepStarted = true
	s.sweepMu.Unlock()

	s.sweepWG.Add(1)
	go func() {
		defer s.sweepWG.Done()
		defer stopTicker()
		defer func() {
			s.sweepMu.Lock()
			s.sweepStarted = false
			s.sweepMu.Unlock()
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-tick:
				s.runApprovalSweepPass(ctx, now.Add(-approvalSweepStaleAfter))
			}
		}
	}()
}

// WaitApprovalSweepStopped blocks until the goroutine started by
// StartApprovalSweep has exited after its context was cancelled. Production
// ties the sweep's lifetime purely to the app context and never calls this;
// it exists so tests and graceful-shutdown callers can join the goroutine
// after cancelling, instead of asserting no-leak immediately after cancel.
func (s *Service) WaitApprovalSweepStopped() {
	s.sweepWG.Wait()
}

// runApprovalSweepPass runs one discovery-and-recover pass, sharing the
// startup pass's per-row order and error rules (recovery.go's
// StartupRecoveryPass/recoverStaleRow), differing only in cutoff.
func (s *Service) runApprovalSweepPass(ctx context.Context, cutoff time.Time) {
	defer func() {
		if s.afterSweepPass != nil {
			s.afterSweepPass()
		}
	}()
	rows, err := s.store.ListApprovingClaimedBeforeAnyKind(ctx, cutoff)
	if err != nil {
		s.logger.Warn("approval sweep: discovery query failed", zap.Error(err))
		return
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return
		}
		s.recoverStaleRow(ctx, row, cutoff)
	}
}
