package websocket

import (
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
)

func TestLSPDetachCleanupPrecedesSuccessorDocumentWrites(t *testing.T) {
	manager := newLSPLeaseManager(2, testLogger())
	lease := newTestLSPLease(manager)
	upstream, upstreamPeer := newLSPTestWebSocketPair(t)
	upstreamFrames := observeLSPFrames(t, upstreamPeer)
	lease.upstream = upstream
	lease.ready = true
	lease.readyStatus = map[string]any{"status": "ready"}
	lease.initializeResult = []byte(`{"capabilities":{}}`)
	if err := manager.add(lease); err != nil {
		t.Fatal(err)
	}

	closeEntered := make(chan struct{})
	releaseClose := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseClose) }) }
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	wrapped := &lspBlockingCloseListener{Listener: listener, entered: closeEntered, release: releaseClose}
	connections := make(chan *gorillaws.Conn, 2)
	httpServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := lspUpgrader.Upgrade(w, r, nil)
		if err == nil {
			connections <- conn
		}
	})}
	go func() { _ = httpServer.Serve(wrapped) }()
	var first, firstPeer, second, secondPeer *gorillaws.Conn
	t.Cleanup(func() {
		release()
		if first != nil {
			_ = first.Close()
		}
		if firstPeer != nil {
			_ = firstPeer.Close()
		}
		if second != nil {
			_ = second.Close()
		}
		if secondPeer != nil {
			_ = secondPeer.Close()
		}
		_ = upstream.Close()
		_ = httpServer.Close()
		_ = listener.Close()
	})
	dialBrowser := func() (*gorillaws.Conn, *gorillaws.Conn) {
		t.Helper()
		client, response, err := gorillaws.DefaultDialer.Dial("ws://"+listener.Addr().String(), nil)
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if err != nil {
			t.Fatalf("dial test browser: %v", err)
		}
		var server *gorillaws.Conn
		select {
		case server = <-connections:
		case <-time.After(wsTestTimeout):
			t.Fatal("test browser did not connect")
		}
		return server, client
	}

	first, firstPeer = dialBrowser()
	firstGeneration, _, err := lease.attach(first)
	if err != nil {
		t.Fatal(err)
	}
	_ = readLSPLeaseStatus(t, firstPeer)
	if err := lease.handleBrowserMessage(firstGeneration, []byte(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///old.go","languageId":"go","version":1,"text":"package old"}}}`)); err != nil {
		t.Fatal(err)
	}
	if message := decodeLSPJSONRPCMessage(t, nextObservedLSPFrame(t, upstreamFrames)); string(message["method"]) != `"textDocument/didOpen"` {
		t.Fatalf("first upstream document frame = %s, want didOpen", message["method"])
	}

	detachDone := make(chan struct{})
	go func() {
		lease.detach(firstGeneration)
		close(detachDone)
	}()
	select {
	case <-closeEntered:
	case <-time.After(wsTestTimeout):
		t.Fatal("detach did not enter its controlled browser-close barrier")
	}

	second, secondPeer = dialBrowser()
	secondGeneration, _, err := lease.attach(second)
	if err != nil {
		t.Fatal(err)
	}
	_ = readLSPLeaseStatus(t, secondPeer)
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- lease.handleBrowserMessage(secondGeneration, []byte(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///new.go","languageId":"go","version":1,"text":"package current"}}}`))
	}()
	select {
	case frame := <-upstreamFrames:
		message := decodeLSPJSONRPCMessage(t, frame)
		release()
		<-detachDone
		t.Fatalf("successor frame %s overtook old detach cleanup", message["method"])
	case <-time.After(50 * time.Millisecond):
	}

	release()
	select {
	case <-detachDone:
	case <-time.After(wsTestTimeout):
		t.Fatal("detach did not finish after its close barrier released")
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	closed := decodeLSPJSONRPCMessage(t, nextObservedLSPFrame(t, upstreamFrames))
	opened := decodeLSPJSONRPCMessage(t, nextObservedLSPFrame(t, upstreamFrames))
	if string(closed["method"]) != `"textDocument/didClose"` || string(opened["method"]) != `"textDocument/didOpen"` {
		t.Fatalf("upstream order = %s then %s, want old didClose then successor didOpen", closed["method"], opened["method"])
	}
}
