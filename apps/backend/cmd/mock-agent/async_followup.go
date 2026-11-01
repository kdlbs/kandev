package main

import (
	"context"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// emitAsyncFollowup reproduces the recovered seq 1173 ordering: a prompt turn
// ends normally, then an agent-started follow-up emits output and a
// metadata-only tool_call_update for a tool that already reported completion.
// The status-less update must not keep the follow-up turn open.
func emitAsyncFollowup(e *emitter) {
	e.text("Foreground response before the async follow-up.")
	e.startTool("async-followup-check", "Check follow-up", acp.ToolKindRead, nil)
	e.completeTool("async-followup-check", "follow-up check complete")

	background := &emitter{ctx: context.Background(), conn: e.conn, sid: e.sid}
	go func() {
		time.Sleep(time.Second)
		// The provider's task-notification boundary opens the agent-started
		// follow-up turn.
		background.completeDetachedWork()
		background.text("Background follow-up started.")
		background.startTool("async-followup-work", "Follow-up work", acp.ToolKindOther, nil)

		time.Sleep(time.Second)
		// PostToolUse metadata-only frame for the tool that already finished
		// in the foreground turn.
		background.metadataOnlyToolUpdate("async-followup-check")

		time.Sleep(time.Second)
		background.completeTool("async-followup-work", "follow-up work complete")
		background.text("Background follow-up finished.")

		// Keep the follow-up turn open a little longer so the browser flow has a
		// deterministic window to queue a message while it runs.
		time.Sleep(2 * time.Second)
		background.text("Background follow-up still running.")
	}()
}

// metadataOnlyToolUpdate emits the status-less tool_call_update Claude's
// PostToolUse hook publishes: no status, title, input, content, or locations,
// and no later terminal frame for the same tool.
func (e *emitter) metadataOnlyToolUpdate(id acp.ToolCallId) {
	_ = e.conn.SessionUpdate(e.ctx, acp.SessionNotification{
		SessionId: e.sid,
		Update:    acp.UpdateToolCall(id),
	})
}
