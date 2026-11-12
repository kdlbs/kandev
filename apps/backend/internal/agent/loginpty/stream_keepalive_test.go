package loginpty

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/common/wskeepalive"
)

func TestLoginStreamKeepaliveIdle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mgr := newTestManager(t, nil)
	sess, err := mgr.Start("keepalive-test", []string{"sh", "-c", "sleep 90"}, 80, 24)
	if err != nil {
		t.Fatalf("start login session: %v", err)
	}
	t.Cleanup(func() { sess.stop() })

	server := newLoginKeepaliveServer(t, mgr, wskeepalive.Timing{WriteWait: time.Second, PongWait: time.Second, PingPeriod: 25 * time.Millisecond})
	conn := connectHostShellStream(t, server.URL, sess.ID)
	pings := observeLoginStreamPings(t, conn)

	select {
	case <-pings:
	case <-time.After(3 * time.Second):
		t.Fatal("login stream did not send a terminal keepalive ping")
	}
	_ = conn.Close()
}

func TestLoginStreamKeepaliveIdleProxy(t *testing.T) {
	mgr := newTestManager(t, nil)
	sess := makeLoginSessionWithCommand(t, mgr, "proxy-session", []string{
		"sh", "-c", `IFS= read -r line; printf '\nACK:%s\n' "$line"; sleep 90`,
	})
	proxyTimeout := 70 * time.Millisecond
	server := newLoginKeepaliveServer(t, mgr, shortLoginKeepalive(20*time.Millisecond, 120*time.Millisecond))
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse login server URL: %v", err)
	}
	proxy := newLoginIdleProxy(t, serverURL.Host, proxyTimeout)
	conn := dialLoginStreamThroughProxy(t, proxy.address(), sess.ID)
	observer := observeLoginStreamFrames(t, conn, true)
	waitLoginSubscriberCount(t, sess, 1)
	for range 3 {
		waitLoginPing(t, observer.pings)
	}
	time.Sleep(3 * proxyTimeout)
	select {
	case err := <-observer.closed:
		t.Fatalf("login stream closed during proxy idle windows: %v", err)
	default:
	}

	marker := "roundtrip-after-idle"
	if err := conn.WriteMessage(gorillaws.BinaryMessage, []byte(marker+"\n")); err != nil {
		t.Fatalf("write input after proxy idle: %v", err)
	}
	waitLoginOutputContaining(t, observer, "ACK:"+marker)
	_ = conn.Close()
	waitLoginSubscriberCount(t, sess, 0)
}

func TestLoginStreamKeepaliveReadsPongsDuringBlockedInput(t *testing.T) {
	mgr := newTestManager(t, nil)
	sess, pty := newLoginSessionWithBlockedInput(t, mgr, "blocked-input-session")
	server := newLoginKeepaliveServer(t, mgr, shortLoginKeepalive(20*time.Millisecond, 80*time.Millisecond))
	conn := connectHostShellStream(t, server.URL, sess.ID)
	observer := observeLoginStreamFrames(t, conn, true)
	waitLoginSubscriberCount(t, sess, 1)

	if err := conn.WriteMessage(gorillaws.BinaryMessage, []byte("large-paste")); err != nil {
		t.Fatalf("write first input: %v", err)
	}
	select {
	case <-pty.writeStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("PTY writer did not block on the first input")
	}
	if err := conn.WriteMessage(gorillaws.BinaryMessage, []byte("input-after-paste")); err != nil {
		t.Fatalf("write input queued behind paste: %v", err)
	}

	for range 6 {
		waitLoginPing(t, observer.pings)
	}

	pty.releaseWrite()
	select {
	case <-pty.firstWriteDone:
	case <-time.After(3 * time.Second):
		t.Fatal("blocked PTY write did not resume")
	}
	if got := <-pty.writes; got != "large-paste" {
		t.Fatalf("first PTY input = %q, want %q", got, "large-paste")
	}
	select {
	case got := <-pty.writes:
		if got != "input-after-paste" {
			t.Fatalf("queued PTY input = %q, want %q", got, "input-after-paste")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("input queued behind the blocked write was not delivered")
	}
	select {
	case err := <-observer.closed:
		t.Fatalf("login stream closed after processing pongs during blocked input: %v", err)
	default:
	}

	_ = conn.Close()
	waitLoginSubscriberCount(t, sess, 0)
}

func TestLoginStreamKeepaliveUnresponsivePeer(t *testing.T) {
	mgr := newTestManager(t, nil)
	silent := makeLoginSession(t, mgr, "silent-session")
	responsive := makeLoginSession(t, mgr, "responsive-session")
	server := newLoginKeepaliveServer(t, mgr, shortLoginKeepalive(30*time.Millisecond, 180*time.Millisecond))
	silentConn := connectHostShellStream(t, server.URL, silent.ID)
	responsiveConn := connectHostShellStream(t, server.URL, responsive.ID)
	silentObserver := observeLoginStreamFrames(t, silentConn, false)
	responsiveObserver := observeLoginStreamFrames(t, responsiveConn, true)
	waitLoginSubscriberCount(t, silent, 1)
	waitLoginSubscriberCount(t, responsive, 1)

	stopOutput, outputWrites := startLoginStreamOutput(t, silent)
	waitLoginPing(t, silentObserver.pings)
	waitLoginPing(t, responsiveObserver.pings)
	_ = waitLoginStreamFrame(t, silentObserver)
	select {
	case err := <-silentObserver.closed:
		if err == nil {
			t.Fatal("unresponsive login stream closed without a read error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("login stream stayed open without receiving pongs")
	}
	stopOutput()
	if got := outputWrites.Load(); got < 3 {
		t.Fatalf("continued output writes before liveness close = %d, want at least 3", got)
	}
	waitLoginSubscriberCount(t, silent, 0)

	responsiveOutput := []byte("responsive stream remains live")
	responsive.broadcast(responsiveOutput)
	frame := waitLoginStreamFrame(t, responsiveObserver)
	if string(frame.data) != string(responsiveOutput) {
		t.Fatalf("responsive stream output = %q, want %q", frame.data, responsiveOutput)
	}
	if err := responsiveConn.WriteMessage(gorillaws.BinaryMessage, []byte("responsive input")); err != nil {
		t.Fatalf("write to responsive login stream: %v", err)
	}
	select {
	case err := <-responsiveObserver.closed:
		t.Fatalf("responsive login stream closed: %v", err)
	default:
	}
}

func TestLoginStreamKeepaliveExit(t *testing.T) {
	mgr := newTestManager(t, nil)
	sess := makeLoginSession(t, mgr, "exiting-session")
	server := newLoginKeepaliveServer(t, mgr, shortLoginKeepalive(20*time.Millisecond, 120*time.Millisecond))
	conn := connectHostShellStream(t, server.URL, sess.ID)
	observer := observeLoginStreamFrames(t, conn, true)
	waitLoginSubscriberCount(t, sess, 1)
	sess.stop()
	for {
		frame := waitLoginStreamFrame(t, observer)
		if frame.kind == gorillaws.TextMessage && strings.Contains(string(frame.data), `"type":"exit"`) {
			break
		}
	}
	select {
	case <-observer.closed:
	case <-time.After(3 * time.Second):
		t.Fatal("login stream did not close after final exit frame")
	}
	waitLoginSubscriberCount(t, sess, 0)
}

func TestLoginStreamKeepaliveDisconnect(t *testing.T) {
	mgr := newTestManager(t, nil)
	sess := makeLoginSession(t, mgr, "disconnect-session")
	server := newLoginKeepaliveServer(t, mgr, shortLoginKeepalive(20*time.Millisecond, 120*time.Millisecond))
	conn := connectHostShellStream(t, server.URL, sess.ID)
	waitLoginSubscriberCount(t, sess, 1)
	_ = conn.Close()
	waitLoginSubscriberCount(t, sess, 0)
}

func observeLoginStreamPings(t *testing.T, conn *gorillaws.Conn) <-chan struct{} {
	t.Helper()
	return observeLoginStreamFrames(t, conn, true).pings
}

func newLoginKeepaliveServer(t *testing.T, mgr *Manager, timing wskeepalive.Timing) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := NewHandlers(mgr, nil, mgr.log.Zap(), func(*http.Request) bool { return true })
	handlers.keepalive = timing
	handlers.RegisterRoutes(router)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server
}

type loginStreamFrame struct {
	kind int
	data []byte
}

type loginStreamObserver struct {
	pings  chan struct{}
	frames chan loginStreamFrame
	closed chan error
	stop   chan struct{}
	done   chan struct{}
}

func observeLoginStreamFrames(t *testing.T, conn *gorillaws.Conn, replyToPings bool) *loginStreamObserver {
	t.Helper()
	observer := &loginStreamObserver{
		pings:  make(chan struct{}, 32),
		frames: make(chan loginStreamFrame, 128),
		closed: make(chan error, 1),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	conn.SetPingHandler(func(payload string) error {
		select {
		case observer.pings <- struct{}{}:
		default:
		}
		if !replyToPings {
			return nil
		}
		return conn.WriteControl(gorillaws.PongMessage, []byte(payload), time.Now().Add(time.Second))
	})
	go func() {
		defer close(observer.done)
		for {
			kind, data, err := conn.ReadMessage()
			if err != nil {
				observer.closed <- err
				return
			}
			select {
			case observer.frames <- loginStreamFrame{kind: kind, data: data}:
			case <-observer.stop:
				return
			}
		}
	}()
	t.Cleanup(func() {
		close(observer.stop)
		_ = conn.Close()
		<-observer.done
	})
	return observer
}

func waitLoginStreamFrame(t *testing.T, observer *loginStreamObserver) loginStreamFrame {
	t.Helper()
	select {
	case frame := <-observer.frames:
		return frame
	case <-time.After(3 * time.Second):
		t.Fatal("login stream did not deliver a frame")
		return loginStreamFrame{}
	}
}

func waitLoginOutputContaining(t *testing.T, observer *loginStreamObserver, needle string) string {
	t.Helper()
	var output strings.Builder
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case frame := <-observer.frames:
			if frame.kind != gorillaws.BinaryMessage {
				continue
			}
			output.Write(frame.data)
			if strings.Contains(output.String(), needle) {
				return output.String()
			}
		case err := <-observer.closed:
			t.Fatalf("login stream closed before output %q: %v; received %q", needle, err, output.String())
			return output.String()
		case <-timer.C:
			t.Fatalf("timed out waiting for login output %q; received %q", needle, output.String())
			return output.String()
		}
	}
}

func waitLoginSubscriberCount(t *testing.T, sess *Session, want int) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		sess.subMu.Lock()
		got := len(sess.subscribers)
		sess.subMu.Unlock()
		if got == want {
			return
		}
		select {
		case <-ticker.C:
		case <-timer.C:
			t.Fatalf("login stream subscribers = %d, want %d", got, want)
		}
	}
}

func waitLoginPing(t *testing.T, pings <-chan struct{}) {
	t.Helper()
	select {
	case <-pings:
	case <-time.After(3 * time.Second):
		t.Fatal("login stream did not send a keepalive ping")
	}
}

func makeLoginSession(t *testing.T, mgr *Manager, key string) *Session {
	t.Helper()
	return makeLoginSessionWithCommand(t, mgr, key, []string{"sh", "-c", "sleep 90"})
}

func makeLoginSessionWithCommand(t *testing.T, mgr *Manager, key string, command []string) *Session {
	t.Helper()
	sess, err := mgr.Start(key, command, 80, 24)
	if err != nil {
		t.Fatalf("start login session: %v", err)
	}
	t.Cleanup(func() { sess.stop() })
	return sess
}

type blockedLoginPty struct {
	writeStarted   chan struct{}
	firstWriteDone chan struct{}
	writes         chan string
	release        chan struct{}
	closed         chan struct{}
	startOnce      sync.Once
	releaseOnce    sync.Once
	closeOnce      sync.Once
}

func newLoginSessionWithBlockedInput(t *testing.T, mgr *Manager, id string) (*Session, *blockedLoginPty) {
	t.Helper()
	pty := &blockedLoginPty{
		writeStarted:   make(chan struct{}),
		firstWriteDone: make(chan struct{}),
		writes:         make(chan string, 4),
		release:        make(chan struct{}),
		closed:         make(chan struct{}),
	}
	t.Cleanup(func() { _ = pty.Close() })
	sess := &Session{
		ID:          id,
		AgentID:     id,
		subscribers: make(map[chan<- []byte]struct{}),
		pty:         pty,
		running:     true,
		startedAt:   time.Now(),
		log:         mgr.log.WithFields(),
	}
	mgr.mu.Lock()
	mgr.byID[id] = sess
	mgr.mu.Unlock()
	t.Cleanup(func() {
		mgr.mu.Lock()
		delete(mgr.byID, id)
		mgr.mu.Unlock()
	})
	return sess, pty
}

func (p *blockedLoginPty) Read([]byte) (int, error) {
	<-p.closed
	return 0, io.EOF
}

func (p *blockedLoginPty) Write(data []byte) (int, error) {
	p.writes <- string(data)
	p.startOnce.Do(func() {
		close(p.writeStarted)
		<-p.release
		close(p.firstWriteDone)
	})
	return len(data), nil
}

func (p *blockedLoginPty) Close() error {
	p.closeOnce.Do(func() { close(p.closed) })
	p.releaseWrite()
	return nil
}

func (p *blockedLoginPty) Resize(uint16, uint16) error { return nil }

func (p *blockedLoginPty) releaseWrite() {
	p.releaseOnce.Do(func() { close(p.release) })
}

func startLoginStreamOutput(t *testing.T, sess *Session) (func(), *atomic.Int32) {
	t.Helper()
	stop := make(chan struct{})
	done := make(chan struct{})
	var stopOnce sync.Once
	var writes atomic.Int32
	stopAndJoin := func() {
		stopOnce.Do(func() { close(stop) })
		<-done
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				sess.broadcast([]byte("continued output"))
				writes.Add(1)
			}
		}
	}()
	t.Cleanup(stopAndJoin)
	return stopAndJoin, &writes
}

func shortLoginKeepalive(pingPeriod, pongWait time.Duration) wskeepalive.Timing {
	return wskeepalive.Timing{WriteWait: time.Second, PingPeriod: pingPeriod, PongWait: pongWait}
}

func dialLoginStreamThroughProxy(t *testing.T, proxyAddress, sessionID string) *gorillaws.Conn {
	t.Helper()
	conn, _, err := gorillaws.DefaultDialer.Dial("ws://"+proxyAddress+fmt.Sprintf("/api/v1/agent-login/sessions/%s/stream", sessionID), nil)
	if err != nil {
		t.Fatalf("dial login stream through proxy: %v", err)
	}
	return conn
}
