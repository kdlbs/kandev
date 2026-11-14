package orchestrator

import "context"

// LSPLeaseLifecycle is the narrow runtime bridge needed to keep retained
// language servers alive through idle reclaim and stop them with their task.
type LSPLeaseLifecycle interface {
	HasActiveLSPLease(sessionID string) bool
	HasActiveLSPLeaseForExecution(executionID string) bool
	StopLSPLeasesForSession(sessionID string)
	StopLSPLeasesForTask(taskID string)
	StopLSPLeasesForExecution(executionID string)
}

// SetLSPLeaseLifecycle wires the optional browser-independent language-server
// runtime. Nil preserves legacy idle-reclaim behavior.
func (s *Service) SetLSPLeaseLifecycle(lifecycle LSPLeaseLifecycle) {
	s.lspLeases = lifecycle
}

// AcquireSessionLifecycleFence exposes the existing per-session lifecycle
// lock to LSP and workspace-process admission so a retained lease or a newly
// started command cannot race idle cleanup.
func (s *Service) AcquireSessionLifecycleFence(sessionID string) func() {
	return s.acquireSessionLifecycleLock(sessionID)
}

// AcquireSessionLifecycleFenceContext is AcquireSessionLifecycleFence for a
// request-scoped caller: it stops waiting and reports false once ctx ends, so a
// canceled process start does not stay blocked behind a slow reclaim.
func (s *Service) AcquireSessionLifecycleFenceContext(ctx context.Context, sessionID string) (func(), bool) {
	if sessionID == "" {
		return func() {}, true
	}
	return s.acquireSessionLifecycleLockWithContext(ctx, sessionID)
}
