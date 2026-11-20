package websocket

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
)

func TestTerminalKeepaliveIdleProxy(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	proxyIdleTimeout := 80 * time.Millisecond
	handler.keepalive = shortTerminalKeepalive(proxyIdleTimeout + 50*time.Millisecond)
	upstream, agentctl := newTerminalWSPair(t)
	bridgeDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testWSUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		handler.bridgeBinaryWebSockets(conn, upstream, "proxy-terminal")
		close(bridgeDone)
	}))
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse bridge URL: %v", err)
	}
	proxy := newTerminalIdleProxy(t, target.Host, proxyIdleTimeout)
	browser, _, err := gorillaws.DefaultDialer.Dial("ws://"+proxy.address(), nil)
	if err != nil {
		t.Fatalf("dial through idle proxy: %v", err)
	}
	frames := observeTerminalFrames(t, browser, true)

	for range 3 {
		waitTerminalPing(t, frames.pings)
	}
	time.Sleep(3 * proxyIdleTimeout)
	select {
	case err := <-frames.closed:
		t.Fatalf("terminal closed during idle proxy windows: %v", err)
	default:
	}

	input := []byte("input-after-idle")
	if err := browser.WriteMessage(gorillaws.BinaryMessage, input); err != nil {
		t.Fatalf("write input after idle: %v", err)
	}
	if got := readBinary(t, agentctl); string(got) != string(input) {
		t.Fatalf("upstream input = %q, want %q", got, input)
	}
	output := []byte("output-after-idle")
	if err := agentctl.WriteMessage(gorillaws.BinaryMessage, output); err != nil {
		t.Fatalf("write output after idle: %v", err)
	}
	if got := waitTerminalFrame(t, frames.frames); string(got.data) != string(output) {
		t.Fatalf("browser output = %q, want %q", got.data, output)
	}

	_ = browser.Close()
	joinWithin(t, bridgeDone, "proxy terminal bridge")
}

type terminalFrame struct {
	kind int
	data []byte
}

type terminalFrameReader struct {
	pings  chan struct{}
	frames chan terminalFrame
	closed chan error
	done   chan struct{}
}

func observeTerminalFrames(t *testing.T, conn *gorillaws.Conn, replyToPings bool) *terminalFrameReader {
	t.Helper()
	reader := &terminalFrameReader{
		pings:  make(chan struct{}, 32),
		frames: make(chan terminalFrame, 128),
		closed: make(chan error, 1),
		done:   make(chan struct{}),
	}
	conn.SetPingHandler(func(payload string) error {
		select {
		case reader.pings <- struct{}{}:
		default:
		}
		if !replyToPings {
			return nil
		}
		return conn.WriteControl(gorillaws.PongMessage, []byte(payload), time.Now().Add(time.Second))
	})
	go func() {
		defer close(reader.done)
		for {
			kind, data, err := conn.ReadMessage()
			if err != nil {
				reader.closed <- err
				return
			}
			reader.frames <- terminalFrame{kind: kind, data: data}
		}
	}()
	t.Cleanup(func() {
		_ = conn.Close()
		<-reader.done
	})
	return reader
}

func waitTerminalPing(t *testing.T, pings <-chan struct{}) {
	t.Helper()
	select {
	case <-pings:
	case <-time.After(wsTestTimeout):
		t.Fatal("browser did not receive a keepalive ping")
	}
}

func waitTerminalFrame(t *testing.T, frames <-chan terminalFrame) terminalFrame {
	t.Helper()
	select {
	case frame := <-frames:
		return frame
	case <-time.After(wsTestTimeout):
		t.Fatal("browser did not receive a terminal frame")
		return terminalFrame{}
	}
}

type terminalIdleProxy struct {
	listener net.Listener
	timeout  time.Duration
	mu       sync.Mutex
	closed   bool
	conns    map[net.Conn]struct{}
	wg       sync.WaitGroup
}

func newTerminalIdleProxy(t *testing.T, target string, timeout time.Duration) *terminalIdleProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for idle proxy: %v", err)
	}
	proxy := &terminalIdleProxy{listener: listener, timeout: timeout, conns: make(map[net.Conn]struct{})}
	proxy.wg.Add(1)
	go proxy.accept(target)
	t.Cleanup(proxy.close)
	return proxy
}

func (p *terminalIdleProxy) address() string {
	return p.listener.Addr().String()
}

func (p *terminalIdleProxy) accept(target string) {
	defer p.wg.Done()
	for {
		client, err := p.listener.Accept()
		if err != nil {
			return
		}
		p.wg.Add(1)
		go p.serve(client, target)
	}
}

func (p *terminalIdleProxy) serve(client net.Conn, target string) {
	defer p.wg.Done()
	if !p.track(client) {
		return
	}
	defer p.untrack(client)
	backend, err := net.Dial("tcp", target)
	if err != nil {
		_ = client.Close()
		return
	}
	if !p.track(backend) {
		_ = client.Close()
		_ = backend.Close()
		return
	}
	defer p.untrack(backend)
	activity := make(chan struct{}, 1)
	finished := make(chan struct{})
	var closeOnce sync.Once
	closeBoth := func() {
		closeOnce.Do(func() {
			close(finished)
			_ = client.Close()
			_ = backend.Close()
		})
	}
	var copies sync.WaitGroup
	copies.Add(2)
	go func() {
		defer copies.Done()
		copyTerminalProxy(client, backend, activity)
		closeBoth()
	}()
	go func() {
		defer copies.Done()
		copyTerminalProxy(backend, client, nil)
		closeBoth()
	}()
	timer := time.NewTimer(p.timeout)
	defer timer.Stop()
	for {
		select {
		case <-finished:
			copies.Wait()
			return
		case <-activity:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(p.timeout)
		case <-timer.C:
			closeBoth()
			copies.Wait()
			return
		}
	}
}

func copyTerminalProxy(destination, source net.Conn, activity chan<- struct{}) {
	buffer := make([]byte, 32*1024)
	for {
		read, err := source.Read(buffer)
		if read > 0 {
			written, writeErr := destination.Write(buffer[:read])
			if activity != nil && written > 0 {
				select {
				case activity <- struct{}{}:
				default:
				}
			}
			if writeErr != nil || written != read {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (p *terminalIdleProxy) track(conn net.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		_ = conn.Close()
		return false
	}
	p.conns[conn] = struct{}{}
	return true
}

func (p *terminalIdleProxy) untrack(conn net.Conn) {
	p.mu.Lock()
	delete(p.conns, conn)
	p.mu.Unlock()
}

func (p *terminalIdleProxy) close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	_ = p.listener.Close()
	for conn := range p.conns {
		_ = conn.Close()
	}
	p.mu.Unlock()
	p.wg.Wait()
}
