// Package wsinput keeps a WebSocket reader active while consumers process
// already-read application messages.
package wsinput

import (
	"errors"
	"sync"

	"github.com/gorilla/websocket"
)

const (
	maxBufferedMessages = 256
	maxBufferedBytes    = 1 << 20
)

// ErrBacklogFull is returned when a consumer cannot keep up with incoming
// messages. The connection is closed instead of blocking control-frame reads.
var ErrBacklogFull = errors.New("websocket input backlog is full")

// Message is an application data message read from a WebSocket.
type Message struct {
	Type int
	Data []byte
	size int
}

// Reader owns the only read loop for a connection. It buffers up to 256
// messages and 1 MiB of pending payload so a consumer can block without
// stopping pong handling.
type Reader struct {
	messages      chan Message
	done          chan struct{}
	err           error
	bufferedMu    sync.Mutex
	bufferedCount int
	bufferedBytes int
}

// New starts the connection's single read loop.
func New(conn *websocket.Conn) *Reader {
	reader := &Reader{
		messages: make(chan Message, maxBufferedMessages),
		done:     make(chan struct{}),
	}
	go reader.read(conn)
	return reader
}

// Messages returns the ordered application messages until the read loop exits.
func (r *Reader) Messages() <-chan Message { return r.messages }

// Consume releases the buffered capacity after a caller receives a message.
func (r *Reader) Consume(message Message) {
	r.bufferedMu.Lock()
	r.bufferedCount--
	r.bufferedBytes -= message.size
	r.bufferedMu.Unlock()
}

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
		if !r.reserve(len(data)) {
			r.err = ErrBacklogFull
			_ = conn.Close()
			return
		}
		r.messages <- Message{Type: messageType, Data: data, size: len(data)}
	}
}

func (r *Reader) reserve(messageBytes int) bool {
	r.bufferedMu.Lock()
	defer r.bufferedMu.Unlock()
	if r.bufferedCount >= maxBufferedMessages {
		return false
	}
	// Keep a single large paste valid when no other message is buffered.
	if r.bufferedCount > 0 && messageBytes > maxBufferedBytes-r.bufferedBytes {
		return false
	}
	r.bufferedCount++
	r.bufferedBytes += messageBytes
	return true
}
