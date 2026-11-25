package sshtransport

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

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
