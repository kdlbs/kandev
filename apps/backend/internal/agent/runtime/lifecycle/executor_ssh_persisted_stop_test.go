package lifecycle

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/ssh"
)

// Tests for reaping a remote agentctl whose in-memory session state did not
// survive a backend restart, driven only by persisted executors_running
// metadata (ssh_remote_agentctl_pid / ssh_remote_session_dir plus the SSH
// connection keys read by targetFromMetadata).

func TestSSHExecutorStopInstanceFromPersistedMetadataOnly(t *testing.T) {
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir /remote/session")},
		sshScriptRule{match: "kill 4242", result: sshOK},
	).handle)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())

	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"

	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "orphaned-instance",
		StopReason: "startup terminal session cleanup",
		Metadata:   metadata,
	}, true)
	if err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
	if _, ok := server.lastCommandContaining("kill 4242"); !ok {
		t.Fatalf("expected the persisted pid to be killed, commands: %v", server.commands())
	}
	if _, ok := server.lastCommandContaining("rm -rf '/remote/session'"); !ok {
		t.Fatalf("expected the persisted session dir to be removed, commands: %v", server.commands())
	}
	if len(exec.sessions) != 0 {
		t.Fatal("a persisted-metadata stop must not leave tracked session state behind")
	}
}

// TestSSHExecutorStopInstanceIdentityMismatchSkipsKill covers F2: a pid
// recovered from persisted metadata that no longer belongs to our agentctl
// (reused by an unrelated process, or the command line simply doesn't match)
// must never be signalled. The session directory is still reclaimed.
func TestSSHExecutorStopInstanceIdentityMismatchSkipsKill(t *testing.T) {
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/usr/bin/some-other-process --unrelated")},
		sshScriptRule{match: "rm -rf '/remote/session'", result: sshOK},
	).handle)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())

	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"

	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "orphaned-instance",
		StopReason: "startup terminal session cleanup",
		Metadata:   metadata,
	}, true)
	if err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
	if _, ok := server.lastCommandContaining("kill 4242"); ok {
		t.Fatalf("expected no kill for a pid that doesn't match our agentctl, commands: %v", server.commands())
	}
	if _, ok := server.lastCommandContaining("rm -rf '/remote/session'"); !ok {
		t.Fatalf("expected the session dir to still be reclaimed, commands: %v", server.commands())
	}
}

// TestSSHExecutorStopInstanceIdentityProbeFailurePreservesRow covers F2's
// other branch: when the identity probe itself can't be answered (an
// SSH-level fault, not merely "no such process"), StopInstance must error so
// the caller preserves the executors_running row instead of reporting a
// possibly-live orphan as reaped.
func TestSSHExecutorStopInstanceIdentityProbeFailurePreservesRow(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	exec.verifyRemoteIdentity = func(context.Context, *ssh.Client, int, string, string) (bool, error) {
		return false, errors.New("ssh: channel closed mid-command")
	}

	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"

	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "orphaned-instance",
		StopReason: "startup terminal session cleanup",
		Metadata:   metadata,
	}, true)
	if err == nil {
		t.Fatal("expected StopInstance to error when the identity probe itself fails")
	}
	if len(server.commands()) != 0 {
		t.Fatalf("expected no remote command after a failed identity probe, commands: %v", server.commands())
	}
}

// TestSSHExecutorStopInstanceStopCommandFailurePropagatesError covers F1: a
// failing remote stop command must surface as an error, not be swallowed,
// so the caller preserves the executors_running row and retries instead of
// deleting it out from under a still-live orphan.
func TestSSHExecutorStopInstanceStopCommandFailurePropagatesError(t *testing.T) {
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir /remote/session")},
		sshScriptRule{match: "kill 4242", result: sshFail("permission denied")},
	).handle)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())

	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"

	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "orphaned-instance",
		StopReason: "startup terminal session cleanup",
		Metadata:   metadata,
	}, true)
	if err == nil {
		t.Fatal("expected StopInstance to propagate a failing remote stop command")
	}
}

func TestSSHExecutorStopInstancePersistedMetadataPreservesOnBackendShutdown(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())

	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"

	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "orphaned-instance",
		StopReason: StopReasonBackendShutdown,
		Metadata:   metadata,
	}, false)
	if err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
	if len(server.commands()) != 0 {
		t.Fatalf("a non-forced graceful shutdown must not touch the remote host, commands: %v", server.commands())
	}
}

func TestSSHExecutorStopInstanceNoPersistedMetadataIsNoOp(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]interface{}
	}{
		{name: "nil metadata", metadata: nil},
		{name: "no pid", metadata: map[string]interface{}{
			MetadataKeySSHRemoteSessionDir: "/remote/session",
		}},
		{name: "no session dir", metadata: map[string]interface{}{
			MetadataKeySSHRemoteAgentctlPID: "4242",
		}},
		{name: "zero pid", metadata: map[string]interface{}{
			MetadataKeySSHRemoteAgentctlPID: "0",
			MetadataKeySSHRemoteSessionDir:  "/remote/session",
		}},
		{name: "negative pid", metadata: map[string]interface{}{
			MetadataKeySSHRemoteAgentctlPID: "-1",
			MetadataKeySSHRemoteSessionDir:  "/remote/session",
		}},
		{name: "non-numeric pid", metadata: map[string]interface{}{
			MetadataKeySSHRemoteAgentctlPID: "not-a-pid",
			MetadataKeySSHRemoteSessionDir:  "/remote/session",
		}},
		{name: "blank session dir", metadata: map[string]interface{}{
			MetadataKeySSHRemoteAgentctlPID: "4242",
			MetadataKeySSHRemoteSessionDir:  "   ",
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
			err := exec.StopInstance(context.Background(), &ExecutorInstance{
				InstanceID: "orphaned-instance",
				StopReason: "startup terminal session cleanup",
				Metadata:   tc.metadata,
			}, true)
			if err != nil {
				t.Fatalf("StopInstance: %v", err)
			}
		})
	}
}

func TestSSHExecutorStopInstancePersistedMetadataUnresolvableTargetErrors(t *testing.T) {
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "orphaned-instance",
		StopReason: "startup terminal session cleanup",
		Metadata: map[string]interface{}{
			MetadataKeySSHRemoteAgentctlPID: "4242",
			MetadataKeySSHRemoteSessionDir:  "/remote/session",
			// No host, host_alias, or fingerprint: targetFromMetadata must fail.
		},
	}, true)
	if err == nil {
		t.Fatal("expected an error resolving an SSH target with no host information")
	}
}
