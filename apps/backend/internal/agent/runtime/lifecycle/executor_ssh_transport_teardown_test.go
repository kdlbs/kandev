package lifecycle

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type clientReleaseListener struct {
	clientClosed <-chan struct{}
	closed       chan struct{}
	once         sync.Once
}

func (l *clientReleaseListener) Accept() (net.Conn, error) {
	<-l.closed
	return nil, net.ErrClosed
}

func (l *clientReleaseListener) Close() error {
	<-l.clientClosed
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *clientReleaseListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}
}

// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.4
func TestSSHExecutorStopInstanceClosesClientBeforeRuntimeAPITunnel(t *testing.T) {
	clientClosed := make(chan struct{})
	listenerClosed := make(chan struct{})
	listener := &clientReleaseListener{clientClosed: clientClosed, closed: listenerClosed}

	exec := NewSSHExecutor(nil, nil, nil, newTestLogger())
	exec.stopRemote = func(context.Context, *ssh.Client, string, int) error { return nil }
	exec.closeClient = func(*ssh.Client) error {
		close(clientClosed)
		return nil
	}
	exec.sessions["instance-1"] = &sshSessionState{
		client:           &ssh.Client{},
		runtimeAPITunnel: &sshRuntimeAPITunnel{listener: listener},
		remoteDir:        "/remote/session",
		pid:              4242,
	}

	done := make(chan error, 1)
	go func() {
		done <- exec.StopInstance(context.Background(), &ExecutorInstance{
			InstanceID: "instance-1",
			StopReason: StopReasonTaskDeleted,
		}, false)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StopInstance: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("StopInstance waited for the runtime API tunnel before closing the SSH client")
	}
	select {
	case <-listenerClosed:
	case <-time.After(time.Second):
		t.Fatal("runtime API tunnel did not close")
	}
}

// @covers AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.5
func TestSSHKeepaliveTeardownClosesRuntimeAPITunnelAfterClient(t *testing.T) {
	exec, _ := newObservedSSHExecutor(t)
	server := newFakeSSHServer(t, nil)
	client := server.dial(t)

	clientClosed := make(chan struct{})
	listenerClosed := make(chan struct{})
	state := &sshSessionState{
		target: &SSHTarget{Host: "build.example", User: "deploy", PinnedFingerprint: "SHA256:x"},
		client: client,
		runtimeAPITunnel: &sshRuntimeAPITunnel{listener: &clientReleaseListener{
			clientClosed: clientClosed,
			closed:       listenerClosed,
		}},
	}
	exec.closeClient = func(c *ssh.Client) error {
		close(clientClosed)
		return c.Close()
	}

	exec.transportTeardown("instance-1", state, sshTransportLostReasonDeadline, time.Second)
	select {
	case <-listenerClosed:
	case <-time.After(time.Second):
		t.Fatal("transport teardown did not close the runtime API tunnel")
	}
}

type orderedCloseConn struct {
	bastionClosed <-chan struct{}
	closed        chan struct{}
	once          sync.Once
}

func (c *orderedCloseConn) Read([]byte) (int, error) { return 0, io.EOF }

func (c *orderedCloseConn) Write(p []byte) (int, error) { return len(p), nil }

func (c *orderedCloseConn) Close() error {
	<-c.bastionClosed
	c.once.Do(func() { close(c.closed) })
	return nil
}

func (c *orderedCloseConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}
}

func (c *orderedCloseConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}
}

func (c *orderedCloseConn) SetDeadline(time.Time) error      { return nil }
func (c *orderedCloseConn) SetReadDeadline(time.Time) error  { return nil }
func (c *orderedCloseConn) SetWriteDeadline(time.Time) error { return nil }

type closeSignal struct {
	closed chan struct{}
	once   sync.Once
}

func (s *closeSignal) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

func TestSSHProxyJumpConnClosesBastionBeforeTunnel(t *testing.T) {
	bastionClosed := make(chan struct{})
	tunnelClosed := make(chan struct{})
	conn := &sshProxyJumpConn{
		Conn:    &orderedCloseConn{bastionClosed: bastionClosed, closed: tunnelClosed},
		bastion: &closeSignal{closed: bastionClosed},
	}

	done := make(chan error, 1)
	go func() { done <- conn.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("sshProxyJumpConn.Close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("sshProxyJumpConn.Close waited for the tunnel before closing the bastion")
	}
	select {
	case <-tunnelClosed:
	case <-time.After(time.Second):
		t.Fatal("sshProxyJumpConn did not close the tunnel")
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("second sshProxyJumpConn.Close: %v", err)
	}
}
