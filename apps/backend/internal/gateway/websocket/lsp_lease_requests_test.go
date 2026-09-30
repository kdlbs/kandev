package websocket

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestLSPPendingClientRequestsAreBounded(t *testing.T) {
	upstream, _ := newLSPTestWebSocketPair(t)
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	lease.upstream = upstream
	lease.ready = true
	lease.readyStatus = map[string]any{"status": "ready"}
	lease.clientRequestTimeout = time.Hour
	browser, browserPeer := newLSPTestWebSocketPair(t)
	generation, _, err := lease.attach(browser)
	if err != nil {
		t.Fatal(err)
	}
	_ = readLSPLeaseStatus(t, browserPeer)

	for id := 0; id < lspLeaseMaxPendingClientRequests; id++ {
		message := []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"workspace/symbol","params":{"query":"x"}}`, id))
		if err := lease.handleBrowserMessage(generation, message); err != nil {
			t.Fatalf("forward request %d: %v", id, err)
		}
	}
	if err := lease.handleBrowserMessage(generation, []byte(`{"jsonrpc":"2.0","id":999,"method":"workspace/symbol","params":{"query":"overflow"}}`)); err == nil {
		t.Fatal("request above the pending count limit was accepted")
	}
	if got := len(lease.clientRequests); got != lspLeaseMaxPendingClientRequests {
		t.Fatalf("pending requests = %d, want limit %d", got, lspLeaseMaxPendingClientRequests)
	}
	if lease.pendingClientRequestBytes > lspLeaseMaxPendingClientRequestBytes {
		t.Fatalf("pending request bytes = %d, exceeds limit %d", lease.pendingClientRequestBytes, lspLeaseMaxPendingClientRequestBytes)
	}

	lease.detach(generation)
}

func TestLSPPendingClientRequestIDSizeIsBounded(t *testing.T) {
	upstream, _ := newLSPTestWebSocketPair(t)
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	lease.upstream = upstream
	lease.ready = true
	lease.readyStatus = map[string]any{"status": "ready"}
	lease.clientRequestTimeout = time.Hour
	browser, browserPeer := newLSPTestWebSocketPair(t)
	generation, _, err := lease.attach(browser)
	if err != nil {
		t.Fatal(err)
	}
	_ = readLSPLeaseStatus(t, browserPeer)
	id := `"` + strings.Repeat("x", lspLeaseMaxClientRequestIDBytes) + `"`
	message := []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"method":"workspace/symbol","params":{"query":"x"}}`, id))
	if err := lease.handleBrowserMessage(generation, message); err == nil {
		t.Fatal("request with an oversized ID was accepted")
	}
	if got := len(lease.clientRequests); got != 0 {
		t.Fatalf("oversized ID created %d pending requests, want none", got)
	}
	lease.detach(generation)
}

func TestLSPPendingClientRequestExpiresWhenServerDoesNotRespond(t *testing.T) {
	upstream, upstreamPeer := newLSPTestWebSocketPair(t)
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	lease.upstream = upstream
	lease.ready = true
	lease.readyStatus = map[string]any{"status": "ready"}
	lease.clientRequestTimeout = 20 * time.Millisecond
	browser, browserPeer := newLSPTestWebSocketPair(t)
	generation, _, err := lease.attach(browser)
	if err != nil {
		t.Fatal(err)
	}
	_ = readLSPLeaseStatus(t, browserPeer)
	if err := lease.handleBrowserMessage(generation, []byte(`{"jsonrpc":"2.0","id":7,"method":"workspace/symbol","params":{"query":"x"}}`)); err != nil {
		t.Fatal(err)
	}
	if request := readLSPJSONRPCMessage(t, upstreamPeer); string(request["method"]) != `"workspace/symbol"` {
		t.Fatalf("upstream request method = %s, want workspace/symbol", request["method"])
	}

	deadline := time.Now().Add(wsTestTimeout)
	for time.Now().Before(deadline) {
		lease.mu.Lock()
		pending := len(lease.clientRequests)
		lease.mu.Unlock()
		if pending == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	lease.mu.Lock()
	pending := len(lease.clientRequests)
	lease.mu.Unlock()
	if pending != 0 {
		t.Fatalf("pending requests after timeout = %d, want zero", pending)
	}
	response := readLSPJSONRPCMessage(t, browserPeer)
	if string(response["id"]) != "7" || len(response["error"]) == 0 {
		t.Fatalf("timeout response = %#v, want an error for request 7", response)
	}
	cancel := readLSPJSONRPCMessage(t, upstreamPeer)
	if string(cancel["method"]) != `"$/cancelRequest"` {
		t.Fatalf("timeout upstream frame = %s, want cancelRequest", cancel["method"])
	}
	lease.detach(generation)
}

func TestLSPPendingClientRequestResponseReleasesBoundedState(t *testing.T) {
	upstream, upstreamPeer := newLSPTestWebSocketPair(t)
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	lease.upstream = upstream
	lease.ready = true
	lease.readyStatus = map[string]any{"status": "ready"}
	lease.clientRequestTimeout = time.Hour
	browser, browserPeer := newLSPTestWebSocketPair(t)
	generation, _, err := lease.attach(browser)
	if err != nil {
		t.Fatal(err)
	}
	_ = readLSPLeaseStatus(t, browserPeer)
	if err := lease.handleBrowserMessage(generation, []byte(`{"jsonrpc":"2.0","id":"query-1","method":"workspace/symbol","params":{"query":"x"}}`)); err != nil {
		t.Fatal(err)
	}
	request := readLSPJSONRPCMessage(t, upstreamPeer)
	response := []byte(`{"jsonrpc":"2.0","id":` + string(request["id"]) + `,"result":[]}`)
	if err := lease.handleServerResponse(jsonRPCMessage{ID: request["id"], Result: []byte(`[]`)}, response); err != nil {
		t.Fatal(err)
	}
	if forwarded := readLSPJSONRPCMessage(t, browserPeer); string(forwarded["id"]) != `"query-1"` {
		t.Fatalf("response client ID = %s, want query-1", forwarded["id"])
	}
	if len(lease.clientRequests) != 0 || lease.pendingClientRequestBytes != 0 {
		t.Fatalf("pending state remains after response: count=%d bytes=%d", len(lease.clientRequests), lease.pendingClientRequestBytes)
	}
	lease.detach(generation)
}
