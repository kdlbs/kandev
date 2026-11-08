package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/common/sshlaunch"
)

// errSSHRemoteSessionDirectoryMissing marks a verified controller whose
// session directory is positively gone. It is the only verification failure
// that may select the directory-independent reconciliation route; every other
// failure keeps the existing refusal.
var errSSHRemoteSessionDirectoryMissing = errors.New("remote agentctl session directory disappeared before pidfile verification")

var (
	// errSSHLaunchIdentityMissing reports a live owner with no recorded proof.
	// It is actionable (an operator must reconcile the host) but never grants
	// authority.
	errSSHLaunchIdentityMissing = errors.New("ssh: launch identity is missing")
	// errSSHLaunchIdentityConflict reports a receipt that disagrees with the
	// recorded owner or the recorded PID.
	errSSHLaunchIdentityConflict = errors.New("ssh: launch identity conflicts with the recorded owner")
	// errSSHLaunchReconciliationUnresolved reports a helper result that could
	// not settle the recorded owner.
	errSSHLaunchReconciliationUnresolved = errors.New("ssh: controller reconciliation unresolved")
)

// maxSSHReconcileResponseBytes bounds the helper response so a broken or
// hostile remote cannot stream unbounded data into the backend.
const maxSSHReconcileResponseBytes = 32 * 1024

// reconcileLostSSHSessionDirectory is the directory-loss route. It requires a
// complete receipt for this exact owner and a helper that can re-prove the
// process before signalling. Anything less keeps the recorded owner.
func (r *SSHExecutor) reconcileLostSSHSessionDirectory(
	ctx context.Context,
	client *ssh.Client,
	instance *ExecutorInstance,
	pid int,
	verifyErr error,
) error {
	receipt, err := persistedSSHLaunchReceipt(instance.Metadata, instance.TaskID, instance.SessionID)
	if err != nil {
		return fmt.Errorf("%w: %w", errSSHLaunchIdentityConflict, err)
	}
	if receipt == nil {
		return fmt.Errorf("%w for instance %q: %w", errSSHLaunchIdentityMissing, instance.InstanceID, verifyErr)
	}
	if receipt.Process.PID != pid {
		return fmt.Errorf("%w: receipt pid %d does not match recorded pid %d",
			errSSHLaunchIdentityConflict, receipt.Process.PID, pid)
	}
	result := r.reconcileRecordedController(ctx, client, sshReconcileRequest(*receipt))
	if err := result.Validate(); err != nil {
		return fmt.Errorf("%w: malformed helper result: %w", errSSHLaunchReconciliationUnresolved, err)
	}
	switch result.Outcome {
	case sshlaunch.OutcomeVerified:
		return fmt.Errorf("%w: verify result cannot settle a terminate request", errSSHLaunchReconciliationUnresolved)
	case sshlaunch.OutcomeStopped:
		r.logger.Info("stopped SSH controller after session-directory loss",
			zap.String("instance_id", instance.InstanceID),
			zap.Int("pid", pid))
		return nil
	case sshlaunch.OutcomeAbsent:
		r.logger.Info("SSH controller was already gone after session-directory loss",
			zap.String("instance_id", instance.InstanceID),
			zap.Int("pid", pid))
		return nil
	default:
		return fmt.Errorf("%w: %s (%s)", errSSHLaunchReconciliationUnresolved, result.Reason, result.Detail)
	}
}

// verifyTrackedControllerBeforeStop revalidates a live tracked session's
// directory ownership and launch proof immediately before a destructive stop.
// It returns handled=true when the directory-loss helper already settled the
// controller, so the caller must skip its numeric stop.
func (r *SSHExecutor) verifyTrackedControllerBeforeStop(
	ctx context.Context,
	client *ssh.Client,
	instance *ExecutorInstance,
	state *sshSessionState,
) (bool, error) {
	if state == nil || state.launchReceipt == nil || client == nil {
		return false, nil
	}
	verify := r.verifyRemoteIdentity
	if verify == nil {
		verify = verifyRemoteAgentctlIdentity
	}
	isOurs, err := verify(ctx, client, state.pid, state.remoteDir, state.remoteTaskDir)
	if err != nil {
		if errors.Is(err, errSSHRemoteSessionDirectoryMissing) {
			return true, r.reconcileLostSSHSessionDirectory(ctx, client, instance, state.pid, err)
		}
		return false, fmt.Errorf("ssh: verify tracked agentctl identity: %w", err)
	}
	if !isOurs {
		return false, fmt.Errorf("%w: tracked session directory no longer matches the controller",
			errSSHLaunchIdentityConflict)
	}
	if err := r.verifyRecordedControllerIdentity(ctx, client, *state.launchReceipt); err != nil {
		return false, err
	}
	return false, nil
}

// verifyRecordedControllerIdentity re-proves a receipt-bearing controller
// before a legacy destructive branch or a recovery adoption. Only a verified
// live process permits the caller to proceed.
func (r *SSHExecutor) verifyRecordedControllerIdentity(
	ctx context.Context,
	client *ssh.Client,
	receipt sshlaunch.Receipt,
) error {
	result := r.reconcileRecordedController(ctx, client, sshReconcileVerifyRequest(receipt))
	if err := result.Validate(); err != nil {
		return fmt.Errorf("%w: malformed helper result: %w", errSSHLaunchReconciliationUnresolved, err)
	}
	if result.Outcome != sshlaunch.OutcomeVerified {
		return fmt.Errorf("%w: %s (%s)", errSSHLaunchReconciliationUnresolved, result.Reason, result.Detail)
	}
	return nil
}

// reconcileRecordedController dispatches one helper request and normalizes a
// missing seam into an unresolved result.
func (r *SSHExecutor) reconcileRecordedController(
	ctx context.Context,
	client *ssh.Client,
	request sshlaunch.ReconcileRequest,
) sshlaunch.ReconcileResult {
	reconcile := r.reconcileRemote
	if reconcile == nil {
		return sshlaunch.Unresolved(sshlaunch.ReasonUnsupportedIdentity, "no verified remote helper is available")
	}
	return reconcile(ctx, client, request)
}

// reconcileRemoteController resolves the verified helper for the remote
// platform through the existing resolver/upload path and runs the private
// reconcile command. The dispatch is synchronous and belongs to the current
// stop call, so it shares that call's cleanup deadline.
func (r *SSHExecutor) reconcileRemoteController(
	ctx context.Context,
	client *ssh.Client,
	req sshlaunch.ReconcileRequest,
) sshlaunch.ReconcileResult {
	if r.agentctlResolver == nil {
		return sshlaunch.Unresolved(sshlaunch.ReasonUnsupportedIdentity, "remote helper resolver is unavailable")
	}
	info, err := detectRemoteInfo(ctx, client)
	if err != nil {
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "remote platform detection failed")
	}
	if err := requireSupportedRemotePlatform(info.Platform); err != nil {
		return sshlaunch.Unresolved(sshlaunch.ReasonUnsupportedIdentity, "remote platform does not support reconciliation")
	}
	bin, err := ensureAgentctlOnHostWithProgress(ctx, ctx, client, r.agentctlResolver, info.Platform, r.logger, nil)
	if err != nil {
		return sshlaunch.Unresolved(sshlaunch.ReasonUnsupportedIdentity, "verified remote helper is unavailable")
	}
	return runRemoteSSHReconcile(ctx, client, bin, req)
}

// runRemoteSSHReconcile invokes `agentctl ssh-reconcile` and parses its bounded
// JSON response. A transport fault, lost reply, oversized body, or malformed
// output is unknown, never success.
func runRemoteSSHReconcile(
	ctx context.Context,
	client *ssh.Client,
	bin string,
	req sshlaunch.ReconcileRequest,
) sshlaunch.ReconcileResult {
	body, err := json.Marshal(req)
	if err != nil {
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "reconcile request could not be encoded")
	}
	stdout, _, overflow, err := runSSHCommandStdinBounded(
		ctx, client, shellQuote(bin)+" ssh-reconcile", bytes.NewReader(body), maxSSHReconcileResponseBytes,
	)
	if overflow {
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "reconcile helper response exceeded its bound")
	}
	if err != nil {
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "reconcile helper did not complete")
	}
	var result sshlaunch.ReconcileResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "reconcile helper returned malformed output")
	}
	if err := result.Validate(); err != nil {
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "reconcile helper returned an invalid result")
	}
	return result
}
