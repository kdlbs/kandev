package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/plugins"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type recordingPermissionResolver struct {
	request   orchestrator.ResolveAgentPermissionRequest
	cancelled bool
	err       error
}

func (r *recordingPermissionResolver) ResolveAgentPermission(
	_ context.Context, request orchestrator.ResolveAgentPermissionRequest,
) (*orchestrator.ResolveAgentPermissionResult, error) {
	r.request = request
	if r.err != nil {
		return nil, r.err
	}
	return &orchestrator.ResolveAgentPermissionResult{PendingID: request.PendingID}, nil
}

func (r *recordingPermissionResolver) RespondToPermission(
	_ context.Context, taskID, sessionID, requestID, pendingID, optionID string, cancelled, _ bool,
) error {
	r.request = orchestrator.ResolveAgentPermissionRequest{
		TaskID: taskID, SessionID: sessionID, RequestID: requestID,
		PendingID: pendingID, OptionID: optionID,
	}
	r.cancelled = cancelled
	return r.err
}

type recordingBundleResolver struct {
	pendingID string
	outcome   clarification.Outcome
	claimed   bool
	err       error
}

func (r *recordingBundleResolver) ResolveBundle(
	_ context.Context, pendingID string, outcome clarification.Outcome,
) (*clarification.Resolution, bool, error) {
	r.pendingID, r.outcome = pendingID, outcome
	if r.err != nil {
		return nil, false, r.err
	}
	return &clarification.Resolution{}, r.claimed, nil
}

func TestPluginsInteractionResponderForwardsPermissionIdentity(t *testing.T) {
	permissions := &recordingPermissionResolver{}
	adapter := pluginsInteractionResponderAdapter{permissions: permissions}

	if err := adapter.RespondToPermission(context.Background(), plugins.PluginPermissionResponse{
		TaskID: "task-1", SessionID: "session-1", RequestID: "request-1",
		PendingID: "pending-1", OptionID: "deny",
	}); err != nil {
		t.Fatalf("RespondToPermission: %v", err)
	}
	if permissions.request.TaskID != "task-1" || permissions.request.SessionID != "session-1" ||
		permissions.request.RequestID != "request-1" || permissions.request.PendingID != "pending-1" ||
		permissions.request.OptionID != "deny" {
		t.Fatalf("resolver saw %+v", permissions.request)
	}
	// A plugin response must not be recorded as a browser one: the audit trail
	// distinguishes sources, and mislabelling it would make a plugin's action
	// indistinguishable from a person's after the fact.
	if permissions.request.Source == taskmodels.PermissionSourceWeb {
		t.Fatalf("source = %q, want a non-web source for a plugin response", permissions.request.Source)
	}
}

// TestPluginsInteractionResponderDismissalUsesCancelPath keeps a dismissal off
// the option path: there is no option to select, and inventing one would
// report an outcome the agent never offered.
func TestPluginsInteractionResponderDismissalUsesCancelPath(t *testing.T) {
	permissions := &recordingPermissionResolver{}
	adapter := pluginsInteractionResponderAdapter{permissions: permissions}

	if err := adapter.RespondToPermission(context.Background(), plugins.PluginPermissionResponse{
		TaskID: "task-1", SessionID: "session-1", RequestID: "request-1",
		PendingID: "pending-1", Cancelled: true,
	}); err != nil {
		t.Fatalf("RespondToPermission: %v", err)
	}
	if !permissions.cancelled {
		t.Fatal("dismissal did not reach the cancel path")
	}
	if permissions.request.OptionID != "" {
		t.Fatalf("option = %q, want empty on a dismissal", permissions.request.OptionID)
	}
}

func TestPluginsInteractionResponderMapsPermissionOutcomes(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want codes.Code
	}{
		{"unknown permission", orchestrator.ErrPermissionNotFound, codes.NotFound},
		{"unknown task or session", orchestrator.ErrPermissionTaskOrSessionNotFound, codes.NotFound},
		{"option never offered", orchestrator.ErrPermissionOptionNotOffered, codes.InvalidArgument},
		// Everything that means "this can no longer take your response"
		// collapses to the one code terminal-once promises.
		{"already resolved", orchestrator.ErrPermissionAlreadyResolved, codes.FailedPrecondition},
		{"stale", orchestrator.ErrPermissionStale, codes.FailedPrecondition},
		{"another responder mid-flight", orchestrator.ErrPermissionResolutionInProgress, codes.FailedPrecondition},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adapter := pluginsInteractionResponderAdapter{
				permissions: &recordingPermissionResolver{err: tc.in},
			}
			err := adapter.RespondToPermission(context.Background(), plugins.PluginPermissionResponse{
				PendingID: "pending-1", OptionID: "allow",
			})
			if got := status.Code(err); got != tc.want {
				t.Fatalf("code = %v (%v), want %v", got, err, tc.want)
			}
		})
	}
}

func TestPluginsInteractionResponderForwardsClarificationAnswers(t *testing.T) {
	resolver := &recordingBundleResolver{claimed: true}
	adapter := pluginsInteractionResponderAdapter{clarifications: resolver}

	err := adapter.AnswerClarification(context.Background(), "pending-2", []plugins.PluginClarificationAnswer{
		{QuestionID: "q1", SelectedOptions: []string{"a"}, CustomText: "note"},
	})
	if err != nil {
		t.Fatalf("AnswerClarification: %v", err)
	}
	if resolver.pendingID != "pending-2" || len(resolver.outcome.Answers) != 1 {
		t.Fatalf("resolver saw %q / %+v", resolver.pendingID, resolver.outcome)
	}
	if resolver.outcome.Rejected {
		t.Fatal("an answer must not be delivered as a rejection")
	}
}

// TestPluginsInteractionResponderDeclineUsesRejectOutcome pins the deliberate
// routing choice: the operator cancel route needs the in-memory pending entry
// and therefore fails after a restart, while the reject outcome is
// durable-claim based. A plugin reconciling a bundle whose waiter went away is
// exactly the caller that needs the second one.
func TestPluginsInteractionResponderDeclineUsesRejectOutcome(t *testing.T) {
	resolver := &recordingBundleResolver{claimed: true}
	adapter := pluginsInteractionResponderAdapter{clarifications: resolver}

	if err := adapter.DeclineClarification(context.Background(), "pending-3", "user stepped away"); err != nil {
		t.Fatalf("DeclineClarification: %v", err)
	}
	if !resolver.outcome.Rejected || resolver.outcome.RejectReason != "user stepped away" {
		t.Fatalf("decline outcome = %+v", resolver.outcome)
	}
	if len(resolver.outcome.Answers) != 0 {
		t.Fatalf("decline carried answers: %+v", resolver.outcome.Answers)
	}
}

// TestPluginsInteractionResponderLostClaimIsFailedPrecondition covers the race
// the resolver reports as a non-claimed resolution: another responder settled
// the bundle first. It must read as the same code the host returns when it
// catches the terminal state itself.
func TestPluginsInteractionResponderLostClaimIsFailedPrecondition(t *testing.T) {
	adapter := pluginsInteractionResponderAdapter{
		clarifications: &recordingBundleResolver{claimed: false},
	}
	err := adapter.DeclineClarification(context.Background(), "pending-4", "")
	if got := status.Code(err); got != codes.FailedPrecondition {
		t.Fatalf("code = %v (%v), want FailedPrecondition", got, err)
	}
}

func TestPluginsInteractionResponderMapsClarificationOutcomes(t *testing.T) {
	adapter := pluginsInteractionResponderAdapter{
		clarifications: &recordingBundleResolver{err: clarification.ErrBundleNotFound},
	}
	if got := status.Code(adapter.DeclineClarification(context.Background(), "pending-5", "")); got != codes.NotFound {
		t.Fatalf("unknown bundle code = %v, want NotFound", got)
	}

	sentinel := errors.New("transport exploded")
	adapter = pluginsInteractionResponderAdapter{
		clarifications: &recordingBundleResolver{err: sentinel},
	}
	if got := status.Code(adapter.DeclineClarification(context.Background(), "pending-6", "")); got != codes.Internal {
		t.Fatalf("unexpected failure code = %v, want Internal", got)
	}
}

func TestPluginsInteractionResponderUnwiredIsUnimplemented(t *testing.T) {
	adapter := pluginsInteractionResponderAdapter{}
	ctx := context.Background()

	if got := status.Code(adapter.RespondToPermission(ctx, plugins.PluginPermissionResponse{})); got != codes.Unimplemented {
		t.Fatalf("permission code = %v", got)
	}
	if got := status.Code(adapter.AnswerClarification(ctx, "p", nil)); got != codes.Unimplemented {
		t.Fatalf("answer code = %v", got)
	}
	if got := status.Code(adapter.DeclineClarification(ctx, "p", "")); got != codes.Unimplemented {
		t.Fatalf("decline code = %v", got)
	}
}
