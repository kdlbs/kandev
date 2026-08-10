package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/types"
)

// TestHandleWorkspaceStreamWS_ConcurrentEventsAndPings pins the single-writer
// invariant of the workspace stream: the forwarding loop and the input loop
// share one *websocket.Conn, and gorilla supports exactly one concurrent
// writer. Before the write funnel existed, a tracker event racing a client
// ping tripped the race detector inside Conn.beginMessage.
func TestHandleWorkspaceStreamWS_ConcurrentEventsAndPings(t *testing.T) {
	srv := newTestServer(t)
	ts, handlerDone := newWorkspaceStreamTestServer(t, srv)
	conn := dialWorkspaceStream(t, ts)
	// Registered here rather than at the end of the test: every read below can
	// t.Fatal, which skips end-of-test statements and would strand the client
	// conn, the listener, and the handler's goroutines — the forwarding loop
	// parks in its select — for the rest of the package run. Cleanups are LIFO,
	// so this joins the handler before the server's own cleanup closes down.
	t.Cleanup(func() {
		closeWorkspaceStream(t, conn)
		joinWorkspaceStreamHandler(t, srv, handlerDone)
	})
	// One deadline for every read in the test, so an unexpected message type
	// cannot extend the run by a fresh per-message timeout.
	deadline := workspaceStreamDeadline(t)

	if msg := readWorkspaceStreamMessage(t, conn, deadline); msg.Type != types.WorkspaceMessageTypeConnected {
		t.Fatalf("first message type = %q, want %q", msg.Type, types.WorkspaceMessageTypeConnected)
	}

	// Drive both writers at once: the client pings (answered by the input
	// goroutine) while the tracker broadcasts commits (written by the
	// forwarding loop).
	const rounds = 25
	var writers sync.WaitGroup
	writers.Add(2)
	go func() {
		defer writers.Done()
		for i := 0; i < rounds; i++ {
			if err := conn.WriteJSON(types.NewWorkspacePing()); err != nil {
				return
			}
		}
	}()
	go func() {
		defer writers.Done()
		tracker := srv.procMgr.GetWorkspaceTracker()
		for i := 0; i < rounds; i++ {
			tracker.NotifyGitCommit(&types.GitCommitNotification{
				Timestamp: time.Now(),
				CommitSHA: fmt.Sprintf("sha-%02d", i),
			})
		}
	}()
	writers.Wait()

	// Both writers must have reached the wire intact — a corrupted frame
	// stream shows up here as a decode failure rather than a missing message.
	want := map[types.WorkspaceMessageType]bool{
		types.WorkspaceMessageTypePong:      false,
		types.WorkspaceMessageTypeGitCommit: false,
	}
	for !want[types.WorkspaceMessageTypePong] || !want[types.WorkspaceMessageTypeGitCommit] {
		msg := readWorkspaceStreamMessage(t, conn, deadline)
		if _, tracked := want[msg.Type]; tracked {
			want[msg.Type] = true
		}
	}
}

// newWorkspaceStreamTestServer serves the API router over a real listener and
// reports when the single expected handler invocation has returned, so
// goleak-checked tests can join the handler's goroutines.
func newWorkspaceStreamTestServer(t *testing.T, srv *Server) (*httptest.Server, <-chan struct{}) {
	t.Helper()
	done := make(chan struct{})
	var once sync.Once
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer once.Do(func() { close(done) })
		srv.router.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	return ts, done
}

func dialWorkspaceStream(t *testing.T, ts *httptest.Server) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/workspace/stream"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	return conn
}

// workspaceStreamDeadline bounds a whole test's reads. It never outruns the
// budget `go test -timeout` already enforces, so a stuck server surfaces as a
// test failure rather than a runner-level panic.
func workspaceStreamDeadline(t *testing.T) time.Time {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	if runnerDeadline, ok := t.Deadline(); ok && runnerDeadline.Before(deadline) {
		return runnerDeadline
	}
	return deadline
}

func readWorkspaceStreamMessage(t *testing.T, conn *websocket.Conn, deadline time.Time) types.WorkspaceStreamMessage {
	t.Helper()
	if err := conn.SetReadDeadline(deadline); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	var msg types.WorkspaceStreamMessage
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	return msg
}

// closeWorkspaceStream reports a close failure with Errorf rather than Fatalf:
// it runs from a cleanup, and a Fatal there would skip the remaining cleanups
// that join the handler and shut the listener down.
func closeWorkspaceStream(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err := conn.Close(); err != nil {
		t.Errorf("close client conn: %v", err)
	}
}

// joinWorkspaceStreamHandler waits for handleWorkspaceStreamWS to return. The
// forwarding loop only notices a departed client on its next write, so the
// tracker is nudged until the write fails and the handler unwinds.
func joinWorkspaceStreamHandler(t *testing.T, srv *Server, done <-chan struct{}) {
	t.Helper()
	nudge := time.NewTicker(5 * time.Millisecond)
	defer nudge.Stop()
	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case <-done:
			return
		case <-timeout.C:
			t.Fatal("workspace stream handler did not return after client disconnect")
		case <-nudge.C:
			srv.procMgr.GetWorkspaceTracker().NotifyGitCommit(&types.GitCommitNotification{
				Timestamp: time.Now(),
				CommitSHA: "shutdown-nudge",
			})
		}
	}
}
