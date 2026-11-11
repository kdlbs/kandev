package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/models"
	"go.uber.org/zap"
)

func newWebhookTestService(t *testing.T, repo *notificationTestRepository, allowPrivateNetworks ...bool) *Service {
	t.Helper()
	t.Setenv(desktopNativeNotificationsEnv, "true")
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	allowPrivate := len(allowPrivateNetworks) > 0 && allowPrivateNetworks[0]
	return NewService(repo, nil, nil, log, nil, allowPrivate)
}

func TestNewServiceRegistersWebhookProvider(t *testing.T) {
	repo := &notificationTestRepository{subscriptions: map[string][]*models.Subscription{}}
	svc := newWebhookTestService(t, repo)
	if svc.providers[models.ProviderTypeWebhook] == nil {
		t.Fatal("webhook provider must be registered")
	}
	if !svc.providers[models.ProviderTypeWebhook].Available() {
		t.Fatal("webhook provider must be available without detection")
	}
}

func TestNewServicePassesWebhookNetworkPolicy(t *testing.T) {
	repo := &notificationTestRepository{subscriptions: map[string][]*models.Subscription{}}
	service := newWebhookTestService(t, repo, true)
	if err := service.providers[models.ProviderTypeWebhook].Validate(map[string]interface{}{
		"url": "http://127.0.0.1/hook",
	}); err != nil {
		t.Fatalf("private destination with the configured opt-in: %v", err)
	}

	service = newWebhookTestService(t, repo)
	if err := service.providers[models.ProviderTypeWebhook].Validate(map[string]interface{}{
		"url": "http://127.0.0.1/hook",
	}); err == nil {
		t.Fatal("private destination must remain blocked without the configured opt-in")
	}
}

func TestWebhookProviderCreateListAndUpdate(t *testing.T) {
	repo := &notificationTestRepository{subscriptions: map[string][]*models.Subscription{}}
	svc := newWebhookTestService(t, repo)
	ctx := context.Background()
	userID := "user-1"

	config := map[string]interface{}{
		"url":     "https://93.184.216.34/hooks/kandev/turn-finished",
		"headers": map[string]interface{}{"X-Tenant": "acme"},
		"secret":  "gateway-token",
	}
	provider, err := svc.CreateProvider(ctx, userID, "GoClaw", models.ProviderTypeWebhook, config, true, []string{EventTaskSessionTurnFinished})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if provider.Type != models.ProviderTypeWebhook {
		t.Fatalf("type = %q", provider.Type)
	}

	providers, subscriptions, err := svc.ListProviders(ctx, userID)
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	var found *models.Provider
	for _, candidate := range providers {
		if candidate.ID == provider.ID {
			found = candidate
		}
	}
	if found == nil {
		t.Fatal("webhook provider missing from list")
	}
	if events := subscriptions[provider.ID]; len(events) != 1 || events[0] != EventTaskSessionTurnFinished {
		t.Fatalf("subscriptions = %#v", events)
	}

	nextURL := "https://93.184.216.34/hooks/kandev/clarification"
	updated, err := svc.UpdateProvider(ctx, userID, provider.ID, ProviderUpdate{
		Config: map[string]interface{}{"url": nextURL},
	})
	if err != nil {
		t.Fatalf("update provider: %v", err)
	}
	if updated.Config["url"] != nextURL {
		t.Fatalf("updated url = %v", updated.Config["url"])
	}
}

func TestWebhookProviderRejectsInvalidURL(t *testing.T) {
	repo := &notificationTestRepository{subscriptions: map[string][]*models.Subscription{}}
	svc := newWebhookTestService(t, repo)
	if _, err := svc.CreateProvider(context.Background(), "user-1", "Broken", models.ProviderTypeWebhook, map[string]interface{}{"url": "ftp://example.test"}, true, nil); err == nil {
		t.Fatal("expected invalid url error")
	}
}

func TestWebhookProviderRejectsPrivateDestination(t *testing.T) {
	repo := &notificationTestRepository{subscriptions: map[string][]*models.Subscription{}}
	svc := newWebhookTestService(t, repo)
	if _, err := svc.CreateProvider(context.Background(), "user-1", "Metadata", models.ProviderTypeWebhook, map[string]interface{}{"url": "http://169.254.169.254/latest/meta-data"}, true, nil); err == nil {
		t.Fatal("expected private destination error")
	}
}

func TestWebhookProviderTestButtonSendsGenericSchema(t *testing.T) {
	type captured struct {
		event   string
		title   string
		message string
		fields  map[string]string
	}
	received := make(chan captured, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Event   string            `json:"event"`
			Title   string            `json:"title"`
			Message string            `json:"message"`
			Fields  map[string]string `json:"fields"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		received <- captured{event: payload.Event, title: payload.Title, message: payload.Message, fields: payload.Fields}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	repo := &notificationTestRepository{subscriptions: map[string][]*models.Subscription{}}
	svc := newWebhookTestService(t, repo, true)
	ctx := context.Background()
	provider, err := svc.CreateProvider(ctx, "user-1", "GoClaw", models.ProviderTypeWebhook, map[string]interface{}{"url": server.URL}, true, nil)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if err := svc.TestProvider(ctx, "user-1", provider.ID); err != nil {
		t.Fatalf("test provider: %v", err)
	}
	result := <-received
	if result.event != EventTaskSessionClarificationAsked {
		t.Fatalf("event = %q", result.event)
	}
	if result.title != "Test notification" {
		t.Fatalf("title = %q", result.title)
	}
	if len(result.fields) != 0 {
		t.Fatalf("fields = %#v, want empty", result.fields)
	}
}
