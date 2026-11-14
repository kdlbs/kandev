package backendapp

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/orchestrator"
)

// coordinatorAnswerDelivery delivers an approved answer through the same
// operations the in-place answer uses: the clarification resolver for a
// question bundle and the orchestrator for a permission request.
type coordinatorAnswerDelivery struct {
	bundles     clarificationBundleResolver
	permissions answerPermissionResponder
}

// answerPermissionResponder is the orchestrator operation a permission answer
// calls.
type answerPermissionResponder interface {
	RespondToPermission(
		ctx context.Context, taskID, sessionID, requestID, pendingID, optionID string, cancelled, rejected bool,
	) error
}

func (d coordinatorAnswerDelivery) ResolveBundle(
	ctx context.Context, pendingID string, outcome clarification.Outcome,
) (*clarification.Resolution, bool, error) {
	return d.bundles.ResolveBundle(ctx, pendingID, outcome)
}

func (d coordinatorAnswerDelivery) RespondToPermission(
	ctx context.Context, taskID, sessionID, requestID, pendingID, optionID string,
) error {
	return answerDeliveryError(d.permissions.RespondToPermission(ctx, taskID, sessionID, requestID, pendingID, optionID, false, false))
}

// answerDeliveryError maps the orchestrator's permission errors onto the
// coordinator's delivery sentinels. An error outside the listed set is
// returned unchanged and settles as an unknown outcome.
func answerDeliveryError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, orchestrator.ErrPermissionNotFound),
		errors.Is(err, orchestrator.ErrPermissionStale),
		errors.Is(err, orchestrator.ErrPermissionAlreadyResolved):
		return coordinator.ErrDeliveryStale
	case errors.Is(err, orchestrator.ErrPermissionTaskOrSessionNotFound):
		return coordinator.ErrDeliveryTaskOrSessionGone
	case errors.Is(err, orchestrator.ErrPermissionOptionNotOffered):
		return coordinator.ErrDeliveryOptionNotOffered
	case errors.Is(err, orchestrator.ErrPermissionResolutionInProgress):
		return coordinator.ErrDeliveryInProgress
	case errors.Is(err, orchestrator.ErrPermissionHumanInputRequired):
		return coordinator.ErrDeliveryNotDelivered
	default:
		return err
	}
}
