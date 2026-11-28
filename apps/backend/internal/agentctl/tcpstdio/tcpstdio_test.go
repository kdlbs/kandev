package tcpstdio

import (
	"bytes"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// startPeer accepts one connection on Pod-style loopback and hands it to serve.
func startPeer(t *testing.T, serve func(net.Conn)) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		serve(conn)
	}()
	return listener.Addr().(*net.TCPAddr).Port
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func portArgs(port int) []string { return []string{"--port", strconv.Itoa(port)} }

// TestRunPreservesResponseAfterRequestEnds is the request-then-response case:
// the client sends its whole request and closes stdin, and only then does the
// server answer, after a pause and in several chunks. The bridge must
// half-close the TCP write side (the server sees EOF) and keep copying the
// response until the server closes.
func TestRunPreservesResponseAfterRequestEnds(t *testing.T) {
	const request = "GET /health HTTP/1.1\r\nHost: agentctl\r\n\r\n"
	chunks := []string{"HTTP/1.1 200 OK\r\n", "Content-Length: 2\r\n", "\r\n", "o", "k"}
	received := make(chan string, 1)
	port := startPeer(t, func(conn net.Conn) {
		got, _ := io.ReadAll(conn) // returns only after the bridge half-closes
		received <- string(got)
		time.Sleep(300 * time.Millisecond)
		for _, chunk := range chunks {
			_, _ = conn.Write([]byte(chunk))
			time.Sleep(100 * time.Millisecond)
		}
	})
	stdin, stdinWriter := io.Pipe()
	go func() {
		_, _ = stdinWriter.Write([]byte(request))
		_ = stdinWriter.Close()
	}()
	var stdout, stderr lockedBuffer
	done := make(chan int, 1)

	go func() { done <- Run(portArgs(port), stdin, &stdout, &stderr) }()

	select {
	case got := <-received:
		if got != request {
			t.Fatalf("server received %q, want %q", got, request)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server never saw EOF: stdin EOF did not half-close the TCP write side")
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("tcp-stdio did not exit after the server closed")
	}
	if got, want := stdout.String(), strings.Join(chunks, ""); got != want {
		t.Fatalf("stdout = %q, want the full response %q", got, want)
	}
}

func TestRunExitsWhenPeerClosesWhileStdinStaysOpen(t *testing.T) {
	port := startPeer(t, func(conn net.Conn) {
		_, _ = conn.Write([]byte("bye"))
	})
	stdin, stdinWriter := io.Pipe()
	defer func() { _ = stdinWriter.Close() }()
	var stdout, stderr lockedBuffer
	done := make(chan int, 1)

	go func() { done <- Run(portArgs(port), stdin, &stdout, &stderr) }()

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("tcp-stdio did not exit after the TCP peer closed while stdin stayed open")
	}
	if got := stdout.String(); got != "bye" {
		t.Fatalf("stdout = %q, want %q", got, "bye")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("stdout closed") }

func TestBridgeStopsWhenStdoutFails(t *testing.T) {
	release := make(chan struct{})
	port := startPeer(t, func(conn net.Conn) {
		_, _ = conn.Write([]byte("data"))
		<-release
	})
	defer close(release)
	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)

	go func() { done <- Bridge(conn, strings.NewReader(""), failingWriter{}) }()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "stdout closed") {
			t.Fatalf("bridge error = %v, want the stdout failure", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("bridge kept running after stdout failed")
	}
}

func TestRunReportsDialFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	var stderr lockedBuffer

	code := Run(portArgs(port), strings.NewReader(""), io.Discard, &stderr)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "dial 127.0.0.1:"+strconv.Itoa(port)) {
		t.Fatalf("stderr = %q, want the loopback dial error", stderr.String())
	}
}

func TestParseArgsRejectsInvalidInput(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"--port", "0"},
		{"--port", "65536"},
		{"--port", "8765", "extra"},
		{"--host", "10.0.0.1", "--port", "8765"},
	} {
		if _, err := ParseArgs(args, io.Discard); err == nil {
			t.Fatalf("ParseArgs(%q) succeeded, want an error", args)
		}
	}
	port, err := ParseArgs([]string{"--port", "8765"}, io.Discard)
	if err != nil || port != 8765 {
		t.Fatalf("ParseArgs = %d, %v", port, err)
	}
	if code := Run([]string{"--port", "0"}, strings.NewReader(""), io.Discard, io.Discard); code != 2 {
		t.Fatalf("Run with invalid args = %d, want 2", code)
	}
}
