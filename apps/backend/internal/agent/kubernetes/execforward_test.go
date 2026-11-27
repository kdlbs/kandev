package kubernetes

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeBridgeExecutor plays the in-Pod `agentctl tcp-stdio` side of an exec
// stream: it dials the target for the requested port and copies bytes both
// ways until the target closes or the exec context ends.
type fakeBridgeExecutor struct {
	mu       sync.Mutex
	targets  map[string]string
	requests []ExecRequest
	execErr  error
	block    bool
}

func (f *fakeBridgeExecutor) Exec(ctx context.Context, request ExecRequest) error {
	f.mu.Lock()
	f.requests = append(f.requests, request)
	execErr, block := f.execErr, f.block
	target := f.targets[request.Command[len(request.Command)-1]]
	f.mu.Unlock()
	if execErr != nil {
		return execErr
	}
	if block {
		<-ctx.Done()
		return ctx.Err()
	}
	upstream, err := (&net.Dialer{}).DialContext(ctx, "tcp", target)
	if err != nil {
		return err
	}
	defer func() { _ = upstream.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = upstream.Close() })
	defer stop()
	go func() {
		_, _ = io.Copy(upstream, request.Stdin)
		_ = upstream.(*net.TCPConn).CloseWrite()
	}()
	_, err = io.Copy(request.Stdout, upstream)
	return err
}

func (f *fakeBridgeExecutor) recorded() []ExecRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ExecRequest(nil), f.requests...)
}

func startExecForwardTarget(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "agentctl:"+r.URL.Path)
	}))
	t.Cleanup(server.Close)
	return server, server.Listener.Addr().String()
}

func execForwardRequest(remotePort uint16) PortForwardRequest {
	return PortForwardRequest{
		Namespace: "kandev-agents", Pod: "session-pod", Container: "kandev-agent",
		LocalAddress: loopbackAddress, RemotePort: remotePort,
	}
}

func TestExecPortForwarderCarriesHTTPOverOneExecPerConnection(t *testing.T) {
	_, target := startExecForwardTarget(t)
	executor := &fakeBridgeExecutor{targets: map[string]string{"8765": target}}
	forwarder := &ExecPortForwarder{Executor: executor, BridgePath: "/opt/kandev/agentctl"}
	session, err := NewStreamOperations(executor, forwarder).Forward(context.Background(), execForwardRequest(8765))
	require.NoError(t, err)
	defer func() { _ = session.Close() }()
	<-session.Ready()
	require.NotZero(t, session.LocalPort())

	baseURL := "http://127.0.0.1:" + strconv.Itoa(int(session.LocalPort()))
	for _, path := range []string{"/health", "/api/v1/status"} {
		client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
		response, err := client.Get(baseURL + path)
		require.NoError(t, err)
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		require.NoError(t, err)
		require.Equal(t, "agentctl:"+path, string(body))
	}

	requests := executor.recorded()
	require.Len(t, requests, 2)
	for _, request := range requests {
		require.Equal(t, "kandev-agents", request.Namespace)
		require.Equal(t, "session-pod", request.Pod)
		require.Equal(t, "kandev-agent", request.Container)
		require.Equal(t, []string{"/opt/kandev/agentctl", "tcp-stdio", "--port", "8765"}, request.Command)
		require.False(t, request.TTY)
		require.NotNil(t, request.Stdin)
		require.NotNil(t, request.Stdout)
	}
}

func TestExecPortForwarderServesConcurrentConnections(t *testing.T) {
	_, target := startExecForwardTarget(t)
	executor := &fakeBridgeExecutor{targets: map[string]string{"41001": target}}
	forwarder := &ExecPortForwarder{Executor: executor, BridgePath: "/opt/kandev/agentctl"}
	session, err := forwarder.Forward(context.Background(), execForwardRequest(41001))
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	const connections = 8
	var wg sync.WaitGroup
	errs := make(chan error, connections)
	for i := 0; i < connections; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
			response, err := client.Get("http://127.0.0.1:" + strconv.Itoa(int(session.LocalPort())) + "/c" + strconv.Itoa(i))
			if err != nil {
				errs <- err
				return
			}
			body, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err == nil && string(body) != "agentctl:/c"+strconv.Itoa(i) {
				err = errors.New("unexpected body " + string(body))
			}
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Len(t, executor.recorded(), connections)
}

func TestExecPortForwarderCloseCancelsInFlightExecAndReportsCleanStop(t *testing.T) {
	executor := &fakeBridgeExecutor{block: true}
	forwarder := &ExecPortForwarder{Executor: executor, BridgePath: "/opt/kandev/agentctl"}
	session, err := forwarder.Forward(context.Background(), execForwardRequest(8765))
	require.NoError(t, err)

	conn, err := net.Dial("tcp", net.JoinHostPort(loopbackAddress, strconv.Itoa(int(session.LocalPort()))))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	require.Eventually(t, func() bool { return len(executor.recorded()) == 1 }, 2*time.Second, 10*time.Millisecond)

	closed := make(chan struct{})
	go func() { _ = session.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not cancel the in-flight exec stream")
	}
	require.NoError(t, <-session.Done())
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, readErr := conn.Read(make([]byte, 1))
	require.Error(t, readErr, "local connection must be closed with the session")

	_, dialErr := net.DialTimeout("tcp", net.JoinHostPort(loopbackAddress, strconv.Itoa(int(session.LocalPort()))), 200*time.Millisecond)
	require.Error(t, dialErr, "listener must stop accepting after Close")
}

func TestExecPortForwarderReportsParentCancellation(t *testing.T) {
	executor := &fakeBridgeExecutor{block: true}
	forwarder := &ExecPortForwarder{Executor: executor, BridgePath: "/opt/kandev/agentctl"}
	ctx, cancel := context.WithCancel(context.Background())
	session, err := forwarder.Forward(ctx, execForwardRequest(8765))
	require.NoError(t, err)
	cancel()
	select {
	case doneErr := <-session.Done():
		require.ErrorIs(t, doneErr, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("parent cancellation did not stop the session")
	}
	require.NoError(t, session.Close())
}

func TestExecPortForwarderClosesConnectionWhenExecFails(t *testing.T) {
	executor := &fakeBridgeExecutor{execErr: errors.New("pods \"session-pod\" is forbidden")}
	forwarder := &ExecPortForwarder{Executor: executor, BridgePath: "/opt/kandev/agentctl"}
	session, err := forwarder.Forward(context.Background(), execForwardRequest(8765))
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	conn, err := net.Dial("tcp", net.JoinHostPort(loopbackAddress, strconv.Itoa(int(session.LocalPort()))))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, readErr := conn.Read(make([]byte, 1))
	require.ErrorIs(t, readErr, io.EOF)

	select {
	case doneErr := <-session.Done():
		t.Fatalf("a failed connection must not stop the session, got %v", doneErr)
	default:
	}
}

func TestExecPortForwarderRejectsIncompleteRequests(t *testing.T) {
	executor := &fakeBridgeExecutor{}
	forwarder := &ExecPortForwarder{Executor: executor, BridgePath: "/opt/kandev/agentctl"}

	missingContainer := execForwardRequest(8765)
	missingContainer.Container = ""
	_, err := forwarder.Forward(context.Background(), missingContainer)
	var fieldErr *FieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "port_forward.container", fieldErr.Path)

	_, err = forwarder.Forward(context.Background(), execForwardRequest(0))
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "port_forward.remote_port", fieldErr.Path)

	_, err = (&ExecPortForwarder{Executor: executor}).Forward(context.Background(), execForwardRequest(8765))
	require.ErrorContains(t, err, "not configured")

	_, err = NewStreamOperations(executor, forwarder).Forward(context.Background(), PortForwardRequest{
		Namespace: "kandev-agents", Pod: "session-pod", Container: "Bad_Name", RemotePort: 8765,
	})
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "port_forward.container", fieldErr.Path)
}

func TestExecBridgeCommand(t *testing.T) {
	require.Equal(t,
		[]string{"/opt/kandev/agentctl", "tcp-stdio", "--port", "41001"},
		ExecBridgeCommand("/opt/kandev/agentctl", 41001),
	)
}
