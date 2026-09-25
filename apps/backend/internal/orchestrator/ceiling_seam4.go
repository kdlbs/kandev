package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// seam4ResumePayload builds the AC-42 "resume" replay row from
// ResumeTaskSessionWithOptions's own frame, at the point the gate is consulted.
func seam4ResumePayload(sessionID string, options executor.ResumeOptions) map[string]interface{} {
	return seam4ResumePayloadWithBinding(sessionID, options, nil)
}

func seam4ResumePayloadWithBinding(
	sessionID string,
	options executor.ResumeOptions,
	binding *models.CeilingWorkflowEntryBinding,
) map[string]interface{} {
	payload := map[string]interface{}{
		metaKeySessionID:           sessionID,
		"allow_branch_replacement": options.AllowBranchReplacement,
	}
	if binding != nil {
		payload[models.CeilingLaunchEntryBindingKey] = map[string]interface{}{
			"workflow_id":            binding.WorkflowID,
			"destination_step_id":    binding.DestinationStepID,
			"route_operation_id":     binding.RouteOperationID,
			"entry_identity":         binding.EntryIdentity,
			"destination_session_id": binding.DestinationSessionID,
		}
	}
	return payload
}

// admitOrDeferSeam4 is Service.ResumeTaskSessionWithOptions's gate: consulted
// after the resumability and Office checks, before the executor's resume call,
// keyed by the session being resumed — it already exists, so there is no
// launch-scoped window and no redirect to re-key onto (unlike seam 2). On
// refusal it persists the ceiling_deferred record from the caller-supplied
// AC-42 "resume" payload and reports the decision so the caller returns
// without calling the executor.
//
// The returned reservation is non-nil only when the resume is admitted. The
// caller is responsible for deferring reservation.releaseIfNotConsumed and for
// calling reservation.consume once the resume succeeds.
func (s *Service) admitOrDeferSeam4(
	ctx context.Context, taskID, sessionID string, origin launchOrigin, startPayload map[string]interface{},
) (*sessionKeyedCeilingReservation, bool, error) {
	return s.admitOrDeferSessionKeyedLaunch(ctx, taskID, sessionID, origin, "resumeTaskSession",
		models.CeilingLaunchResume, startPayload, "the resume could not be admitted or recorded")
}
