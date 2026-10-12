package service

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/models"
	"go.uber.org/zap"
)

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.3, AC-AGENTS-RUNTIME-NOTIFY-002.6
func TestRuntimeOccurrencesRespectPreferencesAndPersistAcrossRestart(t *testing.T) {
	t.Setenv(desktopNativeNotificationsEnv, "true")
	log, _ := logger.NewFromZap(zap.NewNop())
	repo := &notificationTestRepository{providers: []*models.Provider{{ID: "local", Type: models.ProviderTypeLocal, Enabled: true}, {ID: "muted", Type: models.ProviderTypeApprise, Enabled: true}}, subscriptions: map[string][]*models.Subscription{"local": {{ProviderID: "local", EventType: EventSystemUpdateAvailable, Enabled: true}}, "muted": {{ProviderID: "muted", EventType: EventSystemUpdateAvailable, Enabled: false}}}}
	local, muted := &captureProvider{}, &captureProvider{}
	makeService := func() *Service {
		s := NewService(repo, nil, nil, log, nil)
		s.providers[models.ProviderTypeLocal] = local
		s.providers[models.ProviderTypeApprise] = muted
		return s
	}
	svc := makeService()
	for _, name := range []string{"gemini", "codex-app-server"} {
		notice := agents.RuntimeUpdateNotice{AgentID: name, RuntimeID: "npm:" + name, DisplayName: name, PreviousVersion: "1.0.0", Version: "2.0.0", Status: "available", OccurrenceID: "agent-runtime:" + name + ":2.0.0", URL: "/settings/agents#runtime-updates"}
		svc.HandleAgentRuntimeUpdate(context.Background(), notice)
		makeService().HandleAgentRuntimeUpdate(context.Background(), notice)
	}
	if len(local.messages) != 2 || len(muted.messages) != 0 {
		t.Fatalf("delivery counts local=%d muted=%d", len(local.messages), len(muted.messages))
	}
	message := local.messages[0]
	if message.Title != "gemini runtime update available" || message.Payload["agent_name"] != "gemini" || message.Payload["runtime_id"] != "npm:gemini" || message.Payload["previous_version"] != "1.0.0" || message.Payload["runtime_update_status"] != "available" || message.Payload["url"] != "/settings/agents#runtime-updates" {
		t.Fatalf("runtime identity lost: %+v", message)
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.6
func TestInterruptedRuntimeNoticeDoesNotGuessActivation(t *testing.T) {
	_, body := runtimeUpdateMessage(map[string]string{"display_name": "Gemini", "previous_version": "1.0.0", "version": "2.0.0", "runtime_update_status": "interrupted"})
	if !strings.Contains(body, "could not be confirmed") {
		t.Fatalf("interrupted result must be unknown: %s", body)
	}
}
