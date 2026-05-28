package acp

import (
	"context"
	"time"
)

// syntheticToolCallRaceWindow bounds how long handlePermissionRequest waits
// for a concurrently-dispatched ToolCall notification to populate
// activeToolCalls before falling back to emitting a synthetic tool_call. Sized
// to cover the goroutine-scheduling gap between the SDK delivering a
// SessionUpdate.ToolCall and a request_permission for the same toolCallID,
// without adding noticeable latency to permission prompts.
const syntheticToolCallRaceWindow = 100 * time.Millisecond

// waitForActiveToolCall polls activeToolCalls for the given id, sleeping in
// small increments, and returns true if the entry appears within timeout.
// Returns false when the wait expires without the entry materializing — the
// expected outcome when the agent really did skip the tool_call notification.
// Honors ctx so a cancelled session aborts the poll loop immediately rather
// than blocking the handler for the full timeout window.
func (a *Adapter) waitForActiveToolCall(ctx context.Context, toolCallID string, timeout time.Duration) bool {
	const pollInterval = 10 * time.Millisecond
	deadline := time.Now().Add(timeout)
	for {
		a.mu.RLock()
		_, tracked := a.activeToolCalls[toolCallID]
		a.mu.RUnlock()
		if tracked {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(pollInterval):
		}
	}
}
