package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/proclive"
)

// orphanReapSignal is a portable enum for the two signals this phase ever
// sends. Platform files (resource_cleanup_orphan_reap_signal_*.go) translate
// it to the real OS signal.
type orphanReapSignal int

const (
	orphanReapSigterm orphanReapSignal = iota
	orphanReapSigkill
)

// orphanReapVerifyTimeout bounds the single-process re-verification read.
// It is deliberately independent of orphanReapSnapshotTimeout: this read
// sits outside that combined bound.
const orphanReapVerifyTimeout = 2 * time.Second

// orphanReapSignaler sends a signal to one PID and checks its liveness. The
// real implementation is a thin wrapper over sendOrphanReapSignal (a raw
// per-PID syscall, never a process group) and proclive.Alive. It exists as an
// interface so tests can exercise ownership, ordering, batching, and
// cancellation logic against fabricated PIDs without ever issuing a real
// signal — this feature signals processes Kandev does not own, so a test
// must never risk sending a real signal to an arbitrary host PID.
type orphanReapSignaler interface {
	Signal(pid int, sig orphanReapSignal) error
	Alive(pid int) (alive, known bool)
}

type realOrphanReapSignaler struct{}

func (realOrphanReapSignaler) Signal(pid int, sig orphanReapSignal) error {
	return sendOrphanReapSignal(pid, sig)
}

func (realOrphanReapSignaler) Alive(pid int) (bool, bool) {
	return proclive.Alive(int64(pid))
}

type orphanReapPendingCandidate struct {
	orphanReapCandidate
	lastSignal string
}

// signalOrphanReapCandidates escalates SIGTERM -> SIGKILL per PID: every
// SIGTERM is sent before the shared grace period starts, and identity is
// re-verified immediately before every signal. It returns a retryable error
// when any candidate survives SIGKILL, or when cancellation interrupts a
// signal already sent.
func (s *Service) signalOrphanReapCandidates(
	ctx context.Context, taskID string, candidates []orphanReapCandidate, snapshot *taskResourceCleanupSnapshot,
) []error {
	verifier := s.orphanReapVerifier
	if verifier == nil {
		verifier = defaultOrphanReapVerifier()
	}
	signaler := s.orphanReapSignaler
	if signaler == nil {
		signaler = realOrphanReapSignaler{}
	}
	if ctx.Err() != nil {
		return []error{errOrphanReapCancelledMidPhase}
	}

	pending := s.sendOrphanReapSigterms(ctx, taskID, candidates, snapshot, verifier, signaler)
	if ctx.Err() != nil {
		// Cancellation may have broken the loop before anything reached
		// pending; report it either way rather than falling through to the
		// empty-pending "nothing to do" return below.
		return s.recordOrphanReapCancelledMidPhase(snapshot, taskID, pending)
	}
	if len(pending) == 0 {
		return nil
	}
	graceTimer := time.NewTimer(orphanReapGraceDelay)
	defer graceTimer.Stop()
	select {
	case <-ctx.Done():
		return s.recordOrphanReapCancelledMidPhase(snapshot, taskID, pending)
	case <-graceTimer.C:
	}

	killPending := s.sendOrphanReapSigkills(ctx, taskID, pending, snapshot, verifier, signaler)
	if ctx.Err() != nil {
		return s.recordOrphanReapCancelledMidPhase(snapshot, taskID, killPending)
	}
	if len(killPending) == 0 {
		return nil
	}
	settleTimer := time.NewTimer(orphanReapSettleDelay)
	defer settleTimer.Stop()
	select {
	case <-ctx.Done():
		return s.recordOrphanReapCancelledMidPhase(snapshot, taskID, killPending)
	case <-settleTimer.C:
	}

	return s.resolveOrphanReapSurvivors(ctx, taskID, killPending, snapshot, verifier, signaler)
}

func (s *Service) sendOrphanReapSigterms(
	ctx context.Context,
	taskID string,
	candidates []orphanReapCandidate,
	snapshot *taskResourceCleanupSnapshot,
	verifier orphanReapVerifier,
	signaler orphanReapSignaler,
) []orphanReapPendingCandidate {
	pending := make([]orphanReapPendingCandidate, 0, len(candidates))
	for _, cand := range candidates {
		if ctx.Err() != nil {
			// Cancellation stops further signalling immediately; the
			// caller's ctx.Done() branch records everyone already in
			// pending as skipped. A context-blind verifier (e.g. Linux's
			// VerifyCwd) must never let this loop keep sending signals
			// after cancellation.
			break
		}
		if !orphanReapReverifyInsideRoot(ctx, verifier, cand.PID, cand.Root) {
			s.recordOrphanReapSkip(snapshot, taskID, cand, "pid reused or moved before signal")
			continue
		}
		if ctx.Err() != nil {
			// Cancellation can land during the reverify call itself when the
			// verifier is context-blind (e.g. Linux's bare os.Readlink);
			// re-check immediately before the signal that would otherwise
			// follow a successful reverify.
			break
		}
		if err := signaler.Signal(cand.PID, orphanReapSigterm); err != nil {
			s.recordOrphanReapSignalError(snapshot, taskID, cand, "sigterm", err)
			continue
		}
		pending = append(pending, orphanReapPendingCandidate{orphanReapCandidate: cand, lastSignal: "sigterm already sent"})
	}
	return pending
}

func (s *Service) sendOrphanReapSigkills(
	ctx context.Context,
	taskID string,
	pending []orphanReapPendingCandidate,
	snapshot *taskResourceCleanupSnapshot,
	verifier orphanReapVerifier,
	signaler orphanReapSignaler,
) []orphanReapPendingCandidate {
	killPending := make([]orphanReapPendingCandidate, 0, len(pending))
	for i, cand := range pending {
		if ctx.Err() != nil {
			// Cancellation stops further escalation. Every remaining
			// candidate, including this one, already received SIGTERM in a
			// prior pass, so it carries forward unmodified rather than being
			// dropped: the caller's cancellation branch persists everything
			// still in this slice.
			return append(killPending, pending[i:]...)
		}
		alive, known := signaler.Alive(cand.PID)
		if known && !alive {
			s.recordOrphanReapCandidate(snapshot, taskID, orphanReapRecordFor(cand.orphanReapCandidate, orphanReapOutcomeTerminated, ""))
			continue
		}
		if !orphanReapReverifyInsideRoot(ctx, verifier, cand.PID, cand.Root) {
			s.recordOrphanReapSkip(snapshot, taskID, cand.orphanReapCandidate, "pid reused or moved before kill signal")
			continue
		}
		if ctx.Err() != nil {
			// Same recheck as sendOrphanReapSigterms: cancellation can land
			// during the reverify call itself when the verifier is
			// context-blind.
			return append(killPending, pending[i:]...)
		}
		if err := signaler.Signal(cand.PID, orphanReapSigkill); err != nil {
			s.recordOrphanReapSignalError(snapshot, taskID, cand.orphanReapCandidate, "sigkill", err)
			continue
		}
		cand.lastSignal = "sigkill already sent"
		killPending = append(killPending, cand)
	}
	return killPending
}

func (s *Service) resolveOrphanReapSurvivors(
	ctx context.Context,
	taskID string,
	killPending []orphanReapPendingCandidate,
	snapshot *taskResourceCleanupSnapshot,
	verifier orphanReapVerifier,
	signaler orphanReapSignaler,
) []error {
	survived := false
	for i, cand := range killPending {
		if ctx.Err() != nil {
			// The settle-delay select's two channels can both already be
			// ready, so cancellation is not guaranteed to have won that
			// select even when it raced the timer, and cancellation can
			// also land between candidates in this loop rather than only
			// before it starts. Recheck every iteration rather than resolve
			// the not-yet-classified remainder to a normal outcome; a
			// candidate already classified above keeps its accurate one.
			return s.recordOrphanReapCancelledMidPhase(snapshot, taskID, killPending[i:])
		}
		alive, known := signaler.Alive(cand.PID)
		if known && !alive {
			s.recordOrphanReapCandidate(snapshot, taskID, orphanReapRecordFor(cand.orphanReapCandidate, orphanReapOutcomeKilled, ""))
			continue
		}
		if !orphanReapReverifyInsideRoot(ctx, verifier, cand.PID, cand.Root) {
			s.recordOrphanReapSkip(snapshot, taskID, cand.orphanReapCandidate, "cannot confirm identity after kill signal")
			continue
		}
		survived = true
		s.recordOrphanReapCandidate(snapshot, taskID,
			orphanReapRecordFor(cand.orphanReapCandidate, orphanReapOutcomeSurvived, "still alive after SIGKILL settle window"))
	}
	if survived {
		return []error{errOrphanReapCandidateSurvived}
	}
	return nil
}

func (s *Service) recordOrphanReapCancelledMidPhase(
	snapshot *taskResourceCleanupSnapshot, taskID string, pending []orphanReapPendingCandidate,
) []error {
	for _, cand := range pending {
		s.recordOrphanReapCandidate(snapshot, taskID, orphanReapRecordFor(cand.orphanReapCandidate, orphanReapOutcomeSkipped, cand.lastSignal))
	}
	if s.logger != nil {
		s.logger.Warn("orphan reap: cancelled mid-phase; no further signal sent",
			zap.String("task_id", taskID), zap.Int("pending", len(pending)))
	}
	return []error{errOrphanReapCancelledMidPhase}
}

func (s *Service) recordOrphanReapSkip(
	snapshot *taskResourceCleanupSnapshot, taskID string, cand orphanReapCandidate, reason string,
) {
	s.recordOrphanReapCandidate(snapshot, taskID, orphanReapRecordFor(cand, orphanReapOutcomeSkipped, reason))
}

// recordOrphanReapSignalError classifies a failed signal send: the process
// already being gone counts as terminated with no error, permission denied
// is a non-retryable skip, and anything else is treated conservatively as a
// skip so a transient OS error never masquerades as a successful reap.
func (s *Service) recordOrphanReapSignalError(
	snapshot *taskResourceCleanupSnapshot, taskID string, cand orphanReapCandidate, phase string, err error,
) {
	switch {
	case isOrphanReapProcessGone(err):
		s.recordOrphanReapCandidate(snapshot, taskID, orphanReapRecordFor(cand, orphanReapOutcomeTerminated, ""))
	case isOrphanReapPermissionDenied(err):
		s.recordOrphanReapCandidate(snapshot, taskID, orphanReapRecordFor(cand, orphanReapOutcomeSkipped, "permission denied"))
	default:
		s.recordOrphanReapCandidate(snapshot, taskID,
			orphanReapRecordFor(cand, orphanReapOutcomeSkipped, phase+" failed: "+err.Error()))
	}
}

func orphanReapRecordFor(cand orphanReapCandidate, outcome, reason string) orphanReapCandidateRecord {
	return orphanReapCandidateRecord{
		PID: cand.PID, Cwd: cand.Cwd, Root: cand.Root, Command: cand.Command,
		Outcome: outcome, Reason: reason,
	}
}

func orphanReapReverifyInsideRoot(ctx context.Context, verifier orphanReapVerifier, pid int, root string) bool {
	verifyCtx, cancel := context.WithTimeout(ctx, orphanReapVerifyTimeout)
	defer cancel()
	cwd, err := verifier.VerifyCwd(verifyCtx, pid)
	if err != nil || cwd == "" {
		return false
	}
	return orphanReapPathWithinRoot(root, cwd)
}
