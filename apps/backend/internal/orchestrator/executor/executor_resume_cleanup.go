package executor

import "context"

// SetOnStaleExecutionCleanup runs after cleanup succeeds and before a successor
// launches. The resume caller holds the session lifecycle lock during the call.
func (e *Executor) SetOnStaleExecutionCleanup(fn func(context.Context, string)) {
	e.onStaleExecutionCleanup = fn
}

func (e *Executor) cleanupStaleExecutionBeforeResume(ctx context.Context, sessionID string) error {
	if err := e.agentManager.CleanupStaleExecutionBySessionID(ctx, sessionID); err != nil {
		return err
	}
	if e.onStaleExecutionCleanup != nil {
		e.onStaleExecutionCleanup(ctx, sessionID)
	}
	return nil
}
