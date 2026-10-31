package backendapp

import agentruntime "github.com/kandev/kandev/internal/agent/runtime"

// RuntimeStorageObservation forwards the lifecycle-owned advisory projection
// so the orchestrator's optional provider assertion works in production.
func (a *lifecycleAdapter) RuntimeStorageObservation(
	sessionID string,
) (agentruntime.RuntimeStorageObservation, bool) {
	if a == nil || a.mgr == nil || sessionID == "" {
		return agentruntime.RuntimeStorageObservation{}, false
	}
	return a.mgr.RuntimeStorageObservation(sessionID)
}
