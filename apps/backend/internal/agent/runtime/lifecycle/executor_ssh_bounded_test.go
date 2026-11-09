package lifecycle

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/common/sshlaunch"
)

func TestBoundedSSHOutputCapsRetentionAndDrains(t *testing.T) {
	overflowed := 0
	output := newBoundedSSHOutput(8, func() { overflowed++ })
	payload := strings.Repeat("x", 64)
	written, err := output.Write([]byte(payload))
	require.NoError(t, err)
	require.Equal(t, len(payload), written, "the writer must consume every byte so the channel drains")
	require.Len(t, output.String(), 8)
	require.True(t, output.Overflowed())

	_, _ = output.Write([]byte("more"))
	require.GreaterOrEqual(t, overflowed, 1, "overflow is reported to the caller")
	require.Len(t, output.String(), 8)
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.21
func TestRunSSHCommandStdinBoundedRejectsOversizedStdout(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	server.setStreamHandler(func(_ string, stream ssh.Channel) int {
		_, _ = io.WriteString(stream, strings.Repeat("x", 4096))
		return 0
	})
	stdout, stderr, overflow, err := runSSHCommandStdinBounded(
		context.Background(), server.dial(t), "helper", nil, 64)
	require.NoError(t, err)
	require.True(t, overflow)
	require.LessOrEqual(t, len(stdout), 64)
	require.LessOrEqual(t, len(stderr), 64)
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.21
func TestRunSSHCommandStdinBoundedRejectsOversizedStderr(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	server.setStreamHandler(func(_ string, stream ssh.Channel) int {
		_, _ = io.WriteString(stream.Stderr(), strings.Repeat("e", 4096))
		return 0
	})
	stdout, stderr, overflow, err := runSSHCommandStdinBounded(
		context.Background(), server.dial(t), "helper", nil, 64)
	require.NoError(t, err)
	require.True(t, overflow)
	require.LessOrEqual(t, len(stdout), 64)
	require.LessOrEqual(t, len(stderr), 64)
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.21
func TestRunRemoteSSHReconcileOverflowIsUnresolved(t *testing.T) {
	receipt := testSSHLaunchReceipt(t, testSSHLaunchOwner(), 4242)
	server := newFakeSSHServer(t, nil)
	server.setStreamHandler(func(_ string, stream ssh.Channel) int {
		_, _ = io.WriteString(stream, strings.Repeat("x", maxSSHReconcileResponseBytes+1024))
		return 0
	})
	result := runRemoteSSHReconcile(context.Background(), server.dial(t), "/bin/agentctl", sshReconcileRequest(receipt))
	require.Equal(t, sshlaunch.OutcomeUnresolved, result.Outcome)
	require.Equal(t, sshlaunch.ReasonProbeUnknown, result.Reason)
}
