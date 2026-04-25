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

// plan sends an ACP Plan update with the provided entries.
func (e *emitter) plan(entries []acp.PlanEntry) {
	_ = e.conn.SessionUpdate(e.ctx, acp.SessionNotification{
		SessionId: e.sid,
		Update:    acp.UpdatePlan(entries...),
	})
}

// monitorClaudeMeta builds the `_meta.claudeCode.toolName=Monitor` payload
// claude-agent-acp tags Monitor tool_call notifications with. Used by
// e2e:monitor_* directives to reproduce the wire format exactly so the
// kandev ACP adapter's Monitor recognizers fire the same way they do in
// production.
func monitorClaudeMeta() any {
	return map[string]any{"claudeCode": map[string]any{"toolName": "Monitor"}}
}

// monitorClaudeMetaWithTask is like monitorClaudeMeta but also embeds a
// `toolResponse.taskId` field — claude-agent-acp emits this on the
// registration update.
func monitorClaudeMetaWithTask(taskID string) any {
	return map[string]any{
		"claudeCode": map[string]any{
			"toolName":     "Monitor",
			"toolResponse": map[string]any{"taskId": taskID},
		},
	}
}

// startMonitorTool emits the initial Monitor tool_call (status=pending) and
// then a registration tool_call_update whose rawOutput banner carries the
// taskID. This reproduces the two-frame wire pattern the kandev adapter
// expects: a recognizable banner is the only signal it has that an apparent
// "completed" really means "registered".
//
// The acp-go-sdk does not expose `WithStartMeta` / `WithUpdateMeta` helpers
// at the time of writing, so we set Meta via a local option closure.
func (e *emitter) startMonitorTool(id acp.ToolCallId, taskID, command string) {
	withStartMeta := func(meta any) acp.ToolCallStartOpt {
		return func(tc *acp.SessionUpdateToolCall) { tc.Meta = toMetaMap(meta) }
	}
	withUpdateMeta := func(meta any) acp.ToolCallUpdateOpt {
		return func(tu *acp.SessionToolCallUpdate) { tu.Meta = toMetaMap(meta) }
	}
	_ = e.conn.SessionUpdate(e.ctx, acp.SessionNotification{
		SessionId: e.sid,
		Update: acp.StartToolCall(id, "Monitor",
			acp.WithStartKind(acp.ToolKindOther),
			acp.WithStartStatus(acp.ToolCallStatusPending),
			acp.WithStartRawInput(map[string]any{"command": command}),
			withStartMeta(monitorClaudeMeta()),
		),
	})
	banner := fmt.Sprintf("Monitor started (task %s, timeout 60000ms). You will be notified on each event.", taskID)
	_ = e.conn.SessionUpdate(e.ctx, acp.SessionNotification{
		SessionId: e.sid,
		Update: acp.UpdateToolCall(id,
			acp.WithUpdateStatus(acp.ToolCallStatusCompleted),
			acp.WithUpdateRawOutput(banner),
			withUpdateMeta(monitorClaudeMetaWithTask(taskID)),
		),
	})
}

// toMetaMap is a small adapter so the local meta builders can return `any`
// (matching the kandev-side recognizer signature) while the SDK's Meta field
// requires `map[string]any` specifically.
func toMetaMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// emitMonitorEvent reproduces the model-echoed task-notification envelope
// that fires when a real Monitor's stdout produces a line. The kandev
// adapter parses these out of agent_message_chunks and routes them back to
// the originating Monitor's tool_call card.
func (e *emitter) emitMonitorEvent(taskID, body string) {
	envelope := fmt.Sprintf(
		"Human: <task-notification>\n<task-id>%s</task-id>\n<event>%s</event>\n</task-notification>",
		taskID, body,
	)
	_ = e.conn.SessionUpdate(e.ctx, acp.SessionNotification{
		SessionId: e.sid,
		Update:    acp.UpdateAgentMessageText(envelope),
	})
}

// endMonitorTool emits the terminal Monitor tool_call_update — the kandev
// adapter would normally synthesize this on its own at prompt-end, but tests
// drive it explicitly so they don't have to wait for the prompt to finish.
func (e *emitter) endMonitorTool(id acp.ToolCallId) {
	withUpdateMeta := func(meta any) acp.ToolCallUpdateOpt {
		return func(tu *acp.SessionToolCallUpdate) { tu.Meta = toMetaMap(meta) }
	}
	_ = e.conn.SessionUpdate(e.ctx, acp.SessionNotification{
		SessionId: e.sid,
		Update: acp.UpdateToolCall(id,
			acp.WithUpdateStatus(acp.ToolCallStatusCompleted),
			acp.WithUpdateRawOutput("Monitor exited"),
			withUpdateMeta(monitorClaudeMeta()),
		),
	})
}

// requestPermission asks the client for permission to proceed with a tool call.
// Returns true if permission was granted, false otherwise.
func (e *emitter) requestPermission(toolCallID acp.ToolCallId, title string, kind acp.ToolKind, input any) bool {
	resp, err := e.conn.RequestPermission(e.ctx, acp.RequestPermissionRequest{
		SessionId: e.sid,
		ToolCall: acp.ToolCallUpdate{
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
