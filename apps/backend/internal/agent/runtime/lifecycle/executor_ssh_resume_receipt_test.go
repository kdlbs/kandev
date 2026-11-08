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

func resumeMetadataForTest(t *testing.T, server *fakeSSHServer, receipt sshlaunch.Receipt) map[string]interface{} {
	t.Helper()
	metadata := sshLaunchStopMetadata(t, server, receipt)
	metadata[MetadataKeySSHRemoteAgentctlPort] = "43123"
	return metadata
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHResumeRejectsReceiptPIDMismatch(t *testing.T) {
	owner := testSSHLaunchOwner()
	server := newFakeSSHServer(t, nil)
	// The receipt names a different process than the recorded metadata PID.
	receipt := testSSHLaunchReceiptForServer(t, server, 4343)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	err := exec.ResumeRemoteInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: owner.ControllerInstanceID,
		TaskID:     owner.TaskID,
		SessionID:  owner.SessionID,
		AuthToken:  "token",
		Metadata:   resumeMetadataForTest(t, server, receipt),
	})
	require.ErrorIs(t, err, errSSHLaunchIdentityConflict)
	require.Empty(t, server.commands(), "a PID-mismatched receipt must fail before dialing")
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHResumeRejectsUnverifiedLiveController(t *testing.T) {
	owner := testSSHLaunchOwner()
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: `printf %s "$HOME"`, result: sshOut("/home/kandev")},
		sshScriptRule{match: "kill -0 4242", result: sshOK},
	).handle)
	receipt := testSSHLaunchReceiptForServer(t, server, 4242)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	exec.reconcileRemote = func(_ context.Context, _ *ssh.Client, req sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
		require.Equal(t, sshlaunch.ReconcileModeVerify, req.Mode)
		return sshlaunch.Unresolved(sshlaunch.ReasonIdentityConflict, "recorded process identity changed")
	}
	err := exec.ResumeRemoteInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: owner.ControllerInstanceID,
		TaskID:     owner.TaskID,
		SessionID:  owner.SessionID,
		AuthToken:  "token",
		Metadata:   resumeMetadataForTest(t, server, receipt),
	})
	require.ErrorIs(t, err, errSSHLaunchReconciliationUnresolved)
	for _, command := range server.commands() {
		if strings.Contains(command, "kill 4242") || strings.Contains(command, "rm -rf") {
			t.Fatalf("resume issued a destructive command after failed launch proof: %q", command)
		}
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHBrokerResumeVerifiesControllerBeforeStop(t *testing.T) {
	owner := testSSHLaunchOwner()
	server := newFakeSSHServer(t, nil)
	receipt := testSSHLaunchReceiptForServer(t, server, 4242)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	exec.brokerPreflight = func(context.Context, *ssh.Client, *ExecutorCreateRequest, SSHRemotePlatform) error {
		return nil
	}
	exec.reconcileRemote = func(context.Context, *ssh.Client, sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
		return sshlaunch.Unresolved(sshlaunch.ReasonIdentityConflict, "recorded process identity changed")
	}
	stopCalls := 0
	exec.stopRemote = func(context.Context, *ssh.Client, string, int) error {
		stopCalls++
		return nil
	}
	err := exec.ResumeRemoteInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: owner.ControllerInstanceID,
		TaskID:     owner.TaskID,
		SessionID:  owner.SessionID,
		Metadata:   resumeMetadataForTest(t, server, receipt),
		Env: map[string]string{
			githubauth.CredentialBrokerURLEnv: "https://broker.example.test",
			githubauth.CredentialLeaseEnv:     "lease-1",
		},
	})
	require.ErrorIs(t, err, errSSHLaunchReconciliationUnresolved)
	require.Zero(t, stopCalls, "the broker refresh must not signal an unverified controller")
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.22
func TestSSHResumeRetainsProofWhenAbsentChildUnconfirmed(t *testing.T) {
	owner := testSSHLaunchOwner()
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: `printf %s "$HOME"`, result: sshOut("/home/kandev")},
		sshScriptRule{match: "kill -0 4242", result: sshFail("no such process")},
	).handle)
	receipt := testSSHLaunchReceiptForServer(t, server, 4242)
	metadata := resumeMetadataForTest(t, server, receipt)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	exec.reconcileRemote = func(context.Context, *ssh.Client, sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
		return sshlaunch.Unresolved(sshlaunch.ReasonTerminationUnconfirmed, "a session child survived the controller")
	}
	err := exec.ResumeRemoteInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: owner.ControllerInstanceID, TaskID: owner.TaskID, SessionID: owner.SessionID,
		AuthToken: "token", Metadata: metadata,
	})
	require.ErrorIs(t, err, errSSHLaunchReconciliationUnresolved)
	require.Contains(t, metadata, MetadataKeySSHLaunchReceipt,
		"unconfirmed child drainage must preserve the proof and prevent replacement")
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.19
func TestSSHBrokerResumeRetainsProofWhenChildUnconfirmed(t *testing.T) {
	owner := testSSHLaunchOwner()
	server := newFakeSSHServer(t, nil)
	receipt := testSSHLaunchReceiptForServer(t, server, 4242)
	metadata := resumeMetadataForTest(t, server, receipt)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	exec.brokerPreflight = func(context.Context, *ssh.Client, *ExecutorCreateRequest, SSHRemotePlatform) error {
		return nil
	}
	verifyCalls := 0
	exec.reconcileRemote = func(context.Context, *ssh.Client, sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
		verifyCalls++
		if verifyCalls == 1 {
			return sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeVerified}
		}
		return sshlaunch.Unresolved(sshlaunch.ReasonTerminationUnconfirmed, "a session child survived the controller")
	}
	exec.stopRemote = func(context.Context, *ssh.Client, string, int) error { return nil }
	err := exec.ResumeRemoteInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: owner.ControllerInstanceID, TaskID: owner.TaskID, SessionID: owner.SessionID,
		Metadata: metadata,
		Env: map[string]string{
			githubauth.CredentialBrokerURLEnv: "https://broker.example.test",
			githubauth.CredentialLeaseEnv:     "lease-1",
		},
	})
	require.ErrorIs(t, err, errSSHLaunchReconciliationUnresolved)
	require.Equal(t, 2, verifyCalls, "the post-stop absence check must run after verified teardown")
	require.Contains(t, metadata, MetadataKeySSHLaunchReceipt,
		"unconfirmed child drainage must preserve the proof and prevent replacement")
}
