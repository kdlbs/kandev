// plugins_interactions.go adapts kandev's first-party interaction response
// paths to the narrow interface the plugin Host interaction API needs (ADR
// 0052). It lives in backendapp for the same reason the task-write adapters
// do: internal/plugins cannot import the orchestrator or the clarification
// resolver without an import cycle.
//
// Nothing here re-implements a response. Permissions go through
// orchestrator.ResolveAgentPermission — the claim/deliver/finalize path the
// web and external-MCP surfaces share, which owns the durable claim that keeps
// two responders from both reaching the agent. Clarification bundles go
// through the same clarification.Resolver instance the REST route and the MCP
// handlers use. The adapter's only real work is translating each path's error
// vocabulary into the gRPC codes the plugin contract promises.
package backendapp

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/plugins"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// permissionResolver is the orchestrator slice the permission path needs.
// ResolveAgentPermission is used rather than the RespondToPermission wrapper
// so the plugin's responses are attributed to their own source rather than
// silently recorded as web.
type permissionResolver interface {
	ResolveAgentPermission(
		ctx context.Context, request orchestrator.ResolveAgentPermissionRequest,
	) (*orchestrator.ResolveAgentPermissionResult, error)
	// RespondToPermission is the exported wrapper that owns the cancel path;
	// the orchestrator keeps its cancel helper unexported.
	RespondToPermission(
		ctx context.Context, taskID, sessionID, requestID, pendingID, optionID string, cancelled, rejected bool,
	) error
}

// clarificationBundleResolver is the clarification-resolver slice the
// question-bundle path needs.
type clarificationBundleResolver interface {
	ResolveBundle(
		ctx context.Context, pendingID string, outcome clarification.Outcome,
	) (*clarification.Resolution, bool, error)
}

// pluginsInteractionResponderAdapter satisfies internal/plugins'
// interactionResponder.
type pluginsInteractionResponderAdapter struct {
	permissions    permissionResolver
	clarifications clarificationBundleResolver
}

func (a pluginsInteractionResponderAdapter) RespondToPermission(
	ctx context.Context, in plugins.PluginPermissionResponse,
) error {
	if a.permissions == nil {
		return status.Error(codes.Unimplemented, "permission responses are unavailable")
	}
	// A dismissal has no option to select, and the orchestrator keeps its
	// cancel helper unexported, so route it through the exported wrapper
	// rather than inventing an option id the agent never offered.
	if in.Cancelled {
		return permissionResolutionStatus(a.permissions.RespondToPermission(
			ctx, in.TaskID, in.SessionID, in.RequestID, in.PendingID, "", true, false))
	}
	_, err := a.permissions.ResolveAgentPermission(ctx, orchestrator.ResolveAgentPermissionRequest{
		TaskID:    in.TaskID,
		SessionID: in.SessionID,
		RequestID: in.RequestID,
		PendingID: in.PendingID,
		OptionID:  in.OptionID,
		Source:    taskmodels.PermissionSourceAutomation,
	})
	return permissionResolutionStatus(err)
}

func (a pluginsInteractionResponderAdapter) AnswerClarification(
	ctx context.Context, pendingID string, answers []plugins.PluginClarificationAnswer,
) error {
	if a.clarifications == nil {
		return status.Error(codes.Unimplemented, "clarification responses are unavailable")
	}
	outcome := clarification.Outcome{Answers: make([]clarification.Answer, len(answers))}
	for i, answer := range answers {
		outcome.Answers[i] = clarification.Answer{
			QuestionID:      answer.QuestionID,
			SelectedOptions: answer.SelectedOptions,
			CustomText:      answer.CustomText,
		}
	}
	return a.resolveBundle(ctx, pendingID, outcome)
}

// DeclineClarification routes a cancel through the resolver's REJECT outcome
// rather than the operator cancel route. Cancel needs the in-memory pending
// entry, so it only works while the original waiter is still parked; the
// reject outcome is durable-claim based and therefore also settles a bundle
// whose waiter went away in a restart, which is precisely the case a
// reconciling plugin is holding.
func (a pluginsInteractionResponderAdapter) DeclineClarification(
	ctx context.Context, pendingID, reason string,
) error {
	if a.clarifications == nil {
		return status.Error(codes.Unimplemented, "clarification responses are unavailable")
	}
	return a.resolveBundle(ctx, pendingID, clarification.Outcome{
		Rejected:     true,
		RejectReason: reason,
	})
}

// resolveBundle maps the resolver's outcome onto the plugin contract. A
// non-claimed resolution means another responder settled the bundle first,
// which is the same FailedPrecondition the host returns when it detects the
// terminal state itself — so a plugin branches on one code however the race
// was caught.
func (a pluginsInteractionResponderAdapter) resolveBundle(
	ctx context.Context, pendingID string, outcome clarification.Outcome,
) error {
	_, claimed, err := a.clarifications.ResolveBundle(ctx, pendingID, outcome)
	if err != nil {
		return clarificationResolutionStatus(err)
	}
	if !claimed {
		return status.Errorf(codes.FailedPrecondition,
			"clarification %s was already resolved by another responder", pendingID)
	}
	return nil
}

func clarificationResolutionStatus(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, clarification.ErrBundleNotFound):
		return status.Error(codes.NotFound, err.Error())
	case clarification.IsValidationError(err):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

// permissionResolutionStatus maps the orchestrator's permission vocabulary
// onto the plugin contract. Everything that means "this interaction can no
// longer take the response you submitted" collapses to FailedPrecondition,
// which is what terminal-once promises; an unknown identity is NotFound, and
// an option the agent never offered is the caller's mistake.
func permissionResolutionStatus(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, orchestrator.ErrPermissionNotFound),
		errors.Is(err, orchestrator.ErrPermissionTaskOrSessionNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, orchestrator.ErrPermissionOptionNotOffered):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, orchestrator.ErrPermissionAlreadyResolved),
		errors.Is(err, orchestrator.ErrPermissionStale),
		errors.Is(err, orchestrator.ErrPermissionResolutionInProgress):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return err
	}
}
