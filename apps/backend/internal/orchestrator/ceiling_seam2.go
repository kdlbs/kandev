package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seam2Reservation tracks the session-keyed admission reservation seam 2 takes
// for a session that already exists (AC-4d). Unlike seam 1's launch-scoped
// reservation, this one is keyed by the session id from the moment it is taken.
// rekeyToSession only moves it when startCreatedSession's on_turn_start redirect
// switches to a different, already-active session mid-start (AC-4e).
type seam2Reservation struct {
	controller *sessionCeilingController
	key        string
	consumed   bool
}

// rekeyToSession moves the reservation from the session it was gated under onto
// the replacement session the redirect actually launches, as one operation so
// the population never momentarily drops.
func (r *seam2Reservation) rekeyToSession(ctx context.Context, sessionID string) {
	if r == nil || r.controller == nil || r.consumed || sessionID == "" || sessionID == r.key {
		return
	}
	if r.controller.rekey(ctx, r.key, sessionID) {
		r.key = sessionID
	}
}

// consume marks the reservation as belonging to a session that is now actually
// running, so releaseIfNotConsumed becomes a no-op.
func (r *seam2Reservation) consume() {
	if r == nil {
		return
	}
	r.consumed = true
}

// releaseIfNotConsumed is the deferred cleanup for every return path between
// admission and the launch actually succeeding.
func (r *seam2Reservation) releaseIfNotConsumed() {
	if r == nil || r.controller == nil || r.consumed {
		return
	}
	r.controller.release(r.key)
}

// seam2StartCreatedPayload builds the AC-42d "start_created" replay row from
// startCreatedSession's own frame, at the point the gate is consulted — before
// the profile override, prompt composition, and workflow/plan transforms below
// it, so the recorded payload is the caller's original request.
func seam2StartCreatedPayload(
	sessionID, agentProfileID, prompt string,
	skipMessageRecord, planMode, autoStart bool,
	attachments []v1.MessageAttachment,
	references []v1.EntityReference,
	promptReferenceContext string,
	options startCreatedSessionOptions,
) map[string]interface{} {
	return map[string]interface{}{
		metaKeySessionID:                 sessionID,
		metaKeyAgentProfileID:            agentProfileID,
		"prompt":                         prompt,
		"plan_mode":                      planMode,
		"attachments":                    attachments,
		"references":                     references,
		"prompt_reference_context":       promptReferenceContext,
		"skip_message_record":            skipMessageRecord,
		"auto_start":                     autoStart,
		"skip_task_description_fallback": options.skipTaskDescriptionFallback,
		"prompt_already_composed":        options.promptAlreadyComposed,
		"retry_prompt":                   options.retryPrompt,
	}
}

// admitOrDeferSeam2 is Service.startCreatedSession's AC-4d gate: consulted
// before its own claimDeferredLaunchForStart, keyed by the session that already
// exists (AC-4d — unlike seam 1, there is no launch-scoped window here). On
// refusal it persists the ceiling_deferred record from the caller-supplied
// AC-42d "start_created" payload and reports the decision so the caller returns
// without claiming the launch intent it holds or dispatching an agent.
//
// The returned reservation is non-nil only when the launch is admitted. The
// caller is responsible for deferring reservation.releaseIfNotConsumed, for
// calling reservation.rekeyToSession if the on_turn_start redirect switches
// sessions, and for calling reservation.consume once the launch succeeds.
func (s *Service) admitOrDeferSeam2(
	ctx context.Context, taskID, sessionID string, origin launchOrigin, startPayload map[string]interface{},
) (reservation *seam2Reservation, deferred bool, err error) {
	decision := s.sessionCeiling.admit(ctx, admissionRequest{
		taskID:    taskID,
		sessionID: sessionID,
		origin:    origin,
		seam:      "startCreatedSession",
	})
	if decision.admitted {
		return &seam2Reservation{controller: s.sessionCeiling, key: decision.reservationKey}, false, nil
	}

	if err := s.deferCeilingRefusal(ctx, taskID, models.CeilingLaunchStartCreated, startPayload, decision.reasonCode); err != nil {
		s.logger.Zap().Error("could not persist a ceiling deferral; the launch could not be admitted or recorded",
			zap.String("task_id", taskID), zap.String("session_id", sessionID),
			zap.String("reason_code", ceilingReasonDeferWriteFailed), zap.Error(err))
		return nil, false, err
	}
	return nil, true, nil
}
