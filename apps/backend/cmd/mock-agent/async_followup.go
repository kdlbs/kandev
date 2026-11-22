package main

import (
	"context"
	"fmt"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// emitAsyncFollowup reproduces the recovered seq 1173 ordering: a prompt turn
// ends normally, then an agent-started follow-up emits output and a
// metadata-only tool_call_update for a tool that already reported completion.
// The status-less update must not keep the follow-up turn open.
func emitAsyncFollowup(e *emitter, releasePath string) {
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
		if !waitForAsyncFollowupRelease(background, releasePath) {
			return
		}
		background.text("Background follow-up still running.")
		background.completeTool("async-followup-work", "follow-up work complete")
		background.text("Background follow-up finished.")
	}()
}

// waitForAsyncFollowupRelease keeps the tool active until the browser confirms
// queue admission. Manual scenarios without a marker use a bounded delay.
func waitForAsyncFollowupRelease(e *emitter, path string) bool {
	if path == "" {
		time.Sleep(15 * time.Second)
		return true
	}
	ctx, cancel := context.WithTimeout(e.ctx, 2*time.Minute)
	defer cancel()
	if err := waitForScriptFile(ctx, path); err != nil {
		// Release failure retains the active tool and blocks queue delivery.
		e.text(fmt.Sprintf("Async follow-up release failed: %v", err))
		return false
	}
	return true
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
