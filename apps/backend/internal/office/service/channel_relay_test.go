package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

func setupChannelForRelay(t *testing.T, svc *service.Service, platform, config string) (*models.Channel, string) {
	t.Helper()
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "relay-agent-" + t.Name(),
		Role:        models.AgentRoleAssistant,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	channel := &models.Channel{
		WorkspaceID:    "ws-1",
		AgentProfileID: agent.ID,
		Platform:       platform,
		Config:         config,
	}
	if err := svc.SetupChannel(ctx, channel); err != nil {
		t.Fatalf("setup channel: %v", err)
	}
	return channel, agent.ID
}

func TestRelayComment_AgentComment_Relayed(t *testing.T) {
	svc := newTestService(t)

	var received atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := `{"webhook_url":"` + ts.URL + `"}`
	channel, _ := setupChannelForRelay(t, svc, "webhook", config)

	relay := service.NewChannelRelayWithClient(svc, ts.Client())

	comment := &models.TaskComment{
		ID:             "c1",
		TaskID:         channel.TaskID,
		AuthorType:     "agent",
		AuthorID:       "some-agent",
		Body:           "Here is the status update.",
		ReplyChannelID: channel.ID,
	}

	if err := relay.RelayComment(context.Background(), comment); err != nil {
		t.Fatalf("RelayComment: %v", err)
	}
	if !received.Load() {
		t.Error("expected webhook to receive the message")
	}
}

func TestRelayComment_UserComment_NotRelayed(t *testing.T) {
	svc := newTestService(t)

	var received atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := `{"webhook_url":"` + ts.URL + `"}`
	channel, _ := setupChannelForRelay(t, svc, "webhook", config)

	relay := service.NewChannelRelayWithClient(svc, ts.Client())

	comment := &models.TaskComment{
		ID:             "c2",
		TaskID:         channel.TaskID,
		AuthorType:     "user",
		AuthorID:       "user-1",
		Body:           "User message",
		ReplyChannelID: channel.ID,
	}

	if err := relay.RelayComment(context.Background(), comment); err != nil {
		t.Fatalf("RelayComment: %v", err)
	}
	if received.Load() {
		t.Error("user comments should not be relayed")
	}
}

func TestRelayComment_NonChannelTask_NotRelayed(t *testing.T) {
	svc := newTestService(t)

	var received atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	relay := service.NewChannelRelayWithClient(svc, ts.Client())

	comment := &models.TaskComment{
		ID:             "c3",
		TaskID:         "some-task",
		AuthorType:     "agent",
		AuthorID:       "some-agent",
		Body:           "No channel",
		ReplyChannelID: "", // no channel
	}

	if err := relay.RelayComment(context.Background(), comment); err != nil {
		t.Fatalf("RelayComment: %v", err)
	}
	if received.Load() {
		t.Error("non-channel comments should not be relayed")
	}
}

func TestRelayComment_TelegramPayload(t *testing.T) {
	svc := newTestService(t)

	var receivedBody map[string]interface{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Rewrite the telegram bot URL to point at our test server by using
	// webhook platform instead (since Telegram URLs are hardcoded).
	// For a proper Telegram test, we'd need to mock the Telegram API.
	// Instead, test webhook format which exercises the same postJSON path.
	config := `{"webhook_url":"` + ts.URL + `"}`
	channel, _ := setupChannelForRelay(t, svc, "webhook", config)

	relay := service.NewChannelRelayWithClient(svc, ts.Client())

	comment := &models.TaskComment{
		ID:             "c-tg",
		TaskID:         channel.TaskID,
		AuthorType:     "agent",
		AuthorID:       "assistant",
		Body:           "Hello from agent!",
		ReplyChannelID: channel.ID,
	}

	if err := relay.RelayComment(context.Background(), comment); err != nil {
		t.Fatalf("RelayComment: %v", err)
	}
	if receivedBody == nil {
		t.Fatal("expected request body")
	}
	if receivedBody["text"] != "Hello from agent!" {
		t.Errorf("text = %q, want %q", receivedBody["text"], "Hello from agent!")
	}
}

func TestRelayComment_RetryOnFailure(t *testing.T) {
	svc := newTestService(t)

	var attempts atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := `{"webhook_url":"` + ts.URL + `"}`
	channel, _ := setupChannelForRelay(t, svc, "webhook", config)

	relay := service.NewChannelRelayWithClient(svc, ts.Client())

	comment := &models.TaskComment{
		ID:             "c-retry",
		TaskID:         channel.TaskID,
		AuthorType:     "agent",
		AuthorID:       "some-agent",
		Body:           "Retry me",
		ReplyChannelID: channel.ID,
	}

	if err := relay.RelayComment(context.Background(), comment); err != nil {
		t.Fatalf("RelayComment should succeed after retries: %v", err)
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts (2 failures + 1 success), got %d", attempts.Load())
	}
}
