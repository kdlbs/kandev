package backendapp

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/plugins"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type recordingPermissionResponder struct {
	sessionID, pendingID, optionID string
	cancelled, rejected            bool
	err                            error
}

func (r *recordingPermissionResponder) RespondToPermission(
	_ context.Context, sessionID, pendingID, optionID string, cancelled, rejected bool,
) error {
	r.sessionID, r.pendingID, r.optionID = sessionID, pendingID, optionID
	r.cancelled, r.rejected = cancelled, rejected
	return r.err
}

type recordingClarificationResponder struct {
	pendingID string
	body      clarification.RespondBody
	err       error
}

func (r *recordingClarificationResponder) Respond(
	_ context.Context, pendingID string, body clarification.RespondBody,
) error {
	r.pendingID, r.body = pendingID, body
	return r.err
}

func TestPluginsInteractionResponderForwardsPermission(t *testing.T) {
	permissions := &recordingPermissionResponder{}
	adapter := pluginsInteractionResponderAdapter{permissions: permissions}

	if err := adapter.RespondToPermission(
		context.Background(), "session-1", "pending-1", "deny", false, true,
	); err != nil {
		t.Fatalf("RespondToPermission: %v", err)
	}
	if permissions.sessionID != "session-1" || permissions.pendingID != "pending-1" ||
		permissions.optionID != "deny" || permissions.cancelled || !permissions.rejected {
		t.Fatalf("orchestrator saw %+v", permissions)
	}
}

func TestPluginsInteractionResponderForwardsClarificationAnswers(t *testing.T) {
	clarifications := &recordingClarificationResponder{}
	adapter := pluginsInteractionResponderAdapter{clarifications: clarifications}

	err := adapter.AnswerClarification(context.Background(), "pending-2", []plugins.PluginClarificationAnswer{
		{QuestionID: "q1", SelectedOptions: []string{"a"}, CustomText: "note"},
	})
	if err != nil {
		t.Fatalf("AnswerClarification: %v", err)
	}
	if clarifications.pendingID != "pending-2" || len(clarifications.body.Answers) != 1 {
		t.Fatalf("clarification handler saw %+v", clarifications)
	}
	answer := clarifications.body.Answers[0]
	if answer.QuestionID != "q1" || answer.CustomText != "note" || len(answer.SelectedOptions) != 1 {
		t.Fatalf("answer = %+v", answer)
	}
	if clarifications.body.Rejected {
		t.Fatal("an answer must not be delivered as a rejection")
	}
}

// TestPluginsInteractionResponderDeclineUsesRejectPath pins the deliberate
// routing choice: Cancel needs the in-memory pending entry and therefore fails
// after a restart, while the reject path is durable-claim based. A plugin
// reconciling a bundle whose waiter went away is exactly the caller that
// needs the second one.
func TestPluginsInteractionResponderDeclineUsesRejectPath(t *testing.T) {
	clarifications := &recordingClarificationResponder{}
	adapter := pluginsInteractionResponderAdapter{clarifications: clarifications}

	if err := adapter.DeclineClarification(context.Background(), "pending-3", "user stepped away"); err != nil {
		t.Fatalf("DeclineClarification: %v", err)
	}
	if !clarifications.body.Rejected || clarifications.body.RejectReason != "user stepped away" {
		t.Fatalf("decline body = %+v", clarifications.body)
	}
	if len(clarifications.body.Answers) != 0 {
		t.Fatalf("decline carried answers: %+v", clarifications.body.Answers)
	}
}

func TestPluginsInteractionResponderMapsClarificationOutcomes(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want codes.Code
	}{
		{"malformed answer set", &clarification.RespondError{Status: http.StatusBadRequest, Message: "bad"}, codes.InvalidArgument},
		{"unknown bundle", &clarification.RespondError{Status: http.StatusNotFound, Message: "gone"}, codes.NotFound},
		// A conflict means another responder claimed the bundle first. It must
		// land on the SAME code the plugin host returns when it catches the
		// terminal state itself, so a plugin branches on one outcome.
		{"lost the claim race", &clarification.RespondError{Status: http.StatusConflict, Message: "inactive"}, codes.FailedPrecondition},
		{"server failure", &clarification.RespondError{Status: http.StatusInternalServerError, Message: "boom"}, codes.Internal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adapter := pluginsInteractionResponderAdapter{
				clarifications: &recordingClarificationResponder{err: tc.in},
			}
			err := adapter.DeclineClarification(context.Background(), "pending-4", "")
			if got := status.Code(err); got != tc.want {
				t.Fatalf("code = %v (%v), want %v", got, err, tc.want)
			}
		})
	}
}

func TestPluginsInteractionResponderPassesThroughNonRespondErrors(t *testing.T) {
	sentinel := errors.New("transport exploded")
	adapter := pluginsInteractionResponderAdapter{
		clarifications: &recordingClarificationResponder{err: sentinel},
	}
	if err := adapter.DeclineClarification(context.Background(), "pending-5", ""); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want the original error preserved", err)
	}
}

func TestPluginsInteractionResponderUnwiredIsUnimplemented(t *testing.T) {
	adapter := pluginsInteractionResponderAdapter{}
	ctx := context.Background()

	if got := status.Code(adapter.RespondToPermission(ctx, "s", "p", "o", false, false)); got != codes.Unimplemented {
		t.Fatalf("permission code = %v", got)
	}
	if got := status.Code(adapter.AnswerClarification(ctx, "p", nil)); got != codes.Unimplemented {
		t.Fatalf("answer code = %v", got)
	}
	if got := status.Code(adapter.DeclineClarification(ctx, "p", "")); got != codes.Unimplemented {
		t.Fatalf("decline code = %v", got)
	}
}
