package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ErrMessageTurnCapacityExceeded is the stable sentinel a producer can match
// when it must persist durable pending input instead of dispatching.
var ErrMessageTurnCapacityExceeded = errors.New("session ceiling has no capacity for a new message turn")

// metaKeyCapacityOverride marks a trusted internal queue entry that continues
// an explicit execution action across a cancellation-and-requeue boundary.
const metaKeyCapacityOverride = "capacity_override"

// queuedMessageCapacityIntent preserves an explicit override only for entries
// written by trusted internal continuation paths. Ordinary queue entries stay
// bounded.
func queuedMessageCapacityIntent(queuedMsg *messagequeue.QueuedMessage) turnCapacityIntent {
	if queuedMsg != nil && boolField(queuedMsg.Metadata, metaKeyCapacityOverride) {
		return turnCapacityOverride
	}
	return turnCapacityUnspecified
}

// turnCapacityIntent separates caller authority from capacity policy. Ordinary
// message turns are bounded; only explicit Start, Resume, Send Now, legacy Run
// next, and already-accepted continuations request an override.
type turnCapacityIntent uint8

const (
	// turnCapacityUnspecified means the caller did not choose a policy. New
	// message turns default to bounded admission.
	turnCapacityUnspecified turnCapacityIntent = iota
	turnCapacityBounded
	turnCapacityOverride
)

func (i turnCapacityIntent) normalized() turnCapacityIntent {
	if i == turnCapacityUnspecified {
		return turnCapacityBounded
	}
	return i
}

// messageTurnCapacityRefusal is returned before provider dispatch when the
// enabled ceiling has no capacity for a new message turn. Callers that own
// durable input restore it and wait for capacity instead of failing the turn.
type messageTurnCapacityRefusal struct {
	population      int
	populationKnown bool
	ceiling         int
	reasonCode      string
}

func (e *messageTurnCapacityRefusal) Error() string {
	if e == nil {
		return ErrMessageTurnCapacityExceeded.Error()
	}
	if e.populationKnown {
		return fmt.Sprintf("%s (%d of %d)", ErrMessageTurnCapacityExceeded.Error(), e.population, e.ceiling)
	}
	return fmt.Sprintf("%s (ceiling %d)", ErrMessageTurnCapacityExceeded.Error(), e.ceiling)
}

func (e *messageTurnCapacityRefusal) Unwrap() error { return ErrMessageTurnCapacityExceeded }

func messageTurnCapacityRefusalOf(err error) (*messageTurnCapacityRefusal, bool) {
	if err == nil {
		return nil, false
	}
	refusal, ok := err.(*messageTurnCapacityRefusal)
	return refusal, ok
}

// IsMessageTurnCapacityRefusal reports whether err is the capacity refusal a
// producer can turn into durable pending input.
func IsMessageTurnCapacityRefusal(err error) bool {
	if _, ok := messageTurnCapacityRefusalOf(err); ok {
		return true
	}
	return errors.Is(err, ErrMessageTurnCapacityExceeded)
}

// messageTurnReservation is one attempt-owned message-turn ticket. The token is
// the release identity: startup and launch callbacks never own it, so they
// cannot release a successor's capacity.
type messageTurnReservation struct {
	controller    *sessionCeilingController
	token         string
	taskID        string
	sessionID     string
	incarnationID string
	takenAt       time.Time

	manualOverride  bool
	population      int
	populationKnown bool
	ceiling         int

	released atomic.Bool
}

func (r *messageTurnReservation) release() {
	if r == nil || r.controller == nil || r.released.Swap(true) {
		return
	}
	r.controller.releaseTurn(r.token)
}

func (r *messageTurnReservation) matches(taskID, sessionID, incarnationID string) bool {
	if r == nil {
		return false
	}
	return r.taskID == taskID && r.sessionID == sessionID &&
		(r.incarnationID == "" || incarnationID == "" || r.incarnationID == incarnationID)
}

// messageTurnReservationContextKey carries an admitted handler reservation into
// asynchronous prompt dispatch without exposing controller internals.
type messageTurnReservationContextKey struct{}

func withMessageTurnReservation(ctx context.Context, reservation *messageTurnReservation) context.Context {
	if reservation == nil {
		return ctx
	}
	return context.WithValue(ctx, messageTurnReservationContextKey{}, reservation)
}

func messageTurnReservationFromContext(ctx context.Context) *messageTurnReservation {
	reservation, _ := ctx.Value(messageTurnReservationContextKey{}).(*messageTurnReservation)
	return reservation
}

// MessageTurnReservation is the cross-package handle for a capacity slot
// reserved before a direct request is acknowledged. Ownership transfers to the
// context passed to prompt dispatch; an un-dispatched request must Release it.
type MessageTurnReservation struct {
	reservation *messageTurnReservation
	service     *Service
}

// Release returns an unused reservation to the controller. Safe to call more
// than once and safe after prompt dispatch has already released it.
func (r *MessageTurnReservation) Release() {
	if r == nil {
		return
	}
	r.reservation.release()
	if r.service != nil {
		r.service.signalCeilingSweep()
	}
}

// Context returns a context that transfers this reservation to prompt dispatch.
func (r *MessageTurnReservation) Context(ctx context.Context) context.Context {
	if r == nil {
		return ctx
	}
	return withMessageTurnReservation(ctx, r.reservation)
}

// ReserveMessageTurn reserves one bounded message-turn slot before a direct
// request is acknowledged. admitted=false is a normal full or unreadable
// population outcome: the caller persists durable pending input instead of
// returning a failed request.
func (s *Service) ReserveMessageTurn(
	ctx context.Context, taskID, sessionID, incarnationID string,
) (*MessageTurnReservation, bool) {
	if s == nil || s.sessionCeiling == nil {
		return nil, true
	}
	reservation, refusal := s.sessionCeiling.admitTurn(ctx, turnAdmissionRequest{
		taskID: taskID, sessionID: sessionID, incarnationID: incarnationID,
		intent: turnCapacityBounded, seam: "message_request",
	})
	if refusal != nil {
		return nil, false
	}
	if reservation == nil {
		return nil, true
	}
	return &MessageTurnReservation{reservation: reservation, service: s}, true
}

// admitPromptTaskMessageTurn consumes a handler reservation bound to the same
// task and session incarnation, or makes an attempt-owned admission of its own.
// It runs before warm readiness and cold resume so neither can bypass capacity.
func (s *Service) admitPromptTaskMessageTurn(
	ctx context.Context,
	taskID, sessionID string,
	session *models.TaskSession,
	options promptTaskOptions,
) (*messageTurnReservation, error) {
	if reservation := messageTurnReservationFromContext(ctx); reservation != nil {
		if s.sessionCeiling != nil &&
			s.sessionCeiling.ownsTurnReservation(reservation) &&
			reservation.matches(taskID, sessionID, session.QueueIncarnationID) {
			return reservation, nil
		}
		// A released, expired, or replaced-incarnation handle must never be
		// reused. Return its slot (a no-op for an expired token) and reacquire
		// bounded capacity under this attempt's own ownership.
		reservation.release()
	}
	if s == nil || s.sessionCeiling == nil {
		return nil, nil
	}
	reservation, refusal := s.sessionCeiling.admitTurn(ctx, turnAdmissionRequest{
		taskID: taskID, sessionID: sessionID, incarnationID: session.QueueIncarnationID,
		intent: options.capacityIntent, seam: "prompt_task",
	})
	if refusal != nil {
		return nil, refusal
	}
	if reservation != nil && reservation.manualOverride {
		s.recordManualOverrideIfAdmitted(ctx, taskID, sessionID, reservation.manualOverride,
			reservation.population, reservation.populationKnown, reservation.ceiling)
	}
	return reservation, nil
}

// ownsTurnReservation reports whether the controller still holds this exact
// ticket. A released or expired ticket, or a token replaced by a successor,
// cannot authorize a retry.
func (c *sessionCeilingController) ownsTurnReservation(reservation *messageTurnReservation) bool {
	if c == nil || reservation == nil || reservation.released.Load() {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	current, ok := c.turnReservations[reservation.token]
	return ok && current == reservation
}

type turnAdmissionRequest struct {
	taskID        string
	sessionID     string
	incarnationID string
	intent        turnCapacityIntent
	seam          string
}

// admitTurn decides one message turn and records its attempt-owned ticket
// before returning. The population read and ticket write share the controller
// mutex, so two contenders for the final slot cannot both be admitted.
func (c *sessionCeilingController) admitTurn(
	ctx context.Context, req turnAdmissionRequest,
) (*messageTurnReservation, *messageTurnCapacityRefusal) {
	if c == nil {
		return nil, nil
	}
	intent := req.intent.normalized()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ceiling == unlimitedSessionCeiling {
		return c.reserveTurnLocked(req), nil
	}

	counted, err := c.countedRowsLocked(ctx)
	if err != nil {
		c.logger.Error("session ceiling could not read the admitted session population for a message turn",
			zap.String("seam", req.seam), zap.String("task_id", req.taskID),
			zap.String("session_id", req.sessionID), zap.String("intent", intent.String()), zap.Error(err))
		if intent == turnCapacityOverride {
			reservation := c.reserveTurnLocked(req)
			reservation.manualOverride = true
			return reservation, nil
		}
		return nil, &messageTurnCapacityRefusal{
			ceiling: c.ceiling, reasonCode: ceilingReasonUnknownPopulation,
		}
	}

	population := c.populationLocked(counted)
	if c.sessionOwnsCapacityLocked(req.sessionID, counted) {
		return c.reserveTurnLocked(req), nil
	}
	if population < c.ceiling {
		return c.reserveTurnLocked(req), nil
	}
	if intent == turnCapacityOverride {
		reservation := c.reserveTurnLocked(req)
		reservation.manualOverride = true
		reservation.population = population
		reservation.populationKnown = true
		reservation.ceiling = c.ceiling
		c.logger.Info("session ceiling admitted a message turn with an explicit override",
			zap.String("seam", req.seam), zap.String("task_id", req.taskID),
			zap.String("session_id", req.sessionID), zap.Int("population", population),
			zap.Int(ceilingFieldCeiling, c.ceiling))
		return reservation, nil
	}
	c.logger.Info("session ceiling refused a message turn",
		zap.String("seam", req.seam), zap.String("task_id", req.taskID),
		zap.String("session_id", req.sessionID), zap.Int("population", population),
		zap.Int(ceilingFieldCeiling, c.ceiling), zap.String(ceilingFieldReasonCode, ceilingReasonRefused))
	return nil, &messageTurnCapacityRefusal{
		population: population, populationKnown: true, ceiling: c.ceiling,
		reasonCode: ceilingReasonRefused,
	}
}

// disposeMessageTurnCapacityRefusal preserves the existing automatic
// prompt_ensure replay for bounded automatic prompts that have no producer-owned
// durable input. Queued dispatches and other non-reconstructable calls return
// the refusal unchanged to their queue owner.
func (s *Service) disposeMessageTurnCapacityRefusal(
	ctx context.Context, taskID, sessionID, prompt, model string,
	planMode bool, attachments []v1.MessageAttachment, dispatchOnly bool,
	origin launchOrigin, options promptTaskOptions, refusal *messageTurnCapacityRefusal,
) error {
	if origin != launchOriginAutomatic || refusal == nil || seam3PromptEnsureNonReconstructableOptionsSet(options) {
		return nil
	}
	seamRefusal := &seam3Refusal{
		reasonCode: refusal.reasonCode, population: refusal.population,
		populationKnown: refusal.populationKnown, ceiling: refusal.ceiling,
	}
	if err := s.disposeSeam3PromptEnsureRefusal(
		ctx, taskID, sessionID, prompt, model, planMode, attachments, dispatchOnly, options, seamRefusal,
	); err != nil {
		return nil
	}
	if seamRefusal.deferred {
		return seamRefusal
	}
	return nil
}

func (i turnCapacityIntent) String() string {
	switch i {
	case turnCapacityBounded:
		return "bounded"
	case turnCapacityOverride:
		return "override"
	default:
		return "unspecified"
	}
}

func (c *sessionCeilingController) reserveTurnLocked(req turnAdmissionRequest) *messageTurnReservation {
	token := "turn-" + uuid.NewString()
	reservation := &messageTurnReservation{
		controller: c, token: token, taskID: req.taskID, sessionID: req.sessionID,
		incarnationID: req.incarnationID, takenAt: c.now(), ceiling: c.ceiling,
	}
	c.turnReservations[token] = reservation
	return reservation
}

func (c *sessionCeilingController) sessionOwnsCapacityLocked(
	sessionID string, counted map[string]struct{},
) bool {
	if sessionID == "" {
		return false
	}
	if c.hasTurnReservationLocked(sessionID) {
		return true
	}
	if _, held := c.reservations[sessionID]; held {
		return true
	}
	_, isCounted := counted[sessionID]
	return isCounted
}

func (c *sessionCeilingController) hasTurnReservationLocked(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	for _, reservation := range c.turnReservations {
		if reservation.sessionID == sessionID {
			return true
		}
	}
	return false
}

func (c *sessionCeilingController) releaseTurn(token string) {
	if c == nil || token == "" {
		return
	}
	c.mu.Lock()
	delete(c.turnReservations, token)
	c.mu.Unlock()
}

// turnCapacityAvailableForSession reports whether this session can take a new
// turn now. A session that already owns a counted row, launch reservation, or
// turn ticket is admitted even when its own slot fills the ceiling.
func (c *sessionCeilingController) turnCapacityAvailableForSession(ctx context.Context, sessionID string) bool {
	if c == nil {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ceiling == unlimitedSessionCeiling {
		return true
	}
	counted, err := c.countedRowsLocked(ctx)
	if err != nil {
		return false
	}
	if c.sessionOwnsCapacityLocked(sessionID, counted) {
		return true
	}
	return c.populationLocked(counted) < c.ceiling
}

// turnCapacityAvailable reports whether a new message turn could be admitted
// now. It is an observation only; the admission decision still reads the
// population under the controller mutex.
func (c *sessionCeilingController) turnCapacityAvailable(ctx context.Context) bool {
	if c == nil {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ceiling == unlimitedSessionCeiling {
		return true
	}
	counted, err := c.countedRowsLocked(ctx)
	if err != nil {
		return false
	}
	return c.populationLocked(counted) < c.ceiling
}
