package executor

import (
	"context"
	"fmt"
)

// PrepareSessionStopForExecution pins cancellation to the requested execution.
// The caller releases its cancellation guard before awaited runtime teardown.
func (e *Executor) PrepareSessionStopForExecution(ctx context.Context, sessionID, expectedExecutionID, reason string) (SessionStopResult, error) {
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return SessionStopResult{}, err
	}
	if session == nil {
		return SessionStopResult{}, ErrExecutionNotFound
	}
	current, err := e.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if err != nil {
		return SessionStopResult{}, err
	}
	if expectedExecutionID == "" || current != expectedExecutionID {
		return SessionStopResult{}, fmt.Errorf("awaited stop execution identity changed")
	}
	return e.stopSession(ctx, session, reason, false, expectedExecutionID)
}
