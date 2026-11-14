package service

import (
	"errors"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

// A transport deadline describes the request, not the lifetime of its child.
// The capacity owner attaches exact proof only after provider/capacity settlement.
func deadlineAttemptSettled(err error) bool {
	var failure *agentruntime.InferenceAttemptError
	if !errors.As(err, &failure) || failure.Attempt == nil {
		return false
	}
	status := failure.Attempt
	return failure.AttemptID != "" && status.AttemptID == failure.AttemptID &&
		status.OwnerID != "" && status.InstanceID != "" && status.Known && status.Settled &&
		(status.State == utility.AttemptTerminated || status.State == utility.AttemptNotStarted)
}

func deadlineRoutingFailure(err error) *routingerr.Error {
	if !deadlineAttemptSettled(err) {
		// A live parent or completed HTTP request does not prove the child stopped.
		return &routingerr.Error{Code: routingerr.CodeUnknownProvider, Class: routingerr.ClassUnclassified,
			CatalogueVersion: routingerr.CatalogueVersion, Confidence: routingerr.ConfLow,
			Phase: routingerr.PhasePromptSend, ClassifierRule: "utility.unconfirmed_request_deadline",
			RawExcerpt: routingerr.Sanitize(err.Error())}
	}
	// The original deadline remains the diagnostic cause; this is its retry policy.
	return &routingerr.Error{Code: routingerr.CodeNetworkUnavailable, Class: routingerr.ClassTransient,
		CatalogueVersion: routingerr.CatalogueVersion, Confidence: routingerr.ConfHigh,
		Phase: routingerr.PhasePromptSend, FallbackAllowed: true, AutoRetryable: true,
		ClassifierRule: "utility.settled_child_request_deadline", RawExcerpt: routingerr.Sanitize(err.Error())}
}
