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
	"github.com/kandev/kandev/internal/notifications/models"
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
		Payload:      map[string]string{"version": "v1.2.3", "url": "https://example.test/releases/v1.2.3", "agent_name": "gemini", "runtime_id": "npm:@google/gemini-cli", "display_name": "Gemini", "previous_version": "1.0.0", "runtime_update_status": "available"},
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
		Version  string `json:"version"`
		Agent    string `json:"agent_name"`
		Runtime  string `json:"runtime_id"`
		Name     string `json:"display_name"`
		Previous string `json:"previous_version"`
		Status   string `json:"runtime_update_status"`
		URL      string `json:"url"`
	}
	if err := message.ParsePayload(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Agent != "gemini" || payload.Runtime != "npm:@google/gemini-cli" || payload.Name != "Gemini" || payload.Previous != "1.0.0" || payload.Status != "available" {
		t.Fatalf("lost runtime payload: %+v", payload)
	}
	if message.Action != "system.update_available" || payload.Version != "v1.2.3" || payload.URL != "https://example.test/releases/v1.2.3" {
		t.Fatalf("forwarded notification = %#v with payload %#v", message, payload)
	}

	members := []models.RuntimeUpdateMember{
		{OccurrenceID: "codex-3", AgentName: "codex-app-server", RuntimeID: "npm:@openai/codex", DisplayName: "Codex", PreviousVersion: "1.0.0", Version: "3.0.0"},
		{OccurrenceID: "gemini-2", AgentName: "gemini", RuntimeID: "npm:@google/gemini-cli", DisplayName: "Gemini", PreviousVersion: "1.0.0", Version: "2.0.0"},
	}
	if err := provider.Send(context.Background(), Message{
		EventType: "system.update_available", OccurrenceID: "summary-1", UserID: "user-1",
		Title: "2 agent runtime updates available", Body: "Review versions in Settings > Agents.",
		Payload:        map[string]string{"notification_kind": "agent_runtime_summary", "url": "/settings/agents#runtime-updates"},
		RuntimeUpdates: members,
	}); err != nil {
		t.Fatalf("send summary: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set summary read deadline: %v", err)
	}
	_, data, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if err := json.Unmarshal(data, &message); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	var summary struct {
		Version          string                       `json:"version"`
		NotificationKind string                       `json:"notification_kind"`
		URL              string                       `json:"url"`
		RuntimeUpdates   []models.RuntimeUpdateMember `json:"runtime_updates"`
	}
	if err := message.ParsePayload(&summary); err != nil {
		t.Fatalf("decode summary payload: %v", err)
	}
	if summary.Version != "" || summary.NotificationKind != "agent_runtime_summary" || summary.URL != "/settings/agents#runtime-updates" || len(summary.RuntimeUpdates) != 2 || summary.RuntimeUpdates[0] != members[0] || summary.RuntimeUpdates[1] != members[1] {
		t.Fatalf("summary payload = %+v", summary)
	}
}
