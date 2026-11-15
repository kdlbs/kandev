package wskeepalive

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gorilla/websocket"
)

func TestKeepaliveDefaults(t *testing.T) {
	got := DefaultTiming()
	if got.WriteWait != 10*time.Second || got.PongWait != 60*time.Second || got.PingPeriod != 54*time.Second {
		t.Fatalf("DefaultTiming() = %+v, want write=10s ping=54s pong=60s", got)
	}
}

func TestKeepalivePongDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		conn := &fakeKeepaliveConn{}
		timing := Timing{WriteWait: time.Second, PingPeriod: 5 * time.Second, PongWait: 20 * time.Second}
		stop, err := start(conn, timing)
		if err != nil {
			t.Fatalf("start keepalive: %v", err)
		}
		defer stop()
		initialDeadline := conn.readDeadline

		synctest.Wait()
		time.Sleep(timing.PingPeriod)
		synctest.Wait()
		if len(conn.pings) != 1 {
			t.Fatalf("ping writes = %d, want 1", len(conn.pings))
		}
		if got := conn.writeDeadline.Sub(conn.writeAt); got != timing.WriteWait {
			t.Fatalf("control write deadline delta = %s, want %s", got, timing.WriteWait)
		}
		if conn.readDeadline != initialDeadline {
			t.Fatalf("outbound ping changed read deadline to %v, want %v", conn.readDeadline, initialDeadline)
		}

		pongAt := time.Now()
		if err := conn.pongHandler("ignored"); err != nil {
			t.Fatalf("pong handler: %v", err)
		}
		if want := pongAt.Add(timing.PongWait); conn.readDeadline != want {
			t.Fatalf("pong deadline = %v, want %v", conn.readDeadline, want)
		}
	})
}

func TestKeepaliveWriteFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		conn := &fakeKeepaliveConn{writeErr: errors.New("control write failed")}
		timing := Timing{WriteWait: time.Second, PingPeriod: time.Second, PongWait: 3 * time.Second}
		stop, err := start(conn, timing)
		if err != nil {
			t.Fatalf("start keepalive: %v", err)
		}
		time.Sleep(timing.PingPeriod)
		synctest.Wait()
		if !conn.closed {
			t.Fatal("write failure did not close the connection")
		}
		stop()
	})
}

func TestKeepaliveStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		conn := &fakeKeepaliveConn{}
		timing := Timing{WriteWait: time.Second, PingPeriod: time.Hour, PongWait: 2 * time.Hour}
		stop, err := start(conn, timing)
		if err != nil {
			t.Fatalf("start keepalive: %v", err)
		}
		stop()
		stop()
		synctest.Wait()
		if conn.closed {
			t.Fatal("stopping the loop closed the connection")
		}
	})
}

func TestKeepaliveRejectsInvalidTiming(t *testing.T) {
	conn := &fakeKeepaliveConn{}
	_, err := start(conn, Timing{WriteWait: time.Second, PingPeriod: time.Second, PongWait: time.Second})
	if !errors.Is(err, errInvalidTiming) {
		t.Fatalf("start error = %v, want invalid timing", err)
	}
	if conn.deadlineCalls != 0 {
		t.Fatalf("read deadline calls = %d, want 0 for invalid timing", conn.deadlineCalls)
	}
}

func TestKeepaliveSetupFailureClosesConnection(t *testing.T) {
	wantErr := errors.New("deadline rejected")
	conn := &fakeKeepaliveConn{deadlineErr: wantErr}
	_, err := start(conn, DefaultTiming())
	if !errors.Is(err, wantErr) {
		t.Fatalf("start error = %v, want wrapped deadline error", err)
	}
	if !conn.closed {
		t.Fatal("failed initial deadline setup did not close the connection")
	}
}

type fakeKeepaliveConn struct {
	readDeadline  time.Time
	deadlineCalls int
	pongHandler   func(string) error
	pings         [][]byte
	writeAt       time.Time
	writeDeadline time.Time
	writeErr      error
	deadlineErr   error
	closed        bool
}

func (c *fakeKeepaliveConn) SetReadDeadline(deadline time.Time) error {
	c.readDeadline = deadline
	c.deadlineCalls++
	return c.deadlineErr
}

func (c *fakeKeepaliveConn) SetPongHandler(handler func(string) error) {
	c.pongHandler = handler
}

func (c *fakeKeepaliveConn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	if messageType != websocket.PingMessage {
		return errors.New("unexpected control message type")
	}
	c.pings = append(c.pings, append([]byte(nil), data...))
	c.writeAt = time.Now()
	c.writeDeadline = deadline
	return c.writeErr
}

func (c *fakeKeepaliveConn) Close() error {
	c.closed = true
	return nil
}
