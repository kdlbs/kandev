package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/common/sshlaunch"
	"github.com/kandev/kandev/internal/githubauth"
)

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.9
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.18
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.22
func TestSSHBrokerResumeConfirmedAbsentController(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		name := "untracked"
		if tracked {
			name = "tracked"
		}
		t.Run(name, func(t *testing.T) {
			owner := testSSHLaunchOwner()
			server := newFakeSSHServer(t, nil)
			receipt := testSSHLaunchReceiptForServer(t, server, 4242)
			metadata := sshLaunchStopMetadata(t, server, receipt)
			metadata[MetadataKeySSHRemoteAgentctlPort] = "43123"
			metadata[MetadataKeySSHWorkdirRoot] = "/home/kandev/.kandev"

			exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
			preflightCalls, verifyCalls, stopCalls, closeCalls := 0, 0, 0, 0
			exec.brokerPreflight = func(context.Context, *ssh.Client, *ExecutorCreateRequest, SSHRemotePlatform) error {
				preflightCalls++
				return nil
			}
			exec.reconcileRemote = func(_ context.Context, _ *ssh.Client, request sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
				verifyCalls++
				require.Equal(t, sshlaunch.ReconcileModeVerify, request.Mode)
				return sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeAbsent}
			}
			exec.stopRemote = func(context.Context, *ssh.Client, string, int) error {
				stopCalls++
				return nil
			}
			exec.closeClient = func(*ssh.Client) error {
				closeCalls++
				return nil
			}

			if tracked {
				client := server.dial(t)
				exec.sessions[owner.ControllerInstanceID] = &sshSessionState{
					client: client, pid: 4242, remoteDir: owner.SessionDir,
					remoteTaskDir: owner.TaskDir, launchReceipt: &receipt,
				}
			}
			req := &ExecutorCreateRequest{
				InstanceID: owner.ControllerInstanceID,
				TaskID:     owner.TaskID,
				SessionID:  owner.SessionID,
				Metadata:   metadata,
				Env: map[string]string{
					githubauth.CredentialBrokerURLEnv: "https://broker.example.test",
					githubauth.CredentialLeaseEnv:     "lease-1",
				},
			}
			err := exec.ResumeRemoteInstance(context.Background(), req)
			require.NoError(t, err)
			require.Equal(t, 1, preflightCalls)
			require.Equal(t, 1, verifyCalls)
			require.Zero(t, stopCalls, "confirmed absence must bypass the numeric stop")
			require.NotContains(t, exec.sessions, owner.ControllerInstanceID)
			if tracked {
				require.Equal(t, 1, closeCalls, "tracked SSH client must be closed")
			} else {
				require.Equal(t, 0, closeCalls)
			}
			for _, key := range []string{
				MetadataKeySSHRemoteSessionDir, MetadataKeySSHRemoteAgentctlPort,
				MetadataKeySSHRemoteControlPort, MetadataKeySSHRemoteAgentctlPID,
				MetadataKeySSHLocalForwardPort, MetadataKeySSHRemoteAgentctlURL,
				MetadataKeySSHLaunchReceipt,
			} {
				require.NotContains(t, req.Metadata, key)
			}
			require.Equal(t, owner.TaskDir, req.Metadata[MetadataKeySSHRemoteTaskDir])
			require.Equal(t, "/home/kandev/.kandev", req.Metadata[MetadataKeySSHWorkdirRoot])
			require.Equal(t, "127.0.0.1", req.Metadata[MetadataKeySSHHost])
			for _, command := range server.commands() {
				require.NotContains(t, command, "kill 4242")
			}
		})
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.9
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.20
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHResumePreviousBootController(t *testing.T) {
	owner := testSSHLaunchOwner()
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: `printf %s "$HOME"`, result: sshOut("/home/kandev")},
		sshScriptRule{match: "kill -0 4242", result: sshOK},
	).handle)
	receipt := testSSHLaunchReceiptForServer(t, server, 4242)
	metadata := resumeMetadataForTest(t, server, receipt)
	metadata[MetadataKeySSHRemoteAgentctlPort] = "43123"
	metadata[MetadataKeySSHWorkdirRoot] = "/home/kandev/.kandev"
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	verifyCalls := 0
	exec.reconcileRemote = func(_ context.Context, _ *ssh.Client, request sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
		verifyCalls++
		require.Equal(t, sshlaunch.ReconcileModeVerify, request.Mode)
		return sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeAbsent}
	}

	err := exec.ResumeRemoteInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: owner.ControllerInstanceID,
		TaskID:     owner.TaskID,
		SessionID:  owner.SessionID,
		AuthToken:  "persisted-token",
		Metadata:   metadata,
	})
	require.NoError(t, err)
	require.Equal(t, 1, verifyCalls)
	require.Empty(t, exec.sessions, "the recorded PID holder must not be adopted")
	require.Equal(t, owner.TaskDir, metadata[MetadataKeySSHRemoteTaskDir])
	require.Equal(t, "/home/kandev/.kandev", metadata[MetadataKeySSHWorkdirRoot])
	require.Equal(t, "127.0.0.1", metadata[MetadataKeySSHHost])
	for _, key := range []string{
		MetadataKeySSHRemoteSessionDir, MetadataKeySSHRemoteAgentctlPort,
		MetadataKeySSHRemoteControlPort, MetadataKeySSHRemoteAgentctlPID,
		MetadataKeySSHLocalForwardPort, MetadataKeySSHRemoteAgentctlURL,
		MetadataKeySSHLaunchReceipt,
	} {
		require.NotContains(t, metadata, key)
	}
	for _, command := range server.commands() {
		require.NotContains(t, command, "kill 4242")
		require.NotContains(t, command, "rm -rf")
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.9
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.20
func TestSSHResumePreviousBootSkipsPermissionDeniedPIDProbe(t *testing.T) {
	owner := testSSHLaunchOwner()
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: `printf %s "$HOME"`, result: sshOut("/home/kandev")},
		sshScriptRule{match: "kill -0 4242", result: sshFail("kill: 4242: Operation not permitted")},
	).handle)
	receipt := testSSHLaunchReceiptForServer(t, server, 4242)
	birthToken := strings.Split(receipt.Process.BirthToken, ":")
	require.Len(t, birthToken, 3)
	birthToken[1] = "previous-" + birthToken[1]
	receipt.Process.BirthToken = strings.Join(birthToken, ":")
	metadata := resumeMetadataForTest(t, server, receipt)
	metadata[MetadataKeySSHRemoteAgentctlPort] = "43123"
	metadata[MetadataKeySSHWorkdirRoot] = "/home/kandev/.kandev"
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	verifyCalls := 0
	exec.reconcileRemote = func(_ context.Context, _ *ssh.Client, request sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
		verifyCalls++
		require.Equal(t, sshlaunch.ReconcileModeVerify, request.Mode)
		require.Equal(t, receipt.Process.BirthToken, request.Receipt.Process.BirthToken)
		return sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeAbsent}
	}

	err := exec.ResumeRemoteInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: owner.ControllerInstanceID,
		TaskID:     owner.TaskID,
		SessionID:  owner.SessionID,
		AuthToken:  "persisted-token",
		Metadata:   metadata,
	})
	require.NoError(t, err)
	require.Equal(t, 1, verifyCalls, "the previous-boot receipt proves absence before numeric PID access")
	require.Empty(t, exec.sessions, "the reused PID holder must not be adopted")
	for _, command := range server.commands() {
		require.NotContains(t, command, "kill -0 4242", "receipt-backed recovery must not probe a reused PID")
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.10
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.23
func TestSSHBrokerResumeAbsentDiagnosticContainsOutcome(t *testing.T) {
	owner := testSSHLaunchOwner()
	server := newFakeSSHServer(t, nil)
	receipt := testSSHLaunchReceiptForServer(t, server, 4242)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	exec.brokerPreflight = func(context.Context, *ssh.Client, *ExecutorCreateRequest, SSHRemotePlatform) error { return nil }
	exec.reconcileRemote = func(context.Context, *ssh.Client, sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
		return sshlaunch.Unresolved(sshlaunch.ReasonProbeUnknown, "broker credential response unavailable")
	}
	metadata := sshLaunchStopMetadata(t, server, receipt)
	metadata[MetadataKeySSHRemoteAgentctlPort] = "43123"
	req := &ExecutorCreateRequest{
		InstanceID: owner.ControllerInstanceID, TaskID: owner.TaskID, SessionID: owner.SessionID,
		Metadata: metadata,
		Env: map[string]string{
			githubauth.CredentialBrokerURLEnv: "https://broker.example.test",
			githubauth.CredentialLeaseEnv:     "lease-1",
		},
	}
	err := exec.ResumeRemoteInstance(context.Background(), req)
	require.ErrorIs(t, err, errSSHLaunchReconciliationUnresolved)
	require.True(t, strings.Contains(err.Error(), "outcome=unresolved"), "error = %v", err)
	require.NotContains(t, err.Error(), "lease-1")
	require.NotContains(t, err.Error(), "https://broker.example.test")
	require.Contains(t, req.Metadata, MetadataKeySSHLaunchReceipt)
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.20
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHVerifyRecordedControllerIdentityRequiresLiveController(t *testing.T) {
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	exec.reconcileRemote = func(context.Context, *ssh.Client, sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
		return sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeAbsent}
	}
	err := exec.verifyRecordedControllerIdentity(context.Background(), nil, testSSHLaunchReceipt(t, testSSHLaunchOwner(), 4242))
	require.ErrorIs(t, err, errSSHLaunchReconciliationUnresolved)
}
