package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/agent/executor"
)

// lockAgentctlLifecycleWithin takes the write side of agentctlLifecycleMu, or
// gives up when ctx ends first. A queued writer is still granted the lock
// later, so a caller that gives up releases it from the background. Read
// leases pin the client across HTTP calls that a frozen agentctl holds for the
// client timeout, and an unbounded writer would wait out all of them.
func (ae *AgentExecution) lockAgentctlLifecycleWithin(ctx context.Context) bool {
	acquired := make(chan struct{})
	go func() {
		ae.agentctlLifecycleMu.Lock()
		close(acquired)
	}()
	select {
	case <-acquired:
		return true
	case <-ctx.Done():
		go func() {
			<-acquired
			ae.agentctlLifecycleMu.Unlock()
		}()
		return false
	}
}

// closeAgentctlWhenIdle closes the current agentctl client once the read leases
// drain, for a stop that could not take the write side in time.
func (ae *AgentExecution) closeAgentctlWhenIdle() {
	go func() {
		ae.agentctlLifecycleMu.Lock()
		defer ae.agentctlLifecycleMu.Unlock()
		if client := ae.currentAgentCtlClient(); client != nil {
			client.Close()
		}
	}()
}

// detachAgentctlAfterStop detaches the agentctl client of a stopped execution.
// A plugin_remote runtime waits for the write side only within the graceful
// stop budget and finishes the detach once the read leases drain; the
// execution is already out of the stop path by then. Other runtimes wait.
func (ae *AgentExecution) detachAgentctlAfterStop(ctx context.Context) {
	if ae.RuntimeName == executor.NamePluginRemote {
		waitCtx, cancel := pluginRemoteGracefulStopContext(ctx, ae.RuntimeName)
		defer cancel()
		if !ae.lockAgentctlLifecycleWithin(waitCtx) {
			go func() {
				ae.agentctlLifecycleMu.Lock()
				ae.detachAgentctlClient()
				ae.agentctlLifecycleMu.Unlock()
			}()
			return
		}
	} else {
		ae.agentctlLifecycleMu.Lock()
	}
	ae.detachAgentctlClient()
	ae.agentctlLifecycleMu.Unlock()
}
