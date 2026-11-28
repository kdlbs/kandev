package kubernetes

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// stderrFailingExecutor fails every exec stream the way a dead Pod or a
// refused in-Pod dial does: it writes bridge stderr, then returns an error.
type stderrFailingExecutor struct {
	stderr string
	err    error
}

func (e *stderrFailingExecutor) Exec(_ context.Context, request ExecRequest) error {
	if request.Stderr != nil {
		_, _ = io.WriteString(request.Stderr, e.stderr)
	}
	return e.err
}

func observedExecForwarder(executor PodExecutor) (*ExecPortForwarder, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	return &ExecPortForwarder{Executor: executor, BridgePath: "/opt/kandev/agentctl", Logger: zap.New(core)}, logs
}

func waitForLogs(t *testing.T, logs *observer.ObservedLogs, message string, count int) []observer.LoggedEntry {
	t.Helper()
	var entries []observer.LoggedEntry
	require.Eventually(t, func() bool {
		entries = logs.FilterMessage(message).All()
		return len(entries) >= count
	}, 5*time.Second, 10*time.Millisecond, "waiting for %d %q log entries", count, message)
	return entries
}

func TestExecPortForwarderLogsConnectionOpenAndCloseAtDebug(t *testing.T) {
	_, target := startExecForwardTarget(t)
	forwarder, logs := observedExecForwarder(&fakeBridgeExecutor{targets: map[string]string{"8765": target}})
	session, err := forwarder.Forward(context.Background(), execForwardRequest(8765))
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	response, err := client.Get("http://127.0.0.1:" + strconv.Itoa(int(session.LocalPort())) + "/health")
	require.NoError(t, err)
	_, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()

	opened := waitForLogs(t, logs, "exec transport connection opened", 1)
	closed := waitForLogs(t, logs, "exec transport connection closed", 1)
	require.Equal(t, zapcore.DebugLevel, opened[0].Level)
	require.Equal(t, zapcore.DebugLevel, closed[0].Level)
	fields := opened[0].ContextMap()
	require.Equal(t, "kandev-agents", fields["namespace"])
	require.Equal(t, "session-pod", fields["pod"])
	require.Equal(t, "kandev-agent", fields["container"])
	require.EqualValues(t, 8765, fields["remote_port"])
	require.NotEmpty(t, fields["local_peer"])
	require.Equal(t, false, closed[0].ContextMap()["stream_error"])
	require.Empty(t, logs.FilterLevelExact(zapcore.WarnLevel).All(), "a clean stream is not a warning")
}

func TestExecPortForwarderWarnsOnStreamErrorWithBoundedRedactedStderr(t *testing.T) {
	secret := "ghp_" + strings.Repeat("A", 36)
	stderr := strings.Repeat("x", 3*execBridgeStderrLimit) +
		"\nagentctl tcp-stdio: dial tcp 127.0.0.1:8765: connect: connection refused token=" + secret + "\n"
	forwarder, logs := observedExecForwarder(&stderrFailingExecutor{
		stderr: stderr, err: errors.New("command terminated with exit code 1"),
	})
	session, err := forwarder.Forward(context.Background(), execForwardRequest(8765))
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	conn, err := net.Dial("tcp", net.JoinHostPort(loopbackAddress, strconv.Itoa(int(session.LocalPort()))))
	require.NoError(t, err)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, readErr := conn.Read(make([]byte, 1))
	require.Error(t, readErr, "the failed exec closes the local connection")
	_ = conn.Close()

	warnings := waitForLogs(t, logs, "exec transport stream failed", 1)
	require.Equal(t, zapcore.WarnLevel, warnings[0].Level)
	fields := warnings[0].ContextMap()
	loggedStderr, ok := fields["stderr"].(string)
	require.True(t, ok)
	require.Contains(t, loggedStderr, "connection refused")
	require.NotContains(t, loggedStderr, secret, "bridge stderr is credential-redacted")
	require.LessOrEqual(t, len(loggedStderr), execBridgeStderrLimit+len("…"), "only the stderr tail is kept")
	require.Equal(t, "command terminated with exit code 1", fields["error"])
	require.Equal(t, "session-pod", fields["pod"])
	closed := waitForLogs(t, logs, "exec transport connection closed", 1)
	require.Equal(t, true, closed[0].ContextMap()["stream_error"])
}

func TestExecPortForwarderDoesNotWarnWhenSessionCloses(t *testing.T) {
	forwarder, logs := observedExecForwarder(&fakeBridgeExecutor{block: true})
	session, err := forwarder.Forward(context.Background(), execForwardRequest(8765))
	require.NoError(t, err)
	conn, err := net.Dial("tcp", net.JoinHostPort(loopbackAddress, strconv.Itoa(int(session.LocalPort()))))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	waitForLogs(t, logs, "exec transport connection opened", 1)

	require.NoError(t, session.Close())

	waitForLogs(t, logs, "exec transport connection closed", 1)
	require.Empty(t, logs.FilterMessage("exec transport stream failed").All(),
		"cancelling the session is not a stream failure")
}

func TestExecPortForwarderWithoutLoggerStaysSilent(t *testing.T) {
	forwarder := &ExecPortForwarder{
		Executor:   &stderrFailingExecutor{stderr: "boom", err: errors.New("exec failed")},
		BridgePath: "/opt/kandev/agentctl",
	}
	session, err := forwarder.Forward(context.Background(), execForwardRequest(8765))
	require.NoError(t, err)
	defer func() { _ = session.Close() }()
	conn, err := net.Dial("tcp", net.JoinHostPort(loopbackAddress, strconv.Itoa(int(session.LocalPort()))))
	require.NoError(t, err)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, readErr := conn.Read(make([]byte, 1))
	require.Error(t, readErr)
	_ = conn.Close()
}

func TestBoundedTailKeepsValidUTF8Tail(t *testing.T) {
	tail := &boundedTail{limit: 4}
	_, _ = tail.Write([]byte("abc"))
	_, _ = tail.Write([]byte("dé"))
	require.Equal(t, "…cdé", tail.redacted())
	short := &boundedTail{limit: 16}
	_, _ = short.Write([]byte("  ok \n"))
	require.Equal(t, "ok", short.redacted())
}
