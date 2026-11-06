package lifecycle

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"github.com/kandev/kandev/internal/task/models"
)

// ReconnectDeliveryInstance stages a same-process transport without starting work.
func (r *SSHExecutor) ReconnectDeliveryInstance(ctx context.Context, instance *ExecutorInstance) (*RemoteInstanceRefresh, error) {
	if instance == nil || instance.RuntimeName != r.Name() {
		return nil, ErrDeliveryOwnerMismatch
	}
	r.mu.Lock()
	prior := r.sessions[instance.InstanceID]
	lost := prior != nil && prior.transportLost
	r.mu.Unlock()
	if prior == nil {
		return nil, ErrDeliveryTransportUnavailable
	}
	if !lost {
		return nil, nil
	}
	record := &models.ExecutorRunning{
		SessionID: instance.SessionID, TaskID: instance.TaskID, AgentExecutionID: instance.InstanceID,
		Runtime: r.Name(), Metadata: cloneSSHMetadata(instance.Metadata), TransientAuthToken: instance.AuthToken,
	}
	record.PID, _ = strconv.Atoi(getMetadataString(record.Metadata, MetadataKeySSHRemoteAgentctlPID))
	identity, ok := sshRecoveryIdentity(record)
	if !ok || prior.pid != identity.pid || prior.remoteDir != identity.sessionDir || prior.remoteTaskDir != identity.taskDir || prior.authToken != identity.authToken {
		return nil, ErrDeliveryOwnerMismatch
	}
	candidate, state, outcome := r.prepareSSHRecovery(ctx, record)
	if candidate == nil {
		return nil, errors.Join(ErrDeliveryTransportUnavailable, errors.New(string(outcome)))
	}
	return r.stagedSSHDeliveryRefresh(ctx, instance.InstanceID, prior, candidate, state), nil
}

func (r *SSHExecutor) stagedSSHDeliveryRefresh(ctx context.Context, id string, prior *sshSessionState, instance *ExecutorInstance, candidate *sshSessionState) *RemoteInstanceRefresh {
	var mu sync.Mutex
	finished := false
	abort := func() {
		mu.Lock()
		defer mu.Unlock()
		if !finished {
			finished = true
			r.discardRecoveredSSHState(id, candidate)
		}
	}
	return &RemoteInstanceRefresh{Instance: instance, Abort: abort, Commit: func(publish func()) error {
		mu.Lock()
		defer mu.Unlock()
		if finished {
			return ErrDeliveryOwnerMismatch
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		r.mu.Lock()
		if r.sessions[id] != prior || !prior.transportLost {
			r.mu.Unlock()
			return ErrDeliveryOwnerMismatch
		}
		r.sessions[id] = candidate
		r.startWatchdogLocked(id, candidate)
		publish()
		finished = true
		r.mu.Unlock()
		r.discardRecoveredSSHState(id, prior)
		return nil
	}}
}
