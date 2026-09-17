package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// seam5DynamicRelaunchPayload builds the AC-42f "dynamic_relaunch" replay row
// from relaunchDynamicTaskAfterFailure's own frame, at the point the gate is
// consulted.
func seam5DynamicRelaunchPayload(data watcher.AgentEventData, executionProfileID string) map[string]interface{} {
	return map[string]interface{}{
		metaKeySessionID:       data.SessionID,
		metaKeyTaskID:          data.TaskID,
		"agent_execution_id":   data.AgentExecutionID,
		"execution_profile_id": executionProfileID,
		metaKeyAgentProfileID:  data.AgentProfileID,
	}
}

func seam5DynamicRelaunchPayloadWithBinding(
	data watcher.AgentEventData,
	executionProfileID string,
	binding *models.CeilingWorkflowEntryBinding,
) map[string]interface{} {
	payload := seam5DynamicRelaunchPayload(data, executionProfileID)
	if binding != nil {
		payload[models.CeilingLaunchEntryBindingKey] = ceilingEntryBindingValue(*binding)
	}
	return payload
}

// admitOrDeferSeam5 is Service.relaunchDynamicTaskAfterFailure's gate, consulted
// at entry, before the CREATED transition and before the Office/non-Office
// fork. It uses handOffOrAdmit rather than admit: on the automatic path the
// predecessor session is still counted, so this is AC-38's capacity hand-off,
// not an ordinary admission request; only the two LaunchDynamicRouteAction
// callers, which arrive after the predecessor has already left the counted
// population, fall through to an ordinary AC-13e-governed admission.
//
// The returned reservation is non-nil only when the relaunch is admitted or
// handed off. The caller is responsible for deferring
// reservation.releaseIfNotConsumed and for calling reservation.consume once a
// replacement launch is actually dispatched.
func (s *Service) admitOrDeferSeam5(
	ctx context.Context, taskID string, origin launchOrigin, relaunchPayload map[string]interface{},
) (reservation *sessionKeyedCeilingReservation, deferred bool, err error) {
	return s.admitOrDeferSeam5WithBinding(ctx, taskID, origin, relaunchPayload, nil)
}

func (s *Service) admitOrDeferSeam5WithBinding(
	ctx context.Context, taskID string, origin launchOrigin, relaunchPayload map[string]interface{},
	binding *models.CeilingWorkflowEntryBinding,
) (reservation *sessionKeyedCeilingReservation, deferred bool, err error) {
	if binding != nil {
		relaunchPayload = cloneCeilingPayload(relaunchPayload)
		relaunchPayload[models.CeilingLaunchEntryBindingKey] = ceilingEntryBindingValue(*binding)
	}
	sessionID, _ := relaunchPayload[metaKeySessionID].(string)
	decision := s.sessionCeiling.handOffOrAdmit(ctx, admissionRequest{
		taskID:    taskID,
		sessionID: sessionID,
		origin:    origin,
		seam:      "relaunchDynamicTaskAfterFailure",
	})
	if decision.admitted {
		return &sessionKeyedCeilingReservation{
			controller: s.sessionCeiling, key: decision.reservationKey,
			manualOverride: decision.manualOverride, population: decision.population,
			populationKnown: decision.populationKnown, ceiling: decision.ceiling,
		}, false, nil
	}
	relaunchPayload = s.enrichCeilingLaunchPayload(ctx, taskID, sessionID, relaunchPayload)

	if err := s.deferCeilingRefusal(ctx, taskID, sessionID, models.CeilingLaunchDynamicRelaunch, relaunchPayload, decision.reasonCode,
		decision.population, decision.populationKnown, decision.ceiling); err != nil {
		s.logger.Zap().Error("could not persist a ceiling deferral; the dynamic relaunch could not be admitted or recorded",
			zap.String("task_id", taskID), zap.String("session_id", sessionID),
			zap.String(ceilingFieldReasonCode, ceilingReasonDeferWriteFailed), zap.Error(err))
		return nil, false, err
	}
	return nil, true, nil
}
