package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/models"
	"go.uber.org/zap"
)

type failRuntimeDeliveryOnce struct {
	*notificationTestRepository
	occurrenceID string
	failed       bool
}

func (r *failRuntimeDeliveryOnce) InsertDelivery(ctx context.Context, delivery *models.Delivery) (bool, error) {
	if delivery.OccurrenceID == r.occurrenceID && !r.failed {
		r.failed = true
		return false, errors.New("temporary claim failure")
	}
	return r.notificationTestRepository.InsertDelivery(ctx, delivery)
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-003.3, AC-AGENTS-RUNTIME-NOTIFY-003.5, AC-AGENTS-RUNTIME-NOTIFY-003.9
func TestRuntimeUpdateSummaryClaimsMembersAndRetriesFailedProvider(t *testing.T) {
	log, _ := logger.NewFromZap(zap.NewNop())
	repo := &notificationTestRepository{
		providers: []*models.Provider{
			{ID: "local", UserID: "user-1", Type: models.ProviderTypeLocal, Enabled: true},
			{ID: "external", UserID: "user-1", Type: models.ProviderTypeApprise, Enabled: true},
		},
		subscriptions: map[string][]*models.Subscription{
			"local":    {{ProviderID: "local", EventType: EventSystemUpdateAvailable, Enabled: true}},
			"external": {{ProviderID: "external", EventType: EventSystemUpdateAvailable, Enabled: true}},
		},
	}
	local := &captureProvider{}
	external := &failOnceProvider{}
	newService := func() *Service {
		svc := NewService(repo, nil, nil, log, nil)
		svc.providers[models.ProviderTypeLocal] = local
		svc.providers[models.ProviderTypeApprise] = external
		return svc
	}
	svc := newService()
	notices := []agents.RuntimeUpdateNotice{
		{AgentID: "gemini", RuntimeID: "npm:@google/gemini-cli", DisplayName: "Gemini", PreviousVersion: "1.0.0", Version: "2.0.0", Status: "available", OccurrenceID: "gemini-2", URL: "/settings/agents#runtime-update-gemini"},
		{AgentID: "codex-app-server", RuntimeID: "npm:@openai/codex", DisplayName: "Codex", PreviousVersion: "1.0.0", Version: "3.0.0", Status: "available", OccurrenceID: "codex-3", URL: "/settings/agents#runtime-update-codex-app-server"},
	}

	svc.HandleAgentRuntimeUpdates(context.Background(), notices)
	if len(local.messages) != 1 || len(local.messages[0].RuntimeUpdates) != 2 {
		t.Fatalf("local summary messages = %+v", local.messages)
	}
	first := local.messages[0]
	if first.Title != "2 agent runtime updates available" || first.Payload["notification_kind"] != "agent_runtime_summary" || first.Payload["url"] != "/settings/agents#runtime-updates" {
		t.Fatalf("summary presentation = %+v", first)
	}
	if first.RuntimeUpdates[0].AgentName != "codex-app-server" || first.RuntimeUpdates[1].AgentName != "gemini" {
		t.Fatalf("summary members are not stably ordered: %+v", first.RuntimeUpdates)
	}
	if external.attempts != 1 || len(external.messages) != 0 {
		t.Fatalf("first external delivery attempts=%d messages=%d", external.attempts, len(external.messages))
	}

	newService().HandleAgentRuntimeUpdates(context.Background(), []agents.RuntimeUpdateNotice{notices[1], notices[0]})
	if len(local.messages) != 1 {
		t.Fatalf("successful member claims replayed to local provider: %d", len(local.messages))
	}
	if external.attempts != 2 || len(external.messages) != 1 {
		t.Fatalf("external replay attempts=%d messages=%d", external.attempts, len(external.messages))
	}
	if external.messages[0].OccurrenceID != first.OccurrenceID || len(external.messages[0].RuntimeUpdates) != 2 {
		t.Fatalf("replayed summary identity or members changed: %+v", external.messages[0])
	}

	claimed := map[string]int{}
	for _, delivery := range repo.deliveries {
		claimed[delivery.ProviderID]++
	}
	if claimed["local"] != 2 || claimed["external"] != 2 {
		t.Fatalf("member delivery claims = %+v", claimed)
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-003.3, AC-AGENTS-RUNTIME-NOTIFY-003.5
func TestRuntimeUpdateSummaryKeepsRecipientClaimsIsolated(t *testing.T) {
	ctx := context.Background()
	log, _ := logger.NewFromZap(zap.NewNop())
	repo := newMultiUserRepository()
	for _, userID := range []string{"user-1", "user-2"} {
		provider := &models.Provider{UserID: userID, Type: models.ProviderTypeLocal, Enabled: true}
		if err := repo.CreateProvider(ctx, provider); err != nil {
			t.Fatal(err)
		}
		if err := repo.ReplaceSubscriptions(ctx, provider.ID, userID, []string{EventSystemUpdateAvailable}); err != nil {
			t.Fatal(err)
		}
	}
	local := &captureProvider{}
	svc := NewService(repo, nil, nil, log, nil)
	svc.providers[models.ProviderTypeLocal] = local
	svc.HandleAgentRuntimeUpdates(ctx, []agents.RuntimeUpdateNotice{
		{AgentID: "gemini", RuntimeID: "npm:@google/gemini-cli", DisplayName: "Gemini", PreviousVersion: "1.0.0", Version: "2.0.0", Status: "available", OccurrenceID: "gemini-2"},
		{AgentID: "codex-app-server", RuntimeID: "npm:@openai/codex", DisplayName: "Codex", PreviousVersion: "1.0.0", Version: "3.0.0", Status: "available", OccurrenceID: "codex-3"},
	})
	if len(local.messages) != 2 {
		t.Fatalf("recipient summaries = %d, want 2", len(local.messages))
	}
	users := map[string]bool{}
	for _, message := range local.messages {
		if len(message.RuntimeUpdates) != 2 {
			t.Fatalf("recipient got partial summary: %+v", message)
		}
		users[message.UserID] = true
	}
	if !users["user-1"] || !users["user-2"] || len(repo.deliveries) != 4 {
		t.Fatalf("recipient claims users=%v deliveries=%d", users, len(repo.deliveries))
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-003.4, AC-AGENTS-RUNTIME-NOTIFY-003.5
func TestRuntimeUpdateSummaryUsesNamedNoticeForOneClaimedMember(t *testing.T) {
	message := runtimeUpdateBatchMessage([]agents.RuntimeUpdateNotice{{
		AgentID: "gemini", RuntimeID: "npm:@google/gemini-cli", DisplayName: "Gemini",
		PreviousVersion: "1.0.0", Version: "2.0.0", Status: "available",
		OccurrenceID: "gemini-2", URL: "/settings/agents#runtime-update-gemini",
	}})
	if message.Title != "Gemini runtime update available" || message.Payload["url"] != "/settings/agents#runtime-update-gemini" || message.Payload["notification_kind"] != "" || len(message.RuntimeUpdates) != 0 {
		t.Fatalf("one-member notice became a group: %+v", message)
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-003.3, AC-AGENTS-RUNTIME-NOTIFY-003.5
func TestRuntimeUpdateSummaryOmitsFailedMemberClaimAndRetriesItAlone(t *testing.T) {
	log, _ := logger.NewFromZap(zap.NewNop())
	base := &notificationTestRepository{
		providers: []*models.Provider{{ID: "local", UserID: "user-1", Type: models.ProviderTypeLocal, Enabled: true}},
		subscriptions: map[string][]*models.Subscription{
			"local": {{ProviderID: "local", EventType: EventSystemUpdateAvailable, Enabled: true}},
		},
	}
	repo := &failRuntimeDeliveryOnce{notificationTestRepository: base, occurrenceID: "gemini-2"}
	local := &captureProvider{}
	svc := NewService(repo, nil, nil, log, nil)
	svc.providers[models.ProviderTypeLocal] = local
	notices := []agents.RuntimeUpdateNotice{
		{AgentID: "codex-app-server", RuntimeID: "npm:@openai/codex", DisplayName: "Codex", PreviousVersion: "1.0.0", Version: "3.0.0", Status: "available", OccurrenceID: "codex-3", URL: "/settings/agents#runtime-update-codex-app-server"},
		{AgentID: "gemini", RuntimeID: "npm:@google/gemini-cli", DisplayName: "Gemini", PreviousVersion: "1.0.0", Version: "2.0.0", Status: "available", OccurrenceID: "gemini-2", URL: "/settings/agents#runtime-update-gemini"},
	}
	svc.HandleAgentRuntimeUpdates(context.Background(), notices)
	if len(local.messages) != 1 || local.messages[0].Title != "Codex runtime update available" || len(local.messages[0].RuntimeUpdates) != 0 {
		t.Fatalf("failed claim was included in first message: %+v", local.messages)
	}
	svc.HandleAgentRuntimeUpdates(context.Background(), notices)
	if len(local.messages) != 2 || local.messages[1].Title != "Gemini runtime update available" || len(base.deliveries) != 2 {
		t.Fatalf("failed member was not replayed alone: messages=%+v claims=%+v", local.messages, base.deliveries)
	}
}
