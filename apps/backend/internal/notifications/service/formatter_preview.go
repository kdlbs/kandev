package service

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/notifications/store"
	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/protobuf/proto"
)

type FormatterPreview struct {
	Title        string `json:"title"`
	Body         string `json:"body"`
	UsedFallback bool   `json:"used_fallback"`
}

const (
	formatterSampleMissingURL = "missing_url"
	formatterSampleNormal     = "normal"
)

// PreviewFormatter constructs closed synthetic context; it has no sender or
// task/transcript lookup. Opaque draft configuration belongs to the formatter.
func (s *Service) PreviewFormatter(ctx context.Context, owner, id string, binding models.FormatterBinding, event, sample string, draft []byte) (*FormatterPreview, error) {
	kind, err := s.NotificationProviderType(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	request, err := notificationPreviewSample(event, sample)
	if err != nil {
		return nil, err
	}
	binding.ProviderID, binding.ProviderType, binding.ConfigurationRevision = id, kind, "preview"
	request.ProviderId, request.OwnerId, request.FormatterKey = id, owner, binding.FormatterKey
	request.ConfigurationRevision, request.ContractVersion = "preview", 1
	request.Preview, request.PreviewConfiguration, request.PreviewSample = true, draft, sample
	if proto.Size(request) > 128*1024 {
		return nil, store.ErrFormatterInvalid
	}
	result := &FormatterPreview{Title: request.DefaultTitle, Body: request.DefaultBody, UsedFallback: true}
	runtime := s.runtimeFormatter()
	if runtime == nil {
		return nil, store.ErrFormatterInvalid
	}
	previewCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	formatted, err := runtime.FormatNotification(previewCtx, &binding, request)
	if err == nil && previewCtx.Err() == nil && pluginsdk.ValidateNotificationFormat(formatted) == nil && formatted.Custom {
		result.Title, result.Body, result.UsedFallback = formatted.Title, formatted.Body, false
	}
	return result, nil
}

func notificationPreviewSample(event, sample string) (*pluginsdk.NotificationFormatRequest, error) {
	request := &pluginsdk.NotificationFormatRequest{EventType: event}
	switch event {
	case EventTaskSessionTurnFinished, EventTaskSessionClarificationAsked, EventTaskSessionPermissionAsked, EventTaskSessionFailed:
		if sample != formatterSampleNormal && sample != "missing_title" && sample != "missing_session" && sample != formatterSampleMissingURL {
			return nil, store.ErrFormatterInvalid
		}
		title := "Example task"
		if sample == "missing_title" {
			title = ""
		}
		request.DefaultTitle, request.DefaultBody = semanticMessageCopy(event, title)
		request.Task = &pluginsdk.NotificationTaskContext{TaskId: "example-task", Title: title, SessionId: "example-session"}
		if sample == "missing_session" {
			request.Task.SessionId = ""
		}
	case EventOfficeInboxItem:
		if sample != formatterSampleNormal && sample != formatterSampleMissingURL {
			return nil, store.ErrFormatterInvalid
		}
		request.DefaultTitle, request.DefaultBody = "Inbox: approval", "Example inbox item"
		request.Inbox = &pluginsdk.NotificationInboxContext{WorkspaceId: "example-workspace", Type: "approval", Title: "Example inbox item"}
	case EventSystemUpdateAvailable:
		return notificationUpdatePreview(sample)
	default:
		return nil, store.ErrFormatterInvalid
	}
	return request, nil
}

func notificationUpdatePreview(sample string) (*pluginsdk.NotificationFormatRequest, error) {
	request := &pluginsdk.NotificationFormatRequest{EventType: EventSystemUpdateAvailable}
	request.DefaultTitle, request.DefaultBody = semanticMessageCopy(EventSystemUpdateAvailable, "1.2.3")
	request.Update = &pluginsdk.NotificationUpdateContext{Kind: "kandev", Version: "1.2.3", ReleaseUrl: "https://example.invalid/releases/1.2.3"}
	switch sample {
	case formatterSampleNormal, formatterSampleMissingURL:
		return request, nil
	case "runtime_summary":
		request.DefaultTitle, request.DefaultBody = "2 agent runtime updates available", "Review runtime versions in Settings > Agents."
		request.Update = &pluginsdk.NotificationUpdateContext{Kind: "agent_runtime_summary", Status: "available", Count: 2}
	case "runtime_available", "runtime_succeeded", "runtime_failed", "runtime_interrupted":
		status := sample[len("runtime_"):]
		request.DefaultTitle, request.DefaultBody = runtimeUpdateMessage(map[string]string{"display_name": "Example Agent", "previous_version": "1.0.0", "version": "1.2.3", "runtime_update_status": status})
		request.Update = &pluginsdk.NotificationUpdateContext{Kind: "agent_runtime", AgentName: "Example Agent", AgentId: "example-agent", RuntimeId: "example-runtime", PreviousVersion: "1.0.0", Version: "1.2.3", Status: status, Count: 1}
	default:
		return nil, store.ErrFormatterInvalid
	}
	return request, nil
}
