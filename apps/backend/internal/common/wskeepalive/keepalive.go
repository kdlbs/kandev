// Package wskeepalive provides connection-level liveness for browser WebSockets.
package wskeepalive

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// DefaultWriteWait bounds a WebSocket control-frame write.
	DefaultWriteWait = 10 * time.Second
	// DefaultPongWait bounds the time between received pong frames.
	DefaultPongWait = 60 * time.Second
	// DefaultPingPeriod sends pings before the default pong deadline.
	DefaultPingPeriod = 54 * time.Second
)

var errInvalidTiming = errors.New("invalid websocket keepalive timing")

// Timing defines the per-connection ping and pong deadlines.
type Timing struct {
	WriteWait  time.Duration
	PongWait   time.Duration
	PingPeriod time.Duration
}

// DefaultTiming returns the production terminal connection policy.
func DefaultTiming() Timing {
	return Timing{
		WriteWait:  DefaultWriteWait,
		PongWait:   DefaultPongWait,
		PingPeriod: DefaultPingPeriod,
	}
}

func (timing Timing) validate() error {
	if timing.WriteWait <= 0 || timing.PongWait <= 0 || timing.PingPeriod <= 0 || timing.PingPeriod >= timing.PongWait {
		return fmt.Errorf("%w: write=%s ping=%s pong=%s", errInvalidTiming, timing.WriteWait, timing.PingPeriod, timing.PongWait)
	}
	return nil
}

type keepaliveConn interface {
	SetReadDeadline(time.Time) error
	SetPongHandler(func(string) error)
	WriteControl(int, []byte, time.Time) error
	Close() error
}

// Start installs the pong deadline before starting a per-connection ping loop.
// The returned stop function is safe to call more than once and joins the loop.
func Start(conn *websocket.Conn, timing Timing) (func(), error) {
	if conn == nil {
		return nil, errors.New("websocket connection is nil")
	}
	return start(conn, timing)
}

func start(conn keepaliveConn, timing Timing) (func(), error) {
	if err := timing.validate(); err != nil {
		return nil, err
	}
	if err := conn.SetReadDeadline(time.Now().Add(timing.PongWait)); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("set initial websocket pong deadline: %w", err)
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(timing.PongWait))
	})

	stopCh := make(chan struct{})
	done := make(chan struct{})
	var stopOnce sync.Once
	go func() {
		defer close(done)
		ticker := time.NewTicker(timing.PingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				deadline := time.Now().Add(timing.WriteWait)
				if err := conn.WriteControl(websocket.PingMessage, nil, deadline); err != nil {
					_ = conn.Close()
					return
				}
			}
		}
	}()

	return func() {
		stopOnce.Do(func() {
			close(stopCh)
			<-done
		})
	}, nil
}
