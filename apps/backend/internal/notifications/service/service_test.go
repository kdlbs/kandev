package service

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	gatewayws "github.com/kandev/kandev/internal/gateway/websocket"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/notifications/providers"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

type notificationTestRepository struct {
	mu            sync.Mutex
	providers     []*models.Provider
	subscriptions map[string][]*models.Subscription
	deliveries    []*models.Delivery
}

func (r *notificationTestRepository) CreateProvider(_ context.Context, provider *models.Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers = append(r.providers, provider)
	return nil
}
func (r *notificationTestRepository) UpdateProvider(context.Context, *models.Provider) error {
	return nil
}
func (r *notificationTestRepository) GetProvider(_ context.Context, id string) (*models.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, provider := range r.providers {
		if provider.ID == id {
			return provider, nil
		}
	}
	return nil, nil
}
func (r *notificationTestRepository) ListProvidersByUser(context.Context, string) ([]*models.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*models.Provider(nil), r.providers...), nil
}
func (r *notificationTestRepository) DeleteProvider(context.Context, string) error { return nil }
func (r *notificationTestRepository) ListSubscriptionsByProvider(_ context.Context, providerID string) ([]*models.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*models.Subscription(nil), r.subscriptions[providerID]...), nil
}
func (r *notificationTestRepository) ReplaceSubscriptions(_ context.Context, providerID, userID string, events []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscriptions[providerID] = make([]*models.Subscription, 0, len(events))
	for _, eventType := range events {
		r.subscriptions[providerID] = append(r.subscriptions[providerID], &models.Subscription{ProviderID: providerID, UserID: userID, EventType: eventType, Enabled: true})
	}
	return nil
}
func (r *notificationTestRepository) InsertDelivery(_ context.Context, delivery *models.Delivery) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.deliveries {
		if existing.ProviderID == delivery.ProviderID && existing.EventType == delivery.EventType && existing.OccurrenceID == delivery.OccurrenceID {
			return false, nil
		}
	}
	r.deliveries = append(r.deliveries, delivery)
	return true, nil
}
func (r *notificationTestRepository) DeleteDelivery(_ context.Context, providerID, eventType, occurrenceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index, delivery := range r.deliveries {
		if delivery.ProviderID == providerID && delivery.EventType == eventType && delivery.OccurrenceID == occurrenceID {
			r.deliveries = append(r.deliveries[:index], r.deliveries[index+1:]...)
			return nil
		}
	}
	return nil
}
func (r *notificationTestRepository) Close() error { return nil }

type captureProvider struct{ messages []providers.Message }

func (*captureProvider) Available() bool                       { return true }
func (*captureProvider) Validate(map[string]interface{}) error { return nil }
func (p *captureProvider) Send(_ context.Context, message providers.Message) error {
	p.messages = append(p.messages, message)
	return nil
}

type failOnceProvider struct {
	attempts int
	messages []providers.Message
}

func (*failOnceProvider) Available() bool                       { return true }
func (*failOnceProvider) Validate(map[string]interface{}) error { return nil }
func (p *failOnceProvider) Send(_ context.Context, message providers.Message) error {
	p.attempts++
	if p.attempts == 1 {
		return context.DeadlineExceeded
	}
	p.messages = append(p.messages, message)
	return nil
}

type notificationTestTaskGetter struct{ task *taskmodels.Task }

func (g notificationTestTaskGetter) GetTask(context.Context, string) (*taskmodels.Task, error) {
	return g.task, nil
}

func TestNewServiceSuppressesSystemProviderForDesktopOwnedLaunch(t *testing.T) {
	t.Setenv("KANDEV_DESKTOP_NATIVE_NOTIFICATIONS", "true")
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}

	svc := NewService(nil, nil, nil, log)

	if _, exists := svc.providers[models.ProviderTypeSystem]; exists {
		t.Fatal("system notification provider must be suppressed for a desktop-owned launch")
	}
	if _, exists := svc.providers[models.ProviderTypeLocal]; !exists {
		t.Fatal("local websocket notification provider must remain enabled")
	}
}

func TestNewServiceRetainsSystemProviderForNonDesktopLaunch(t *testing.T) {
	t.Setenv("KANDEV_DESKTOP_NATIVE_NOTIFICATIONS", "")
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}

	svc := NewService(nil, nil, nil, log)

	if _, exists := svc.providers[models.ProviderTypeSystem]; !exists {
		t.Fatal("system notification provider must remain enabled outside the desktop-owned launch")
	}
}

func TestSemanticOccurrencesUseEventSpecificCopyAndOccurrenceIdempotency(t *testing.T) {
	// Keep the auto-provisioned system provider out so the test only exercises
	// the single configured local provider; SystemProvider.Available() is true
	// on desktop platforms (e.g. macOS) and would add an extra delivery.
	t.Setenv(desktopNativeNotificationsEnv, "true")
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	repo := &notificationTestRepository{
		providers: []*models.Provider{{ID: "provider-1", Type: models.ProviderTypeLocal, Enabled: true}},
		subscriptions: map[string][]*models.Subscription{
			"provider-1": {
				{ProviderID: "provider-1", EventType: EventTaskSessionTurnFinished, Enabled: true},
				{ProviderID: "provider-1", EventType: EventTaskSessionClarificationAsked, Enabled: true},
			},
		},
	}
	service := NewService(repo, notificationTestTaskGetter{task: &taskmodels.Task{Title: "Fix delivery"}}, nil, log)
	capture := &captureProvider{}
	service.providers[models.ProviderTypeLocal] = capture

	service.HandleTaskTurnFinished(context.Background(), "task-1", "session-1", "turn-1")
	service.HandleTaskTurnFinished(context.Background(), "task-1", "session-1", "turn-1")
	service.HandleTaskTurnFinished(context.Background(), "task-1", "session-1", "turn-2")
	service.HandleClarificationRequested(context.Background(), "task-1", "session-1", "pending-1")

	if len(capture.messages) != 3 {
		t.Fatalf("sent %d notifications, want 3 semantic occurrences", len(capture.messages))
	}
	if got := capture.messages[0]; got.EventType != EventTaskSessionTurnFinished || got.Title != "Agent turn finished" || got.Body != "The agent finished a turn on \"Fix delivery\"." {
		t.Fatalf("turn notification = %#v", got)
	}
	if got := capture.messages[2]; got.EventType != EventTaskSessionClarificationAsked || got.Title != "Agent needs your answer" || got.Body != "The agent asked a question on \"Fix delivery\"." {
		t.Fatalf("clarification notification = %#v", got)
	}
	if len(repo.deliveries) != 3 || repo.deliveries[0].OccurrenceID != "turn-1" || repo.deliveries[2].OccurrenceID != "pending-1" {
		t.Fatalf("delivery occurrences = %#v", repo.deliveries)
	}
}

func TestUpdateAvailabilityIsAvailableEvent(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	service := NewService(&notificationTestRepository{}, nil, nil, log)
	if !containsEvent(service.AvailableEvents(), "system.update_available") {
		t.Fatalf("available events = %#v, want system.update_available", service.AvailableEvents())
	}
}

func TestFreshLocalProviderSubscribesToUpdateAvailability(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	repo := &notificationTestRepository{subscriptions: make(map[string][]*models.Subscription)}
	service := NewService(repo, nil, nil, log)
	if _, _, err := service.ListProviders(context.Background(), "user-1"); err != nil {
		t.Fatalf("list providers: %v", err)
	}
	for _, provider := range repo.providers {
		if provider.Type == models.ProviderTypeLocal && !containsEvent(subscriptionEventTypes(repo.subscriptions[provider.ID]), EventSystemUpdateAvailable) {
			t.Fatalf("local defaults = %#v, want update availability", repo.subscriptions[provider.ID])
		}
	}
}

func TestConcurrentProviderInitializationCreatesSingletonDefaultsAndClaimsOneUpdatePerProvider(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	repo := &notificationTestRepository{subscriptions: make(map[string][]*models.Subscription)}
	service := NewService(repo, nil, nil, log)
	local, system := &captureProvider{}, &captureProvider{}
	service.providers[models.ProviderTypeLocal] = local
	service.providers[models.ProviderTypeSystem] = system

	var calls sync.WaitGroup
	for range 32 {
		calls.Add(1)
		go func() {
			defer calls.Done()
			if _, _, err := service.ListProviders(context.Background(), "user-1"); err != nil {
				t.Errorf("list providers: %v", err)
			}
		}()
	}
	calls.Wait()

	localCount, systemCount := 0, 0
	for _, provider := range repo.providers {
		switch provider.Type {
		case models.ProviderTypeLocal:
			localCount++
		case models.ProviderTypeSystem:
			systemCount++
		}
	}
	if localCount != 1 || systemCount != 1 {
		t.Fatalf("default providers: local=%d system=%d, want one of each", localCount, systemCount)
	}

	service.HandleUpdateAvailable(context.Background(), "v1.2.3", "https://example.test/releases/v1.2.3")
	service.HandleUpdateAvailable(context.Background(), "v1.2.3", "https://example.test/releases/v1.2.3")
	if len(local.messages) != 1 || len(system.messages) != 1 || len(repo.deliveries) != 2 {
		t.Fatalf("update delivery: local=%d system=%d claims=%d, want one per default provider", len(local.messages), len(system.messages), len(repo.deliveries))
	}
}

func subscriptionEventTypes(subscriptions []*models.Subscription) []string {
	events := make([]string, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		events = append(events, subscription.EventType)
	}
	return events
}

func TestUpdateAvailabilityRoutesProviderScopedOccurrenceWithReleasePayload(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	repo := &notificationTestRepository{
		providers: []*models.Provider{
			{ID: "local", Type: models.ProviderTypeLocal, Enabled: true},
			{ID: "apprise", Type: models.ProviderTypeApprise, Enabled: true},
			{ID: "system", Type: models.ProviderTypeSystem, Enabled: true},
		},
		subscriptions: map[string][]*models.Subscription{
			"local":   {{ProviderID: "local", EventType: EventSystemUpdateAvailable, Enabled: true}},
			"apprise": {{ProviderID: "apprise", EventType: EventSystemUpdateAvailable, Enabled: true}},
			"system":  {{ProviderID: "system", EventType: EventSystemUpdateAvailable, Enabled: true}},
		},
	}
	service := NewService(repo, nil, nil, log)
	local, apprise, system := &captureProvider{}, &captureProvider{}, &captureProvider{}
	service.providers[models.ProviderTypeLocal] = local
	service.providers[models.ProviderTypeApprise] = apprise
	service.providers[models.ProviderTypeSystem] = system

	service.HandleUpdateAvailable(context.Background(), "v1.2.3", "https://example.test/releases/v1.2.3")
	service.HandleUpdateAvailable(context.Background(), "v1.2.3", "https://example.test/releases/v1.2.3")

	for providerType, captured := range map[string]*captureProvider{"local": local, "apprise": apprise, "system": system} {
		if len(captured.messages) != 1 {
			t.Fatalf("%s messages = %#v, want exactly one", providerType, captured.messages)
		}
		message := captured.messages[0]
		if message.EventType != EventSystemUpdateAvailable || message.OccurrenceID != "v1.2.3" || message.TaskSessionID != "" || message.Title != "Kandev update available" || message.Body != "Kandev v1.2.3 is available. Open Settings > System > Updates to review it." {
			t.Fatalf("%s update message = %#v", providerType, message)
		}
		if message.Payload["version"] != "v1.2.3" || message.Payload["url"] != "https://example.test/releases/v1.2.3" {
			t.Fatalf("%s payload = %#v", providerType, message.Payload)
		}
	}
}

func TestNoEligibleLocalUpdateSubscriberReleasesOccurrenceClaimForReplay(t *testing.T) {
	// Keep the auto-provisioned system provider out so the test only exercises
	// the single configured local provider; SystemProvider.Available() is true
	// on desktop platforms (e.g. macOS) and would add an extra delivery.
	t.Setenv(desktopNativeNotificationsEnv, "true")
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	repo := &notificationTestRepository{
		providers: []*models.Provider{{ID: "local", Type: models.ProviderTypeLocal, Enabled: true}},
		subscriptions: map[string][]*models.Subscription{
			"local": {{ProviderID: "local", EventType: EventSystemUpdateAvailable, Enabled: true}},
		},
	}
	service := NewService(repo, nil, gatewayws.NewHub(nil, log), log)

	service.HandleUpdateAvailable(context.Background(), "v1.2.3", "https://example.test/releases/v1.2.3")
	if len(repo.deliveries) != 0 {
		t.Fatalf("failed local delivery claims = %#v, want none", repo.deliveries)
	}

	capture := &captureProvider{}
	service.providers[models.ProviderTypeLocal] = capture
	service.HandleUpdateAvailable(context.Background(), "v1.2.3", "https://example.test/releases/v1.2.3")
	if len(capture.messages) != 1 || len(repo.deliveries) != 1 {
		t.Fatalf("replayed local delivery: messages=%#v claims=%#v, want one", capture.messages, repo.deliveries)
	}
}

func TestFailedSemanticDeliveryReleasesOnlyItsOccurrenceClaim(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	repo := &notificationTestRepository{
		providers: []*models.Provider{{ID: "provider-1", Type: models.ProviderTypeLocal, Enabled: true}},
		subscriptions: map[string][]*models.Subscription{
			"provider-1": {{ProviderID: "provider-1", EventType: EventTaskSessionTurnFinished, Enabled: true}},
		},
	}
	service := NewService(repo, nil, nil, log)
	provider := &failOnceProvider{}
	service.providers[models.ProviderTypeLocal] = provider

	service.HandleTaskTurnFinished(context.Background(), "task-1", "session-1", "turn-1")
	service.HandleTaskTurnFinished(context.Background(), "task-1", "session-1", "turn-2")
	service.HandleTaskTurnFinished(context.Background(), "task-1", "session-1", "turn-1")

	if provider.attempts != 3 {
		t.Fatalf("send attempts = %d, want failed occurrence replayed independently", provider.attempts)
	}
	if len(provider.messages) != 2 {
		t.Fatalf("successful messages = %#v", provider.messages)
	}
}

func TestProviderSendsClarificationAction(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	provider := &models.Provider{ID: "provider-1", Type: models.ProviderTypeLocal}
	service := NewService(&notificationTestRepository{providers: []*models.Provider{provider}}, nil, nil, log)
	capture := &captureProvider{}
	service.providers[models.ProviderTypeLocal] = capture

	if err := service.TestProvider(context.Background(), provider.ID); err != nil {
		t.Fatalf("test provider: %v", err)
	}
	if len(capture.messages) != 1 || capture.messages[0].EventType != EventTaskSessionClarificationAsked {
		t.Fatalf("test message = %#v, want clarification action", capture.messages)
	}
}
