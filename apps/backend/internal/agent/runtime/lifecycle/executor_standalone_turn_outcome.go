package lifecycle

import (
	"context"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// fetchTurnOutcomeWithRetry retrieves instanceID's retained terminal turn
// outcome within the same bounded per-attempt timeout and retry count as
// listInstancesWithRetry/stopWithRetry (AC-EXECUTORS-SURVIVAL-004.5 requires
// the same bound as AC-EXECUTORS-SURVIVAL-002.13). Returns (nil, nil) when
// the read succeeded and nothing was retained -- ControlClient.GetTurnOutcome
// already distinguishes that from a failure, and this method preserves the
// distinction rather than treating it as an error to retry.
func (r *StandaloneExecutor) fetchTurnOutcomeWithRetry(ctx context.Context, instanceID string) (*agentctl.TurnOutcome, error) {
	return r.fetchTurnOutcomeForEpoch(ctx, instanceID, 0)
}

func (r *StandaloneExecutor) fetchTurnOutcomeForEpoch(ctx context.Context, instanceID string, runtimeEpoch uint64) (*agentctl.TurnOutcome, error) {
	control, controlCtx, release, lease, err := r.acquireControl(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if runtimeEpoch != 0 && lease != nil && lease.Epoch() != runtimeEpoch {
		return nil, agentctl.ErrRuntimeLeaseRetired
	}
	if runtimeEpoch != 0 && lease == nil && r.runtimeOwner != nil {
		return nil, agentctl.ErrRuntimeLeaseRetired
	}
	timeout := r.recoveryReadTimeout
	if timeout <= 0 {
		timeout = defaultRecoveryReadTimeout
	}
	retries := r.recoveryReadRetries
	if retries < 0 {
		retries = defaultRecoveryReadRetries
	}

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		attemptCtx, cancel := context.WithTimeout(controlCtx, timeout)
		outcome, err := control.GetTurnOutcome(attemptCtx, instanceID)
		cancel()
		if err == nil {
			return outcome, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// ackTurnOutcome acknowledges instanceID's retained outcome by turn
// identifier, discarding it (AC-EXECUTORS-SURVIVAL-004.6). Bounded by a
// single attempt at the same per-attempt timeout as the read: the
// acknowledgement is idempotent and safe to retry (naming an identifier the
// control server no longer holds changes nothing), so a failed ack here is
// left for the next backend's re-application of the same outcome rather than
// retried in a loop that would delay this recovery pass.
func (r *StandaloneExecutor) ackTurnOutcome(ctx context.Context, instanceID string, turnID int64) error {
	return r.ackTurnOutcomeForEpoch(ctx, instanceID, turnID, 0)
}

func (r *StandaloneExecutor) ackTurnOutcomeForEpoch(ctx context.Context, instanceID string, turnID int64, runtimeEpoch uint64) error {
	control, controlCtx, release, lease, err := r.acquireControl(ctx)
	if err != nil {
		return err
	}
	defer release()
	if runtimeEpoch != 0 && lease != nil && lease.Epoch() != runtimeEpoch {
		return agentctl.ErrRuntimeLeaseRetired
	}
	if runtimeEpoch != 0 && lease == nil && r.runtimeOwner != nil {
		return agentctl.ErrRuntimeLeaseRetired
	}
	timeout := r.recoveryReadTimeout
	if timeout <= 0 {
		timeout = defaultRecoveryReadTimeout
	}
	attemptCtx, cancel := context.WithTimeout(controlCtx, timeout)
	defer cancel()
	return control.AckTurnOutcome(attemptCtx, instanceID, turnID)
}
