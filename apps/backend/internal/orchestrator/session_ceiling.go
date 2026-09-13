package orchestrator

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/constants"
)

// reservationExpiryAllowance is the slice of the backstop that follows the session's
// STARTING write: the start deadline inside the launch goroutine. The preparation
// phase before it is already bounded by constants.AgentLaunchTimeout, which is read
// rather than copied so this tracks an operator-raised preparation budget.
const reservationExpiryAllowance = 5 * time.Minute

// launchOrigin is the explicit automatic/manual classification threaded from the
// caller into the admission controller. It is never inferred from the transport or
// the handler, both of which carry human clicks and server-initiated automation.
type launchOrigin string

const (
	launchOriginAutomatic launchOrigin = "automatic"
	launchOriginManual    launchOrigin = "manual"
)

// Reason codes carried verbatim by the admission log line and the card surface.
const (
	ceilingReasonRefused           = "ceiling"
	ceilingReasonManualOverride    = "ceiling_manual_override"
	ceilingReasonUnknownPopulation = "ceiling_unknown_population"
	ceilingReasonSuperseded        = "ceiling_superseded"
	ceilingReasonDeferWriteFailed  = "ceiling_defer_write_failed"
)

// admittedSessionLister supplies the persisted half of the population. It returns
// ids rather than a count because the population unions rows with reservations by
// session id, so a session holding both is counted once.
type admittedSessionLister interface {
	ListAdmittedSessionIDs(ctx context.Context) ([]string, error)
}

// ceilingReservation is an in-flight admission held until the session is observed
// running or the launch fails. sessionID is empty while the reservation is still
// keyed by a launch-scoped identifier, which is the window between admission at
// seam 1 and the session's creation.
type ceilingReservation struct {
	sessionID string
	takenAt   time.Time
}

// admissionRequest is one launch asking for capacity.
type admissionRequest struct {
	taskID    string
	sessionID string
	origin    launchOrigin
	seam      string
}

// admissionDecision is the controller's answer.
type admissionDecision struct {
	admitted        bool
	manualOverride  bool
	reservationKey  string
	population      int
	populationKnown bool
	ceiling         int
	reasonCode      string
	handedOff       bool
}

// sessionCeilingController is the single admission controller. Every mutation of
// the reservation set happens under its one mutex, together with the population
// read it is compared against.
type sessionCeilingController struct {
	mu           sync.Mutex
	ceiling      int
	reservations map[string]*ceilingReservation
	lister       admittedSessionLister
	logger       *zap.Logger
	now          func() time.Time
	newKey       func() string
}

func newSessionCeilingController(ceiling int, lister admittedSessionLister, logger *zap.Logger) *sessionCeilingController {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &sessionCeilingController{
		ceiling:      ceiling,
		reservations: make(map[string]*ceilingReservation),
		lister:       lister,
		logger:       logger,
		now:          time.Now,
		newKey:       func() string { return "launch-" + uuid.NewString() },
	}
}

// population returns the admitted session population.
func (c *sessionCeilingController) population(ctx context.Context) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	counted, err := c.countedRowsLocked(ctx)
	if err != nil {
		return 0, err
	}
	return c.populationLocked(counted), nil
}

// countedRowsLocked reads the persisted half of the population. It is derived at
// decision time rather than kept as a running tally, so a restart, a panic or a
// missed release cannot leak capacity permanently.
func (c *sessionCeilingController) countedRowsLocked(ctx context.Context) (map[string]struct{}, error) {
	if c.lister == nil {
		return map[string]struct{}{}, nil
	}
	ids, err := c.lister.ListAdmittedSessionIDs(ctx)
	if err != nil {
		return nil, err
	}
	counted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		counted[id] = struct{}{}
	}
	return counted, nil
}

// populationLocked unions the persisted rows with the session-bound reservations by
// session id, then adds the launch-scoped reservations, each of which can collide
// with no row because no row for it exists yet. It is deliberately not the sum of
// two independent numbers.
func (c *sessionCeilingController) populationLocked(counted map[string]struct{}) int {
	population := len(counted)
	for _, reservation := range c.reservations {
		if reservation.sessionID == "" {
			population++
			continue
		}
		if _, alreadyCounted := counted[reservation.sessionID]; !alreadyCounted {
			population++
		}
	}
	return population
}

// reserveLocked records an in-flight reservation, keyed by session id where one
// exists and by a launch-scoped identifier otherwise.
func (c *sessionCeilingController) reserveLocked(sessionID string) string {
	key := sessionID
	if key == "" {
		key = c.newKey()
	}
	c.reservations[key] = &ceilingReservation{sessionID: sessionID, takenAt: c.now()}
	return key
}

// admit decides one launch and, where it admits, records the reservation before
// returning. The population read and the reservation write share one critical
// section, so two launches arriving together cannot both see the same free slot.
func (c *sessionCeilingController) admit(ctx context.Context, req admissionRequest) admissionDecision {
	origin := req.origin
	if origin != launchOriginManual && origin != launchOriginAutomatic {
		origin = launchOriginAutomatic
		c.logger.Warn("launch reached the session ceiling with no origin set; classified as automatic",
			zap.String("seam", req.seam), zap.String("task_id", req.taskID), zap.String("session_id", req.sessionID))
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	counted, err := c.countedRowsLocked(ctx)
	if err != nil {
		return c.decideUnknownPopulationLocked(req, origin, err)
	}
	return c.decideLocked(req, origin, counted)
}

// decideLocked is the ordinary admission decision, taken against a population the
// caller has already read inside the critical section.
func (c *sessionCeilingController) decideLocked(req admissionRequest, origin launchOrigin, counted map[string]struct{}) admissionDecision {
	population := c.populationLocked(counted)

	if decision, ok := c.alreadyAdmittedLocked(req, counted, population); ok {
		return decision
	}

	decision := admissionDecision{
		population:      population,
		populationKnown: true,
		ceiling:         c.ceiling,
	}
	switch {
	case c.ceiling == unlimitedSessionCeiling || population < c.ceiling:
		decision.admitted = true
		decision.reservationKey = c.reserveLocked(req.sessionID)
	case origin == launchOriginManual:
		decision.admitted = true
		decision.manualOverride = true
		decision.reasonCode = ceilingReasonManualOverride
		decision.reservationKey = c.reserveLocked(req.sessionID)
	default:
		decision.reasonCode = ceilingReasonRefused
	}
	c.logDecision(req, origin, decision)
	return decision
}

// alreadyAdmittedLocked answers a request for a session that already holds a
// reservation or is already counted, so one launch consumes at most one unit of
// capacity however many seams it passes through.
func (c *sessionCeilingController) alreadyAdmittedLocked(req admissionRequest, counted map[string]struct{}, population int) (admissionDecision, bool) {
	if req.sessionID == "" {
		return admissionDecision{}, false
	}
	decision := admissionDecision{
		admitted:        true,
		population:      population,
		populationKnown: true,
		ceiling:         c.ceiling,
	}
	if _, held := c.reservations[req.sessionID]; held {
		decision.reservationKey = req.sessionID
		return decision, true
	}
	if _, isCounted := counted[req.sessionID]; isCounted {
		return decision, true
	}
	return admissionDecision{}, false
}

// decideUnknownPopulationLocked fails closed for automatic launches and open for
// manual ones. Reservations already held are untouched: the failure concerns the
// counted rows only.
func (c *sessionCeilingController) decideUnknownPopulationLocked(req admissionRequest, origin launchOrigin, err error) admissionDecision {
	c.logger.Error("session ceiling could not read the admitted session population",
		zap.String("seam", req.seam), zap.String("task_id", req.taskID),
		zap.String("session_id", req.sessionID), zap.String("origin", string(origin)), zap.Error(err))

	decision := admissionDecision{
		ceiling:    c.ceiling,
		reasonCode: ceilingReasonUnknownPopulation,
	}
	if origin == launchOriginManual {
		decision.admitted = true
		decision.manualOverride = true
		decision.reservationKey = c.reserveLocked(req.sessionID)
	}
	c.logDecision(req, origin, decision)
	return decision
}

// logDecision emits the admission log line, which carries every reason code and is
// the only universal observable of a decision.
func (c *sessionCeilingController) logDecision(req admissionRequest, origin launchOrigin, decision admissionDecision) {
	fields := []zap.Field{
		zap.String("seam", req.seam),
		zap.String("task_id", req.taskID),
		zap.String("session_id", req.sessionID),
		zap.String("origin", string(origin)),
		zap.Int("ceiling", decision.ceiling),
		zap.Bool("admitted", decision.admitted),
	}
	if decision.populationKnown {
		fields = append(fields, zap.Int("population", decision.population))
	}
	if decision.reasonCode != "" {
		fields = append(fields, zap.String("reason_code", decision.reasonCode))
	}
	c.logger.Info("session ceiling admission decision", fields...)
}

// release drops a reservation by its key. Releasing an unknown or already-released
// key succeeds and decrements nothing.
func (c *sessionCeilingController) release(key string) {
	if key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.reservations, key)
}

// rebind moves a launch-scoped reservation onto the session id that launch just
// created, as one operation rather than a release followed by an acquire, so the
// population never momentarily drops and no concurrent admission can take the
// freed unit.
func (c *sessionCeilingController) rebind(launchKey, sessionID string) bool {
	if launchKey == "" || sessionID == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	reservation, held := c.reservations[launchKey]
	if !held {
		return false
	}
	delete(c.reservations, launchKey)
	reservation.sessionID = sessionID
	c.reservations[sessionID] = reservation
	return true
}

// rekey moves a reservation from the session that was gated onto the replacement
// session that will actually be launched. Where the replacement is already counted
// or already reserved, the original is released and no second unit is consumed:
// this is still one launch.
func (c *sessionCeilingController) rekey(ctx context.Context, fromSessionID, toSessionID string) bool {
	if fromSessionID == "" || toSessionID == "" || fromSessionID == toSessionID {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	reservation, held := c.reservations[fromSessionID]
	if !held {
		return false
	}
	delete(c.reservations, fromSessionID)

	if _, alreadyReserved := c.reservations[toSessionID]; alreadyReserved {
		return true
	}
	if counted, err := c.countedRowsLocked(context.WithoutCancel(ctx)); err == nil {
		if _, alreadyCounted := counted[toSessionID]; alreadyCounted {
			return true
		}
	}
	reservation.sessionID = toSessionID
	c.reservations[toSessionID] = reservation
	return true
}

// handOffOrAdmit serves the dynamic-route relaunch seam. Where the session is still
// in the population, its counted membership is converted into a reservation under
// the same mutex: a relaunch replaces one agent process with another rather than
// adding one, so the hand-off is never refused and never changes the count.
// Where the session is not counted there is no slot to hand off and this is an
// ordinary admission request.
func (c *sessionCeilingController) handOffOrAdmit(ctx context.Context, req admissionRequest) admissionDecision {
	origin := req.origin
	if origin != launchOriginManual && origin != launchOriginAutomatic {
		origin = launchOriginAutomatic
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	counted, err := c.countedRowsLocked(ctx)
	if err != nil {
		return c.decideUnknownPopulationLocked(req, origin, err)
	}
	if _, isCounted := counted[req.sessionID]; req.sessionID == "" || !isCounted {
		return c.decideLocked(req, origin, counted)
	}

	decision := admissionDecision{
		admitted:        true,
		handedOff:       true,
		reservationKey:  req.sessionID,
		population:      c.populationLocked(counted),
		populationKnown: true,
		ceiling:         c.ceiling,
	}
	c.reservations[req.sessionID] = &ceilingReservation{sessionID: req.sessionID, takenAt: c.now()}
	c.logDecision(req, origin, decision)
	return decision
}

// expireStaleReservations releases reservations whose launch neither reached a
// counted state nor reported failure inside the launch budget. It is the backstop,
// not the primary release edge.
func (c *sessionCeilingController) expireStaleReservations() int {
	budget := constants.AgentLaunchTimeout + reservationExpiryAllowance
	now := c.now()

	c.mu.Lock()
	defer c.mu.Unlock()
	released := 0
	for key, reservation := range c.reservations {
		if now.Sub(reservation.takenAt) <= budget {
			continue
		}
		delete(c.reservations, key)
		released++
		c.logger.Warn("session ceiling released a reservation whose launch never reached a counted state",
			zap.String("reservation_key", key),
			zap.String("session_id", reservation.sessionID),
			zap.Duration("held_for", now.Sub(reservation.takenAt)),
			zap.Duration("budget", budget))
	}
	return released
}
