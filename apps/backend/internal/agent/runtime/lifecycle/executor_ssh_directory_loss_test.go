package lifecycle

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/sshlaunch"
	"github.com/kandev/kandev/internal/task/models"
)

// testSSHLaunchReceiptForServer builds a receipt whose target binding is the
// fake server's own connection tuple, so the persisted metadata and the
// receipt describe the same owner.
func testSSHLaunchReceiptForServer(t *testing.T, server *fakeSSHServer, pid int) sshlaunch.Receipt {
	t.Helper()
	owner := testSSHLaunchOwner()
	owner.Target = sshlaunch.TargetBinding{
		Host:            "127.0.0.1",
		Port:            strconv.Itoa(server.port()),
		User:            "kandev",
		HostFingerprint: server.hostKeyFingerprint(),
	}
	return testSSHLaunchReceipt(t, owner, pid)
}

func directoryLossServer(t *testing.T, taskDir, sessionDir string) *fakeSSHServer {
	t.Helper()
	return newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir " + taskDir)},
		sshScriptRule{match: "cat -- '" + sessionDir + "/agentctl.pid'", result: sshFail("")},
		sshScriptRule{match: "test -d '" + sessionDir + "'", result: sshFail("")},
	).handle)
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.19
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.22
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHDirectoryLossReconciliationOriginalProcessAndRetry(t *testing.T) {
	owner := testSSHLaunchOwner()
	server := directoryLossServer(t, owner.TaskDir, owner.SessionDir)
	receipt := testSSHLaunchReceiptForServer(t, server, 4242)

	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	var calls []sshlaunch.ReconcileRequest
	exec.reconcileRemote = func(_ context.Context, _ *ssh.Client, req sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
		calls = append(calls, req)
		return sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeStopped}
	}

	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "instance-1",
		TaskID:     owner.TaskID,
		SessionID:  owner.SessionID,
		StopReason: "task cleanup",
		Metadata:   sshLaunchStopMetadata(t, server, receipt),
	}, true)
	if err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("reconcile calls = %d, want exactly one", len(calls))
	}
	if err := calls[0].Receipt.Validate(); err != nil {
		t.Fatalf("dispatched receipt is invalid: %v", err)
	}
	if calls[0].Receipt.Generation != receipt.Generation {
		t.Fatalf("dispatched generation = %q, want %q", calls[0].Receipt.Generation, receipt.Generation)
	}
	for _, command := range server.commands() {
		if strings.Contains(command, "kill ") || strings.Contains(command, "rm -rf") {
			t.Fatalf("directory-loss reconciliation issued a legacy signal/removal command: %q", command)
		}
	}

	// Retry after the controller exited: the recorded process is absent, so
	// cleanup completes from persisted absence evidence and never dispatches
	// the helper again.
	retryServer := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: "ps -p 4242 -o command=", result: sshFail("")},
		sshScriptRule{match: "cat -- '" + owner.SessionDir + "/agentctl.pid'", result: sshFail("")},
		sshScriptRule{match: "test -d '" + owner.SessionDir + "'", result: sshFail("")},
		sshScriptRule{match: "rm -rf '" + owner.SessionDir + "'", result: sshOK},
	).handle)
	retryExec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	retryCalls := 0
	retryExec.reconcileRemote = func(_ context.Context, _ *ssh.Client, req sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
		retryCalls++
		require.Equal(t, sshlaunch.ReconcileModeVerify, req.Mode)
		return sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeAbsent}
	}
	retryReceipt := testSSHLaunchReceiptForServer(t, retryServer, 4242)
	err = retryExec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "instance-1",
		TaskID:     owner.TaskID,
		SessionID:  owner.SessionID,
		StopReason: "task cleanup",
		Metadata:   sshLaunchStopMetadata(t, retryServer, retryReceipt),
	}, true)
	if err != nil {
		t.Fatalf("retry StopInstance: %v", err)
	}
	if retryCalls != 1 {
		t.Fatalf("retry dispatched the helper %d times, want one absence verification", retryCalls)
	}
	if _, killed := retryServer.lastCommandContaining("kill "); killed {
		t.Fatalf("retry sent a signal after confirmed controller exit: %v", retryServer.commands())
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.21
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.23
func TestSSHDirectoryLossReconciliationPreservesUnknown(t *testing.T) {
	owner := testSSHLaunchOwner()

	t.Run("legacy record without launch proof stays unresolved", func(t *testing.T) {
		server := directoryLossServer(t, owner.TaskDir, owner.SessionDir)
		receipt := testSSHLaunchReceiptForServer(t, server, 4242)
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		calls := 0
		exec.reconcileRemote = func(context.Context, *ssh.Client, sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
			calls++
			return sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeStopped}
		}
		metadata := sshLaunchStopMetadata(t, server, receipt)
		delete(metadata, MetadataKeySSHLaunchReceipt)
		err := exec.StopInstance(context.Background(), &ExecutorInstance{
			InstanceID: "instance-1", TaskID: owner.TaskID, SessionID: owner.SessionID,
			StopReason: "task cleanup", Metadata: metadata,
		}, true)
		if !errors.Is(err, errSSHLaunchIdentityMissing) {
			t.Fatalf("error = %v, want errSSHLaunchIdentityMissing", err)
		}
		if calls != 0 {
			t.Fatalf("helper was dispatched %d times for a legacy record", calls)
		}
	})

	t.Run("unresolved helper result preserves the recorded owner", func(t *testing.T) {
		server := directoryLossServer(t, owner.TaskDir, owner.SessionDir)
		receipt := testSSHLaunchReceiptForServer(t, server, 4242)
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		exec.reconcileRemote = func(context.Context, *ssh.Client, sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
			return sshlaunch.Unresolved(sshlaunch.ReasonTerminationUnconfirmed, "still alive")
		}
		err := exec.StopInstance(context.Background(), &ExecutorInstance{
			InstanceID: "instance-1", TaskID: owner.TaskID, SessionID: owner.SessionID,
			StopReason: "task cleanup", Metadata: sshLaunchStopMetadata(t, server, receipt),
		}, true)
		if !errors.Is(err, errSSHLaunchReconciliationUnresolved) {
			t.Fatalf("error = %v, want errSSHLaunchReconciliationUnresolved", err)
		}
		for _, command := range server.commands() {
			if strings.Contains(command, "kill ") || strings.Contains(command, "rm -rf") {
				t.Fatalf("unresolved reconciliation issued a legacy signal/removal command: %q", command)
			}
		}
	})

	t.Run("a receipt for another pid never dispatches", func(t *testing.T) {
		server := directoryLossServer(t, owner.TaskDir, owner.SessionDir)
		otherPID := testSSHLaunchReceiptForServer(t, server, 4343)
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		calls := 0
		exec.reconcileRemote = func(context.Context, *ssh.Client, sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
			calls++
			return sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeStopped}
		}
		err := exec.StopInstance(context.Background(), &ExecutorInstance{
			InstanceID: "instance-1", TaskID: owner.TaskID, SessionID: owner.SessionID,
			StopReason: "task cleanup", Metadata: sshLaunchStopMetadata(t, server, otherPID),
		}, true)
		if !errors.Is(err, errSSHLaunchIdentityConflict) {
			t.Fatalf("error = %v, want errSSHLaunchIdentityConflict", err)
		}
		if calls != 0 {
			t.Fatalf("helper was dispatched %d times for a mismatched receipt", calls)
		}
	})
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.20
func TestSSHDirectoryLossReconciliationNotSelectedForOtherVerificationErrors(t *testing.T) {
	owner := testSSHLaunchOwner()
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir " + owner.TaskDir)},
		sshScriptRule{match: "cat -- '" + owner.SessionDir + "/agentctl.pid'", result: sshOut("9999")},
	).handle)
	receipt := testSSHLaunchReceiptForServer(t, server, 4242)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	calls := 0
	exec.reconcileRemote = func(context.Context, *ssh.Client, sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
		calls++
		return sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeStopped}
	}
	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "instance-1", TaskID: owner.TaskID, SessionID: owner.SessionID,
		StopReason: "task cleanup", Metadata: sshLaunchStopMetadata(t, server, receipt),
	}, true)
	if err == nil {
		t.Fatal("a pidfile that names another launch must keep the existing refusal")
	}
	if calls != 0 {
		t.Fatalf("helper was dispatched %d times for a conflicting pidfile", calls)
	}
}

func sshLaunchStopMetadata(t *testing.T, server *fakeSSHServer, receipt sshlaunch.Receipt) map[string]interface{} {
	t.Helper()
	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteSessionDir] = receipt.Owner.SessionDir
	metadata[MetadataKeySSHRemoteTaskDir] = receipt.Owner.TaskDir
	metadata[MetadataKeySSHAgentctlInstanceID] = receipt.Owner.ControllerInstanceID
	applySSHLaunchReceiptMetadata(metadata, &receipt)
	return metadata
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.21
// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHReceiptValidationPrecedesDestructiveBranches(t *testing.T) {
	owner := testSSHLaunchOwner()

	cases := map[string]func(*testing.T, map[string]interface{}, sshlaunch.Receipt) map[string]interface{}{
		"null receipt": func(_ *testing.T, metadata map[string]interface{}, _ sshlaunch.Receipt) map[string]interface{} {
			metadata[MetadataKeySSHLaunchReceipt] = nil
			return metadata
		},
		"malformed receipt": func(_ *testing.T, metadata map[string]interface{}, _ sshlaunch.Receipt) map[string]interface{} {
			metadata[MetadataKeySSHLaunchReceipt] = map[string]interface{}{"version": sshlaunch.Version}
			return metadata
		},
		"foreign receipt": func(t *testing.T, metadata map[string]interface{}, receipt sshlaunch.Receipt) map[string]interface{} {
			foreign := receipt
			foreign.Owner.SessionID = "session-2"
			projected, err := foreign.Metadata()
			require.NoError(t, err)
			metadata[MetadataKeySSHLaunchReceipt] = projected
			return metadata
		},
		"pid mismatch": func(t *testing.T, metadata map[string]interface{}, receipt sshlaunch.Receipt) map[string]interface{} {
			other := testSSHLaunchReceipt(t, owner, 4343)
			projected, err := other.Metadata()
			require.NoError(t, err)
			metadata[MetadataKeySSHLaunchReceipt] = projected
			return metadata
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			// No scripted commands: any remote command would fail the test.
			server := newFakeSSHServer(t, nil)
			receipt := testSSHLaunchReceiptForServer(t, server, 4242)
			metadata := sshLaunchStopMetadata(t, server, receipt)
			metadata = mutate(t, metadata, receipt)
			exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
			exec.reconcileRemote = func(context.Context, *ssh.Client, sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
				t.Fatal("a malformed or foreign receipt must not reach the helper")
				return sshlaunch.ReconcileResult{}
			}
			err := exec.StopInstance(context.Background(), &ExecutorInstance{
				InstanceID: "instance-1", TaskID: owner.TaskID, SessionID: owner.SessionID,
				StopReason: "task cleanup", Metadata: metadata,
			}, true)
			if err == nil {
				t.Fatal("a malformed or foreign receipt must fail closed")
			}
			for _, command := range server.commands() {
				if strings.Contains(command, "kill ") || strings.Contains(command, "rm -rf") {
					t.Fatalf("invalid proof reached a destructive command: %q", command)
				}
			}
		})
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHPresentDirectoryVerifiesBirthIdentityBeforeLegacyStop(t *testing.T) {
	owner := testSSHLaunchOwner()
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir " + owner.TaskDir)},
		sshScriptRule{match: "cat -- '" + owner.SessionDir + "/agentctl.pid'", result: sshOut("4242")},
		sshScriptRule{match: "kill 4242", result: sshOK},
		sshScriptRule{match: "rm -rf '" + owner.SessionDir + "'", result: sshOK},
	).handle)
	receipt := testSSHLaunchReceiptForServer(t, server, 4242)
	metadata := sshLaunchStopMetadata(t, server, receipt)

	t.Run("identity conflict refuses the legacy kill and removal", func(t *testing.T) {
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		exec.reconcileRemote = func(_ context.Context, _ *ssh.Client, req sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
			require.Equal(t, sshlaunch.ReconcileModeVerify, req.Mode)
			return sshlaunch.Unresolved(sshlaunch.ReasonIdentityConflict, "recorded process identity changed")
		}
		err := exec.StopInstance(context.Background(), &ExecutorInstance{
			InstanceID: "instance-1", TaskID: owner.TaskID, SessionID: owner.SessionID,
			StopReason: "task cleanup", Metadata: metadata,
		}, true)
		require.ErrorIs(t, err, errSSHLaunchReconciliationUnresolved)
		for _, command := range server.commands() {
			if strings.Contains(command, "kill ") || strings.Contains(command, "rm -rf") {
				t.Fatalf("a failed identity verification reached a destructive command: %q", command)
			}
		}
	})

	t.Run("verified identity permits the guarded legacy stop", func(t *testing.T) {
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
		exec.reconcileRemote = func(_ context.Context, _ *ssh.Client, req sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
			require.Equal(t, sshlaunch.ReconcileModeVerify, req.Mode)
			return sshlaunch.ReconcileResult{Outcome: sshlaunch.OutcomeVerified}
		}
		err := exec.StopInstance(context.Background(), &ExecutorInstance{
			InstanceID: "instance-1", TaskID: owner.TaskID, SessionID: owner.SessionID,
			StopReason: "task cleanup", Metadata: metadata,
		}, true)
		require.NoError(t, err)
		_, killed := server.lastCommandContaining("kill 4242")
		require.True(t, killed, "a verified identity must still permit the guarded stop")
	})
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.24
func TestSSHRecoveryRejectsAReplacementProcess(t *testing.T) {
	owner := testSSHLaunchOwner()
	server := directoryLossServer(t, owner.TaskDir, owner.SessionDir)
	receipt := testSSHLaunchReceiptForServer(t, server, 4242)
	metadata := sshLaunchStopMetadata(t, server, receipt)
	metadata[MetadataKeySSHRemoteAgentctlPort] = "43123"
	record := &models.ExecutorRunning{
		SessionID: owner.SessionID, TaskID: owner.TaskID,
		Runtime:          agentruntime.RuntimeSSH,
		AgentExecutionID: "execution-1", PID: 4242,
		TransientAuthToken: "token", Metadata: metadata,
	}
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	exec.reconcileRemote = func(context.Context, *ssh.Client, sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
		return sshlaunch.Unresolved(sshlaunch.ReasonIdentityConflict, "recorded process identity changed")
	}

	instances, outcomes, err := exec.RecoverInstancesDetailed(context.Background(), []*models.ExecutorRunning{record})
	require.NoError(t, err)
	require.Empty(t, instances, "a replacement process must never be adopted")
	require.Equal(t, RecoveryOutcomeUnknown, outcomes[owner.SessionID])
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.22
func TestSSHAbandonedReceiptRetainsSurvivingChildEvidence(t *testing.T) {
	owner := testSSHLaunchOwner()
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: "ps -p 4242 -o command=", result: sshFail("")},
		sshScriptRule{match: "cat -- '" + owner.SessionDir + "/agentctl.pid'", result: sshFail("")},
		sshScriptRule{match: "test -d '" + owner.SessionDir + "'", result: sshFail("")},
	).handle)
	receipt := testSSHLaunchReceiptForServer(t, server, 4242)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	exec.reconcileRemote = func(context.Context, *ssh.Client, sshlaunch.ReconcileRequest) sshlaunch.ReconcileResult {
		return sshlaunch.Unresolved(sshlaunch.ReasonTerminationUnconfirmed, "a session child survived the controller")
	}
	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "instance-1", TaskID: owner.TaskID, SessionID: owner.SessionID,
		StopReason: "task cleanup", Metadata: sshLaunchStopMetadata(t, server, receipt),
	}, true)
	require.ErrorIs(t, err, errSSHLaunchReconciliationUnresolved)
	for _, command := range server.commands() {
		if strings.Contains(command, "rm -rf") || strings.Contains(command, "kill ") {
			t.Fatalf("surviving child evidence reached a destructive command: %q", command)
		}
	}
}
