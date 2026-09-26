package controller

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/dto"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/notifications/service"
	"go.uber.org/zap"
)

func TestCreateProviderDefaultsOmittedEventsToClarificationOnly(t *testing.T) {
	testCreateProviderEvents(t, decodeCreateProviderRequest(t, `{"type":"local"}`), []string{service.EventTaskSessionClarificationAsked})
}

func TestCreateProviderPreservesExplicitEmptyEvents(t *testing.T) {
	testCreateProviderEvents(t, decodeCreateProviderRequest(t, `{"type":"local","events":[]}`), []string{})
}

func TestCreateProviderPreservesExplicitEvents(t *testing.T) {
	testCreateProviderEvents(t, decodeCreateProviderRequest(t, `{"type":"local","events":["session.turn_finished"]}`), []string{service.EventTaskSessionTurnFinished})
}

func decodeCreateProviderRequest(t *testing.T, body string) dto.UpsertProviderRequest {
	t.Helper()
	var request dto.UpsertProviderRequest
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	return request
}

func testCreateProviderEvents(t *testing.T, request dto.UpsertProviderRequest, want []string) {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	repo := &controllerRepository{}
	controller := NewController(service.NewService(repo, nil, nil, log, nil))
	provider, err := controller.CreateProvider(context.Background(), request)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if got := repo.events[provider.ID]; !sameEvents(got, want) {
		t.Fatalf("stored events = %#v, want %#v", got, want)
	}
}

func sameEvents(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

type controllerRepository struct {
	providers map[string]*models.Provider
	events    map[string][]string
}

func (r *controllerRepository) CreateProvider(_ context.Context, provider *models.Provider) error {
	if r.providers == nil {
		r.providers = make(map[string]*models.Provider)
	}
	provider.ID = "provider-1"
	r.providers[provider.ID] = provider
	return nil
}

func (r *controllerRepository) UpdateProvider(context.Context, *models.Provider) error { return nil }
func (r *controllerRepository) GetProvider(_ context.Context, _, id string) (*models.Provider, error) {
	return r.providers[id], nil
}
func (r *controllerRepository) ListProvidersByUser(context.Context, string) ([]*models.Provider, error) {
	return nil, nil
}
func (r *controllerRepository) ListProviderUserIDs(context.Context) ([]string, error) {
	return nil, nil
}
func (r *controllerRepository) DeleteProvider(context.Context, string, string) error { return nil }
func (r *controllerRepository) ListSubscriptionsByProvider(context.Context, string) ([]*models.Subscription, error) {
	return nil, nil
}
func (r *controllerRepository) ReplaceSubscriptions(_ context.Context, providerID, _ string, events []string) error {
	if r.events == nil {
		r.events = make(map[string][]string)
	}
	r.events[providerID] = append([]string(nil), events...)
	return nil
}
func (r *controllerRepository) InsertDelivery(context.Context, *models.Delivery) (bool, error) {
	return false, nil
}
func (r *controllerRepository) DeleteDelivery(context.Context, string, string, string) error {
	return nil
}
func (r *controllerRepository) Close() error { return nil }
