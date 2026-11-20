package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/common/sshlaunch"
)

func trackedStopFixture(t *testing.T) (*SSHExecutor, *ssh.Client, *ExecutorInstance, *sshSessionState) {
	t.Helper()
	owner := testSSHLaunchOwner()
	receipt := testSSHLaunchReceipt(t, owner, 4242)
	server := newFakeSSHServer(t, nil)
	state := &sshSessionState{
		pid:           4242,
		remoteDir:     owner.SessionDir,
		remoteTaskDir: owner.TaskDir,
		launchReceipt: &receipt,
	}
	instance := &ExecutorInstance{
		InstanceID: owner.ControllerInstanceID,
		TaskID:     owner.TaskID,
		SessionID:  owner.SessionID,
		Metadata:   testSSHLaunchMetadata(receipt),
	}
	return NewSSHExecutor(nil, nil, nil, newTestLogger()), server.dial(t), instance, state
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.20
func TestSSHTrackedStopRevalidation(t *testing.T) {
	t.Run("conflicting live proof refuses the numeric stop", func(t *testing.T) {
		exec, client, instance, state := trackedStopFixture(t)
		exec.verifyRemoteIdentity = func(context.Context, *ssh.Client, int, string, string) (bool, error) {
			return true, nil
		}
		exec.reconcileRemote = func(context.Context, *ssh.Client, sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
			return sshlaunch.Unresolved(sshlaunch.ReasonIdentityConflict, "recorded process identity changed")
		}
		handled, err := exec.verifyTrackedControllerBeforeStop(context.Background(), client, instance, state)
		require.False(t, handled)
		require.ErrorIs(t, err, errSSHLaunchReconciliationUnresolved)
	})

	t.Run("directory ownership mismatch refuses", func(t *testing.T) {
		exec, client, instance, state := trackedStopFixture(t)
		exec.verifyRemoteIdentity = func(context.Context, *ssh.Client, int, string, string) (bool, error) {
			return false, nil
		}
		handled, err := exec.verifyTrackedControllerBeforeStop(context.Background(), client, instance, state)
		require.False(t, handled)
		require.ErrorIs(t, err, errSSHLaunchIdentityConflict)
	})

	t.Run("missing directory dispatches pidfd termination", func(t *testing.T) {
		exec, client, instance, state := trackedStopFixture(t)
		exec.verifyRemoteIdentity = func(context.Context, *ssh.Client, int, string, string) (bool, error) {
			return false, errSSHRemoteSessionDirectoryMissing
		}
		exec.reconcileRemote = func(context.Context, *ssh.Client, sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
			return sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeStopped}
		}
		handled, err := exec.verifyTrackedControllerBeforeStop(context.Background(), client, instance, state)
		require.NoError(t, err)
		require.True(t, handled, "the helper result must suppress the numeric stop")
	})
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.19
func TestSSHTrackedStopRevalidationAllowsVerifiedLegacyStop(t *testing.T) {
	exec, client, instance, state := trackedStopFixture(t)
	exec.verifyRemoteIdentity = func(context.Context, *ssh.Client, int, string, string) (bool, error) {
		return true, nil
	}
	exec.reconcileRemote = func(context.Context, *ssh.Client, sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
		return sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeVerified}
	}
	handled, err := exec.verifyTrackedControllerBeforeStop(context.Background(), client, instance, state)
	require.NoError(t, err)
	require.False(t, handled, "a verified controller keeps the guarded legacy stop")
}
