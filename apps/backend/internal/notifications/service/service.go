package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/common/logger"
	gatewayws "github.com/kandev/kandev/internal/gateway/websocket"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/notifications/providers"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	userstore "github.com/kandev/kandev/internal/user/store"
	"go.uber.org/zap"
)

const (
	EventTaskSessionTurnFinished       = "session.turn_finished"
	EventTaskSessionClarificationAsked = "session.clarification_requested"
	EventOfficeInboxItem               = "office.inbox_item"
	EventSystemUpdateAvailable         = "system.update_available"
	desktopNativeNotificationsEnv      = "KANDEV_DESKTOP_NATIVE_NOTIFICATIONS"
)

// ErrProviderNotFound is the store sentinel, re-exported so HTTP handlers can
// keep mapping it to 404 without importing the store package. A provider owned
// by another user surfaces as this error too, so a 404 never reveals that
// somebody else's provider exists.
var ErrProviderNotFound = notificationstore.ErrProviderNotFound

// TaskContextReader resolves the workspace a notified task belongs to and that
// workspace's owner. Notification recipients are derived from it: a task event
// belongs to whoever owns the workspace it happened in, never to a constant.
type TaskContextReader interface {
	GetTask(ctx context.Context, id string) (*taskmodels.Task, error)
	GetWorkspace(ctx context.Context, id string) (*taskmodels.Workspace, error)
}

// AuthEnforced reports whether authentication is currently enforced. It is a
// predicate rather than a snapshot because the mode flips at runtime when the
// setup wizard completes. A nil AuthEnforced means "not enforced": the
// single-user install that predates authentication.
type AuthEnforced func() bool

type Service struct {
	defaultProvidersMu sync.Mutex
	repo               notificationstore.Repository
	taskRepo           TaskContextReader
	hub                *gatewayws.Hub
	logger             *logger.Logger
	providers          map[models.ProviderType]providers.Provider
	authEnforced       AuthEnforced
}

// NewService builds the notification service. authEnforced decides what
// happens when a notification's owner cannot be resolved: with authentication
// enforced the notification is dropped, otherwise it falls back to the single
// pre-auth user. It is a constructor parameter rather than a setter so no call
// site can silently leave the fail-closed behavior unwired.
func NewService(repo notificationstore.Repository, taskRepo TaskContextReader, hub *gatewayws.Hub, log *logger.Logger, authEnforced AuthEnforced) *Service {
	providerMap := map[models.ProviderType]providers.Provider{
		models.ProviderTypeLocal:   providers.NewLocalProvider(hub),
		models.ProviderTypeApprise: providers.NewAppriseProvider(),
	}
	if os.Getenv(desktopNativeNotificationsEnv) != "true" {
		providerMap[models.ProviderTypeSystem] = providers.NewSystemProvider()
	}
	return &Service{
		repo:         repo,
		taskRepo:     taskRepo,
		hub:          hub,
		logger:       log.WithFields(zap.String("component", "notifications-service")),
		providers:    providerMap,
		authEnforced: authEnforced,
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
	return []string{EventTaskSessionTurnFinished, EventTaskSessionClarificationAsked, EventOfficeInboxItem, EventSystemUpdateAvailable}
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
	availableProviders := make([]*models.Provider, 0, len(providers))
	for _, provider := range providers {
		if s.providers[provider.Type] == nil {
			continue
		}
		availableProviders = append(availableProviders, provider)
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
	return availableProviders, subscriptions, nil
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

// UpdateProvider mutates a provider the caller owns. A provider ID belonging
// to another user resolves to ErrProviderNotFound before any write happens.
func (s *Service) UpdateProvider(ctx context.Context, userID, providerID string, updates ProviderUpdate) (*models.Provider, error) {
	provider, err := s.ownedProvider(ctx, userID, providerID)
	if err != nil {
		return nil, err
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

// DeleteProvider removes a provider the caller owns.
func (s *Service) DeleteProvider(ctx context.Context, userID, providerID string) error {
	return s.repo.DeleteProvider(ctx, userID, providerID)
}

// ownedProvider reads a provider scoped to userID, normalizing a store double
// that reports a miss as (nil, nil) into ErrProviderNotFound.
func (s *Service) ownedProvider(ctx context.Context, userID, providerID string) (*models.Provider, error) {
	provider, err := s.repo.GetProvider(ctx, userID, providerID)
	if err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, ErrProviderNotFound
	}
	return provider, nil
}

type ProviderUpdate struct {
	Name    *string
	Enabled *bool
	Type    *models.ProviderType
	Config  map[string]interface{}
	Events  *[]string
}

func (s *Service) HandleTaskTurnFinished(ctx context.Context, taskID, sessionID, turnID string) {
	s.handleSemanticOccurrence(ctx, taskID, sessionID, turnID, EventTaskSessionTurnFinished, nil)
}

func (s *Service) HandleClarificationRequested(ctx context.Context, taskID, sessionID, pendingID string) {
	s.handleSemanticOccurrence(ctx, taskID, sessionID, pendingID, EventTaskSessionClarificationAsked, nil)
}

func (s *Service) HandleUpdateAvailable(ctx context.Context, version, releaseURL string) {
	if version == "" {
		return
	}
	s.handleSemanticOccurrence(ctx, "", "", version, EventSystemUpdateAvailable, map[string]string{
		"version": version,
		"url":     releaseURL,
	})
}

func (s *Service) handleSemanticOccurrence(ctx context.Context, taskID, sessionID, occurrenceID, eventType string, payload map[string]string) {
	if occurrenceID == "" {
		return
	}
	recipients, ok := s.recipients(ctx, taskID, eventType)
	if !ok {
		return
	}
	title, body := s.buildSemanticMessage(ctx, taskID, eventType, payload)
	message := notificationPayload{
		TaskID:        taskID,
		TaskSessionID: sessionID,
		OccurrenceID:  occurrenceID,
		EventType:     eventType,
		Title:         title,
		Body:          body,
		Payload:       payload,
	}
	for _, userID := range recipients {
		s.deliverOccurrence(ctx, userID, message)
	}
}

// deliverOccurrence fans one occurrence out over a single recipient's own
// providers. Every provider it can reach belongs to userID, so a webhook only
// ever receives events from workspaces its owner can see.
func (s *Service) deliverOccurrence(ctx context.Context, userID string, message notificationPayload) {
	providers, subscriptions, err := s.ListProviders(ctx, userID)
	if err != nil {
		s.logger.Error("failed to load notification providers", zap.Error(err))
		return
	}
	for _, provider := range providers {
		if !provider.Enabled || !containsEvent(subscriptions[provider.ID], message.EventType) {
			continue
		}
		inserted, err := s.repo.InsertDelivery(ctx, &models.Delivery{
			UserID:        userID,
			ProviderID:    provider.ID,
			EventType:     message.EventType,
			TaskSessionID: message.TaskSessionID,
			OccurrenceID:  message.OccurrenceID,
		})
		if err != nil {
			s.logger.Error("failed to record notification delivery", zap.Error(err))
			continue
		}
		if !inserted {
			continue
		}
		if err := s.dispatchProvider(ctx, userID, provider, message); err != nil {
			s.logger.Warn("notification delivery failed", zap.String("provider_id", provider.ID), zap.Error(err))
			_ = s.repo.DeleteDelivery(ctx, provider.ID, message.EventType, message.OccurrenceID)
		}
	}
}

// recipients resolves who a notification belongs to, reporting false when it
// must not be delivered at all. A task event belongs to the single owner of
// the workspace it happened in; resolving it to a constant is what pushed one
// user's task titles into another user's webhook. An instance-wide event (a
// new release) has no owning workspace, so it fans out to every user that owns
// providers instead of landing on one of them.
func (s *Service) recipients(ctx context.Context, taskID, eventType string) ([]string, bool) {
	if eventType == EventSystemUpdateAvailable {
		return s.providerOwners(ctx)
	}
	owner, ok := s.workspaceOwnerForTask(ctx, taskID)
	if !ok {
		return nil, false
	}
	return []string{owner}, true
}

// workspaceOwnerForTask resolves the owner of the workspace a task lives in.
//
// A failed lookup is NOT the default user. Under enforced authentication that
// substitution would hand the task title and session state to the default
// administrator's webhook and WebSocket connections, which is the very
// cross-user leak this scoping exists to close, just behind a narrower trigger
// (a deleted task, a workspace-deletion race, a transient database error).
// Unresolvable ownership therefore drops the notification; see unresolvedOwner
// for the one case that still falls back.
func (s *Service) workspaceOwnerForTask(ctx context.Context, taskID string) (string, bool) {
	if taskID == "" || s.taskRepo == nil {
		return s.unresolvedOwner("notification names no resolvable task", zap.String("task_id", taskID))
	}
	task, err := s.taskRepo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return s.unresolvedOwner("failed to resolve the task a notification belongs to",
			zap.String("task_id", taskID), zap.Error(err))
	}
	return s.workspaceOwner(ctx, task.WorkspaceID)
}

func (s *Service) workspaceOwner(ctx context.Context, workspaceID string) (string, bool) {
	if workspaceID == "" || s.taskRepo == nil {
		return s.unresolvedOwner("notification names no resolvable workspace",
			zap.String("workspace_id", workspaceID))
	}
	workspace, err := s.taskRepo.GetWorkspace(ctx, workspaceID)
	if err != nil || workspace == nil {
		return s.unresolvedOwner("failed to resolve the workspace a notification belongs to",
			zap.String("workspace_id", workspaceID), zap.Error(err))
	}
	if workspace.OwnerID == "" {
		// A workspace that loaded successfully and is explicitly unowned is a
		// pre-auth row the setup wizard has not claimed yet. That wizard
		// promotes the default-user row into the admin account, so the row
		// already belongs to whoever will own it. This is a known owner, not
		// a failed lookup.
		return userstore.DefaultUserID, true
	}
	return workspace.OwnerID, true
}

// unresolvedOwner decides what an unresolvable owner means. With
// authentication enforced there is more than one account that could wrongly
// receive the notification, so it is dropped. Otherwise the instance has
// exactly one user and the pre-auth default keeps single-user behavior
// byte-identical.
func (s *Service) unresolvedOwner(reason string, fields ...zap.Field) (string, bool) {
	if s.authEnforced == nil || !s.authEnforced() {
		return userstore.DefaultUserID, true
	}
	s.logger.Warn(reason+"; dropping it rather than delivering to another account", fields...)
	return "", false
}

func (s *Service) providerOwners(ctx context.Context) ([]string, bool) {
	owners, err := s.repo.ListProviderUserIDs(ctx)
	if err != nil {
		s.logger.Error("failed to resolve notification recipients", zap.Error(err))
		owner, ok := s.unresolvedOwner("failed to list notification provider owners")
		if !ok {
			return nil, false
		}
		return []string{owner}, true
	}
	if len(owners) == 0 {
		return []string{userstore.DefaultUserID}, true
	}
	return owners, true
}

// HandleInboxItem sends notifications for a new office inbox item. The item's
// workspace names the recipient. An item carrying no workspace context has no
// resolvable owner, so under enforced authentication it is dropped rather than
// delivered to whichever account the fallback would pick.
func (s *Service) HandleInboxItem(ctx context.Context, workspaceID, itemType, title string) {
	userID, ok := s.workspaceOwner(ctx, workspaceID)
	if !ok {
		return
	}
	providers, subscriptions, err := s.ListProviders(ctx, userID)
	if err != nil {
		s.logger.Error("failed to load notification providers for inbox item", zap.Error(err))
		return
	}
	notifTitle := "New inbox item"
	body := title
	if itemType != "" {
		notifTitle = fmt.Sprintf("Inbox: %s", itemType)
	}
	for _, provider := range providers {
		if !provider.Enabled {
			continue
		}
		events := subscriptions[provider.ID]
		if !containsEvent(events, EventOfficeInboxItem) {
			continue
		}
		if err := s.dispatchGenericNotification(ctx, userID, provider, EventOfficeInboxItem, notifTitle, body); err != nil {
			s.logger.Warn("inbox item notification delivery failed",
				zap.String("provider_id", provider.ID), zap.Error(err))
		}
	}
}

func (s *Service) dispatchGenericNotification(ctx context.Context, userID string, provider *models.Provider, eventType, title, body string) error {
	adapter := s.providers[provider.Type]
	if adapter == nil {
		return fmt.Errorf("unknown provider type: %s", provider.Type)
	}
	return adapter.Send(ctx, providers.Message{
		EventType: eventType,
		Title:     title,
		Body:      body,
		UserID:    userID,
		Config:    provider.Config,
	})
}

type notificationPayload struct {
	TaskID        string
	TaskSessionID string
	OccurrenceID  string
	EventType     string
	Title         string
	Body          string
	Payload       map[string]string
}

// dispatchProvider hands the message to the adapter. Message.UserID is the
// resolved recipient rather than a constant: the local provider broadcasts it
// over that user's WebSocket connections.
func (s *Service) dispatchProvider(ctx context.Context, userID string, provider *models.Provider, payload notificationPayload) error {
	adapter := s.providers[provider.Type]
	if adapter == nil {
		return fmt.Errorf("unknown provider type: %s", provider.Type)
	}
	return adapter.Send(ctx, providers.Message{
		EventType:     payload.EventType,
		Title:         payload.Title,
		Body:          payload.Body,
		Payload:       payload.Payload,
		TaskID:        payload.TaskID,
		TaskSessionID: payload.TaskSessionID,
		OccurrenceID:  payload.OccurrenceID,
		UserID:        userID,
		Config:        provider.Config,
	})
}

func (s *Service) buildSemanticMessage(ctx context.Context, taskID, eventType string, payload map[string]string) (string, string) {
	if eventType == EventSystemUpdateAvailable {
		return semanticMessageCopy(eventType, payload["version"])
	}
	title, body := semanticMessageCopy(eventType, "")
	if taskID == "" || s.taskRepo == nil {
		return title, body
	}
	task, err := s.taskRepo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return title, body
	}
	if task.Title != "" {
		_, body = semanticMessageCopy(eventType, task.Title)
	}
	return title, body
}

func semanticMessageCopy(eventType, taskTitle string) (string, string) {
	if eventType == EventSystemUpdateAvailable {
		return "Kandev update available", fmt.Sprintf("Kandev %s is available. Open Settings > System > Updates to review it.", taskTitle)
	}
	if eventType == EventTaskSessionClarificationAsked {
		if taskTitle == "" {
			return "Agent needs your answer", "The agent asked a question."
		}
		return "Agent needs your answer", fmt.Sprintf("The agent asked a question on \"%s\".", taskTitle)
	}
	if taskTitle == "" {
		return "Agent turn finished", "The agent finished a turn."
	}
	return "Agent turn finished", fmt.Sprintf("The agent finished a turn on \"%s\".", taskTitle)
}

func (s *Service) ensureDefaultProviders(ctx context.Context, userID string) error {
	s.defaultProvidersMu.Lock()
	defer s.defaultProvidersMu.Unlock()

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
		if err := s.repo.ReplaceSubscriptions(ctx, provider.ID, userID, []string{
			EventTaskSessionClarificationAsked,
			EventOfficeInboxItem,
			EventSystemUpdateAvailable,
		}); err != nil {
			return err
		}
	}
	if !hasSystem {
		if err := s.ensureSystemProvider(ctx, userID); err != nil {
			return err
		}
	}
	return nil
}

// ensureSystemProvider creates the system notification provider if the adapter is available.
func (s *Service) ensureSystemProvider(ctx context.Context, userID string) error {
	adapter := s.providers[models.ProviderTypeSystem]
	if adapter == nil || !adapter.Available() {
		return nil
	}
	provider := &models.Provider{
		ID:     uuid.New().String(),
		UserID: userID,
		Name:   "System Notifications",
		Type:   models.ProviderTypeSystem,
		Config: map[string]interface{}{
			"sound_enabled": false,
		},
		Enabled: true,
	}
	if err := s.repo.CreateProvider(ctx, provider); err != nil {
		return err
	}
	return s.repo.ReplaceSubscriptions(ctx, provider.ID, userID, []string{
		EventTaskSessionClarificationAsked,
		EventOfficeInboxItem,
		EventSystemUpdateAvailable,
	})
}

// TestProvider fires a test notification through a provider the caller owns.
func (s *Service) TestProvider(ctx context.Context, userID, providerID string) error {
	provider, err := s.ownedProvider(ctx, userID, providerID)
	if err != nil {
		return err
	}
	adapter := s.providers[provider.Type]
	if adapter == nil {
		return fmt.Errorf("unknown provider type: %s", provider.Type)
	}
	return adapter.Send(ctx, providers.Message{
		EventType: EventTaskSessionClarificationAsked,
		Title:     "Test notification",
		Body:      "If you can read this, notifications are working.",
		// The local provider broadcasts to this user's connections, so an
		// unaddressed test notification never reaches the tab that asked
		// for it.
		UserID: userID,
		Config: provider.Config,
	})
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
		EventTaskSessionTurnFinished:       {},
		EventTaskSessionClarificationAsked: {},
		EventOfficeInboxItem:               {},
		EventSystemUpdateAvailable:         {},
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
