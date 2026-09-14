package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// originFromAutoStart derives the launch origin from the existing autoStart
// signal, the widening AC-13a describes for a seam that has no dedicated origin
// parameter of its own. It is not trusted at the two call sites AC-13d names,
// which force automatic regardless of the autoStart value they carry.
func originFromAutoStart(autoStart bool) launchOrigin {
	if autoStart {
		return launchOriginAutomatic
	}
	return launchOriginManual
}

// seam1Reservation tracks the launch-scoped admission reservation seam 1 takes
// before a session exists (AC-6a). Every return path releases it, by deferring
// releaseIfNotRebound at the point of admission, unless it has since been
// rebound onto the session that was actually created.
type seam1Reservation struct {
	controller *sessionCeilingController
	key        string
	rebound    bool

	// manualOverride, population, populationKnown and ceiling are the
	// admission decision's own reading, carried forward so AC-14/AC-53's
	// audit write and card warning can be performed once the session this
	// launch creates actually exists (AC-53, AC-14b).
	manualOverride  bool
	population      int
	populationKnown bool
	ceiling         int
}

// rebindToSession moves the reservation onto the created session's id, as one
// operation rather than a release followed by an acquire (AC-6a).
func (r *seam1Reservation) rebindToSession(sessionID string) {
	if r == nil || r.controller == nil || r.rebound || sessionID == "" {
		return
	}
	if r.controller.rebind(r.key, sessionID) {
		r.rebound = true
	}
}

// releaseIfNotRebound is the deferred cleanup for every failure path between
// admission and session creation.
func (r *seam1Reservation) releaseIfNotRebound() {
	if r == nil || r.controller == nil || r.rebound {
		return
	}
	r.controller.release(r.key)
}

// seam1StartPayload builds the AC-42 "start" replay row from startTask's own
// frame, at the point the gate is consulted — before any workflow-step profile
// resolution, so the recorded profile is the one the caller actually chose.
func seam1StartPayload(
	agentProfileID, executorID, executorProfileID, priority, prompt, workflowStepID string,
	planMode, autoStart bool,
	attachments []v1.MessageAttachment,
	opts startTaskOptions,
) map[string]interface{} {
	payload := map[string]interface{}{
		metaKeyAgentProfileID:  agentProfileID,
		"executor_id":          executorID,
		metaKeyExecutorProfile: executorProfileID,
		"priority":             priority,
		metaKeyPrompt:          prompt,
		metaKeyWorkflowStepID:  workflowStepID,
		metaKeyPlanMode:        planMode,
		metaKeyAttachments:     attachments,
		"env":                  opts.Env,
		"route":                opts.Route,
		"profile_explicit":     opts.ProfileExplicit,
		"auto_start":           autoStart,
	}
	if opts.SpawnOrigin != nil {
		payload["spawn_origin"] = map[string]interface{}{
			metaKeyTaskID:    opts.SpawnOrigin.TaskID,
			metaKeySessionID: opts.SpawnOrigin.SessionID,
			"session_name":   opts.SpawnOrigin.SessionName,
		}
	}
	return payload
}

// admitOrDeferSeam1 is Service.startTask's AC-4a gate: consulted before its own
// claimDeferredLaunchForStart, keyed by no session id yet (AC-6a). On refusal it
// persists the ceiling_deferred record from the caller-supplied AC-42 "start"
// payload and reports the decision so the caller returns without creating a
// session, materializing a workspace, or claiming the launch intent it holds.
//
// The returned reservation is non-nil only when the launch is admitted. The
// caller is responsible for deferring reservation.releaseIfNotRebound and for
// calling reservation.rebindToSession once the session exists.
func (s *Service) admitOrDeferSeam1(
	ctx context.Context, taskID string, origin launchOrigin, startPayload map[string]interface{},
) (reservation *seam1Reservation, deferred bool, err error) {
	decision := s.sessionCeiling.admit(ctx, admissionRequest{
		taskID: taskID,
		origin: origin,
		seam:   "startTask",
	})
	if decision.admitted {
		return &seam1Reservation{
			controller: s.sessionCeiling, key: decision.reservationKey,
			manualOverride: decision.manualOverride, population: decision.population,
			populationKnown: decision.populationKnown, ceiling: decision.ceiling,
		}, false, nil
	}

	if err := s.deferCeilingRefusal(ctx, taskID, "", models.CeilingLaunchStart, startPayload, decision.reasonCode,
		decision.population, decision.populationKnown, decision.ceiling); err != nil {
		s.logger.Zap().Error("could not persist a ceiling deferral; the launch could not be admitted or recorded",
			zap.String("task_id", taskID), zap.String(ceilingFieldReasonCode, ceilingReasonDeferWriteFailed), zap.Error(err))
		return nil, false, err
	}
	return nil, true, nil
}
