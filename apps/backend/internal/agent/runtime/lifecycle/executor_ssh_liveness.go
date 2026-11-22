package lifecycle

import (
	"context"
	"errors"
	"time"
)

// ConfirmAgentctlAbsent uses only the process and connection owned by the
// tracked instance. Transport failure or replacement cannot establish absence.
func (r *SSHExecutor) ConfirmAgentctlAbsent(ctx context.Context, instanceID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	state := r.sessions[instanceID]
	if state == nil || state.client == nil || state.pid <= 0 ||
		classifySSHTransportLocked(state) != sshTransportAnswering {
		r.mu.Unlock()
		return false, errors.New("SSH agentctl process identity or transport is unavailable")
	}
	client, pid := state.client, state.pid
	r.mu.Unlock()

	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	alive, err := probeRemoteAgentctlLiveness(probeCtx, client, pid)
	if err != nil || alive {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions[instanceID] != state || state.client != client || state.pid != pid ||
		classifySSHTransportLocked(state) != sshTransportAnswering {
		return false, errors.New("SSH agentctl instance changed during liveness probe")
	}
	if err := probeCtx.Err(); err != nil {
		return false, err
	}
	return true, nil
}
