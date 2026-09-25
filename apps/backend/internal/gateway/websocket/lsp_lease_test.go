package websocket

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentruntime"
)

func newLSPTestWebSocketPair(t *testing.T) (*gorillaws.Conn, *gorillaws.Conn) {
	t.Helper()
	connected := make(chan *gorillaws.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := lspUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connected <- conn
	}))
	t.Cleanup(server.Close)
	client, response, err := gorillaws.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial websocket test pair: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	var peer *gorillaws.Conn
	select {
	case peer = <-connected:
	case <-time.After(wsTestTimeout):
		t.Fatal("websocket peer did not connect")
	}
	t.Cleanup(func() { _ = peer.Close() })
	return peer, client
}

func newTestLSPLease(manager *lspLeaseManager) *lspLease {
	return newLSPLease(manager, lspLeaseExecution{
		ID: "execution-1", SessionID: "session-1", TaskID: "task-1",
	}, "go", "user-1", map[string]any{}, nil)
}

func TestLSPLeaseManagerHonorsAttachedLeaseHint(t *testing.T) {
	manager := newLSPLeaseManager(3, testLogger())
	execution := lspLeaseExecution{ID: "execution-1", SessionID: "session-1"}
	attached := newTestLSPLease(manager)
	attached.id = "attached-lease"
	attached.browser = &gorillaws.Conn{}
	detached := newTestLSPLease(manager)
	detached.id = "other-detached-lease"
	if err := manager.add(attached); err != nil {
		t.Fatal(err)
	}
	if err := manager.add(detached); err != nil {
		t.Fatal(err)
	}

	if got := manager.detachedCandidate(execution, "go", "user-1", attached.id); got != nil {
		t.Fatalf("attached lease hint selected a different lease: %q", got.id)
	}
	if got := manager.detachedCandidate(execution, "go", "user-1", detached.id); got != detached {
		t.Fatalf("detached lease hint selected %v, want %q", got, detached.id)
	}
	if got := manager.detachedCandidate(execution, "go", "user-1", ""); got != detached {
		t.Fatalf("unhinted candidate = %v, want %q", got, detached.id)
	}
}

func TestLSPLeaseManagerDoesNotReuseDetachedLeaseAcrossUsers(t *testing.T) {
	manager := newLSPLeaseManager(3, testLogger())
	execution := lspLeaseExecution{ID: "execution-1", SessionID: "session-1"}
	lease := newTestLSPLease(manager)
	lease.configuration = map[string]any{"gopls": map[string]any{"buildFlags": []any{"-tags=user-one"}}}
	if err := manager.add(lease); err != nil {
		t.Fatal(err)
	}

	if got := manager.detachedCandidate(execution, "go", "user-2", ""); got != nil {
		t.Fatalf("user 2 selected user 1's detached lease %q", got.id)
	}
	if got := manager.detachedCandidate(execution, "go", "user-1", ""); got != lease {
		t.Fatalf("user 1 candidate = %v, want their detached lease", got)
	}
}

func TestLSPTaskStopUsesTerminalBrowserCloseCode(t *testing.T) {
	manager := newLSPLeaseManager(2, testLogger())
	lease := newTestLSPLease(manager)
	browser, browserPeer := newLSPTestWebSocketPair(t)
	lease.browser = browser
	lease.generation = 1
	if err := manager.add(lease); err != nil {
		t.Fatal(err)
	}

	manager.stopTask("task-1")
	if err := browserPeer.SetReadDeadline(time.Now().Add(wsTestTimeout)); err != nil {
		t.Fatal(err)
	}
	_, _, err := browserPeer.ReadMessage()
	var closeErr *gorillaws.CloseError
	if !errors.As(err, &closeErr) || closeErr.Code != lspCloseRuntimeStopped {
		t.Fatalf("task stop close error = %v, want terminal code %d", err, lspCloseRuntimeStopped)
	}
}

func TestLSPLeaseOldGenerationDetachCannotDetachSuccessor(t *testing.T) {
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	first, _ := newLSPTestWebSocketPair(t)
	firstGeneration, _, err := lease.attach(first)
	if err != nil {
		t.Fatal(err)
	}
	lease.detach(firstGeneration)

	second, _ := newLSPTestWebSocketPair(t)
	secondGeneration, _, err := lease.attach(second)
	if err != nil {
		t.Fatal(err)
	}
	lease.detach(firstGeneration)
	if !lease.isActiveGeneration(secondGeneration) {
		t.Fatal("stale first-handler cleanup detached its successor generation")
	}
}

func TestLSPLeaseTerminationWaitsForBrowserWrite(t *testing.T) {
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	browser, client := newLSPTestWebSocketPair(t)
	lease.browser = browser
	lease.generation = 1

	lease.browserWriteMu.Lock()
	terminated := make(chan struct{})
	go func() {
		lease.terminate(lspCloseTransport, "test termination", lspLeaseReleaseRuntimeStop)
		close(terminated)
	}()
	deadline := time.Now().Add(wsTestTimeout)
	for !lease.isClosed() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !lease.isClosed() {
		t.Fatal("termination did not begin")
	}
	select {
	case <-terminated:
		t.Fatal("termination closed the browser while a browser write was in progress")
	default:
	}
	lease.browserWriteMu.Unlock()

	select {
	case <-terminated:
	case <-time.After(wsTestTimeout):
		t.Fatal("termination did not finish after the browser write completed")
	}
	if err := client.SetReadDeadline(time.Now().Add(wsTestTimeout)); err != nil {
		t.Fatal(err)
	}
	_, _, err := client.ReadMessage()
	var closeErr *gorillaws.CloseError
	if !errors.As(err, &closeErr) || closeErr.Code != lspCloseTransport {
		t.Fatalf("browser close error = %v, want code %d", err, lspCloseTransport)
	}
}

func TestLSPWorkspaceConfigurationPreservesLanguageSections(t *testing.T) {
	upstream, peer := newLSPTestWebSocketPair(t)
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	lease.upstream = upstream
	lease.language = "go"
	lease.configuration = map[string]any{
		"gopls":    map[string]any{"buildFlags": []any{"-tags=integration"}},
		"compiler": map[string]any{"jvmTarget": "21"},
	}
	params := json.RawMessage(`{"items":[{"section":""},{"section":"go"},{"section":"go.gopls"},{"section":"gopls"},{"section":"compiler.jvmTarget"},{"section":"missing"}]}`)
	if err := lease.answerWorkspaceConfiguration(json.RawMessage(`9`), params); err != nil {
		t.Fatal(err)
	}
	response := readJSONRPCResponse(t, peer)
	var values []any
	if err := json.Unmarshal(response.Result, &values); err != nil {
		t.Fatal(err)
	}
	if len(values) != 6 {
		t.Fatalf("configuration response length = %d, want 6", len(values))
	}
	if values[0] == nil || values[1] == nil || values[2] == nil || values[3] == nil {
		t.Fatalf("language and normalized sections must resolve: %#v", values)
	}
	if values[4] != "21" || values[5] != nil {
		t.Fatalf("nested and missing sections = %#v, want 21 and null", values[4:])
	}
}

func TestLSPInitialConfigurationChangeIsForwardedWhenSnapshotMatches(t *testing.T) {
	upstream, upstreamPeer := newLSPTestWebSocketPair(t)
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	lease.upstream = upstream
	lease.ready = true
	lease.readyStatus = map[string]any{"status": "ready"}
	lease.configuration = map[string]any{"gopls": map[string]any{"buildFlags": []any{"-tags=integration"}}}
	if err := lease.manager.add(lease); err != nil {
		t.Fatal(err)
	}

	browser, browserPeer := newLSPTestWebSocketPair(t)
	generation, _, err := lease.attach(browser)
	if err != nil {
		t.Fatal(err)
	}
	_ = readLSPLeaseStatus(t, browserPeer)
	if err := lease.handleBrowserMessage(generation, []byte(`{"jsonrpc":"2.0","method":"workspace/didChangeConfiguration","params":{"settings":{"gopls":{"buildFlags":["-tags=integration"]}}}}`)); err != nil {
		t.Fatal(err)
	}
	message := readLSPJSONRPCMessage(t, upstreamPeer)
	if string(message["method"]) != `"workspace/didChangeConfiguration"` {
		t.Fatalf("first upstream configuration notification = %s, want didChangeConfiguration", message["method"])
	}
}

func TestLSPFailedInitializeResponseIsNotReplayed(t *testing.T) {
	upstream, upstreamPeer := newLSPTestWebSocketPair(t)
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	lease.upstream = upstream
	lease.ready = true
	lease.readyStatus = map[string]any{"status": "ready"}

	browser, browserPeer := newLSPTestWebSocketPair(t)
	generation, _, err := lease.attach(browser)
	if err != nil {
		t.Fatal(err)
	}
	_ = readLSPLeaseStatus(t, browserPeer)
	if err := lease.handleBrowserMessage(generation, []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)); err != nil {
		t.Fatal(err)
	}
	first := readLSPJSONRPCMessage(t, upstreamPeer)
	failedResponse := []byte(`{"jsonrpc":"2.0","id":` + string(first["id"]) + `,"error":{"code":-32002,"message":"initialize failed"}}`)
	if err := lease.handleServerResponse(jsonRPCMessage{ID: first["id"], Error: json.RawMessage(`{"code":-32002,"message":"initialize failed"}`)}, failedResponse); err != nil {
		t.Fatal(err)
	}
	if response := readJSONRPCResponse(t, browserPeer); len(response.Error) == 0 {
		t.Fatalf("initialize response error = %s, want failure", response.Error)
	}
	if len(lease.initializeResponse) != 0 {
		t.Fatalf("failed initialize response was cached: %s", lease.initializeResponse)
	}
	if err := lease.handleBrowserMessage(generation, []byte(`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{}}`)); err != nil {
		t.Fatal(err)
	}
	second := readLSPJSONRPCMessage(t, upstreamPeer)
	if string(second["method"]) != `"initialize"` {
		t.Fatalf("retry upstream method = %s, want a fresh initialize request", second["method"])
	}
}

func TestLSPDynamicRegistrationUpdatesWhileAttachedAndAfterResume(t *testing.T) {
	upstream, server := newLSPTestWebSocketPair(t)
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	lease.upstream = upstream
	lease.ready = true
	lease.readyStatus = map[string]any{"status": "ready"}
	lease.initializeResult = []byte(`{"capabilities":{}}`)

	first, firstPeer := newLSPTestWebSocketPair(t)
	firstGeneration, _, err := lease.attach(first)
	if err != nil {
		t.Fatal(err)
	}
	_ = readLSPLeaseStatus(t, firstPeer)
	register := jsonRPCMessage{
		JSONRPC: "2.0", ID: json.RawMessage(`17`), Method: "client/registerCapability",
		Params: json.RawMessage(`{"registrations":[{"id":"hover-1","method":"textDocument/hover","registerOptions":{"documentSelector":[{"language":"go"}]}}]}`),
	}
	registerRaw := []byte(`{"jsonrpc":"2.0","id":17,"method":"client/registerCapability","params":{"registrations":[{"id":"hover-1","method":"textDocument/hover","registerOptions":{"documentSelector":[{"language":"go"}]}}]}}`)
	if err := lease.handleServerRequest(register, registerRaw); err != nil {
		t.Fatal(err)
	}
	forwarded := readLSPJSONRPCMessage(t, firstPeer)
	var forwardedID json.RawMessage
	if err := json.Unmarshal(forwarded["id"], &forwardedID); err != nil {
		t.Fatal(err)
	}
	responseRaw := []byte(`{"jsonrpc":"2.0","id":` + string(forwardedID) + `,"result":null}`)
	if err := lease.handleBrowserResponse(firstGeneration, jsonRPCMessage{ID: forwardedID}, responseRaw); err != nil {
		t.Fatal(err)
	}
	if response := readJSONRPCResponse(t, server); string(response.ID) != `17` {
		t.Fatalf("registration response id = %s, want 17", response.ID)
	}
	lease.detach(firstGeneration)

	second, secondPeer := newLSPTestWebSocketPair(t)
	secondGeneration, resumed, err := lease.attach(second)
	if err != nil {
		t.Fatal(err)
	}
	if !resumed {
		t.Fatal("reattachment did not use retained initialize state")
	}
	handshake := readLSPLeaseStatus(t, secondPeer)
	registrations, ok := handshake["registrations"].([]any)
	if !ok || len(registrations) != 1 {
		t.Fatalf("resumed registrations = %#v, want one retained registration", handshake["registrations"])
	}

	unregister := jsonRPCMessage{
		JSONRPC: "2.0", ID: json.RawMessage(`18`), Method: "client/unregisterCapability",
		Params: json.RawMessage(`{"unregisterations":[{"id":"hover-1","method":"textDocument/hover"}]}`),
	}
	unregisterRaw := []byte(`{"jsonrpc":"2.0","id":18,"method":"client/unregisterCapability","params":{"unregisterations":[{"id":"hover-1","method":"textDocument/hover"}]}}`)
	if err := lease.handleServerRequest(unregister, unregisterRaw); err != nil {
		t.Fatal(err)
	}
	forwarded = readLSPJSONRPCMessage(t, secondPeer)
	if err := json.Unmarshal(forwarded["id"], &forwardedID); err != nil {
		t.Fatal(err)
	}
	responseRaw = []byte(`{"jsonrpc":"2.0","id":` + string(forwardedID) + `,"result":null}`)
	if err := lease.handleBrowserResponse(secondGeneration, jsonRPCMessage{ID: forwardedID}, responseRaw); err != nil {
		t.Fatal(err)
	}
	if response := readJSONRPCResponse(t, server); string(response.ID) != `18` {
		t.Fatalf("unregistration response id = %s, want 18", response.ID)
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if len(lease.registrations) != 0 {
		t.Fatalf("unregistered capability remains in the broker snapshot: %#v", lease.registrations)
	}
}

func TestLSPInitializeHandshakeSurvivesDetachBeforeResponse(t *testing.T) {
	upstream, server := newLSPTestWebSocketPair(t)
	upstreamFrames := observeLSPFrames(t, server)
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	lease.upstream = upstream
	lease.ready = true
	lease.readyStatus = map[string]any{"status": "ready"}

	first, firstPeer := newLSPTestWebSocketPair(t)
	firstGeneration, _, err := lease.attach(first)
	if err != nil {
		t.Fatal(err)
	}
	_ = readLSPLeaseStatus(t, firstPeer)
	if err := lease.handleBrowserMessage(firstGeneration, []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)); err != nil {
		t.Fatal(err)
	}
	initialize := decodeLSPJSONRPCMessage(t, nextObservedLSPFrame(t, upstreamFrames))
	if string(initialize["method"]) != `"initialize"` {
		t.Fatalf("first upstream request = %s, want initialize", initialize["method"])
	}
	serverID := append([]byte(nil), initialize["id"]...)
	lease.detach(firstGeneration)

	second, secondPeer := newLSPTestWebSocketPair(t)
	secondGeneration, resumed, err := lease.attach(second)
	if err != nil {
		t.Fatal(err)
	}
	if resumed {
		t.Fatal("pending initialize was reported as resumed")
	}
	_ = readLSPLeaseStatus(t, secondPeer)
	if err := lease.handleBrowserMessage(secondGeneration, []byte(`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{}}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-upstreamFrames:
		t.Fatal("detach or reconnect sent an extra upstream frame before initialize completed")
	case <-time.After(50 * time.Millisecond):
	}
	result := json.RawMessage(`{"capabilities":{"hoverProvider":true}}`)
	responseRaw, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": json.RawMessage(serverID), "result": result,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.handleServerResponse(jsonRPCMessage{ID: serverID, Result: result}, responseRaw); err != nil {
		t.Fatal(err)
	}
	response := readJSONRPCResponse(t, secondPeer)
	if string(response.ID) != `2` {
		t.Fatalf("initialize response id = %s, want 2", response.ID)
	}
	if err := lease.handleBrowserMessage(secondGeneration, []byte(`{"jsonrpc":"2.0","method":"initialized","params":{}}`)); err != nil {
		t.Fatal(err)
	}
	initialized := decodeLSPJSONRPCMessage(t, nextObservedLSPFrame(t, upstreamFrames))
	if string(initialized["method"]) != `"initialized"` {
		t.Fatalf("upstream notification = %s, want initialized", initialized["method"])
	}
}

func TestLSPInitializeHandshakeResumesBeforeInitializedNotification(t *testing.T) {
	upstream, server := newLSPTestWebSocketPair(t)
	upstreamFrames := observeLSPFrames(t, server)
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	lease.upstream = upstream
	lease.ready = true
	lease.readyStatus = map[string]any{"status": "ready"}

	first, firstPeer := newLSPTestWebSocketPair(t)
	firstGeneration, _, err := lease.attach(first)
	if err != nil {
		t.Fatal(err)
	}
	_ = readLSPLeaseStatus(t, firstPeer)
	if err := lease.handleBrowserMessage(firstGeneration, []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)); err != nil {
		t.Fatal(err)
	}
	initialize := decodeLSPJSONRPCMessage(t, nextObservedLSPFrame(t, upstreamFrames))
	result := json.RawMessage(`{"capabilities":{"hoverProvider":true}}`)
	responseRaw, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": initialize["id"], "result": result,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.handleServerResponse(jsonRPCMessage{ID: initialize["id"], Result: result}, responseRaw); err != nil {
		t.Fatal(err)
	}
	_ = readJSONRPCResponse(t, firstPeer)
	lease.detach(firstGeneration)

	second, secondPeer := newLSPTestWebSocketPair(t)
	secondGeneration, resumed, err := lease.attach(second)
	if err != nil {
		t.Fatal(err)
	}
	if !resumed {
		t.Fatal("completed initialize response was not reported as resumed")
	}
	handshake := readLSPLeaseStatus(t, secondPeer)
	if handshake["initialized"] != false {
		t.Fatalf("resume handshake initialized = %v, want false", handshake["initialized"])
	}
	if err := lease.handleBrowserMessage(secondGeneration, []byte(`{"jsonrpc":"2.0","method":"initialized","params":{}}`)); err != nil {
		t.Fatal(err)
	}
	initialized := decodeLSPJSONRPCMessage(t, nextObservedLSPFrame(t, upstreamFrames))
	if string(initialized["method"]) != `"initialized"` {
		t.Fatalf("upstream notification = %s, want initialized", initialized["method"])
	}
	lease.mu.Lock()
	gotInitialized := lease.initializedReceived
	lease.mu.Unlock()
	if !gotInitialized {
		t.Fatal("broker did not record the initialized notification")
	}
}

func TestLSPLeaseRewritesMonotonicVersionsAndRejectsStaleDiagnostics(t *testing.T) {
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	lease.resumedGeneration = true
	first, err := lease.rewriteDocumentVersion("textDocument/didOpen", []byte(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///main.go","version":80}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := documentVersionFromMessage(t, first); got != 1 {
		t.Fatalf("first server-facing version = %d, want 1", got)
	}
	change, err := lease.rewriteDocumentVersion("textDocument/didChange", []byte(`{"jsonrpc":"2.0","method":"textDocument/didChange","params":{"textDocument":{"uri":"file:///main.go","version":1}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := documentVersionFromMessage(t, change); got != 2 {
		t.Fatalf("second server-facing version = %d, want 2", got)
	}

	stale, err := lease.acceptDiagnostics(json.RawMessage(`{"uri":"file:///main.go","version":1}`))
	if err != nil || stale {
		t.Fatalf("stale diagnostics accepted = %v, error = %v", stale, err)
	}
	current, err := lease.acceptDiagnostics(json.RawMessage(`{"uri":"file:///main.go","version":2}`))
	if err != nil || !current {
		t.Fatalf("current diagnostics accepted = %v, error = %v", current, err)
	}
	unversioned, err := lease.acceptDiagnostics(json.RawMessage(`{"uri":"file:///main.go"}`))
	if err != nil || unversioned {
		t.Fatalf("unversioned resumed diagnostics accepted = %v, error = %v", unversioned, err)
	}

	_, err = lease.rewriteDocumentVersion("textDocument/didClose", []byte(`{"jsonrpc":"2.0","method":"textDocument/didClose","params":{"textDocument":{"uri":"file:///main.go"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := lease.documentVersions["file:///main.go"]; got != 2 {
		t.Fatalf("didClose changed version counter to %d", got)
	}
}

func TestLSPLeaseDetachedBrokerRepliesOnlyToSupportedRequests(t *testing.T) {
	upstream, peer := newLSPTestWebSocketPair(t)
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	lease.upstream = upstream

	register := jsonRPCMessage{
		JSONRPC: "2.0", ID: json.RawMessage(`17`), Method: "client/registerCapability",
		Params: json.RawMessage(`{"registrations":[{"id":"hover-1","method":"textDocument/hover","registerOptions":{}}]}`),
	}
	if err := lease.handleServerRequest(register, nil); err != nil {
		t.Fatal(err)
	}
	if got := readJSONRPCResponse(t, peer); string(got.Result) != "null" || len(got.Error) != 0 {
		t.Fatalf("detached registration response = %+v, want null result", got)
	}
	if _, ok := lease.registrations["hover-1"]; !ok {
		t.Fatal("detached dynamic registration was not retained")
	}

	unsupported := jsonRPCMessage{
		JSONRPC: "2.0", ID: json.RawMessage(`18`), Method: "workspace/executeCommand",
	}
	if err := lease.handleServerRequest(unsupported, nil); err != nil {
		t.Fatal(err)
	}
	response := readJSONRPCResponse(t, peer)
	var responseError struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(response.Error, &responseError); err != nil {
		t.Fatal(err)
	}
	if responseError.Code != -32601 {
		t.Fatalf("unsupported detached response code = %d, want -32601", responseError.Code)
	}
}

func TestLSPLeaseCancellationUsesMappedServerRequestID(t *testing.T) {
	upstream, peer := newLSPTestWebSocketPair(t)
	lease := newTestLSPLease(newLSPLeaseManager(2, testLogger()))
	lease.upstream = upstream
	lease.generation = 4
	lease.browser = &gorillaws.Conn{}
	lease.clientRequests["42"] = lspLeaseClientRequest{
		serverID: json.RawMessage(`"kandev:client:7"`), generation: 4,
	}

	if err := lease.handleBrowserMessage(4, []byte(`{"jsonrpc":"2.0","method":"$/cancelRequest","params":{"id":42}}`)); err != nil {
		t.Fatal(err)
	}
	_, message, err := peer.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var cancellation struct {
		Params struct {
			ID json.RawMessage `json:"id"`
		} `json:"params"`
	}
	if err := json.Unmarshal(message, &cancellation); err != nil {
		t.Fatal(err)
	}
	if string(cancellation.Params.ID) != `"kandev:client:7"` {
		t.Fatalf("mapped cancellation id = %s, want %s", cancellation.Params.ID, `"kandev:client:7"`)
	}
}

func TestLSPContinuityReconnectsToSameTaskHostStream(t *testing.T) {
	var streamStarts atomic.Int32
	var initializeRequests atomic.Int32
	didOpen := make(chan struct{}, 1)
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := lspUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		streamStarts.Add(1)
		defer func() { _ = conn.Close() }()
		if err := conn.WriteJSON(map[string]any{
			"status": "ready", "workspacePath": "/workspace", "workspaceUri": "file:///workspace",
			"repoSubpaths": []string{},
		}); err != nil {
			return
		}
		for {
			_, message, readErr := conn.ReadMessage()
			if readErr != nil {
				return
			}
			var rpc jsonRPCMessage
			if json.Unmarshal(message, &rpc) != nil {
				continue
			}
			switch rpc.Method {
			case "initialize":
				initializeRequests.Add(1)
				_ = conn.WriteJSON(map[string]any{
					"jsonrpc": "2.0", "id": rpc.ID,
					"result": map[string]any{"capabilities": map[string]any{"hoverProvider": true}},
				})
			case "textDocument/didOpen":
				select {
				case didOpen <- struct{}{}:
				default:
				}
			case "shutdown":
				_ = conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": rpc.ID, "result": nil})
			case "exit":
				return
			}
		}
	}))
	t.Cleanup(host.Close)
	parsed, err := url.Parse(host.URL)
	if err != nil {
		t.Fatal(err)
	}
	hostname, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	execution := &lifecycle.AgentExecution{
		ID: "execution-continuity", TaskID: "task-continuity", SessionID: "session-continuity",
		RuntimeName: agentruntime.RuntimeStandalone,
	}
	execution.SetAgentCtlClientForTesting(agentctlclient.NewClient(hostname, port, testLogger()))
	manager := &recordingLSPLifecycleManager{
		runtimeName: agentruntime.RuntimeStandalone,
		execution:   execution,
	}
	handler := &LSPHandler{
		lifecycleMgr: manager,
		capacity:     newLSPCapacityLimiter(2),
		logger:       testLogger(),
	}
	var fenceHeld atomic.Bool
	handler.EnableContinuity(func(string) func() {
		fenceHeld.Store(true)
		return func() { fenceHeld.Store(false) }
	}, nil)
	t.Cleanup(func() { _ = handler.Close() })

	first, firstServed := dialLSPHandler(t, handler, "/lsp/session-continuity?language=go")
	ready := readLSPLeaseStatus(t, first)
	leaseID, _ := ready["leaseId"].(string)
	if leaseID == "" || ready["resumed"] != false {
		t.Fatalf("first ready status = %v, want an initial lease ID and resumed=false", ready)
	}
	if fenceHeld.Load() {
		t.Fatal("session lifecycle fence remained held after lease admission")
	}
	if err := first.WriteMessage(gorillaws.TextMessage, []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)); err != nil {
		t.Fatal(err)
	}
	initialize := readLSPJSONRPCMessage(t, first)
	if string(initialize["id"]) != "1" {
		t.Fatalf("initialize response id = %s, want 1", initialize["id"])
	}
	_ = first.Close()
	joinWithin(t, firstServed, "first continuity browser attachment")
	waitForLSPLease(t, handler.leases, leaseID, true)

	second, secondServed := dialLSPHandler(t, handler, "/lsp/session-continuity?language=go&leaseId="+leaseID)
	resumed := readLSPLeaseStatus(t, second)
	if resumed["leaseId"] != leaseID || resumed["resumed"] != true {
		t.Fatalf("reattached status = %v, want the same lease ID and resumed=true", resumed)
	}
	if err := second.WriteMessage(gorillaws.TextMessage, []byte(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///workspace/main.go","version":1,"text":"package main"}}}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-didOpen:
	case <-time.After(wsTestTimeout):
		t.Fatal("reattached document did not reach the retained server")
	}
	if err := second.WriteMessage(gorillaws.TextMessage, []byte(`{"kandev":"lsp","action":"attachmentReady","requestId":"sync-2"}`)); err != nil {
		t.Fatal(err)
	}
	if ack := readLSPLeaseStatus(t, second); ack["action"] != "attachmentReady" || ack["requestId"] != "sync-2" {
		t.Fatalf("attachment synchronization acknowledgement = %v", ack)
	}
	if got := streamStarts.Load(); got != 1 {
		t.Fatalf("task-host LSP stream starts = %d, want 1", got)
	}
	if got := initializeRequests.Load(); got != 1 {
		t.Fatalf("initialize requests = %d, want 1", got)
	}

	third, thirdServed := dialLSPHandler(t, handler, "/lsp/session-continuity?language=go")
	independent := readLSPLeaseStatus(t, third)
	independentLeaseID, _ := independent["leaseId"].(string)
	if independentLeaseID == "" || independentLeaseID == leaseID || independent["resumed"] != false {
		t.Fatalf("independent window status = %v, want a new lease", independent)
	}
	if got := streamStarts.Load(); got != 2 {
		t.Fatalf("task-host LSP stream starts with two windows = %d, want 2", got)
	}
	if err := third.WriteMessage(gorillaws.TextMessage, []byte(`{"kandev":"lsp","action":"release","reason":"stop","requestId":"stop-independent"}`)); err != nil {
		t.Fatal(err)
	}
	if ack := readLSPLeaseStatus(t, third); ack["action"] != "released" {
		t.Fatalf("independent window Stop acknowledgement = %v", ack)
	}
	joinWithin(t, thirdServed, "independent continuity browser attachment")
	waitForLSPLeaseRemoved(t, handler.leases, independentLeaseID)
	handler.leases.mu.Lock()
	originalLease := handler.leases.leases[leaseID]
	remainingLeases := len(handler.leases.leases)
	handler.leases.mu.Unlock()
	originalLeaseClosed := originalLease == nil || originalLease.isClosed()
	if originalLease == nil || originalLeaseClosed || remainingLeases != 1 {
		t.Fatalf("stopping independent window affected retained lease: present=%t closed=%t count=%d", originalLease != nil, originalLeaseClosed, remainingLeases)
	}

	if err := second.WriteMessage(gorillaws.TextMessage, []byte(`{"kandev":"lsp","action":"release","reason":"stop","requestId":"stop-1"}`)); err != nil {
		t.Fatal(err)
	}
	ack := readLSPLeaseStatus(t, second)
	if ack["action"] != "released" || ack["requestId"] != "stop-1" {
		t.Fatalf("explicit Stop acknowledgement = %v", ack)
	}
	joinWithin(t, secondServed, "second continuity browser attachment")
}
