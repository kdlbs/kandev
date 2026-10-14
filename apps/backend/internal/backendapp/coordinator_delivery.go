package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

const (
	wakeMessagePageSize = 200
	wakeOrphanedKey     = "coordinator_wake_orphaned"
	wakeTurnIDKey       = "coordinator_wake_turn_id"
)

// deliveryTaskAPI is the part of the task service the delivery adapters use.
type deliveryTaskAPI interface {
	GetPrimarySession(ctx context.Context, taskID string) (*taskmodels.TaskSession, error)
	GetTaskSession(ctx context.Context, sessionID string) (*taskmodels.TaskSession, error)
	ListPendingInteractions(ctx context.Context, filter taskmodels.PendingInteractionFilter) ([]*taskmodels.Interaction, error)
	GetTurn(ctx context.Context, turnID string) (*taskmodels.Turn, error)
	ListMessagesForPlugin(ctx context.Context, filter taskmodels.PluginMessageFilter) ([]*taskmodels.Message, error)
	GetMessage(ctx context.Context, id string) (*taskmodels.Message, error)
	UpdateMessage(ctx context.Context, message *taskmodels.Message) error
}

// deliveryQueue reports whether a session has a queued message.
type deliveryQueue interface {
	HasPendingForSession(ctx context.Context, sessionID string) (bool, error)
}

// orphanTurnCompleter completes an orphan wake turn; satisfied by the
// orchestrator service.
type orphanTurnCompleter interface {
	CompleteUnattendedOrphanTurn(ctx context.Context, sessionID, turnID string) error
}

// coordinatorConversationReader adapts the task service and the message queue
// to coordinator.ConversationReader.
type coordinatorConversationReader struct {
	tasks deliveryTaskAPI
	queue deliveryQueue
}

var _ coordinator.ConversationReader = (*coordinatorConversationReader)(nil)

func (a *coordinatorConversationReader) PrimarySession(ctx context.Context, taskID string) (*coordinator.ConversationSession, error) {
	session, err := a.tasks.GetPrimarySession(ctx, taskID)
	if errors.Is(err, repoerrors.ErrNoPrimarySession) || errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &coordinator.ConversationSession{ID: session.ID, State: string(session.State)}, nil
}

func (a *coordinatorConversationReader) SessionState(ctx context.Context, sessionID string) (string, bool, error) {
	session, err := a.tasks.GetTaskSession(ctx, sessionID)
	if errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(session.State), true, nil
}

func (a *coordinatorConversationReader) ActionPending(ctx context.Context, sessionID string) (bool, error) {
	items, err := a.tasks.ListPendingInteractions(ctx, taskmodels.PendingInteractionFilter{SessionIDs: []string{sessionID}})
	if err != nil {
		return false, err
	}
	return len(items) > 0, nil
}

func (a *coordinatorConversationReader) Queued(ctx context.Context, sessionID string) (bool, error) {
	if a.queue == nil {
		return false, errors.New("message queue is not wired")
	}
	return a.queue.HasPendingForSession(ctx, sessionID)
}

func (a *coordinatorConversationReader) Turn(ctx context.Context, turnID string) (*coordinator.TurnInfo, error) {
	turn, err := a.tasks.GetTurn(ctx, turnID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &coordinator.TurnInfo{Completed: turn.CompletedAt != nil}, nil
}

// coordinatorWakeMessages adapts the message store and the orchestrator to
// coordinator.WakeMessageFinder.
type coordinatorWakeMessages struct {
	tasks deliveryTaskAPI
	orch  orphanTurnCompleter
}

var _ coordinator.WakeMessageFinder = (*coordinatorWakeMessages)(nil)

// FindWakeMessage pages the session's messages created at or after since,
// oldest first, and returns the first carrying the wake turn id.
func (a *coordinatorWakeMessages) FindWakeMessage(ctx context.Context, sessionID, wakeTurnID string, since time.Time) (*coordinator.WakeMessage, error) {
	if _, err := a.tasks.GetTaskSession(ctx, sessionID); errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	from := since.UTC()
	for offset := 0; ; offset += wakeMessagePageSize {
		page, err := a.tasks.ListMessagesForPlugin(ctx, taskmodels.PluginMessageFilter{
			SessionIDs: []string{sessionID}, Since: &from, Limit: wakeMessagePageSize, Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		for _, m := range page {
			if id, _ := m.Metadata[wakeTurnIDKey].(string); id == wakeTurnID && !m.CreatedAt.Before(from) {
				return &coordinator.WakeMessage{ID: m.ID, TurnID: m.TurnID}, nil
			}
		}
		if len(page) < wakeMessagePageSize {
			return nil, nil
		}
	}
}

func (a *coordinatorWakeMessages) MarkOrphan(ctx context.Context, sessionID, messageID string) error {
	msg, err := a.tasks.GetMessage(ctx, messageID)
	if err != nil {
		return err
	}
	if msg.TaskSessionID != sessionID {
		return errors.New("wake message belongs to another session")
	}
	metadata := make(map[string]interface{}, len(msg.Metadata)+1)
	for k, v := range msg.Metadata {
		metadata[k] = v
	}
	metadata[wakeOrphanedKey] = true
	msg.Metadata = metadata
	return a.tasks.UpdateMessage(ctx, msg)
}

func (a *coordinatorWakeMessages) CompleteOrphanTurn(ctx context.Context, sessionID, turnID string) error {
	if a.orch == nil {
		return errors.New("orchestrator is not wired")
	}
	return a.orch.CompleteUnattendedOrphanTurn(ctx, sessionID, turnID)
}

// registerCoordinatorDeliveryWorker starts the delivery worker and its event
// subscriptions synchronously, so the backstop hooks are in place before the
// backstop starts, and returns the startup hook that settles the rows a
// previous process left open.
func registerCoordinatorDeliveryWorker(_ *gin.Engine, eventBus bus.EventBus, svc *coordinator.Service, log *logger.Logger) func(context.Context, time.Time) {
	svc.StartDelivery(eventBus)
	return func(ctx context.Context, t0 time.Time) {
		if err := svc.RecoverUnattendedStartup(ctx, t0); err != nil {
			log.Warn("coordinator unattended turn startup recovery failed", zap.Error(err))
		}
	}
}

// wireCoordinatorDelivery gives the coordinator its conversation reader,
// wake-message finder and wake sender. A missing task service or orchestrator
// leaves both unset; the queue is optional and a missing one makes the queued check fail closed.
func wireCoordinatorDelivery(svc *coordinator.Service, taskSvc *taskservice.Service, orch *orchestrator.Service) {
	if taskSvc == nil || orch == nil {
		return
	}
	reader := &coordinatorConversationReader{tasks: taskSvc}
	if queue := orch.GetMessageQueue(); queue != nil {
		reader.queue = queue
	}
	svc.SetDeliveryDeps(reader, &coordinatorWakeMessages{tasks: taskSvc, orch: orch}, orch)
}
