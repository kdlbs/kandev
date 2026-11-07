package service

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/notifications/models"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// FormatterRuntime exposes only content operations, never a provider sender.
type FormatterRuntime interface {
	NotificationFormatters(context.Context, models.ProviderType) ([]models.FormatterDescriptor, error)
	LeaseFormatterConfiguration(context.Context, string, *models.FormatterBinding) (*pluginsdk.NotificationConfigurationResponse, func(), error)
	FormatNotification(context.Context, *models.FormatterBinding, *pluginsdk.NotificationFormatRequest) (*pluginsdk.NotificationFormatResponse, error)
}

func (s *Service) SetFormatterRuntime(runtime FormatterRuntime) {
	s.formatterMu.Lock()
	defer s.formatterMu.Unlock()
	s.formatterRuntime = runtime
}

func (s *Service) runtimeFormatter() FormatterRuntime {
	s.formatterMu.RLock()
	defer s.formatterMu.RUnlock()
	return s.formatterRuntime
}

func (s *Service) captureFormatter(ctx context.Context, owner string, provider *models.Provider) *models.FormatterBinding {
	repository, ok := s.repo.(notificationstore.FormatterRepository)
	if !ok || s.runtimeFormatter() == nil {
		return nil
	}
	binding, err := repository.GetFormatterBinding(ctx, owner, provider.ID)
	if err != nil || binding == nil || binding.PluginID == "" {
		return nil
	}
	copy := *binding
	return &copy
}

func (s *Service) formatNotification(ctx context.Context, owner string, payload notificationPayload) notificationPayload {
	runtime := s.runtimeFormatter()
	if payload.formatter == nil || runtime == nil || ctx.Err() != nil {
		return payload
	}
	formatCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	request := s.notificationFormatContext(formatCtx, owner, payload)
	if proto.Size(request) > 128*1024 {
		return payload
	}
	result, err := runtime.FormatNotification(formatCtx, payload.formatter, request)
	if err != nil || formatCtx.Err() != nil || pluginsdk.ValidateNotificationFormat(result) != nil {
		s.logger.Warn("notification formatter fallback", zap.String("provider_id", payload.formatter.ProviderID), zap.String("reason", "unavailable_or_invalid_formatter"))
		return payload
	}
	if result.Custom {
		payload.Title, payload.Body = result.Title, result.Body
	}
	return payload
}

func (s *Service) notificationFormatContext(ctx context.Context, owner string, payload notificationPayload) *pluginsdk.NotificationFormatRequest {
	request := &pluginsdk.NotificationFormatRequest{
		ContractVersion: 1, ProviderId: payload.formatter.ProviderID, OwnerId: owner,
		FormatterKey: payload.formatter.FormatterKey, ConfigurationRevision: payload.formatter.ConfigurationRevision,
		EventType: payload.EventType, OccurrenceId: payload.OccurrenceID,
		DefaultTitle: payload.Title, DefaultBody: payload.Body,
	}
	switch payload.EventType {
	case EventOfficeInboxItem:
		request.Inbox = &pluginsdk.NotificationInboxContext{WorkspaceId: payload.Payload["workspace_id"], Type: payload.Payload["inbox_type"], Title: payload.Payload["inbox_title"]}
	case EventSystemUpdateAvailable:
		request.Update = notificationUpdateContext(payload)
	default:
		request.Task = s.notificationTaskContext(ctx, payload)
	}
	return request
}

type notificationSessionReader interface {
	GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error)
}

func (s *Service) notificationTaskContext(ctx context.Context, payload notificationPayload) *pluginsdk.NotificationTaskContext {
	result := &pluginsdk.NotificationTaskContext{}
	if s.taskRepo == nil || payload.TaskID == "" {
		return result
	}
	task, err := s.taskRepo.GetTask(ctx, payload.TaskID)
	if err != nil || task == nil || task.ID != payload.TaskID {
		return result
	}
	result.TaskId, result.Title = task.ID, task.Title
	if reader, ok := s.taskRepo.(notificationSessionReader); ok && payload.TaskSessionID != "" {
		session, err := reader.GetTaskSession(ctx, payload.TaskSessionID)
		if err == nil && session != nil && session.ID == payload.TaskSessionID && session.TaskID == task.ID {
			result.SessionId = session.ID
		}
	}
	return result
}

func notificationUpdateContext(payload notificationPayload) *pluginsdk.NotificationUpdateContext {
	if len(payload.RuntimeUpdates) > 1 {
		return &pluginsdk.NotificationUpdateContext{Kind: "agent_runtime_summary", Status: "available", Count: uint32(len(payload.RuntimeUpdates))}
	}
	v := payload.Payload
	result := &pluginsdk.NotificationUpdateContext{Kind: "kandev", Version: v["version"], ReleaseUrl: v["url"]}
	if v["agent_name"] != "" {
		result.Kind, result.Count = "agent_runtime", 1
		result.AgentId, result.AgentName, result.RuntimeId = v["agent_name"], v["display_name"], v["runtime_id"]
		result.PreviousVersion, result.Status, result.ReleaseUrl = v["previous_version"], v["runtime_update_status"], ""
	}
	return result
}

// NotificationProviderType authorizes the provider without disclosing its config.
func (s *Service) NotificationProviderType(ctx context.Context, owner, id string) (models.ProviderType, error) {
	provider, err := s.repo.GetProvider(ctx, owner, id)
	if err != nil {
		return "", err
	}
	return provider.Type, nil
}

func (s *Service) NotificationFormatterBinding(ctx context.Context, owner, id string) (*models.FormatterBinding, error) {
	repository, ok := s.repo.(notificationstore.FormatterRepository)
	if !ok {
		return nil, errors.New("notification formatter storage unavailable")
	}
	return repository.GetFormatterBinding(ctx, owner, id)
}

func (s *Service) FormatterSettings(ctx context.Context, owner, id string) (*models.FormatterBinding, []models.FormatterDescriptor, error) {
	kind, err := s.NotificationProviderType(ctx, owner, id)
	if err != nil {
		return nil, nil, err
	}
	repository, ok := s.repo.(notificationstore.FormatterRepository)
	if !ok {
		return nil, nil, errors.New("notification formatter storage unavailable")
	}
	binding, err := repository.GetFormatterBinding(ctx, owner, id)
	if err != nil {
		return nil, nil, err
	}
	choices := []models.FormatterDescriptor{}
	if runtime := s.runtimeFormatter(); runtime != nil {
		choices, err = runtime.NotificationFormatters(ctx, kind)
	}
	return binding, choices, err
}

func (s *Service) ActivateFormatter(ctx context.Context, owner, id, expected string, next *models.FormatterBinding) (*models.FormatterBinding, error) {
	release, err := s.BeginFormatterManagement(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	kind, err := s.NotificationProviderType(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	repository, ok := s.repo.(notificationstore.FormatterRepository)
	if !ok {
		return nil, errors.New("notification formatter storage unavailable")
	}
	if next != nil {
		candidate := *next
		candidate.ProviderID, candidate.ProviderType = id, kind
		runtime := s.runtimeFormatter()
		if runtime == nil {
			return nil, errors.New("notification formatter unavailable")
		}
		inspectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		receipt, finish, err := runtime.LeaseFormatterConfiguration(inspectCtx, owner, &candidate)
		if finish != nil {
			defer finish()
		}
		if err != nil || finish == nil || receipt == nil || receipt.Revision != candidate.ConfigurationRevision || receipt.Digest != candidate.Digest {
			return nil, notificationstore.ErrFormatterInvalid
		}
		next = &candidate
		ctx = inspectCtx
	}
	return repository.SetFormatterBinding(ctx, owner, id, expected, next)
}
