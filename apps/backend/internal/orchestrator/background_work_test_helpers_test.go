package orchestrator

import "github.com/kandev/kandev/internal/agentctl/types/streams"

func attestedSubagentPayload(description, prompt, subagentType string) *streams.NormalizedPayload {
	payload := streams.NewSubagentTask(description, prompt, subagentType)
	payload.SetBackgroundWorkIdentity(
		streams.BackgroundWorkKindSubagent,
		"test-subagent",
		false,
		false,
	)
	return payload
}

func attestedBackgroundShellPayload(command string) *streams.NormalizedPayload {
	payload := streams.NewShellExec(command, "", "", 0, true)
	payload.SetBackgroundWorkIdentity(streams.BackgroundWorkKindShell, "", true, false)
	return payload
}

// registerBackgroundTask is a test-only convenience wrapper around
// registerBackgroundWork for tests that don't care about the execution/work
// ID correlation (production call sites always pass those explicitly — see
// registerBackgroundWork's doc comment).
func (s *Service) registerBackgroundTask(sessionID, toolCallID string) {
	s.registerBackgroundWork(sessionID, toolCallID, "", "")
}

// completeBackgroundTask is a test-only convenience wrapper around
// completeBackgroundTaskForExecution for tests that don't care about the
// execution-scoped completion (production call sites always pass the
// execution ID explicitly — see completeBackgroundTaskForExecution's caller
// in event_handlers_streaming.go).
func (s *Service) completeBackgroundTask(sessionID, toolCallID string) bool {
	return s.completeBackgroundTaskForExecution(sessionID, toolCallID, "")
}

// dispatchAndAcceptForegroundClaim drives the real two-step production
// sequence promptTask uses to hand an admitted background-idle claim to the
// agent: beginForegroundDispatch establishes the prompt cycle, and
// acceptForegroundDispatch closes the admission window once agentctl
// acknowledges the prompt (see task_operations.go's promptTask). Tests use
// this instead of the removed completeForegroundClaim so they exercise the
// same production path rather than a test-only shortcut.
func (s *Service) dispatchAndAcceptForegroundClaim(sessionID string, claim *foregroundClaim) bool {
	return s.acceptForegroundDispatch(s.beginForegroundDispatch(sessionID, claim))
}
