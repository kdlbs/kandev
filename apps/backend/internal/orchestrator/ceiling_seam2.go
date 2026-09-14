package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

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
		metaKeyPrompt:                    prompt,
		metaKeyPlanMode:                  planMode,
		metaKeyAttachments:               attachments,
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
) (*sessionKeyedCeilingReservation, bool, error) {
	return s.admitOrDeferSessionKeyedLaunch(ctx, taskID, sessionID, origin, "startCreatedSession",
		models.CeilingLaunchStartCreated, startPayload, "the launch could not be admitted or recorded")
}
