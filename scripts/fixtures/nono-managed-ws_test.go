package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestExchangeAcceptsCoalescedGatewayFrames(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		if _, _, err = conn.ReadMessage(); err != nil {
			t.Error(err)
			return
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte("{\"type\":\"event\",\"action\":\"session.state_changed\"}\n{\"type\":\"response\",\"id\":\"owned\",\"payload\":{\"success\":true}}\n"))
	}))
	defer server.Close()
	connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	previous := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = previous; _ = writer.Close() }()
	err = exchange(connection, request{ID: "owned", Action: "session.stop"})
	_ = writer.Close()
	output, _ := io.ReadAll(reader)
	if err != nil || !strings.Contains(string(output), `"success":true`) {
		t.Fatalf("matching coalesced response lost: err=%v output=%q", err, output)
	}
}
