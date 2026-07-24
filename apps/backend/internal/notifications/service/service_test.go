package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/notifications/providers"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

type notificationTestRepository struct {
	providers     []*models.Provider
	subscriptions map[string][]*models.Subscription
	deliveries    []*models.Delivery
}

func (r *notificationTestRepository) CreateProvider(_ context.Context, provider *models.Provider) error {
	r.providers = append(r.providers, provider)
	return nil
}
func (r *notificationTestRepository) UpdateProvider(context.Context, *models.Provider) error {
	return nil
}
func (r *notificationTestRepository) GetProvider(_ context.Context, id string) (*models.Provider, error) {
	for _, provider := range r.providers {
		if provider.ID == id {
			return provider, nil
		}
	}
	return nil, nil
}
func (r *notificationTestRepository) ListProvidersByUser(context.Context, string) ([]*models.Provider, error) {
	return r.providers, nil
}
func (r *notificationTestRepository) DeleteProvider(context.Context, string) error { return nil }
func (r *notificationTestRepository) ListSubscriptionsByProvider(_ context.Context, providerID string) ([]*models.Subscription, error) {
	return r.subscriptions[providerID], nil
}
func (r *notificationTestRepository) ReplaceSubscriptions(context.Context, string, string, []string) error {
	return nil
}
func (r *notificationTestRepository) InsertDelivery(_ context.Context, delivery *models.Delivery) (bool, error) {
	for _, existing := range r.deliveries {
		if existing.ProviderID == delivery.ProviderID && existing.EventType == delivery.EventType && existing.OccurrenceID == delivery.OccurrenceID {
			return false, nil
		}
	}
	r.deliveries = append(r.deliveries, delivery)
	return true, nil
}
func (r *notificationTestRepository) DeleteDelivery(_ context.Context, providerID, eventType, occurrenceID string) error {
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
