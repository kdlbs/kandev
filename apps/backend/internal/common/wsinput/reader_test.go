package wsinput

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/common/wskeepalive"
)

func TestReaderKeepsHandlingPongsWhileConsumerWaits(t *testing.T) {
	client, server := newWebSocketPair(t)
	stopKeepalive, err := wskeepalive.Start(server, wskeepalive.Timing{
		WriteWait:  time.Second,
		PongWait:   80 * time.Millisecond,
		PingPeriod: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("start keepalive: %v", err)
	}
	reader := New(server)
	pings := make(chan struct{}, 32)
	client.SetPingHandler(func(payload string) error {
		select {
		case pings <- struct{}{}:
		default:
		}
		return client.WriteControl(websocket.PongMessage, []byte(payload), time.Now().Add(time.Second))
	})
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		for {
			if _, _, err := client.ReadMessage(); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		stopKeepalive()
		_ = reader.Err()
		<-clientDone
	})

	if err := client.WriteMessage(websocket.BinaryMessage, []byte("blocked-input")); err != nil {
		t.Fatalf("write first input: %v", err)
	}
	if got := receiveMessage(t, reader); got.Type != websocket.BinaryMessage || string(got.Data) != "blocked-input" {
		t.Fatalf("first message = %+v, want blocked-input binary frame", got)
	}
	queuedInputs := []string{"queued-input", "queued-input-3", "queued-input-4", "queued-input-5"}
	for _, input := range queuedInputs {
		if err := client.WriteMessage(websocket.BinaryMessage, []byte(input)); err != nil {
			t.Fatalf("write queued input: %v", err)
		}
	}
	for range 6 {
		select {
		case <-pings:
		case <-time.After(3 * time.Second):
			t.Fatal("reader stopped processing pongs while its consumer waited")
		}
	}
	for _, want := range queuedInputs {
		got := receiveMessage(t, reader)
		if got.Type != websocket.BinaryMessage || string(got.Data) != want {
			t.Fatalf("queued message = %+v, want %q binary frame", got, want)
		}
	}
}

func TestReaderClosesConnectionWhenInputBacklogIsFull(t *testing.T) {
	client, server := newWebSocketPair(t)
	reader := New(server)
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		_ = reader.Err()
	})

	for range maxBufferedMessages + 1 {
		if err := client.WriteMessage(websocket.BinaryMessage, []byte("input")); err != nil {
			break
		}
	}
	errResult := make(chan error, 1)
	go func() { errResult <- reader.Err() }()
	select {
	case err := <-errResult:
		if err != ErrBacklogFull {
			t.Fatalf("Reader.Err() = %v, want ErrBacklogFull", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reader did not close the connection after its input backlog filled")
	}
	if got := receiveMessage(t, reader); string(got.Data) != "input" {
		t.Fatalf("buffered message = %q, want input", got.Data)
	}
}

func TestReaderClosesConnectionWhenByteBacklogIsFull(t *testing.T) {
	client, server := newWebSocketPair(t)
	reader := New(server)
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		_ = reader.Err()
	})

	payload := strings.Repeat("x", maxBufferedBytes/200+1)
	messageCount := maxBufferedBytes/len(payload) + 1
	for range messageCount {
		if err := client.WriteMessage(websocket.BinaryMessage, []byte(payload)); err != nil {
			break
		}
	}
	if err := reader.Err(); err != ErrBacklogFull {
		t.Fatalf("Reader.Err() = %v, want ErrBacklogFull", err)
	}
}

func TestReaderAcceptsOversizedMessageWhenBacklogIsEmpty(t *testing.T) {
	client, server := newWebSocketPair(t)
	reader := New(server)
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		_ = reader.Err()
	})

	payload := strings.Repeat("x", maxBufferedBytes+1)
	if err := client.WriteMessage(websocket.BinaryMessage, []byte(payload)); err != nil {
		t.Fatalf("write oversized input: %v", err)
	}
	if got := receiveMessage(t, reader); len(got.Data) != len(payload) {
		t.Fatalf("message size = %d, want %d", len(got.Data), len(payload))
	}
}

func newWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()
	serverConn := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			serverConn <- conn
		}
	}))
	t.Cleanup(server.Close)
	clientURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(clientURL, nil)
	if err != nil {
		t.Fatalf("dial test websocket: %v", err)
	}
	select {
	case conn := <-serverConn:
		return client, conn
	case <-time.After(3 * time.Second):
		_ = client.Close()
		t.Fatal("server did not accept websocket connection")
		return nil, nil
	}
}

func receiveMessage(t *testing.T, reader *Reader) Message {
	t.Helper()
	select {
	case message, ok := <-reader.Messages():
		if !ok {
			t.Fatal("reader closed before delivering a message")
		}
		reader.Consume(message)
		return message
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a websocket input message")
		return Message{}
	}
}
