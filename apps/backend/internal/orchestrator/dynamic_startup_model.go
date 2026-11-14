package orchestrator

import (
	"errors"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// A strict model rejection before the first prompt belongs to the selected
// candidate. A logical dynamic profile may follow its declared failure policy;
// a fixed profile still cannot silently use the agent's default model.
func unavailableDynamicStartupModel(err error, data watcher.AgentEventData) *routingerr.Error {
	var failure *agentruntime.BootstrapFailure
	if !errors.As(err, &failure) || failure == nil ||
		failure.SafeCode() != models.AgentErrorCauseCodeModelUnavailable ||
		failure.Reason != models.AgentErrorCauseReasonRequestedNotAdvertised ||
		failure.RequestedModel == "" || failure.PromptNotSent == nil || !*failure.PromptNotSent {
		return nil
	}
	for _, cause := range data.Causes {
		if cause.Operation == models.AgentErrorCauseOperationStart &&
			cause.Code == models.AgentErrorCauseCodeModelUnavailable &&
			cause.Reason == models.AgentErrorCauseReasonRequestedNotAdvertised &&
			cause.PromptNotSent != nil && *cause.PromptNotSent {
			return &routingerr.Error{
				Code: routingerr.CodeModelUnavailable, Class: routingerr.ClassHard,
				CatalogueVersion: routingerr.CatalogueVersion, Confidence: routingerr.ConfHigh,
				Phase: routingerr.PhaseSessionInit, FallbackAllowed: true,
				UserAction: true, ClassifierRule: "startup.requested_model_unavailable",
			}
		}
	}
	return nil
}

func classifiedStartupModelFailureSafe(
	classified *routingerr.Error,
	evidence *dynamicruntime.UnclassifiedFailureEvidence,
) bool {
	return classified != nil && classified.Code == routingerr.CodeModelUnavailable &&
		classified.Phase == routingerr.PhaseSessionInit && classified.FallbackAllowed &&
		classified.ClassifierRule == "startup.requested_model_unavailable" &&
		evidence != nil && evidence.StrictStartupModelRejection &&
		evidence.Origin == dynamicruntime.UnclassifiedOriginAgentStartup &&
		evidence.TaskScope && evidence.CurrentAttempt && evidence.EvidenceKnown &&
		!evidence.OutputObserved && !evidence.EffectObserved
}
