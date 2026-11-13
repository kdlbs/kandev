package orchestrator

import (
	"context"
	"errors"
	"maps"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	ceilingReasonNeedsConfiguration = "ceiling_needs_configuration"
	ceilingReasonGradeLimit         = "ceiling_grade_limit"
	ceilingReasonTotalLimit         = "ceiling_total_limit"
	ceilingReasonBothLimits         = "ceiling_both_limits"
)

type admittedExecutionLister interface {
	ListAdmittedExecutions(context.Context) ([]models.AdmittedExecution, error)
}

// SetExecutionCapacity applies the independent overall and grade settings in
// the same owner lock. Existing reservations survive settings changes.
func (s *Service) SetExecutionCapacity(effective sessioncapacity.Effective) {
	if s == nil || s.sessionCeiling == nil {
		return
	}
	c := s.sessionCeiling
	c.mu.Lock()
	c.ceiling, c.policy = effective.MaxSessions, effective.Policy
	c.gradeLimits = maps.Clone(effective.GradeLimits)
	c.mu.Unlock()
	s.signalCeilingSweep()
}

type gradedPopulation struct {
	counted          map[string]struct{}
	grades           map[string]int
	total, grade     int
	unknown, missing bool
}

func (c *sessionCeilingController) readGradedPopulationLocked(ctx context.Context, grade string) (gradedPopulation, error) {
	population := gradedPopulation{counted: map[string]struct{}{}, grades: map[string]int{}}
	for configured := range c.gradeLimits {
		population.grades[configured] = 0
	}
	lister, ok := c.lister.(admittedExecutionLister)
	if !ok {
		c.logPopulationUnknown("task_sessions", "reader_unavailable", "", "")
		return population, errors.New("admitted execution grades unavailable")
	}
	rows, err := lister.ListAdmittedExecutions(ctx)
	if err != nil {
		c.logPopulationUnknown("task_sessions", populationReadFailure(err), "", "")
		return population, err
	}
	seenGradeSessions := make(map[string]struct{}, len(rows)+len(c.reservations))
	for _, row := range rows {
		if !row.GradeKnown {
			c.logPopulationUnknown("task_sessions", "grade_unknown", "session_id", row.ID)
		}
		population.counted[row.ID] = struct{}{}
		if row.ID != "" {
			seenGradeSessions[row.ID] = struct{}{}
		}
		c.observeGradeLocked(&population, row.Grade, row.GradeKnown, grade)
	}
	if err := c.appendRunSessionGradesLocked(ctx, &population, grade); err != nil {
		return population, err
	}
	if err := c.appendInferenceGradesLocked(ctx, &population, grade); err != nil {
		return population, err
	}
	population.total = c.populationLocked(population.counted)
	for key, held := range c.reservations {
		if held.sessionID != "" {
			if _, observed := seenGradeSessions[held.sessionID]; observed {
				continue
			}
			seenGradeSessions[held.sessionID] = struct{}{}
		}
		if !held.gradeKnown {
			id := held.sessionID
			if id == "" {
				id = key
			}
			c.logPopulationUnknown("reservations", "grade_unknown", "reservation_id", id)
		}
		c.observeGradeLocked(&population, held.grade, held.gradeKnown, grade)
	}
	c.markUncountedTurnGradeUnknown(&population)
	return population, nil
}

// A turn ticket contributes to the shared total before its session row reaches
// an admitted state. It has no grade snapshot of its own, so it must not be
// treated as zero grade usage. Counted rows and launch reservations already
// supply their session's grade and must not be marked unknown a second time.
func (c *sessionCeilingController) markUncountedTurnGradeUnknown(population *gradedPopulation) {
	accounted := maps.Clone(population.counted)
	for _, held := range c.reservations {
		if held.sessionID != "" {
			accounted[held.sessionID] = struct{}{}
		}
	}
	for _, turn := range c.turnReservations {
		if _, known := accounted[turn.sessionID]; turn.sessionID == "" || !known {
			population.unknown = true
			return
		}
	}
}

func (c *sessionCeilingController) observeGradeLocked(population *gradedPopulation, value string, known bool, requested string) {
	if !known {
		population.unknown = true
		return
	}
	if value != "" {
		population.grades[value]++
	}
	_, configured := c.gradeLimits[value]
	if value == "" || !configured {
		population.missing = true
	}
	if value == requested {
		population.grade++
	}
}

func (c *sessionCeilingController) admitAllLocked(ctx context.Context, req admissionRequest, handoff bool) (decision admissionDecision) {
	var observed *gradedPopulation
	var gradeLimit int
	defer func() { c.logGradedAdmissionFlow(ctx, req, decision, observed, gradeLimit) }()
	var err error
	req, err = c.prepareQueuedTaskCapacityLocked(ctx, req)
	if err != nil {
		c.logPopulationUnknown("capacity_queue", populationReadFailure(err), "session_id", req.sessionID)
		return admissionDecision{ceiling: c.ceiling, reasonCode: ceilingReasonUnknownPopulation}
	}
	if req.seam != officeRunCapacitySeam {
		req = c.resolveAdmissionGradeLocked(ctx, req)
	}
	d, p, maximum := c.inspectAllAdmissionLocked(ctx, req)
	observed, gradeLimit = &p, maximum
	// Existing ownership does not consume another slot when configuration
	// changes or the population becomes unreadable.
	if selected, ok := c.reserveSelectedStartLocked(req, p); ok {
		selected.populationKnown = d.populationKnown
		return selected
	}
	if existing, owned := c.existingGradedAdmissionLocked(req, p, handoff); owned {
		existing.populationKnown = d.populationKnown
		return c.persistAdmissionGradeLocked(ctx, req, existing)
	}
	if d.reasonCode != "" {
		return d
	}
	if queued, ok := c.consumeQueuedTaskCapacityLocked(ctx, req, p); ok {
		return queued
	}
	if queued, ok := c.consumeQueuedOfficeCapacityLocked(req, p); ok {
		return queued
	}
	d.reasonCode = c.allCapacityLimitReason(p, maximum)
	if d.reasonCode == "" {
		d.admitted = true
		d.reservationKey = c.reserveGradeLocked(req)
	}
	c.logDecision(req, req.origin, d)
	return c.persistAdmissionGradeLocked(ctx, req, d)
}

func (c *sessionCeilingController) inspectAllAdmissionLocked(ctx context.Context, req admissionRequest) (admissionDecision, gradedPopulation, int) {
	d := admissionDecision{ceiling: c.ceiling}
	maximum, configured := c.requestedGradeLimitLocked(req)
	// The population is read before any refusal so existing owners can still be
	// recognized while configuration is incomplete.
	p, err := c.readGradedPopulationLocked(ctx, req.grade)
	d.population = p.total
	d.populationKnown = err == nil && !p.unknown
	if !configured || (req.gradeKnown && p.missing) {
		d.reasonCode = ceilingReasonNeedsConfiguration
		return d, p, maximum
	}
	if !req.gradeKnown {
		c.logPopulationUnknown("admission_grade", "grade_unknown", "session_id", req.sessionID)
		d.reasonCode = ceilingReasonUnknownPopulation
		return d, p, maximum
	}
	if !d.populationKnown {
		d.reasonCode = ceilingReasonUnknownPopulation
		return d, p, maximum
	}
	return d, p, maximum
}

func (c *sessionCeilingController) allCapacityLimitReason(p gradedPopulation, maximum int) string {
	totalFull, gradeFull := p.total >= c.ceiling, p.grade >= maximum
	switch {
	case totalFull && gradeFull:
		return ceilingReasonBothLimits
	case totalFull:
		return ceilingReasonTotalLimit
	case gradeFull:
		return ceilingReasonGradeLimit
	default:
		return ""
	}
}

func (c *sessionCeilingController) existingGradedAdmissionLocked(req admissionRequest, p gradedPopulation, handoff bool) (admissionDecision, bool) {
	decision, owned := c.alreadyAdmittedLocked(req, p.counted, p.total)
	if owned && handoff && decision.reservationKey == "" {
		decision.reservationKey = c.reserveGradeLocked(req)
		decision.handedOff = true
	}
	return decision, owned
}

func (c *sessionCeilingController) requestedGradeLimitLocked(req admissionRequest) (int, bool) {
	maximum, configured := c.gradeLimits[req.grade]
	return maximum, c.ceiling >= 1 && (!req.gradeKnown || (req.grade != "" && configured))
}

func (c *sessionCeilingController) logGradedAdmissionFlow(ctx context.Context, req admissionRequest, d admissionDecision, p *gradedPopulation, maximum int) {
	e := logger.ExecutionFlow{Stage: "admission", Outcome: "waiting", Reason: d.reasonCode, TaskID: req.taskID, SessionID: req.sessionID, ProfileID: req.profileID, StepID: req.workflowStepID, Grade: req.grade, ReservationID: d.reservationKey, Limit: &d.ceiling, PopulationKnown: &d.populationKnown}
	if d.admitted {
		e.Outcome = "admitted"
	}
	if p != nil && d.populationKnown {
		e.Population, e.GradePopulation, e.GradeLimit = &p.total, &p.grade, &maximum
	}
	logger.EmitExecutionFlow(c.logger, ctx, e)
}
