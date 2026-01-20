package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/common/logger"
	gatewayws "github.com/kandev/kandev/internal/gateway/websocket"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/notifications/providers"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	"github.com/kandev/kandev/internal/task/repository"
	userstore "github.com/kandev/kandev/internal/user/store"
	"go.uber.org/zap"
)

const (
	EventTaskSessionWaitingForInput = "session.waiting_for_input"
)

var ErrProviderNotFound = errors.New("notification provider not found")

type Service struct {
	repo      notificationstore.Repository
	taskRepo  repository.Repository
	hub       *gatewayws.Hub
	logger    *logger.Logger
	providers map[models.ProviderType]providers.Provider
}

func NewService(repo notificationstore.Repository, taskRepo repository.Repository, hub *gatewayws.Hub, log *logger.Logger) *Service {
	providerMap := map[models.ProviderType]providers.Provider{
		models.ProviderTypeLocal:   providers.NewLocalProvider(hub),
		models.ProviderTypeApprise: providers.NewAppriseProvider(),
		models.ProviderTypeSystem:  providers.NewSystemProvider(),
	}
	return &Service{
		repo:      repo,
		taskRepo:  taskRepo,
		hub:       hub,
		logger:    log.WithFields(zap.String("component", "notifications-service")),
		providers: providerMap,
	}
}

func (s *Service) AppriseAvailable() bool {
	provider := s.providers[models.ProviderTypeApprise]
	if provider == nil {
		return false
	}
	return provider.Available()
}

func (s *Service) AvailableEvents() []string {
	return []string{EventTaskSessionWaitingForInput}
}

func (s *Service) ListProviders(ctx context.Context, userID string) ([]*models.Provider, map[string][]string, error) {
	if err := s.ensureDefaultProviders(ctx, userID); err != nil {
		return nil, nil, err
	}
	providers, err := s.repo.ListProvidersByUser(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	subscriptions := make(map[string][]string, len(providers))
	for _, provider := range providers {
		subs, err := s.repo.ListSubscriptionsByProvider(ctx, provider.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, sub := range subs {
			if sub.Enabled {
				subscriptions[provider.ID] = append(subscriptions[provider.ID], sub.EventType)
			}
		}
	}
	return providers, subscriptions, nil
}

func (s *Service) CreateProvider(ctx context.Context, userID, name string, providerType models.ProviderType, config map[string]interface{}, enabled bool, events []string) (*models.Provider, error) {
	if err := s.validateProvider(providerType, config); err != nil {
		return nil, err
	}
	if err := s.validateEvents(events); err != nil {
		return nil, err
	}
	provider := &models.Provider{
		UserID:  userID,
		Name:    name,
		Type:    providerType,
		Config:  config,
		Enabled: enabled,
	}
	if err := s.repo.CreateProvider(ctx, provider); err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceSubscriptions(ctx, provider.ID, userID, events); err != nil {
		return nil, err
	}
	return provider, nil
}

func (s *Service) UpdateProvider(ctx context.Context, providerID string, updates ProviderUpdate) (*models.Provider, error) {
	provider, err := s.repo.GetProvider(ctx, providerID)
	if err != nil {
		return nil, ErrProviderNotFound
	}
	if updates.Name != nil {
		provider.Name = strings.TrimSpace(*updates.Name)
	}
	if updates.Enabled != nil {
		provider.Enabled = *updates.Enabled
	}
	if updates.Config != nil {
		provider.Config = updates.Config
	}
	if updates.Type != nil {
		provider.Type = *updates.Type
	}
	if err := s.validateProvider(provider.Type, provider.Config); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateProvider(ctx, provider); err != nil {
		return nil, err
	}
	if updates.Events != nil {
		if err := s.validateEvents(*updates.Events); err != nil {
			return nil, err
		}
		if err := s.repo.ReplaceSubscriptions(ctx, provider.ID, provider.UserID, *updates.Events); err != nil {
			return nil, err
		}
	}
	return provider, nil
}

func (s *Service) DeleteProvider(ctx context.Context, providerID string) error {
	return s.repo.DeleteProvider(ctx, providerID)
}

type ProviderUpdate struct {
	Name    *string
	Enabled *bool
	Type    *models.ProviderType
	Config  map[string]interface{}
	Events  *[]string
}

func (s *Service) HandleTaskSessionStateChanged(ctx context.Context, taskID, sessionID, newState string) {
	if newState != "WAITING_FOR_INPUT" {
		return
	}
	userID := userstore.DefaultUserID
	providers, subscriptions, err := s.ListProviders(ctx, userID)
	if err != nil {
		s.logger.Error("failed to load notification providers", zap.Error(err))
		return
	}
	title, body := s.buildWaitingForInputMessage(ctx, taskID)
	for _, provider := range providers {
		if !provider.Enabled {
			continue
		}
		events := subscriptions[provider.ID]
		if !containsEvent(events, EventTaskSessionWaitingForInput) {
			continue
		}
		delivery := &models.Delivery{
			UserID:        userID,
			ProviderID:    provider.ID,
			EventType:     EventTaskSessionWaitingForInput,
			TaskSessionID: sessionID,
		}
		inserted, err := s.repo.InsertDelivery(ctx, delivery)
		if err != nil {
			s.logger.Error("failed to record notification delivery", zap.Error(err))
			continue
		}
		if !inserted {
			continue
		}
		if err := s.dispatchProvider(ctx, provider, waitingForInputPayload{
			TaskID:        taskID,
			TaskSessionID: sessionID,
			Title:         title,
			Body:          body,
		}); err != nil {
			s.logger.Warn("notification delivery failed", zap.String("provider_id", provider.ID), zap.Error(err))
			_ = s.repo.DeleteDelivery(ctx, provider.ID, EventTaskSessionWaitingForInput, sessionID)
		}
	}
}

type waitingForInputPayload struct {
	TaskID        string
	TaskSessionID string
	Title         string
	Body          string
}

func (s *Service) dispatchProvider(ctx context.Context, provider *models.Provider, payload waitingForInputPayload) error {
	adapter := s.providers[provider.Type]
	if adapter == nil {
		return fmt.Errorf("unknown provider type: %s", provider.Type)
	}
	return adapter.Send(ctx, providers.Message{
		EventType:     EventTaskSessionWaitingForInput,
		Title:         payload.Title,
		Body:          payload.Body,
		TaskID:        payload.TaskID,
		TaskSessionID: payload.TaskSessionID,
		UserID:        userstore.DefaultUserID,
		Config:        provider.Config,
	})
}

func (s *Service) buildWaitingForInputMessage(ctx context.Context, taskID string) (string, string) {
	title := "Task needs your input"
	body := "An agent is waiting for your input."
	if taskID == "" || s.taskRepo == nil {
		return title, body
	}
	task, err := s.taskRepo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return title, body
	}
	if task.Title != "" {
		body = fmt.Sprintf("An agent is waiting for your input on \"%s\".", task.Title)
	}
	return title, body
}

func (s *Service) ensureDefaultProviders(ctx context.Context, userID string) error {
	providers, err := s.repo.ListProvidersByUser(ctx, userID)
	if err != nil {
		return err
	}
	hasLocal := false
	hasSystem := false
	for _, provider := range providers {
		switch provider.Type {
		case models.ProviderTypeLocal:
			hasLocal = true
		case models.ProviderTypeSystem:
			hasSystem = true
		}
	}
	if !hasLocal {
		provider := &models.Provider{
			ID:      uuid.New().String(),
			UserID:  userID,
			Name:    "Desktop Notifications",
			Type:    models.ProviderTypeLocal,
			Config:  map[string]interface{}{},
			Enabled: true,
		}
		if err := s.repo.CreateProvider(ctx, provider); err != nil {
			return err
		}
		if err := s.repo.ReplaceSubscriptions(ctx, provider.ID, userID, []string{EventTaskSessionWaitingForInput}); err != nil {
			return err
		}
	}
	if !hasSystem {
		adapter := s.providers[models.ProviderTypeSystem]
		if adapter != nil && adapter.Available() {
			provider := &models.Provider{
				ID:      uuid.New().String(),
				UserID:  userID,
				Name:    "System Notifications",
				Type:    models.ProviderTypeSystem,
				Config: map[string]interface{}{
					"sound_enabled": false,
				},
				Enabled: true,
			}
			if err := s.repo.CreateProvider(ctx, provider); err != nil {
				return err
			}
			if err := s.repo.ReplaceSubscriptions(ctx, provider.ID, userID, []string{EventTaskSessionWaitingForInput}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) validateProvider(providerType models.ProviderType, config map[string]interface{}) error {
	adapter := s.providers[providerType]
	if adapter == nil {
		return fmt.Errorf("unsupported provider type: %s", providerType)
	}
	return adapter.Validate(config)
}

func (s *Service) validateEvents(events []string) error {
	allowed := map[string]struct{}{
		EventTaskSessionWaitingForInput: {},
	}
	for _, event := range events {
		if _, ok := allowed[event]; !ok {
			return fmt.Errorf("unsupported event type: %s", event)
		}
	}
	return nil
}

func containsEvent(events []string, target string) bool {
	for _, event := range events {
		if event == target {
			return true
		}
	}
	return false
}
