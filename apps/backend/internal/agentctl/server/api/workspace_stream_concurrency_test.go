package api

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// TestHandleWorkspaceStreamWS_ConcurrentEventsAndPings pins the single-writer
// invariant of the workspace stream: the forwarding loop and the input loop
// share one *websocket.Conn, and gorilla supports exactly one concurrent
// writer. Before the write funnel existed, a tracker event racing a client
// ping tripped the race detector inside Conn.beginMessage.
func TestHandleWorkspaceStreamWS_ConcurrentEventsAndPings(t *testing.T) {
	srv, conn := dialWorkspaceStreamEndpoint(t)

	if got := readWorkspaceMessage(t, conn); got.Type != types.WorkspaceMessageTypeConnected {
		t.Fatalf("first frame type = %q, want %q", got.Type, types.WorkspaceMessageTypeConnected)
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
	// The elapsed-time guard bounds the whole drain: readWorkspaceMessage
	// takes a fresh deadline per call, so an unexpected message type would
	// otherwise extend the loop one full timeout at a time.
	want := map[types.WorkspaceMessageType]bool{
		types.WorkspaceMessageTypePong:      false,
		types.WorkspaceMessageTypeGitCommit: false,
	}
	deadline := time.Now().Add(wsTestTimeout)
	for !want[types.WorkspaceMessageTypePong] || !want[types.WorkspaceMessageTypeGitCommit] {
		if time.Now().After(deadline) {
			t.Fatalf("timed out draining the stream; seen = %v", want)
		}
		msg := readWorkspaceMessage(t, conn)
		if _, tracked := want[msg.Type]; tracked {
			want[msg.Type] = true
		}
	}
}
