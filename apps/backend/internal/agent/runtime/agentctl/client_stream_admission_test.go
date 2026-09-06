package client

import (
	"context"
	"testing"
	"time"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// TestSendStreamRequest_ContextCancelsWhileWriteAdmissionIsHeld guards the
// parked probe budget: admission to the shared WebSocket writer must honor the
// request context even when another writer is already using the connection.
func TestSendStreamRequest_ContextCancelsWhileWriteAdmissionIsHeld(t *testing.T) {
	client, server := newTestClientWithStream(t, func(_ ws.Message) *ws.Message { return nil })
	defer server.Close()
	defer client.Close()

	require.NoError(t, client.acquireStreamWrite(context.Background()))

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	started := time.Now()
	go func() {
		_, err := client.sendStreamRequest(ctx, "test.write_admission", nil)
		done <- err
	}()

	var (
		err      error
		timedOut bool
	)
	select {
	case err = <-done:
	case <-time.After(250 * time.Millisecond):
		timedOut = true
	}

	client.releaseStreamWrite()
	if timedOut {
		// Let the blocked request unwind before failing the test, so the test
		// does not leak a goroutine into the package goleak check.
		err = <-done
	}
	require.False(t, timedOut, "write admission ignored the request context")
	require.Error(t, err)
	require.Less(t, time.Since(started), 250*time.Millisecond)
}
