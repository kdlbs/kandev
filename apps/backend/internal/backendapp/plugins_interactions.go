// plugins_interactions.go adapts kandev's first-party interaction response
// paths to the narrow interface the plugin Host interaction API needs (ADR
// 0052). It lives in backendapp for the same reason the task-write adapters
// do: internal/plugins cannot import the orchestrator or the clarification
// handler without an import cycle.
//
// Nothing here re-implements a response. Permissions go through the same
// orchestrator method the WS `permission.respond` action calls; clarification
// bundles go through the same clarification.Handlers instance the REST route
// serves — including its durable exclusive claim, its live-waiter delivery,
// and its detached-resume fallback. The adapter's only real work is
// translating the REST-shaped failure vocabulary into gRPC status codes a
// plugin can branch on.
package backendapp

import (
	"context"
	"errors"
	"net/http"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/plugins"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// permissionResponder is the orchestrator slice the permission path needs.
type permissionResponder interface {
	RespondToPermission(ctx context.Context, sessionID, pendingID, optionID string, cancelled, rejected bool) error
}

// clarificationResponder is the clarification-handler slice the question-bundle
// path needs.
type clarificationResponder interface {
	Respond(ctx context.Context, pendingID string, body clarification.RespondBody) error
}

// pluginsInteractionResponderAdapter satisfies internal/plugins'
// interactionResponder.
type pluginsInteractionResponderAdapter struct {
	permissions    permissionResponder
	clarifications clarificationResponder
}

func (a pluginsInteractionResponderAdapter) RespondToPermission(
	ctx context.Context, sessionID, pendingID, optionID string, cancelled, rejected bool,
) error {
	if a.permissions == nil {
		return status.Error(codes.Unimplemented, "permission responses are unavailable")
	}
	err := a.permissions.RespondToPermission(ctx, sessionID, pendingID, optionID, cancelled, rejected)
	var resolved *orchestrator.PermissionAlreadyResolvedError
	if errors.As(err, &resolved) {
		// Lost the durable claim to another responder. Same code the host
		// returns when it detects the terminal state itself, so a plugin
		// branches on one outcome however the race was caught.
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return err
}

func (a pluginsInteractionResponderAdapter) AnswerClarification(
	ctx context.Context, pendingID string, answers []plugins.PluginClarificationAnswer,
) error {
	if a.clarifications == nil {
		return status.Error(codes.Unimplemented, "clarification responses are unavailable")
	}
	body := clarification.RespondBody{Answers: make([]clarification.Answer, len(answers))}
	for i, answer := range answers {
		body.Answers[i] = clarification.Answer{
			QuestionID:      answer.QuestionID,
			SelectedOptions: answer.SelectedOptions,
			CustomText:      answer.CustomText,
		}
	}
	return interactionResponseStatus(a.clarifications.Respond(ctx, pendingID, body))
}

// DeclineClarification routes a cancel through the REJECT path rather than
// clarification.Cancel. Cancel needs the in-memory pending entry, so it only
// works while the original waiter is still parked; the reject path is
// durable-claim based and therefore also settles a bundle whose waiter went
// away in a restart — precisely the case a reconciling plugin is holding.
func (a pluginsInteractionResponderAdapter) DeclineClarification(
	ctx context.Context, pendingID, reason string,
) error {
	if a.clarifications == nil {
		return status.Error(codes.Unimplemented, "clarification responses are unavailable")
	}
	return interactionResponseStatus(a.clarifications.Respond(ctx, pendingID, clarification.RespondBody{
		Rejected:     true,
		RejectReason: reason,
	}))
}

// interactionResponseStatus maps the clarification handler's REST-shaped
// outcome onto the gRPC vocabulary the interaction contract promises: a
// conflict (another responder claimed the bundle first, or it is no longer
// active) becomes FailedPrecondition, matching what the plugin host returns
// when it detects the terminal state itself, so a plugin branches on one code
// regardless of which layer caught the race.
func interactionResponseStatus(err error) error {
	var respondErr *clarification.RespondError
	if !errors.As(err, &respondErr) {
		return err
	}
	switch respondErr.Status {
	case http.StatusBadRequest:
		return status.Error(codes.InvalidArgument, respondErr.Message)
	case http.StatusNotFound:
		return status.Error(codes.NotFound, respondErr.Message)
	case http.StatusConflict:
		return status.Error(codes.FailedPrecondition, respondErr.Message)
	default:
		return status.Error(codes.Internal, respondErr.Message)
	}
}

// ClaimPermissionResponse atomically resolves a pending permission row so only
// one responder dispatches to the agent (ADR 0052). It hangs off the shared
// message-creator adapter but lives here rather than in adapters.go: the claim
// exists for this contract, and adapters.go is already past the file-length
// limit.
func (a *messageCreatorAdapter) ClaimPermissionResponse(
	ctx context.Context, sessionID, pendingID string, status taskmodels.PermissionStatus,
) (bool, taskmodels.PermissionStatus, error) {
	return a.svc.ClaimPermissionResponse(ctx, sessionID, pendingID, status)
}
