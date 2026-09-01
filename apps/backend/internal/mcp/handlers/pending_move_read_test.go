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

type recordingPendingMoveReader struct {
	actor  messagequeue.PendingMoveCancellationActor
	taskID string
	result *messagequeue.PendingMoveCensusResult
	err    error
	calls  int
}

func (r *recordingPendingMoveReader) ReadPendingMove(
	_ context.Context,
	actor messagequeue.PendingMoveCancellationActor,
	taskID string,
	_ string,
) (*messagequeue.PendingMoveCensusResult, error) {
	r.calls++
	r.actor = actor
	r.taskID = taskID
	return r.result, r.err
}

func readPendingMovePayload() map[string]interface{} {
	return map[string]interface{}{
		"task_id": "33333333-3333-4333-8333-333333333333",
	}
}

func TestHandleReadPendingMoveUsesOnlyServerAttestedActor(t *testing.T) {
	reader := &recordingPendingMoveReader{result: &messagequeue.PendingMoveCensusResult{Found: false}}
	h := &Handlers{pendingMoveReader: reader, logger: testLogger(t)}
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		AutomationID:      "automation-1",
		WorkspaceID:       "88888888-8888-4888-8888-888888888888",
		CallerTaskID:      "99999999-9999-4999-8999-999999999999",
		CallerSessionID:   "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		CallerExecutionID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		Surface:           mcpprofile.SurfaceAutomation,
	})
	ctx = authn.WithIdentity(ctx, authn.Identity{UserID: "owner-1"})
	msg := makeWSMessage(t, ws.ActionMCPReadPendingMove, readPendingMovePayload())

	resp, err := h.handleReadPendingMove(ctx, msg)
	if err != nil {
		t.Fatalf("handleReadPendingMove: %v", err)
	}
	if resp.Type != ws.MessageTypeResponse || reader.calls != 1 {
		t.Fatalf("response=%#v calls=%d", resp, reader.calls)
	}
	if reader.actor.CallerTaskID != "99999999-9999-4999-8999-999999999999" ||
		reader.actor.WorkspaceID != "88888888-8888-4888-8888-888888888888" ||
		reader.actor.CallerExecutionID != "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" ||
		reader.actor.UserID != "owner-1" {
		t.Fatalf("actor was not server-derived: %#v", reader.actor)
	}
	if reader.taskID != "33333333-3333-4333-8333-333333333333" {
		t.Fatalf("taskID = %q", reader.taskID)
	}
}

func TestHandleReadPendingMoveReturnsFoundFalseAsSuccess(t *testing.T) {
	reader := &recordingPendingMoveReader{result: &messagequeue.PendingMoveCensusResult{
		Found: false, TaskID: "33333333-3333-4333-8333-333333333333",
	}}
	h := &Handlers{pendingMoveReader: reader, logger: testLogger(t)}
	msg := makeWSMessage(t, ws.ActionMCPReadPendingMove, readPendingMovePayload())

	resp, err := h.handleReadPendingMove(context.Background(), msg)
	if err != nil {
		t.Fatalf("handleReadPendingMove: %v", err)
	}
	if resp.Type != ws.MessageTypeResponse {
		t.Fatalf("found=false census returned an error response: %#v", resp)
	}
}

func TestHandleReadPendingMoveRejectsAndAuditsCallerFields(t *testing.T) {
	reader := &recordingPendingMoveReader{err: messagequeue.ErrPendingMoveInvalidArgument}
	h := &Handlers{pendingMoveReader: reader, logger: testLogger(t)}
	payload := readPendingMovePayload()
	payload["caller_task_id"] = "forged-task"
	payload["workspace_id"] = "forged-workspace"
	msg := makeWSMessage(t, ws.ActionMCPReadPendingMove, payload)

	resp, err := h.handleReadPendingMove(context.Background(), msg)
	if err != nil {
		t.Fatalf("handleReadPendingMove: %v", err)
	}
	assertWSError(t, resp, PendingMoveInvalidArgumentCode)
	if reader.calls != 1 || reader.taskID != "" {
		t.Fatalf("invalid caller fields were not audited safely: calls=%d taskID=%q", reader.calls, reader.taskID)
	}
}

func TestHandleReadPendingMoveNilReaderIsInternal(t *testing.T) {
	h := &Handlers{logger: testLogger(t)}
	msg := makeWSMessage(t, ws.ActionMCPReadPendingMove, readPendingMovePayload())

	resp, err := h.handleReadPendingMove(context.Background(), msg)
	if err != nil {
		t.Fatalf("handleReadPendingMove: %v", err)
	}
	assertWSError(t, resp, PendingMoveReadFailedCode)
}

func TestHandleReadPendingMoveDeniesOrdinaryAndSyntheticCallersIdentically(t *testing.T) {
	for _, name := range []string{"ordinary", "synthetic"} {
		t.Run(name, func(t *testing.T) {
			reader := &recordingPendingMoveReader{err: messagequeue.ErrPendingMoveNotFoundOrChanged}
			h := &Handlers{pendingMoveReader: reader, logger: testLogger(t)}
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
			msg := makeWSMessage(t, ws.ActionMCPReadPendingMove, readPendingMovePayload())
			resp, err := h.handleReadPendingMove(ctx, msg)
			if err != nil {
				t.Fatalf("handleReadPendingMove: %v", err)
			}
			assertWSError(t, resp, PendingMoveNotFoundOrChangedCode)
			if reader.calls != 1 {
				t.Fatalf("reader calls=%d, want denial audit call", reader.calls)
			}
		})
	}
}

func TestHandleReadPendingMoveMapsStableErrors(t *testing.T) {
	cases := map[string]struct {
		err  error
		code string
	}{
		"invalid": {err: messagequeue.ErrPendingMoveInvalidArgument, code: PendingMoveInvalidArgumentCode},
		"changed": {err: messagequeue.ErrPendingMoveNotFoundOrChanged, code: PendingMoveNotFoundOrChangedCode},
		"failed":  {err: messagequeue.ErrPendingMoveReadFailed, code: PendingMoveReadFailedCode},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			reader := &recordingPendingMoveReader{err: tc.err}
			h := &Handlers{pendingMoveReader: reader, logger: testLogger(t)}
			msg := makeWSMessage(t, ws.ActionMCPReadPendingMove, readPendingMovePayload())
			resp, err := h.handleReadPendingMove(context.Background(), msg)
			if err != nil && !errors.Is(err, tc.err) {
				t.Fatalf("handler error=%v", err)
			}
			assertWSError(t, resp, tc.code)
		})
	}
}
