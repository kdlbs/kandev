package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/types"
)

const wsTestTimeout = 10 * time.Second

// recordingShell stands in for the PTY-backed shell the workspace stream
// forwards `shell_input` to. Reads are synchronised because the input loop
// writes from the server goroutine while the test asserts from its own.
type recordingShell struct {
	mu      sync.Mutex
	written []byte
	got     chan struct{}
}

func newRecordingShell() *recordingShell {
	return &recordingShell{got: make(chan struct{}, 8)}
}

func (r *recordingShell) Write(p []byte) (int, error) {
	r.mu.Lock()
	r.written = append(r.written, p...)
	r.mu.Unlock()
	r.got <- struct{}{}
	return len(p), nil
}

func (r *recordingShell) snapshot() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.written)
}

func (r *recordingShell) awaitWrite(t *testing.T) {
	t.Helper()
	select {
	case <-r.got:
	case <-time.After(wsTestTimeout):
		t.Fatal("timed out waiting for the input loop to forward shell input")
	}
}

// dialWorkspaceInputLoop runs handleWorkspaceStreamInput behind a real
// WebSocket so the read path, the JSON decoding, and the pong write are all
// exercised end to end. Cleanup closes the client, joins the loop, and shuts
// the server down, which is what keeps goleak quiet.
func dialWorkspaceInputLoop(t *testing.T, shell io.Writer) *websocket.Conn {
	t.Helper()
	srv := newTestServer(t)
	loopReturned := make(chan struct{})
	done := make(chan struct{})

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := srv.upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			close(loopReturned)
			return
		}
		defer func() { _ = conn.Close() }()
		srv.handleWorkspaceStreamInput(conn, shell, done)
		close(loopReturned)
	}))

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http"), nil)
	if err != nil {
		httpServer.Close()
		t.Fatalf("dial workspace input loop: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		select {
		case <-loopReturned:
		case <-time.After(wsTestTimeout):
			t.Error("workspace stream input loop did not return after the client disconnected")
		}
		close(done)
		httpServer.Close()
	})
	return conn
}

func readWorkspaceMessage(t *testing.T, conn *websocket.Conn) types.WorkspaceStreamMessage {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(wsTestTimeout))
	var msg types.WorkspaceStreamMessage
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read workspace message: %v", err)
	}
	return msg
}

func writeWorkspaceMessage(t *testing.T, conn *websocket.Conn, msg types.WorkspaceStreamMessage) {
	t.Helper()
	if err := conn.WriteJSON(msg); err != nil {
		t.Fatalf("write workspace message: %v", err)
	}
}

// TestHandleWorkspaceStreamInput_ForwardsShellInput asserts the bytes reach the
// shell verbatim — no framing, no newline added — because the terminal is a raw
// byte pipe and any decoration would show up as garbage keystrokes.
func TestHandleWorkspaceStreamInput_ForwardsShellInput(t *testing.T) {
	shell := newRecordingShell()
	conn := dialWorkspaceInputLoop(t, shell)

	writeWorkspaceMessage(t, conn, types.WorkspaceStreamMessage{
		Type: types.WorkspaceMessageTypeShellInput,
		Data: "ls -la\r",
	})
	shell.awaitWrite(t)

	if got := shell.snapshot(); got != "ls -la\r" {
		t.Errorf("shell received %q, want %q", got, "ls -la\r")
	}
}

// TestHandleWorkspaceStreamInput_AnswersPing pins the keepalive: a ping must
// produce a pong on the same connection. The frontend closes and re-dials the
// workspace stream when the pong stops arriving.
func TestHandleWorkspaceStreamInput_AnswersPing(t *testing.T) {
	conn := dialWorkspaceInputLoop(t, newRecordingShell())

	writeWorkspaceMessage(t, conn, types.WorkspaceStreamMessage{Type: types.WorkspaceMessageTypePing})

	got := readWorkspaceMessage(t, conn)
	if got.Type != types.WorkspaceMessageTypePong {
		t.Errorf("reply type = %q, want %q", got.Type, types.WorkspaceMessageTypePong)
	}
	if got.Timestamp == 0 {
		t.Errorf("pong carries no timestamp")
	}
}

// TestHandleWorkspaceStreamInput_IgnoresNonInputMessages proves resize and
// unknown message types are consumed without being forwarded to the shell.
// Routing a resize into the PTY as keystrokes would inject its JSON into the
// user's command line.
func TestHandleWorkspaceStreamInput_IgnoresNonInputMessages(t *testing.T) {
	shell := newRecordingShell()
	conn := dialWorkspaceInputLoop(t, shell)

	writeWorkspaceMessage(t, conn, types.WorkspaceStreamMessage{
		Type: types.WorkspaceMessageTypeShellResize,
		Cols: 120,
		Rows: 40,
	})
	writeWorkspaceMessage(t, conn, types.WorkspaceStreamMessage{Type: "totally-unknown", Data: "ignored"})
	// The ping is ordered after both, and the loop is single-threaded, so its
	// pong proves the two earlier messages were fully processed.
	writeWorkspaceMessage(t, conn, types.WorkspaceStreamMessage{Type: types.WorkspaceMessageTypePing})

	if got := readWorkspaceMessage(t, conn); got.Type != types.WorkspaceMessageTypePong {
		t.Fatalf("reply type = %q, want %q", got.Type, types.WorkspaceMessageTypePong)
	}
	if got := shell.snapshot(); got != "" {
		t.Errorf("shell received %q; only shell_input may be forwarded", got)
	}
}

// TestHandleWorkspaceStreamWS_ForwardsTrackerEvents drives the full endpoint:
// the greeting frame, then a workspace event published by the tracker while the
// client is attached. The event has to arrive tagged as git_commit with its
// payload intact — this is the fan-out the Commits panel is built on.
//
// The teardown is the interesting part. The forwarding loop only notices a
// departed client on its next write, so after closing the connection the test
// keeps publishing until the handler returns; the channel, not a fixed sleep,
// is what the test waits on.
func TestHandleWorkspaceStreamWS_ForwardsTrackerEvents(t *testing.T) {
	srv := newTestServer(t)
	handlerReturned := make(chan struct{})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerReturned)
		srv.Router().ServeHTTP(w, r)
	}))

	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(httpServer.URL, "http")+"/api/v1/workspace/stream", nil)
	if err != nil {
		httpServer.Close()
		t.Fatalf("dial workspace stream: %v", err)
	}

	if got := readWorkspaceMessage(t, conn); got.Type != types.WorkspaceMessageTypeConnected {
		t.Fatalf("first frame type = %q, want %q", got.Type, types.WorkspaceMessageTypeConnected)
	}

	tracker := srv.procMgr.GetWorkspaceTracker()
	tracker.NotifyGitCommit(&types.GitCommitNotification{
		CommitSHA: "1111111111111111111111111111111111111111",
		ParentSHA: "2222222222222222222222222222222222222222",
		Message:   "streamed commit",
	})

	event := readWorkspaceMessage(t, conn)
	if event.Type != types.WorkspaceMessageTypeGitCommit {
		t.Fatalf("event type = %q, want %q", event.Type, types.WorkspaceMessageTypeGitCommit)
	}
	if event.GitCommit == nil {
		t.Fatalf("git_commit payload is missing: %+v", event)
	}
	if event.GitCommit.CommitSHA != "1111111111111111111111111111111111111111" {
		t.Errorf("commit_sha = %q, want the published sha", event.GitCommit.CommitSHA)
	}
	if event.GitCommit.Message != "streamed commit" {
		t.Errorf("message = %q, want %q", event.GitCommit.Message, "streamed commit")
	}

	// Deliberately no ping here: the input goroutine answers a ping by writing
	// the pong onto this same connection, while the forwarding loop above is
	// also writing to it, and a gorilla *Conn permits only one concurrent
	// writer. Ping/pong is covered against the input loop in isolation by
	// TestHandleWorkspaceStreamInput_AnswersPing.
	_ = conn.Close()
	waitForWorkspaceStreamShutdown(t, tracker.NotifyGitCommit, handlerReturned)
	httpServer.Close()
}

// waitForWorkspaceStreamShutdown republishes until the handler's forwarding
// loop attempts a write on the closed connection and returns. The first write
// after a peer close usually succeeds (it lands in the kernel buffer), so one
// event is not enough to observe the disconnect.
func waitForWorkspaceStreamShutdown(
	t *testing.T,
	publish func(*types.GitCommitNotification),
	handlerReturned <-chan struct{},
) {
	t.Helper()
	deadline := time.After(wsTestTimeout)
	for {
		select {
		case <-handlerReturned:
			return
		case <-deadline:
			t.Fatal("workspace stream handler did not return after the client disconnected")
		default:
		}
		publish(&types.GitCommitNotification{CommitSHA: "3333333333333333333333333333333333333333"})
		select {
		case <-handlerReturned:
			return
		case <-time.After(5 * time.Millisecond):
		}
	}
}
