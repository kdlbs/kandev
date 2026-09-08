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
	// The remote launch command line is always "<agentctlBin> --workdir
	// <taskDir>" (startRemoteAgentctlOnPort) — sessionDir never appears in
	// the argv. The fake server's ps output mirrors that shape so this test
	// exercises the identity branch that actually fires in production.
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir /remote/task")},
		sshScriptRule{match: "cat -- '/remote/session/agentctl.pid'", result: sshOut("4242")},
		sshScriptRule{match: "kill 4242", result: sshOK},
	).handle)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())

	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
	metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"

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

// TestSSHExecutorStopInstanceDeadPidEmptyStderrReapsCleanly covers R3-F1: a
// dead pid probed via `ps -p <pid> -o command=` exits non-zero with empty
// stdout and empty stderr on every observed platform — the ordinary shape
// for the common "the orphan already exited" case, not the exception. That
// must be read as confirmed absence, not as a probe fault: the stop must
// succeed without signalling anything, and the session dir must still be
// reclaimed so the row is prunable.
func TestSSHExecutorStopInstanceDeadPidEmptyStderrReapsCleanly(t *testing.T) {
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: "ps -p 4242 -o command=", result: sshFail("")},
		sshScriptRule{match: "rm -rf '/remote/session'", result: sshOK},
	).handle)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())

	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
	metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"

	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "orphaned-instance",
		StopReason: "startup terminal session cleanup",
		Metadata:   metadata,
	}, true)
	if err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
	if _, ok := server.lastCommandContaining("kill 4242"); ok {
		t.Fatalf("expected no kill for a confirmed-dead pid, commands: %v", server.commands())
	}
	if _, ok := server.lastCommandContaining("rm -rf '/remote/session'"); !ok {
		t.Fatalf("expected the session dir to still be reclaimed, commands: %v", server.commands())
	}
}

// TestSSHExecutorStopInstanceSharedTaskDirIdentityUsesPidfile covers R3-F2:
// taskDir is shared by every sibling session of the same task (workspace
// reuse), so a `ps` argv match alone only proves "some agentctl for this
// task" — a stale row's persisted pid can be recycled by a live sibling
// session's agentctl on the same taskDir. The per-session pidfile at
// <sessionDir>/agentctl.pid must also name the same pid before a kill is
// sent. Two rows share taskDir here — only the row whose own sessionDir
// pidfile matches its persisted pid may be signalled.
func TestSSHExecutorStopInstanceSharedTaskDirIdentityUsesPidfile(t *testing.T) {
	sharedArgv := sshOut("/opt/kandev/bin/agentctl --workdir /remote/task")

	t.Run("row whose own pidfile confirms the pid is still killed despite shared taskDir", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: "ps -p 4242 -o command=", result: sharedArgv},
			// session-a's own launch recorded pid 4242 in its own pidfile —
			// the shared taskDir argv match alone would be ambiguous
			// between the two sibling sessions, but the pidfile disambiguates.
			sshScriptRule{match: "cat -- '/remote/session-a/agentctl.pid'", result: sshOut("4242")},
			sshScriptRule{match: "rm -rf '/remote/session-a'", result: sshOK},
		).handle)
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())

		metadata := sshConnectionMetadata(t, server)
		metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
		metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session-a"
		metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"

		err := exec.StopInstance(context.Background(), &ExecutorInstance{
			InstanceID: "session-a-instance",
			StopReason: "startup terminal session cleanup",
			Metadata:   metadata,
		}, true)
		if err != nil {
			t.Fatalf("StopInstance: %v", err)
		}
		if _, ok := server.lastCommandContaining("kill 4242"); !ok {
			t.Fatalf("expected session-a's own confirmed pid to be killed, commands: %v", server.commands())
		}
	})

	t.Run("row whose metadata pid was corrupted to a sibling's live pid is not signalled", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: "ps -p 4242 -o command=", result: sharedArgv},
			// session-b's own launch recorded a different pid (9999); its
			// persisted metadata pid of 4242 does not belong to it.
			sshScriptRule{match: "cat -- '/remote/session-b/agentctl.pid'", result: sshOut("9999")},
			sshScriptRule{match: "rm -rf '/remote/session-b'", result: sshOK},
		).handle)
		exec := NewSSHExecutor(nil, nil, nil, newTestLogger())

		metadata := sshConnectionMetadata(t, server)
		metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
		metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session-b"
		metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"

		err := exec.StopInstance(context.Background(), &ExecutorInstance{
			InstanceID: "session-b-instance",
			StopReason: "startup terminal session cleanup",
			Metadata:   metadata,
		}, true)
		if err != nil {
			t.Fatalf("StopInstance: %v", err)
		}
		if _, ok := server.lastCommandContaining("kill 4242"); ok {
			t.Fatalf("expected no kill when the pidfile names a different pid, commands: %v", server.commands())
		}
		if _, ok := server.lastCommandContaining("rm -rf '/remote/session-b'"); !ok {
			t.Fatalf("expected session-b's own session dir to still be reclaimed, commands: %v", server.commands())
		}
	})
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

// TestSSHExecutorStopInstanceIdentityMismatchDirRemovalFailurePropagatesError
// covers R2-F3: when the pid doesn't belong to our agentctl and the session
// dir can't be reclaimed, StopInstance must error instead of warning and
// returning nil — otherwise the caller prunes the executors_running row and
// the session dir leaks with nothing left to retry it.
func TestSSHExecutorStopInstanceIdentityMismatchDirRemovalFailurePropagatesError(t *testing.T) {
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/usr/bin/some-other-process --unrelated")},
		sshScriptRule{match: "rm -rf '/remote/session'", result: sshFail("permission denied")},
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
		t.Fatal("expected StopInstance to propagate a failing session dir removal for an unmatched pid")
	}
}

// TestSSHExecutorStopInstanceIdentityTaskDirMismatchSkipsKill covers the arm
// of remoteAgentctlCommandLineMatches that actually fires in production: the
// remote command line has "agentctl" and a --workdir, but it names a
// different taskDir than the one persisted for this row (e.g. a stale row
// whose pid was reused by another task's agentctl). That must not match.
func TestSSHExecutorStopInstanceIdentityTaskDirMismatchSkipsKill(t *testing.T) {
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir /remote/task-other")},
		sshScriptRule{match: "rm -rf '/remote/session'", result: sshOK},
	).handle)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())

	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
	metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task-abc"

	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "orphaned-instance",
		StopReason: "startup terminal session cleanup",
		Metadata:   metadata,
	}, true)
	if err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
	if _, ok := server.lastCommandContaining("kill 4242"); ok {
		t.Fatalf("expected no kill when the remote command line's taskDir doesn't match, commands: %v", server.commands())
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
		sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir /remote/task")},
		sshScriptRule{match: "cat -- '/remote/session/agentctl.pid'", result: sshOut("4242")},
		sshScriptRule{match: "kill 4242", result: sshFail("permission denied")},
	).handle)
	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())

	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
	metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"

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
