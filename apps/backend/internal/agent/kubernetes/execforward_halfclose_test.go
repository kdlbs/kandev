package kubernetes

import (
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/tcpstdio"
)

// inPodBridgeExecutor runs the real `agentctl tcp-stdio` bridge in process on
// the exec stream's stdio, the way the container would after a pods/exec
// upgrade. Only the apiserver/kubelet hop is left out. Like a real executor
// it returns as soon as its context ends.
type inPodBridgeExecutor struct{}

func (inPodBridgeExecutor) Exec(ctx context.Context, request ExecRequest) error {
	done := make(chan int, 1)
	go func() { done <- tcpstdio.Run(request.Command[2:], request.Stdin, request.Stdout, request.Stderr) }()
	select {
	case code := <-done:
		if code != 0 {
			return io.ErrUnexpectedEOF
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// startRespondAfterEOFServer accepts one connection, reads the request until
// EOF, then answers after a pause in several delayed chunks and closes.
func startRespondAfterEOFServer(t *testing.T, chunks []string) (uint16, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	received := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		request, _ := io.ReadAll(conn)
		received <- string(request)
		time.Sleep(300 * time.Millisecond)
		for _, chunk := range chunks {
			_, _ = conn.Write([]byte(chunk))
			time.Sleep(100 * time.Millisecond)
		}
	}()
	return uint16(listener.Addr().(*net.TCPAddr).Port), received
}

// TestExecPortForwarderPreservesResponseAfterRequestEnds sends a request,
// half-closes the client side, and only then lets the server answer. The
// Kandev relay must pass the request EOF into the exec stream and keep
// relaying the response; the agentctl bridge must half-close the TCP write
// side and keep reading until the server closes.
func TestExecPortForwarderPreservesResponseAfterRequestEnds(t *testing.T) {
	chunks := []string{"first ", "second ", "third ", "fourth ", "last"}
	remotePort, received := startRespondAfterEOFServer(t, chunks)
	forwarder := &ExecPortForwarder{Executor: inPodBridgeExecutor{}, BridgePath: "/opt/kandev/agentctl"}
	session, err := forwarder.Forward(context.Background(), execForwardRequest(remotePort))
	require.NoError(t, err)
	defer func() { _ = session.Close() }()
	<-session.Ready()

	conn, err := net.Dial("tcp", net.JoinHostPort(loopbackAddress, strconv.Itoa(int(session.LocalPort()))))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	require.NoError(t, conn.SetDeadline(time.Now().Add(10*time.Second)))
	_, err = conn.Write([]byte("request body"))
	require.NoError(t, err)
	require.NoError(t, conn.(*net.TCPConn).CloseWrite())

	select {
	case got := <-received:
		require.Equal(t, "request body", got)
	case <-time.After(5 * time.Second):
		t.Fatal("server never saw the request EOF")
	}
	response, err := io.ReadAll(conn)
	require.NoError(t, err)
	require.Equal(t, strings.Join(chunks, ""), string(response))
}
