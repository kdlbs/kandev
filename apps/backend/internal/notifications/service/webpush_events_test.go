package service

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/models"
)

func TestAttentionEventsAreAvailableAndValid(t *testing.T) {
	svc := &Service{}
	available := map[string]bool{}
	for _, event := range svc.AvailableEvents() {
		available[event] = true
	}
	for _, event := range []string{EventTaskSessionPermissionAsked, EventTaskSessionFailed} {
		if !available[event] {
			t.Errorf("%s missing from AvailableEvents", event)
		}
		if err := svc.validateEvents([]string{event}); err != nil {
			t.Errorf("%s rejected by validateEvents: %v", event, err)
		}
	}
}

func TestAttentionEventCopyIsShortStatusOnly(t *testing.T) {
	for _, event := range []string{EventTaskSessionPermissionAsked, EventTaskSessionFailed} {
		title, body := semanticMessageCopy(event, "Fix login")
		if title == "" || body == "" {
			t.Errorf("%s: empty copy", event)
		}
		if title == "Agent turn finished" {
			t.Errorf("%s fell through to the turn-finished copy", event)
		}
	}
}

func TestEnsureWebPushProviderCreatesOneProviderWithAttentionEvents(t *testing.T) {
	repo := &notificationTestRepository{subscriptions: map[string][]*models.Subscription{}}
	t.Setenv(desktopNativeNotificationsEnv, "true")
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	svc := NewService(repo, nil, nil, log, nil, false)
	ctx := context.Background()

	if err := svc.EnsureWebPushProvider(ctx, "u1"); err == nil {
		t.Fatal("expected an error while web push is not registered")
	}
	svc.RegisterProvider(models.ProviderTypeWebPush, &captureProvider{})
	for i := 0; i < 2; i++ {
		if err := svc.EnsureWebPushProvider(ctx, "u1"); err != nil {
			t.Fatalf("EnsureWebPushProvider: %v", err)
		}
	}
	var pushProviders []*models.Provider
	for _, p := range repo.providers {
		if p.Type == models.ProviderTypeWebPush {
			pushProviders = append(pushProviders, p)
		}
	}
	if len(pushProviders) != 1 || !pushProviders[0].Enabled {
		t.Fatalf("web push providers = %d, want exactly one enabled", len(pushProviders))
	}
	events := map[string]bool{}
	for _, sub := range repo.subscriptions[pushProviders[0].ID] {
		events[sub.EventType] = true
	}
	for _, want := range []string{EventTaskSessionPermissionAsked, EventTaskSessionFailed, EventTaskSessionClarificationAsked, EventTaskSessionTurnFinished} {
		if !events[want] {
			t.Errorf("default web push events missing %s", want)
		}
	}
}
