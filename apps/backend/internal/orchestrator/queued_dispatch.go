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
	queuedDispatchSupersededByNewDispatch
	queuedDispatchSupersededBySendNow
)

type queuedDispatchReservation struct {
	sessionID string
	entryID   string
	source    *messagequeue.QueuedMessage
	phase     atomic.Uint32
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
// Callers that may be superseded by Send Now should use
// markQueuedDispatchInFlightWithSource so the exact source can be restored.
func (s *Service) markQueuedDispatchInFlight(sessionID, entryID string) *queuedDispatchReservation {
	return s.markQueuedDispatchInFlightWithSource(sessionID, entryID, nil)
}

func (s *Service) markQueuedDispatchInFlightWithSource(
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
	case queuedDispatchAccepted:
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
		return expected, false, nil
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

// supersedeQueuedDispatchForSendNow is called while sessionID's cancellation
// guard is held. It returns the exact source only for a pending automatic FIFO
// reservation. An accepted or source-less reservation is already terminal for
// Send Now and must remain a conflict.
func (s *Service) supersedeQueuedDispatchForSendNow(sessionID string) (*messagequeue.QueuedMessage, error) {
	reservation := s.pendingQueuedDispatch(sessionID)
	if reservation == nil {
		if s.acceptedQueuedDispatchForSession(sessionID) != nil {
			return nil, ErrSendNowConflict
		}
		return nil, nil
	}
	if reservation.currentPhase() != queuedDispatchPending || reservation.source == nil {
		return nil, ErrSendNowConflict
	}
	reservation.phase.Store(uint32(queuedDispatchSupersededBySendNow))
	if !s.dispatchingQueued.CompareAndDelete(sessionID, reservation) {
		return nil, ErrSendNowConflict
	}
	return reservation.source, nil
}

func (s *Service) isQueuedDispatchAccepted(sessionID string) bool {
	return s.acceptedQueuedDispatchForSession(sessionID) != nil
}

// clearQueuedDispatchInFlightIfCurrent clears either phase only for the exact
// entry that owns it. Workers that lost to a newer dispatch cannot remove the
// replacement marker.
func (s *Service) clearQueuedDispatchInFlightIfCurrent(sessionID, entryID string) {
	if sessionID == "" || entryID == "" {
		return
	}
	if pending := s.pendingQueuedDispatch(sessionID); pending != nil && pending.entryID == entryID {
		pending.phase.Store(uint32(queuedDispatchSupersededByNewDispatch))
		s.dispatchingQueued.CompareAndDelete(sessionID, pending)
	}
	if accepted := s.acceptedQueuedDispatchForSession(sessionID); accepted != nil && accepted.entryID == entryID {
		accepted.phase.Store(uint32(queuedDispatchSupersededByNewDispatch))
		s.acceptedQueuedDispatch.CompareAndDelete(sessionID, accepted)
	}
}

// releaseQueuedDispatchPendingIfCurrent is used by the fast prompt-claim
// helpers. Ownership has already moved to the accepted map, so they must not
// clear the accepted marker while the agent turn is still running.
func (s *Service) releaseQueuedDispatchPendingIfCurrent(sessionID, entryID string) {
	if sessionID == "" || entryID == "" {
		return
	}
	if pending := s.pendingQueuedDispatch(sessionID); pending != nil && pending.entryID == entryID {
		pending.phase.Store(uint32(queuedDispatchAccepted))
		s.dispatchingQueued.CompareAndDelete(sessionID, pending)
	}
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
	return s.pendingQueuedDispatch(sessionID) != nil || s.isQueuedDispatchAccepted(sessionID)
}

func (s *Service) clearAcceptedQueuedDispatch(sessionID string) {
	if sessionID != "" {
		s.acceptedQueuedDispatch.Delete(sessionID)
	}
}
