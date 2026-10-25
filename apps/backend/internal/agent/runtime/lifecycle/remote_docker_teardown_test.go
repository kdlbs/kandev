package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// newTeardownSSHServer returns a fake remote host that reports remoteHome as
// $HOME and records every command it is asked to run.
func newTeardownSSHServer(t *testing.T, remoteHome string, rmResult sshExecResult) (*fakeSSHServer, *ssh.Client) {
	t.Helper()
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		switch {
		case strings.Contains(command, remoteHomeCommand):
			return sshOut(remoteHome)
		case strings.HasPrefix(command, "rm -rf"):
			return rmResult
		default:
			return sshOK
		}
	})
	t.Cleanup(server.Close)
	client := server.dial(t)
	t.Cleanup(func() { _ = client.Close() })
	return server, client
}

// TestRemoteDockerTerminalStopRemovesTheLegacySessionDir is the defect this
// work order fixes. A container provisioned before container-native delivery
// bind-mounts a per-instance directory on the remote host, and that directory
// holds the agent's credential files. Nothing removed it, so archiving or
// deleting the task left them there.
func TestRemoteDockerTerminalStopRemovesTheLegacySessionDir(t *testing.T) {
	server, client := newTeardownSSHServer(t, "/home/dev", sshOK)

	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	exec.sessions["instance-1"] = &remoteDockerSession{sshClient: client}

	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID:  "instance-1",
		TaskID:      "task-1",
		ContainerID: "", // nothing to remove on the daemon; the host cleanup still runs
		StopReason:  StopReasonTaskDeleted,
	}, false)
	require.NoError(t, err)

	call, ok := server.lastCommandContaining("rm -rf")
	require.True(t, ok, "no removal issued; commands = %v", server.commands())
	require.Contains(t, call.Command, "/home/dev/.kandev/agent-sessions/instance-1")
}

// TestRemoteDockerRemovalNamesOnlyThePerInstanceDir keeps a recursive delete on
// a machine Kandev does not own pointed at exactly one directory.
func TestRemoteDockerRemovalNamesOnlyThePerInstanceDir(t *testing.T) {
	server, client := newTeardownSSHServer(t, "/home/dev", sshOK)

	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	exec.sessions["instance-1"] = &remoteDockerSession{sshClient: client}

	require.NoError(t, exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "instance-1",
		TaskID:     "task-1",
		StopReason: StopReasonTaskArchived,
		// Metadata a caller could have influenced must not steer the path.
		Metadata: map[string]interface{}{"session_dir": "/etc"},
	}, false))

	call, ok := server.lastCommandContaining("rm -rf")
	require.True(t, ok)
	require.NotContains(t, call.Command, "/etc")
	require.Equal(t, `rm -rf '/home/dev/.kandev/agent-sessions/instance-1'`, strings.TrimSpace(call.Command),
		"the path is composed and quoted, never taken from metadata")
}

// TestRemoteDockerResumableStopKeepsTheSessionDir preserves the directory for a
// container that is coming back. Removing it on an ordinary stop would log the
// agent out on the next resume.
func TestRemoteDockerResumableStopKeepsTheSessionDir(t *testing.T) {
	server, client := newTeardownSSHServer(t, "/home/dev", sshOK)

	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	exec.sessions["instance-1"] = &remoteDockerSession{sshClient: client}

	require.NoError(t, exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "instance-1",
		TaskID:     "task-1",
		StopReason: "", // an ordinary stop
	}, false))

	_, ok := server.lastCommandContaining("rm -rf")
	require.False(t, ok, "an ordinary stop removed the session dir; commands = %v", server.commands())
}

// TestRemoteDockerStopSurvivesAFailedRemoval keeps archive and delete working.
// Stop runs inside both, so a remote that refuses the removal must not fail
// them.
func TestRemoteDockerStopSurvivesAFailedRemoval(t *testing.T) {
	server, client := newTeardownSSHServer(t, "/home/dev", sshFail("rm: permission denied"))

	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	exec.sessions["instance-1"] = &remoteDockerSession{sshClient: client}

	require.NoError(t, exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "instance-1",
		TaskID:     "task-1",
		StopReason: StopReasonTaskDeleted,
	}, false), "a failed removal must not fail the stop")

	_, ok := server.lastCommandContaining("rm -rf")
	require.True(t, ok, "the removal was never attempted")
}

// TestRemoteDockerStopWithoutAConnectionSkipsRemoval keeps the existing
// no-connection outcome intact: there is nothing to reach, and the stop must
// not add a new way to fail.
func TestRemoteDockerStopWithoutAConnectionSkipsRemoval(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	exec.sessions["instance-1"] = &remoteDockerSession{} // no SSH client

	require.NoError(t, exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "instance-1",
		TaskID:     "task-1",
		StopReason: StopReasonTaskDeleted,
	}, false))
}
