package executor

import (
	"context"

	"go.uber.org/zap"
)

// CeilingBackingChecker answers, for a session_id, whether the orchestrator's
// session-ceiling admission controller currently backs it with an in-flight
// reservation or a counted AC-1 population row (AC-41). It is the single
// permitted route by which this package observes the controller: the
// reservation set lives in package orchestrator, which already imports this
// package, so the dependency runs the other way and this interface is
// declared here rather than the check being read directly.
type CeilingBackingChecker interface {
	IsSessionCeilingBacked(ctx context.Context, sessionID string) (bool, error)
}

// auditCeilingBypass is AC-4b's bypass detector for the three entry points
// that start an agent process: runAgentProcessAsync (the sole ERROR site,
// AC-4b1), LaunchPreparedSession and ResumeSessionWithOptions (DEBUG only).
// AC-41a: with no checker injected — every existing construction site,
// including all of this package's tests — this is a complete no-op: no
// detection, no log, no error, no behaviour change. AC-41b: a lookup failure
// (the AC-1 row half is a repository read and can fail) emits no ERROR for
// that launch, logs the failure at DEBUG, and never blocks, retries or delays
// the launch either way — the audit only ever observes.
func (e *Executor) auditCeilingBypass(ctx context.Context, entryPoint, sessionID string, emitError bool, extra ...zap.Field) {
	if e.ceilingBackingChecker == nil || sessionID == "" {
		return
	}
	backed, err := e.ceilingBackingChecker.IsSessionCeilingBacked(ctx, sessionID)
	if err != nil {
		e.logger.Debug("could not determine whether a launch is backed by the session ceiling",
			zap.String("session_id", sessionID), zap.String("entry_point", entryPoint), zap.Error(err))
		return
	}
	if backed {
		return
	}
	fields := append([]zap.Field{zap.String("session_id", sessionID), zap.String("entry_point", entryPoint)}, extra...)
	if emitError {
		e.logger.Error("agent process launch reached an entry point with no session-ceiling reservation or counted row", fields...)
		return
	}
	e.logger.Debug("agent process launch reached an entry point with no session-ceiling reservation or counted row", fields...)
}
