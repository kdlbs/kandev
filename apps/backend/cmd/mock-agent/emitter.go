package main

import (
	"context"
	"fmt"

	acp "github.com/coder/acp-go-sdk"
)

// sessionUpdater abstracts the ACP connection methods used by the emitter.
// The real acp.AgentSideConnection satisfies this; tests provide a mock.
type sessionUpdater interface {
	SessionUpdate(ctx context.Context, n acp.SessionNotification) error
	RequestPermission(ctx context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error)
}

// emitter wraps an ACP connection and session ID to provide
// convenient methods for streaming agent updates.
type emitter struct {
	ctx  context.Context
	conn sessionUpdater
	sid  acp.SessionId
}

// text sends an agent text message update.
func (e *emitter) text(msg string) {
	_ = e.conn.SessionUpdate(e.ctx, acp.SessionNotification{
		SessionId: e.sid,
		Update:    acp.UpdateAgentMessageText(msg),
	})
}

// thought sends an agent thinking/reasoning update.
func (e *emitter) thought(msg string) {
	_ = e.conn.SessionUpdate(e.ctx, acp.SessionNotification{
		SessionId: e.sid,
		Update:    acp.UpdateAgentThoughtText(msg),
	})
}

// startTool announces a new tool call.
func (e *emitter) startTool(id acp.ToolCallId, title string, kind acp.ToolKind, input any, locs ...acp.ToolCallLocation) {
	opts := []acp.ToolCallStartOpt{
		acp.WithStartKind(kind),
		acp.WithStartStatus(acp.ToolCallStatusPending),
		acp.WithStartRawInput(input),
	}
	if len(locs) > 0 {
		opts = append(opts, acp.WithStartLocations(locs))
	}
	_ = e.conn.SessionUpdate(e.ctx, acp.SessionNotification{
		SessionId: e.sid,
		Update:    acp.StartToolCall(id, title, opts...),
	})
}

// completeTool marks a tool call as completed with output.
func (e *emitter) completeTool(id acp.ToolCallId, output any) {
	_ = e.conn.SessionUpdate(e.ctx, acp.SessionNotification{
		SessionId: e.sid,
		Update: acp.UpdateToolCall(id,
			acp.WithUpdateStatus(acp.ToolCallStatusCompleted),
			acp.WithUpdateRawOutput(output),
		),
	})
}

// requestPermission asks the client for permission to proceed with a tool call.
// Returns true if permission was granted, false otherwise.
func (e *emitter) requestPermission(toolCallID acp.ToolCallId, title string, kind acp.ToolKind, input any) bool {
	resp, err := e.conn.RequestPermission(e.ctx, acp.RequestPermissionRequest{
		SessionId: e.sid,
		ToolCall: acp.RequestPermissionToolCall{
			ToolCallId: toolCallID,
			Title:      acp.Ptr(title),
			Kind:       acp.Ptr(kind),
			Status:     acp.Ptr(acp.ToolCallStatusPending),
			RawInput:   input,
		},
		Options: []acp.PermissionOption{
			{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow", OptionId: "allow"},
			{Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject", OptionId: "reject"},
		},
	})
	if err != nil {
		_, _ = fmt.Fprintf(logOutput, "mock-agent: permission request failed: %v\n", err)
		return false
	}
	return resp.Outcome.Selected != nil && string(resp.Outcome.Selected.OptionId) == "allow"
}
