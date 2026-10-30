package backendapp

import (
	"context"
	"encoding/json"
	"strings"

	"go.uber.org/zap"

	commonconfig "github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	notificationservice "github.com/kandev/kandev/internal/notifications/service"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	"github.com/kandev/kandev/internal/notifications/webpush"
)

// sessionStateFailed is the wire value of a failed task session.
const sessionStateFailed = "FAILED"

// configureWebPush enables Web Push when the notification store supports it.
func configureWebPush(
	svc *notificationservice.Service,
	repo notificationstore.Repository,
	settings commonconfig.NotificationsConfig,
	log *logger.Logger,
	registerCleanup func(func() error),
) {
	pushRepo, ok := repo.(notificationstore.WebPushRepository)
	if !ok {
		log.Warn("notification store does not support web push; push notifications disabled")
		return
	}
	allowedHosts := strings.Split(settings.WebPushAllowedHosts, ",")
	pushSvc := webpush.NewService(
		pushRepo,
		webpush.NewHTTPSender(allowedHosts...),
		settings.WebPushSubject,
		log,
		allowedHosts...,
	)
	if registerCleanup != nil {
		registerCleanup(pushSvc.StartAsyncDelivery())
	}
	svc.SetWebPush(pushSvc)
}

// subscribeSessionAttentionNotifications routes the events that need a person
// (an agent blocked on a permission, a session that failed) to the
// notification service. Turn-finished and clarification events are wired
// separately in provideGateway.
func subscribeSessionAttentionNotifications(
	eventBus bus.EventBus,
	svc *notificationservice.Service,
	log *logger.Logger,
) {
	_, err := eventBus.Subscribe(events.BuildPermissionRequestWildcardSubject(), func(ctx context.Context, event *bus.Event) error {
		if taskID, sessionID, pendingID, ok := permissionNotificationOccurrence(event.Data); ok {
			svc.HandlePermissionRequested(ctx, taskID, sessionID, pendingID)
		}
		return nil
	})
	if err != nil {
		log.Error("Failed to subscribe to permission notifications", zap.Error(err))
	}
	_, err = eventBus.Subscribe(events.TaskSessionStateChanged, func(ctx context.Context, event *bus.Event) error {
		if taskID, sessionID, occurrenceID, ok := failedSessionOccurrence(event); ok {
			svc.HandleSessionFailed(ctx, taskID, sessionID, occurrenceID)
		}
		return nil
	})
	if err != nil {
		log.Error("Failed to subscribe to session failure notifications", zap.Error(err))
	}
}

// permissionNotificationOccurrence extracts a notifiable permission request.
// A request agentctl already answered (auto-approved) is an audit record, not a
// prompt for a person, so it never notifies.
func permissionNotificationOccurrence(data any) (taskID, sessionID, pendingID string, ok bool) {
	var payload struct {
		TaskID               string `json:"task_id"`
		SessionID            string `json:"session_id"`
		PendingID            string `json:"pending_id"`
		AutoApprovedOptionID string `json:"auto_approved_option_id"`
		AutoApprovalPending  bool   `json:"auto_approval_pending"`
	}
	if !decodeEventData(data, &payload) {
		return "", "", "", false
	}
	if payload.TaskID == "" || payload.SessionID == "" || payload.PendingID == "" ||
		payload.AutoApprovedOptionID != "" || payload.AutoApprovalPending {
		return "", "", "", false
	}
	return payload.TaskID, payload.SessionID, payload.PendingID, true
}

// failedSessionOccurrence extracts a transition into the failed state. The
// session's update timestamp keeps separate failures of one session distinct.
func failedSessionOccurrence(event *bus.Event) (taskID, sessionID, occurrenceID string, ok bool) {
	var payload struct {
		TaskID    string `json:"task_id"`
		SessionID string `json:"session_id"`
		OldState  string `json:"old_state"`
		NewState  string `json:"new_state"`
		UpdatedAt string `json:"updated_at"`
	}
	if !decodeEventData(event.Data, &payload) {
		return "", "", "", false
	}
	if payload.NewState != sessionStateFailed || payload.OldState == sessionStateFailed ||
		payload.TaskID == "" || payload.SessionID == "" {
		return "", "", "", false
	}
	suffix := payload.UpdatedAt
	if suffix == "" {
		suffix = event.ID
	}
	return payload.TaskID, payload.SessionID, payload.SessionID + ":failed:" + suffix, true
}

// decodeEventData normalizes bus payloads, which are typed structs on the
// in-process bus and generic maps once they cross NATS.
func decodeEventData(data any, out any) bool {
	raw, err := json.Marshal(data)
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}
