package websocket

import (
	"errors"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.4
func TestLSPGracefulReleaseAcknowledgesBeforeUpstreamExit(t *testing.T) {
	for _, reason := range []string{lspLeaseReleaseStop, lspLeaseReleaseEditorIdle} {
		t.Run(reason, func(t *testing.T) {
			lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
			upstream, server := newLSPTestWebSocketPair(t)
			browser, client := newLSPTestWebSocketPair(t)
			lease.upstream = upstream
			generation, _, err := lease.attach(browser)
			require.NoError(t, err)
			require.NoError(t, lease.manager.add(lease))
			go lease.readUpstream()

			released := make(chan error, 1)
			t.Cleanup(func() {
				lease.terminate(gorillaws.CloseNormalClosure, "test cleanup", reason)
				lease.waitForClose(wsTestTimeout)
			})
			go func() { released <- lease.gracefulRelease(generation, reason, "release-1") }()

			shutdown := readLSPJSONRPCMessage(t, server)
			require.JSONEq(t, `"shutdown"`, string(shutdown["method"]))
			require.NoError(t, server.WriteJSON(map[string]any{
				"jsonrpc": "2.0", "id": shutdown["id"], "result": nil,
			}))
			ack := readLSPLeaseStatus(t, client)
			require.Equal(t, "released", ack["action"])
			require.Equal(t, "release-1", ack["requestId"])
			require.Equal(t, reason, ack["reason"])
			exit := readLSPJSONRPCMessage(t, server)
			require.JSONEq(t, `"exit"`, string(exit["method"]))
			require.NoError(t, server.Close())
			select {
			case <-lease.readDone:
			case <-time.After(wsTestTimeout):
				t.Fatal("upstream close was not observed after release")
			}
			select {
			case err := <-released:
				require.NoError(t, err)
			case <-time.After(wsTestTimeout):
				t.Fatal("graceful release did not complete")
			}
			require.True(t, lease.isClosed())
			_, _, err = client.ReadMessage()
			var closeErr *gorillaws.CloseError
			require.True(t, errors.As(err, &closeErr))
			require.Equal(t, gorillaws.CloseNormalClosure, closeErr.Code)
		})
	}
}

func TestLSPGracefulReleaseClosesLeaseAfterAcknowledgementFailure(t *testing.T) {
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	upstream, server := newLSPTestWebSocketPair(t)
	browser, _ := newLSPTestWebSocketPair(t)
	lease.upstream = upstream
	generation, _, err := lease.attach(browser)
	require.NoError(t, err)
	require.NoError(t, lease.manager.add(lease))
	go lease.readUpstream()
	t.Cleanup(func() {
		lease.terminate(gorillaws.CloseNormalClosure, "test cleanup", lspLeaseReleaseStop)
		lease.waitForClose(wsTestTimeout)
	})
	require.NoError(t, browser.Close())
	released := make(chan error, 1)
	go func() { released <- lease.gracefulRelease(generation, lspLeaseReleaseStop, "release-1") }()
	shutdown := readLSPJSONRPCMessage(t, server)
	require.NoError(t, server.WriteJSON(map[string]any{
		"jsonrpc": "2.0", "id": shutdown["id"], "result": nil,
	}))
	exit := readLSPJSONRPCMessage(t, server)
	require.JSONEq(t, `"exit"`, string(exit["method"]))
	select {
	case err := <-released:
		require.Error(t, err)
	case <-time.After(wsTestTimeout):
		t.Fatal("release with a disconnected browser did not finish")
	}
	require.True(t, lease.isClosed())
	select {
	case <-lease.readDone:
	case <-time.After(wsTestTimeout):
		t.Fatal("release did not close the upstream after acknowledgement failure")
	}
}
