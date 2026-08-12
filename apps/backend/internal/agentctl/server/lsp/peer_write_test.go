package lsp

import (
	"context"
	"errors"
	"io"
	"math"
	"os"
	"sync"
	"testing"
	"time"
)

type blockingPeerWriter struct {
	started chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func TestBuildPeerFrame(t *testing.T) {
	frame, err := buildPeerFrame([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(frame), "Content-Length: 2\r\n\r\n{}"; got != want {
		t.Fatalf("frame = %q, want %q", got, want)
	}
	if _, err := checkedPeerFrameSize(math.MaxInt, 1); !errors.Is(err, errPeerFrameTooLarge) {
		t.Fatalf("overflow size error = %v, want frame too large", err)
	}
}

func TestPeerWriteTimeoutClosesBlockedProcessPipe(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Errorf("close pipe reader: %v", err)
		}
	})
	protocolPeer := &peer{stdin: writer, writeTimeout: 20 * time.Millisecond}

	err = protocolPeer.writeFrame(make([]byte, 1<<20))
	if !errors.Is(err, errPeerWriteTimeout) {
		t.Fatalf("process-pipe write error = %v, want peer write timeout", err)
	}
}

func newBlockingPeerWriter() *blockingPeerWriter {
	return &blockingPeerWriter{started: make(chan struct{}), closed: make(chan struct{})}
}

func (w *blockingPeerWriter) Write([]byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.closed
	return 0, io.ErrClosedPipe
}

func (w *blockingPeerWriter) Close() error {
	select {
	case <-w.closed:
	default:
		close(w.closed)
	}
	return nil
}

func TestPeerWriteTimeoutClosesBlockedStdin(t *testing.T) {
	writer := newBlockingPeerWriter()
	protocolPeer := &peer{
		stdin: writer, pending: make(map[string]chan rpcResponse), done: make(chan struct{}),
		writeTimeout: 20 * time.Millisecond,
	}

	startedAt := time.Now()
	err := protocolPeer.write(rpcMessage{JSONRPC: rpcVersion, Method: "test/blocked"})
	if !errors.Is(err, errPeerWriteTimeout) {
		t.Fatalf("write error = %v, want peer write timeout", err)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("blocked write returned after %s", elapsed)
	}
	select {
	case <-writer.closed:
	default:
		t.Fatal("timed-out writer was not closed")
	}
}

func TestPeerCanceledCallDoesNotWaitForBlockedWriter(t *testing.T) {
	writer := newBlockingPeerWriter()
	protocolPeer := &peer{
		stdin: writer, pending: make(map[string]chan rpcResponse), done: make(chan struct{}),
		writeTimeout: time.Second,
	}
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- protocolPeer.write(rpcMessage{JSONRPC: rpcVersion, Method: "test/blocked"})
	}()
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("first write did not block")
	}

	ctx, cancel := context.WithCancel(context.Background())
	callDone := make(chan error, 1)
	go func() {
		_, err := protocolPeer.callRaw(ctx, "textDocument/hover", nil)
		callDone <- err
	}()
	cancel()
	select {
	case err := <-callDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled call error = %v, want context canceled", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("canceled call remained blocked on the peer write lock")
	}

	_ = writer.Close()
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first blocked write did not exit")
	}
}
