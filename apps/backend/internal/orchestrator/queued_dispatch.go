package orchestrator

import (
	"errors"
	"sync/atomic"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

var errQueuedDispatchSupersededBySendNow = errors.New("queued dispatch superseded by send now")

type queuedDispatchPhase uint32

const (
	queuedDispatchPending queuedDispatchPhase = iota + 1
	queuedDispatchAccepted
	queuedDispatchLive
	queuedDispatchSupersededByNewDispatch
	queuedDispatchSupersededBySendNow
)

type queuedDispatchReservation struct {
	sessionID string
	entryID   string
	source    *messagequeue.QueuedMessage
	phase     atomic.Uint32
	// liveEligible is set only for Send Now reservations. It allows the
	// prompt-claim path to move that reservation to live while it still owns
	// the session guard; ordinary FIFO handoffs remain in accepted until their
	// turn settles.
	liveEligible atomic.Bool
	// successorTurn is the replacement turn this dispatch opened. A late
	// complete of the cancelled predecessor must not close that turn or
	// drop this reservation; only the successor's own ready-path settlement
	// may do so.
	successorTurn atomic.Value
}

func newQueuedDispatchReservation(sessionID, entryID string, source *messagequeue.QueuedMessage) *queuedDispatchReservation {
	reservation := &queuedDispatchReservation{
		sessionID: sessionID,
		entryID:   entryID,
		source:    source,
	}
	reservation.phase.Store(uint32(queuedDispatchPending))
	return reservation
}

func (reservation *queuedDispatchReservation) currentPhase() queuedDispatchPhase {
	if reservation == nil {
		return 0
	}
	return queuedDispatchPhase(reservation.phase.Load())
}

// markQueuedDispatchInFlight records a dispatch without retaining a source.
// This unguarded helper is used by tests and setup code; production queue-take
// paths use the Locked variant below while holding the session guard.
func (s *Service) markQueuedDispatchInFlight(sessionID, entryID string) *queuedDispatchReservation {
	return s.markQueuedDispatchInFlightWithSource(sessionID, entryID, nil)
}

// markQueuedDispatchInFlightWithSource is the unguarded setup/test helper.
// Production queue-take paths must call markQueuedDispatchInFlightWithSourceLocked
// while holding the session's cancelInFlight guard.
func (s *Service) markQueuedDispatchInFlightWithSource(
	sessionID, entryID string,
	source *messagequeue.QueuedMessage,
) *queuedDispatchReservation {
	if sessionID == "" || entryID == "" {
		return nil
	}
	return s.markQueuedDispatchInFlightWithSourceLocked(sessionID, entryID, source)
}

// markQueuedDispatchInFlightWithSourceLocked records a dispatch while the
// caller owns sessionID's cancelInFlight guard. All production queue-take
// paths use this variant so a Send Now claim cannot interleave between the
// reservation replacement and the worker launch.
func (s *Service) markQueuedDispatchInFlightWithSourceLocked(
	sessionID, entryID string,
	source *messagequeue.QueuedMessage,
) *queuedDispatchReservation {
	if sessionID == "" || entryID == "" {
		return nil
	}
	reservation := newQueuedDispatchReservation(sessionID, entryID, source)
	if previous, ok := s.dispatchingQueued.Load(sessionID); ok {
		if previousReservation, ok := previous.(*queuedDispatchReservation); ok {
			previousReservation.phase.Store(uint32(queuedDispatchSupersededByNewDispatch))
		}
	}
	// A genuine cancel-and-take path is allowed to replace an accepted queued
	// turn after cancellation has settled. The old accepted marker must not
	// block the new dispatch, while the old worker's compare-and-delete cleanup
	// still cannot remove this reservation.
	s.acceptedQueuedDispatch.Delete(sessionID)
	s.dispatchingQueued.Store(sessionID, reservation)
	return reservation
}

func (s *Service) pendingQueuedDispatch(sessionID string) *queuedDispatchReservation {
	value, ok := s.dispatchingQueued.Load(sessionID)
	if !ok {
		return nil
	}
	reservation, _ := value.(*queuedDispatchReservation)
	return reservation
}

func (s *Service) acceptedQueuedDispatchForSession(sessionID string) *queuedDispatchReservation {
	value, ok := s.acceptedQueuedDispatch.Load(sessionID)
	if !ok {
		return nil
	}
	reservation, _ := value.(*queuedDispatchReservation)
	return reservation
}

// claimQueuedDispatchForExecution moves a pending reservation to the accepted
// phase before the worker performs any visible message or workflow side effect.
// The same per-session guard arbitrates this transition against Send Now.
func (s *Service) claimQueuedDispatchForExecution(
	sessionID, entryID string,
	expected *queuedDispatchReservation,
) (bool, error) {
	if sessionID == "" || entryID == "" {
		return false, nil
	}
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()

	var alreadyTracked bool
	var err error
	expected, alreadyTracked, err = s.resolveQueuedDispatchForClaim(sessionID, entryID, expected)
	if alreadyTracked || err != nil {
		return alreadyTracked, err
	}
	if expected == nil {
		return false, nil
	}

	switch expected.currentPhase() {
	case queuedDispatchSupersededBySendNow:
		return true, errQueuedDispatchSupersededBySendNow
	case queuedDispatchSupersededByNewDispatch:
		return true, errQueuedDispatchSuperseded
	case queuedDispatchAccepted, queuedDispatchLive:
		return true, nil
	}

	if accepted := s.acceptedQueuedDispatchForSession(sessionID); accepted == expected {
		return true, nil
	}
	if pending := s.pendingQueuedDispatch(sessionID); pending != expected || pending.entryID != entryID {
		expected.phase.Store(uint32(queuedDispatchSupersededByNewDispatch))
		return true, errQueuedDispatchSuperseded
	}

	expected.phase.Store(uint32(queuedDispatchAccepted))
	s.dispatchingQueued.CompareAndDelete(sessionID, expected)
	s.acceptedQueuedDispatch.Store(sessionID, expected)
	return true, nil
}

func (s *Service) resolveQueuedDispatchForClaim(
	sessionID, entryID string,
	expected *queuedDispatchReservation,
) (*queuedDispatchReservation, bool, error) {
	if expected != nil {
		if expected.currentPhase() == queuedDispatchSupersededBySendNow {
			return expected, false, nil
		}
		if pending := s.pendingQueuedDispatch(sessionID); pending == expected {
			return expected, false, nil
		}
		if accepted := s.acceptedQueuedDispatchForSession(sessionID); accepted == expected {
			return expected, false, nil
		}
		expected.phase.Store(uint32(queuedDispatchSupersededByNewDispatch))
		return expected, true, errQueuedDispatchSuperseded
	}
	pending := s.pendingQueuedDispatch(sessionID)
	accepted := s.acceptedQueuedDispatchForSession(sessionID)
	if pending != nil && pending.entryID == entryID {
		return pending, false, nil
	}
	if accepted != nil && accepted.entryID == entryID {
		return nil, true, nil
	}
	if pending != nil || accepted != nil {
		return nil, true, errQueuedDispatchSuperseded
	}
	return nil, false, nil
}

// pendingQueuedDispatchForSendNow is called while sessionID's cancellation
// guard is held. It returns the exact pending reservation without changing
// ownership. The source is restored first; only a successful restore may
// supersede the FIFO worker.
func (s *Service) pendingQueuedDispatchForSendNow(sessionID string) (*queuedDispatchReservation, error) {
	reservation := s.pendingQueuedDispatch(sessionID)
	if reservation == nil {
		if accepted := s.acceptedQueuedDispatchForSession(sessionID); accepted != nil {
			// A live Send Now successor is already the running turn. A later
			// Send Now may cancel-and-replace it. FIFO/handoff still in the
			// accepted phase remains a hard conflict.
			if accepted.currentPhase() == queuedDispatchLive {
				return nil, nil
			}
			return nil, ErrSendNowConflict
		}
		return nil, nil
	}
	if reservation.currentPhase() != queuedDispatchPending || reservation.source == nil {
		return nil, ErrSendNowConflict
	}
	return reservation, nil
}

// supersedeQueuedDispatchForSendNow is called while sessionID's cancellation
// guard is held, after the exact FIFO source has been restored successfully.
func (s *Service) supersedeQueuedDispatchForSendNow(
	sessionID string,
	reservation *queuedDispatchReservation,
) error {
	if reservation == nil || s.pendingQueuedDispatch(sessionID) != reservation ||
		reservation.currentPhase() != queuedDispatchPending || reservation.source == nil {
		return ErrSendNowConflict
	}
	reservation.phase.Store(uint32(queuedDispatchSupersededBySendNow))
	if !s.dispatchingQueued.CompareAndDelete(sessionID, reservation) {
		return ErrSendNowConflict
	}
	return nil
}

func (s *Service) isQueuedDispatchAccepted(sessionID string) bool {
	accepted := s.acceptedQueuedDispatchForSession(sessionID)
	return accepted != nil && accepted.currentPhase() == queuedDispatchAccepted
}

// markAcceptedDispatchLive moves a Send Now successor out of the handoff
// conflict window once it owns execution. Stream-complete still protects the
// bound successor turn; a later Send Now may cancel that live turn.
func (s *Service) markAcceptedDispatchLive(sessionID string, reservation *queuedDispatchReservation) {
	if sessionID == "" {
		return
	}
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()
	s.markAcceptedDispatchLiveLocked(sessionID, reservation)
}

// markAcceptedDispatchLiveLocked moves a Send Now successor out of the
// handoff conflict window while the caller owns sessionID's cancellation
// guard. This keeps the phase transition serialized with prompt ownership.
func (s *Service) markAcceptedDispatchLiveLocked(sessionID string, reservation *queuedDispatchReservation) {
	accepted := s.acceptedQueuedDispatchForSession(sessionID)
	if accepted == nil {
		return
	}
	if reservation != nil && accepted != reservation {
		return
	}
	switch accepted.currentPhase() {
	case queuedDispatchAccepted, queuedDispatchLive:
		accepted.phase.Store(uint32(queuedDispatchLive))
	}
}

// clearQueuedDispatchInFlightIfCurrent clears either phase only for the exact
// reservation that owns it. Workers that lost to a newer dispatch cannot
// remove the replacement marker even when the queue entry ID was restored and
// reused.
func (s *Service) clearQueuedDispatchInFlightIfCurrent(
	sessionID string,
	reservation *queuedDispatchReservation,
) {
	if sessionID == "" || reservation == nil {
		return
	}
	if pending := s.pendingQueuedDispatch(sessionID); pending == reservation {
		pending.phase.Store(uint32(queuedDispatchSupersededByNewDispatch))
		s.dispatchingQueued.CompareAndDelete(sessionID, reservation)
	}
	if accepted := s.acceptedQueuedDispatchForSession(sessionID); accepted == reservation {
		accepted.phase.Store(uint32(queuedDispatchSupersededByNewDispatch))
		s.acceptedQueuedDispatch.CompareAndDelete(sessionID, reservation)
	}
}

// releaseQueuedDispatchPendingIfCurrent is used by the fast prompt-claim
// helpers. Ownership has already moved to the accepted map, so they must not
// clear the accepted marker while the agent turn is still running.
func (s *Service) releaseQueuedDispatchPendingIfCurrent(
	sessionID string,
	reservation *queuedDispatchReservation,
) {
	if sessionID == "" || reservation == nil {
		return
	}
	if pending := s.pendingQueuedDispatch(sessionID); pending == reservation {
		pending.phase.Store(uint32(queuedDispatchAccepted))
		s.dispatchingQueued.CompareAndDelete(sessionID, pending)
	}
}

func (s *Service) queuedDispatchReservationForEntry(sessionID, entryID string) *queuedDispatchReservation {
	if sessionID == "" || entryID == "" {
		return nil
	}
	if pending := s.pendingQueuedDispatch(sessionID); pending != nil && pending.entryID == entryID {
		return pending
	}
	if accepted := s.acceptedQueuedDispatchForSession(sessionID); accepted != nil && accepted.entryID == entryID {
		return accepted
	}
	return nil
}

func (s *Service) isCurrentQueuedDispatch(sessionID, entryID string) bool {
	if sessionID == "" || entryID == "" {
		return false
	}
	if pending := s.pendingQueuedDispatch(sessionID); pending != nil && pending.entryID == entryID {
		return true
	}
	accepted := s.acceptedQueuedDispatchForSession(sessionID)
	return accepted != nil && accepted.entryID == entryID
}

// isQueuedDispatchInFlight reports any unsettled queued dispatch. Ordinary
// drains must defer for both phases because the accepted marker exists during
// the short window before session.State becomes RUNNING and remains until the
// successor turn settles. Send Now checks the accepted phase separately so it
// can distinguish a supersedable pending reservation from a terminal conflict.
func (s *Service) isQueuedDispatchInFlight(sessionID string) bool {
	return s.pendingQueuedDispatch(sessionID) != nil || s.acceptedQueuedDispatchForSession(sessionID) != nil
}

func (s *Service) clearAcceptedQueuedDispatch(sessionID string) {
	if sessionID != "" {
		s.acceptedQueuedDispatch.Delete(sessionID)
	}
}

func (reservation *queuedDispatchReservation) successorTurnID() string {
	if reservation == nil {
		return ""
	}
	id, _ := reservation.successorTurn.Load().(string)
	return id
}

func (reservation *queuedDispatchReservation) bindSuccessorTurn(turnID string) {
	if reservation == nil || turnID == "" {
		return
	}
	reservation.successorTurn.Store(turnID)
}

func (s *Service) bindAcceptedDispatchTurn(sessionID, turnID string) {
	if accepted := s.acceptedQueuedDispatchForSession(sessionID); accepted != nil {
		accepted.bindSuccessorTurn(turnID)
	}
}

// acceptedDispatchInFlight reports whether a Send Now / FIFO successor has
// claimed prompt ownership. Stream-only completion of a cancelled predecessor
// must preserve that marker until the successor turn itself settles.
func (s *Service) acceptedDispatchInFlight(sessionID string) bool {
	return s.acceptedQueuedDispatchForSession(sessionID) != nil
}

func (s *Service) acceptedDispatchSuccessorTurn(sessionID string) string {
	if accepted := s.acceptedQueuedDispatchForSession(sessionID); accepted != nil {
		return accepted.successorTurnID()
	}
	return ""
}
