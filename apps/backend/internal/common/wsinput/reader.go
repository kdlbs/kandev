// Package wsinput keeps a WebSocket reader active while consumers process
// already-read application messages.
package wsinput

import (
	"errors"

	"github.com/gorilla/websocket"
)

const messageBufferSize = 1

// ErrBacklogFull is returned when a consumer cannot keep up with incoming
// messages. The connection is closed instead of blocking control-frame reads.
var ErrBacklogFull = errors.New("websocket input backlog is full")

// Message is an application data message read from a WebSocket.
type Message struct {
	Type int
	Data []byte
}

// Reader owns the only read loop for a connection. It buffers one pending
// application message so a consumer can block without stopping pong handling.
type Reader struct {
	messages chan Message
	done     chan struct{}
	err      error
}

// New starts the connection's single read loop.
func New(conn *websocket.Conn) *Reader {
	reader := &Reader{
		messages: make(chan Message, messageBufferSize),
		done:     make(chan struct{}),
	}
	go reader.read(conn)
	return reader
}

// Messages returns the ordered application messages until the read loop exits.
func (r *Reader) Messages() <-chan Message { return r.messages }

// Err waits for the read loop to finish and returns its terminal error.
func (r *Reader) Err() error {
	<-r.done
	return r.err
}

func (r *Reader) read(conn *websocket.Conn) {
	defer close(r.done)
	defer close(r.messages)
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			r.err = err
			return
		}
		select {
		case r.messages <- Message{Type: messageType, Data: data}:
		default:
			r.err = ErrBacklogFull
			_ = conn.Close()
			return
		}
	}
}
