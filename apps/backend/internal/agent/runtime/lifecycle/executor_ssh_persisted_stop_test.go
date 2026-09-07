package lifecycle

import (
	"context"
	"testing"
)

// Tests for reaping a remote agentctl whose in-memory session state did not
// survive a backend restart, driven only by persisted executors_running
// metadata (ssh_remote_agentctl_pid / ssh_remote_session_dir plus the SSH
// connection keys read by targetFromMetadata).

func TestSSHExecutorStopInstanceFromPersistedMetadataOnly(t *testing.T) {
	server := newFakeSSHServer(t, nil)
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
