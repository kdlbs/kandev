package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestClientWritePumpPrioritizesControlAndFairReplaceable(t *testing.T) {
	serverConnCh := make(chan *gorillaws.Conn, 1)
	upgrader := gorillaws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		serverConnCh <- conn
	}))
	defer server.Close()

	clientConn, _, err := gorillaws.DefaultDialer.Dial("ws"+server.URL[len("http"):], nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer func() { _ = clientConn.Close() }()
	serverConn := <-serverConnCh
	defer func() { _ = serverConn.Close() }()

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	c := &Client{
		ID:               "write-pump-priority",
		conn:             serverConn,
		send:             make(chan []byte, 16),
		controlSend:      make(chan []byte, 16),
		notificationWake: make(chan struct{}, 1),
		logger:           log,
	}
	c.controlSend <- []byte("control")
	for i := 0; i < 9; i++ {
		c.send <- []byte("semantic-" + string(rune('0'+i)))
	}
	update, err := ws.NewNotification(ws.ActionSessionMessageUpdated, map[string]any{
		"session_id": "noisy-session",
		"message_id": "message-1",
		"content":    "replaceable",
	})
	if err != nil {
		t.Fatalf("build replaceable frame: %v", err)
	}
	updateBytes, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("marshal replaceable frame: %v", err)
	}
	if !c.sendNotification(updateBytes, update.Action) {
		t.Fatal("queue replaceable frame")
	}

	done := make(chan struct{})
	go func() {
		c.WritePump()
		close(done)
	}()

	var got []string
	for len(got) < 11 {
		messageType, data, readErr := clientConn.ReadMessage()
		if readErr != nil {
			t.Fatalf("read frame %d: %v", len(got), readErr)
		}
		if messageType != gorillaws.TextMessage {
			t.Fatalf("frame %d type = %d, want text", len(got), messageType)
		}
		got = append(got, string(data))
	}

	if got[0] != "control" {
		t.Fatalf("first frame = %q, want control", got[0])
	}
	for index := 1; index <= 8; index++ {
		want := "semantic-" + string(rune('0'+index-1))
		if got[index] != want {
			t.Fatalf("frame %d = %q, want %q", index, got[index], want)
		}
	}
	if got[9] != string(updateBytes) {
		t.Fatalf("replaceable frame = %q, want %q", got[9], updateBytes)
	}
	if got[10] != "semantic-8" {
		t.Fatalf("frame after replaceable = %q, want semantic-8", got[10])
	}

	c.closeSend()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WritePump did not stop after queues closed")
	}
}
