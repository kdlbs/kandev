package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type recordingPendingMoveCanceller struct {
	actor  messagequeue.PendingMoveCancellationActor
	match  messagequeue.ExactPendingMoveMatch
	result *messagequeue.PendingMoveCancellationResult
	err    error
	calls  int
}

func (c *recordingPendingMoveCanceller) ExactCancelPendingMove(
	_ context.Context,
	actor messagequeue.PendingMoveCancellationActor,
	match messagequeue.ExactPendingMoveMatch,
	_ string,
) (*messagequeue.PendingMoveCancellationResult, error) {
	c.calls++
	c.actor = actor
	c.match = match
	return c.result, c.err
}

func exactCancelPayload() map[string]interface{} {
	return map[string]interface{}{
		"pending_move_id":                   "11111111-1111-4111-8111-111111111111",
		"session_id":                        "22222222-2222-4222-8222-222222222222",
		"task_id":                           "33333333-3333-4333-8333-333333333333",
		"move_id":                           "44444444-4444-4444-8444-444444444444",
		"workflow_id":                       "55555555-5555-4555-8555-555555555555",
		"expected_current_workflow_step_id": "66666666-6666-4666-8666-666666666666",
		"expected_target_workflow_step_id":  "77777777-7777-4777-8777-777777777777",
	}
}

func TestHandleCancelPendingMoveUsesOnlyServerAttestedActor(t *testing.T) {
	canceller := &recordingPendingMoveCanceller{result: &messagequeue.PendingMoveCancellationResult{Cancelled: true}}
	h := &Handlers{pendingMoveCanceller: canceller, logger: testLogger(t)}
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		AutomationID:      "automation-1",
		WorkspaceID:       "88888888-8888-4888-8888-888888888888",
		CallerTaskID:      "99999999-9999-4999-8999-999999999999",
		CallerSessionID:   "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		CallerExecutionID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		Surface:           mcpprofile.SurfaceAutomation,
	})
	ctx = authn.WithIdentity(ctx, authn.Identity{UserID: "owner-1"})
	payload := exactCancelPayload()
	payload["caller_task_id"] = "forged-task"
	payload["workspace_id"] = "forged-workspace"
	msg := makeWSMessage(t, ws.ActionMCPCancelPendingMove, payload)

	resp, err := h.handleCancelPendingMove(ctx, msg)
	if err != nil {
		t.Fatalf("handleCancelPendingMove: %v", err)
	}
	if resp.Type != ws.MessageTypeResponse || canceller.calls != 1 {
		t.Fatalf("response=%#v calls=%d", resp, canceller.calls)
	}
	if canceller.actor.CallerTaskID != "99999999-9999-4999-8999-999999999999" ||
		canceller.actor.WorkspaceID != "88888888-8888-4888-8888-888888888888" ||
		canceller.actor.CallerExecutionID != "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" ||
		canceller.actor.UserID != "owner-1" {
		t.Fatalf("actor was not server-derived: %#v", canceller.actor)
	}
}

func TestHandleCancelPendingMoveDeniesOrdinaryAndSyntheticCallersIdentically(t *testing.T) {
	for _, name := range []string{"ordinary", "synthetic"} {
		t.Run(name, func(t *testing.T) {
			canceller := &recordingPendingMoveCanceller{err: messagequeue.ErrPendingMoveNotFoundOrChanged}
			h := &Handlers{pendingMoveCanceller: canceller, logger: testLogger(t)}
			ctx := context.Background()
			if name == "ordinary" {
				ctx = mcpscope.WithPrincipal(ctx, mcpscope.Principal{
					WorkspaceID:       "88888888-8888-4888-8888-888888888888",
					CallerTaskID:      "99999999-9999-4999-8999-999999999999",
					CallerSessionID:   "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
					CallerExecutionID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
					Surface:           mcpprofile.SurfaceKanbanTask,
				})
				ctx = authn.WithIdentity(ctx, authn.Identity{UserID: "owner-1"})
			} else {
				ctx = authn.WithIdentity(ctx, authn.Identity{UserID: "default", Synthetic: true})
			}
			msg := makeWSMessage(t, ws.ActionMCPCancelPendingMove, exactCancelPayload())
			resp, err := h.handleCancelPendingMove(ctx, msg)
			if err != nil {
				t.Fatalf("handleCancelPendingMove: %v", err)
			}
			assertWSError(t, resp, PendingMoveNotFoundOrChangedCode)
			if canceller.calls != 1 {
				t.Fatalf("canceller calls=%d, want denial audit call", canceller.calls)
			}
		})
	}
}

func TestHandleCancelPendingMoveMapsStableErrors(t *testing.T) {
	cases := map[string]struct {
		err  error
		code string
	}{
		"invalid": {err: messagequeue.ErrPendingMoveInvalidArgument, code: PendingMoveInvalidArgumentCode},
		"changed": {err: messagequeue.ErrPendingMoveNotFoundOrChanged, code: PendingMoveNotFoundOrChangedCode},
		"failed":  {err: messagequeue.ErrPendingMoveCancelFailed, code: PendingMoveCancelFailedCode},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			canceller := &recordingPendingMoveCanceller{err: tc.err}
			h := &Handlers{pendingMoveCanceller: canceller, logger: testLogger(t)}
			msg := makeWSMessage(t, ws.ActionMCPCancelPendingMove, exactCancelPayload())
			resp, err := h.handleCancelPendingMove(context.Background(), msg)
			if err != nil && !errors.Is(err, tc.err) {
				t.Fatalf("handler error=%v", err)
			}
			assertWSError(t, resp, tc.code)
		})
	}
}
