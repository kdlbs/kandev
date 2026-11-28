package orchestrator

import (
	"context"
	"maps"
	"time"

	"github.com/kandev/kandev/internal/system/sessioncapacity"
)

// SessionCapacityObservation reads the same population and policy lock as
// admission. It never grants, prunes, claims, wakes or dispatches queued work.
func (s *Service) SessionCapacityObservation(ctx context.Context) sessioncapacity.Observation {
	if s == nil || s.sessionCeiling == nil {
		return sessioncapacity.Observation{}
	}
	c := s.sessionCeiling
	c.mu.Lock()
	defer c.mu.Unlock()
	observed, population, _ := c.capacitySnapshotLocked(ctx)
	at := time.Now().UTC()
	if c.now != nil {
		at = c.now().UTC()
	}
	result := sessioncapacity.Observation{
		Policy: observed.Policy, Enabled: c.ceiling > 0, Known: observed.Known,
		Limit: observed.Limit, GradeLimits: maps.Clone(observed.GradeLimits),
		NeedsConfiguration: observed.NeedsConfiguration, ObservedAt: at,
	}
	if observed.Known {
		result.InUse = &observed.InUse
		result.GradeInUse = maps.Clone(observed.GradeInUse)
	}
	if c.policy == sessioncapacity.PolicyAll {
		result.Waiting = c.capacityWaitingObservationLocked(ctx, population, observed.Known)
	}
	return result
}

func (c *sessionCeilingController) capacityWaitingObservationLocked(ctx context.Context, population gradedPopulation, known bool) sessioncapacity.WaitingObservation {
	requests, err := c.observedCapacityRequestsLocked(ctx, population)
	if err != nil {
		c.logPopulationUnknown("capacity_waiters", populationReadFailure(err), "", "")
		return sessioncapacity.WaitingObservation{}
	}
	counts := map[string]int{ceilingReasonTotalLimit: 0, ceilingReasonGradeLimit: 0,
		ceilingReasonBothLimits: 0, ceilingReasonNeedsConfiguration: 0, ceilingReasonUnknownPopulation: 0}
	for _, request := range requests {
		reason := c.observedCapacityReasonLocked(request, population, known)
		if reason != "" {
			counts[reason]++
		}
	}
	return sessioncapacity.WaitingObservation{Known: true, Counts: counts}
}

func (c *sessionCeilingController) observedCapacityReasonLocked(req admissionRequest, population gradedPopulation, known bool) string {
	maximum, configured := c.requestedGradeLimitLocked(req)
	if !configured {
		return ceilingReasonNeedsConfiguration
	}
	if !req.gradeKnown {
		return ceilingReasonUnknownPopulation
	}
	if population.missing {
		return ceilingReasonNeedsConfiguration
	}
	if !known {
		return ceilingReasonUnknownPopulation
	}
	population.grade = population.grades[req.grade]
	return c.allCapacityLimitReason(population, maximum)
}
