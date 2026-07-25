package providers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	gatewayws "github.com/kandev/kandev/internal/gateway/websocket"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func TestLocalProviderReturnsNoEligibleSubscriberWhenUserHasNoWebSocketClient(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	provider := NewLocalProvider(gatewayws.NewHub(nil, log))
	err = provider.Send(context.Background(), Message{
		EventType:    "system.update_available",
		OccurrenceID: "v1.2.3",
		UserID:       "user-1",
		Title:        "Kandev update available",
		Body:         "Kandev v1.2.3 is available.",
		Payload:      map[string]string{"version": "v1.2.3", "url": "https://example.test/releases/v1.2.3"},
	})
	if !errors.Is(err, ErrNoEligibleSubscriber) {
		t.Fatalf("send error = %v, want no eligible subscriber", err)
	}
}

func TestLocalProviderForwardsUpdatePayloadToSubscribedClient(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	hub := gatewayws.NewHub(nil, log)
	hubCtx, cancelHub := context.WithCancel(context.Background())
	hubDone := make(chan struct{})
	go func() {
		hub.Run(hubCtx)
		close(hubDone)
	}()
	cleanupHub := func() {
		cancelHub()
		<-hubDone
	}
	t.Cleanup(cleanupHub)
	clientReady := make(chan *gatewayws.Client, 1)
	serverDone := make(chan struct{})
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(serverDone)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade connection: %v", err)
			return
		}
		client := gatewayws.NewClient("client-1", authn.Identity{}, conn, hub, log)
		hub.Register(client)
		hub.SubscribeToUser(client, "user-1")
		clientReady <- client
		client.WritePump()
	}))
	t.Cleanup(func() {
		cleanupHub()
		select {
		case <-serverDone:
		case <-time.After(time.Second):
			t.Error("websocket writer did not stop")
		}
		server.Close()
	})

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})
	<-clientReady

	provider := NewLocalProvider(hub)
	if err := provider.Send(context.Background(), Message{
		EventType:    "system.update_available",
		OccurrenceID: "v1.2.3",
		UserID:       "user-1",
		Payload:      map[string]string{"version": "v1.2.3", "url": "https://example.test/releases/v1.2.3"},
	}); err != nil {
		t.Fatalf("send notification: %v", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read notification: %v", err)
	}
	var message ws.Message
	if err := json.Unmarshal(data, &message); err != nil {
		t.Fatalf("decode notification: %v", err)
	}
	var payload struct {
		Version string `json:"version"`
		URL     string `json:"url"`
	}
	if err := message.ParsePayload(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if message.Action != "system.update_available" || payload.Version != "v1.2.3" || payload.URL != "https://example.test/releases/v1.2.3" {
		t.Fatalf("forwarded notification = %#v with payload %#v", message, payload)
	}
}
