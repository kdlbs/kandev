package orchestrator

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
// lock to LSP admission so a retained lease cannot race idle cleanup.
func (s *Service) AcquireSessionLifecycleFence(sessionID string) func() {
	return s.acquireSessionLifecycleLock(sessionID)
}
