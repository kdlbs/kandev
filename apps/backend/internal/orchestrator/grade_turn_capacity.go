package orchestrator

import "context"

// A turn owns both capacity dimensions before a warm dispatch or nested resume.
// Its immutable grade also accounts for the interval before the counted row exists.
func (c *sessionCeilingController) admitAllTurnLocked(ctx context.Context, req turnAdmissionRequest) (*messageTurnReservation, *messageTurnCapacityRefusal) {
	admission := c.resolveAdmissionGradeLocked(ctx, admissionRequest{taskID: req.taskID, sessionID: req.sessionID})
	population, err := c.readGradedPopulationLocked(ctx, admission.grade)
	decision, owned := c.alreadyAdmittedLocked(admission, population.counted, population.total)
	if !owned {
		decision = c.gradeCapacityDecisionLocked(admission, population, err == nil && !population.unknown)
	}
	decision = c.persistAdmissionGradeLocked(ctx, admission, decision)
	if !decision.admitted {
		return nil, &messageTurnCapacityRefusal{
			population: decision.population, populationKnown: decision.populationKnown,
			ceiling: decision.ceiling, reasonCode: decision.reasonCode,
		}
	}
	ticket := c.reserveTurnLocked(req)
	ticket.grade, ticket.gradeKnown = admission.grade, admission.gradeKnown
	return ticket, nil
}
