package orchestrator

import (
	"context"
	"errors"
	"maps"

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
		return population, errors.New("admitted execution grades unavailable")
	}
	rows, err := lister.ListAdmittedExecutions(ctx)
	if err != nil {
		return population, err
	}
	for _, row := range rows {
		population.counted[row.ID] = struct{}{}
		c.observeGradeLocked(&population, row.Grade, row.GradeKnown, grade)
	}
	population.total = c.populationLocked(population.counted)
	for _, held := range c.reservations {
		if _, counted := population.counted[held.sessionID]; held.sessionID != "" && counted {
			continue
		}
		c.observeGradeLocked(&population, held.grade, held.gradeKnown, grade)
	}
	return population, nil
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

func (c *sessionCeilingController) admitAllLocked(ctx context.Context, req admissionRequest, handoff bool) admissionDecision {
	d := admissionDecision{ceiling: c.ceiling}
	p, err := c.readGradedPopulationLocked(ctx, req.grade)
	d.population = p.total
	d.populationKnown = err == nil && !p.unknown
	// Existing ownership does not consume another slot when configuration changes.
	if selected, ok := c.reserveSelectedStartLocked(req, p); ok {
		selected.populationKnown = d.populationKnown
		return selected
	}
	if existing, owned := c.existingGradedAdmissionLocked(req, p, handoff); owned {
		existing.populationKnown = d.populationKnown
		return c.persistAdmissionGradeLocked(ctx, req, existing)
	}
	maximum, configured := c.requestedGradeLimitLocked(req)
	if !configured || (req.gradeKnown && p.missing) {
		d.reasonCode = ceilingReasonNeedsConfiguration
		return d
	}
	if !req.gradeKnown || !d.populationKnown {
		d.reasonCode = ceilingReasonUnknownPopulation
		return d
	}
	totalFull, gradeFull := p.total >= c.ceiling, p.grade >= maximum
	switch {
	case totalFull && gradeFull:
		d.reasonCode = ceilingReasonBothLimits
	case totalFull:
		d.reasonCode = ceilingReasonTotalLimit
	case gradeFull:
		d.reasonCode = ceilingReasonGradeLimit
	default:
		d.admitted = true
		d.reservationKey = c.reserveGradeLocked(req)
	}
	c.logDecision(req, req.origin, d)
	return c.persistAdmissionGradeLocked(ctx, req, d)
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
