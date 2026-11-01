package backendapp

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/questions"
)

// asyncQuestionSessionReader resolves the exact mutable destination binding.
type asyncQuestionSessionReader interface {
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
	GetTask(ctx context.Context, id string) (*models.Task, error)
}

type asyncQuestionActiveSessionReader interface {
	asyncQuestionSessionReader
	GetActiveTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error)
}

// asyncQuestionQueueAdapter translates one immutable delivery intent into the
// existing exact-session queue admission. The stored admission id is the
// replay identity, so a crash after admission repeats the same queue row.
type asyncQuestionReadinessNotifier interface {
	CheckQueueAdmissionReadiness(ctx context.Context, identity messagequeue.QueueSessionIdentity)
}

type asyncQuestionQueueAdapter struct {
	queue     *messagequeue.Service
	sessions  asyncQuestionSessionReader
	readiness asyncQuestionReadinessNotifier
	log       *zap.Logger
}

const asyncQuestionQueueActor = "async-question"

func (a *asyncQuestionQueueAdapter) AdmitAsyncQuestionDelivery(ctx context.Context, delivery *models.AsyncQuestionDelivery) error {
	if delivery == nil {
		return errors.New("async question delivery is nil")
	}
	if a == nil || a.queue == nil {
		return questions.ErrQueueUnavailable
	}
	destination, err := a.resolveDeliveryDestination(ctx, delivery)
	if err != nil {
		return err
	}
	if err := a.validateDestination(ctx, destination); err != nil {
		return err
	}
	identity, err := a.queue.ResolveSessionIdentity(ctx, destination.DestinationTaskID, destination.DestinationSession)
	if err != nil {
		if errors.Is(err, messagequeue.ErrSessionIdentityMismatch) {
			if parentQuestionDelivery(delivery) {
				return questions.ErrDestinationUnavailable
			}
			return questions.ErrDestinationTerminal
		}
		return err
	}
	if destination.DestinationIncarnationID != "" && identity.SessionIncarnationID != destination.DestinationIncarnationID {
		if parentQuestionDelivery(delivery) {
			return questions.ErrDestinationUnavailable
		}
		return questions.ErrDestinationTerminal
	}
	metadata := make(map[string]interface{}, len(delivery.Metadata)+2)
	for key, value := range delivery.Metadata {
		metadata[key] = value
	}
	metadata[questions.MetadataAsyncQuestionNotification] = true
	_, _, err = a.queue.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, delivery.AdmissionID, destination.Content, "", asyncQuestionQueueActor,
		false, nil, metadata, nil,
	)
	if err != nil {
		return err
	}
	// Admission persists the row; this is the wake that lets an idle or parked
	// recipient actually run the accepted continuation. It re-checks the
	// captured incarnation and existing turn/pause barriers.
	if a.readiness != nil {
		a.readiness.CheckQueueAdmissionReadiness(ctx, identity)
	}
	return nil
}

func (a *asyncQuestionQueueAdapter) RemoveAsyncQuestionDelivery(ctx context.Context, delivery *models.AsyncQuestionDelivery) error {
	if delivery == nil {
		return nil
	}
	if a == nil || a.queue == nil {
		return questions.ErrQueueUnavailable
	}
	destination, err := a.resolveDeliveryDestination(ctx, delivery)
	if errors.Is(err, questions.ErrDestinationUnavailable) || errors.Is(err, questions.ErrDestinationTerminal) {
		return nil
	}
	if err != nil {
		return err
	}
	identity, err := a.queue.ResolveSessionIdentity(ctx, destination.DestinationTaskID, destination.DestinationSession)
	if err != nil {
		if errors.Is(err, messagequeue.ErrSessionIdentityMismatch) {
			return nil
		}
		return err
	}
	if destination.DestinationIncarnationID != "" && identity.SessionIncarnationID != destination.DestinationIncarnationID {
		return nil
	}
	return a.removeQueueAdmission(ctx, identity, destination)
}

func (a *asyncQuestionQueueAdapter) removeQueueAdmission(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
	delivery *models.AsyncQuestionDelivery,
) error {
	metadata := make(map[string]interface{}, len(delivery.Metadata)+2)
	for key, value := range delivery.Metadata {
		metadata[key] = value
	}
	metadata[questions.MetadataAsyncQuestionNotification] = true
	queued, found, err := a.queue.LookupQueueAdmissionWithClientQueueID(
		ctx, identity, delivery.AdmissionID, delivery.Content, "", asyncQuestionQueueActor, false, nil, metadata)
	if err != nil {
		return err
	}
	if !found || queued == nil {
		return nil
	}
	if _, err := a.queue.RemoveEntryForSession(ctx, identity, queued.ID); errors.Is(err, messagequeue.ErrEntryNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

func (a *asyncQuestionQueueAdapter) resolveDeliveryDestination(
	ctx context.Context,
	delivery *models.AsyncQuestionDelivery,
) (*models.AsyncQuestionDelivery, error) {
	if !parentQuestionDelivery(delivery) || a.sessions == nil {
		return delivery, nil
	}
	reader, ok := a.sessions.(asyncQuestionActiveSessionReader)
	if !ok {
		return nil, questions.ErrDestinationUnavailable
	}
	session, err := reader.GetActiveTaskSessionByTaskID(ctx, delivery.DestinationTaskID)
	if errors.Is(err, models.ErrTaskSessionNotFound) || (err == nil && session == nil) {
		return nil, questions.ErrDestinationUnavailable
	}
	if err != nil {
		return nil, err
	}
	if session.TaskID != delivery.DestinationTaskID || session.ID == "" || terminalAsyncQuestionSessionState(string(session.State)) {
		return nil, questions.ErrDestinationUnavailable
	}
	resolved := *delivery
	resolved.DestinationSession = session.ID
	resolved.DestinationIncarnationID = session.QueueIncarnationID
	return &resolved, nil
}

func parentQuestionDelivery(delivery *models.AsyncQuestionDelivery) bool {
	return delivery != nil && (delivery.Kind == models.AsyncQuestionDeliveryParentQuestion ||
		delivery.Kind == models.AsyncQuestionDeliveryParentOutcome)
}

func (a *asyncQuestionQueueAdapter) validateDestination(ctx context.Context, delivery *models.AsyncQuestionDelivery) error {
	if a.sessions == nil {
		return nil
	}
	session, err := a.sessions.GetTaskSession(ctx, delivery.DestinationSession)
	if err != nil {
		return err
	}
	if session == nil || session.TaskID != delivery.DestinationTaskID || terminalAsyncQuestionSessionState(string(session.State)) {
		if parentQuestionDelivery(delivery) {
			return questions.ErrDestinationUnavailable
		}
		return questions.ErrDestinationTerminal
	}
	task, err := a.sessions.GetTask(ctx, delivery.DestinationTaskID)
	if err != nil {
		return err
	}
	if task == nil || task.ArchivedAt != nil {
		return questions.ErrDestinationTerminal
	}
	return nil
}

func terminalAsyncQuestionSessionState(state string) bool {
	switch state {
	case "COMPLETED", "FAILED", "CANCELLED":
		return true
	default:
		return false
	}
}

// asyncQuestionNotifier publishes a bounded change notice. Clients invalidate
// their bounded read; a periodic refresh still recovers missed events.
type asyncQuestionNotifier struct {
	bus            bus.EventBus
	pendingActions asyncQuestionPendingActionPublisher
	log            *zap.Logger
}

type asyncQuestionPendingActionPublisher interface {
	AsyncQuestionWaitChanged(ctx context.Context, taskID, sessionID string) error
}

func (n asyncQuestionNotifier) AsyncQuestionChanged(ctx context.Context, change questions.QuestionChange) {
	if n.bus == nil {
		return
	}
	event := bus.NewEvent("task.question."+change.Kind, "async-questions", map[string]interface{}{
		"workspace_id":   change.WorkspaceID,
		"task_id":        change.TaskID,
		"parent_task_id": change.ParentTaskID,
		"session_id":     change.SessionID,
		"recipient":      string(change.Recipient),
		"question_id":    change.QuestionID,
		"kind":           change.Kind,
	})
	_ = n.bus.Publish(ctx, "task.question."+change.Kind, event)
	if change.Kind == "wait_changed" && n.pendingActions != nil {
		if err := n.pendingActions.AsyncQuestionWaitChanged(ctx, change.TaskID, change.SessionID); err != nil && n.log != nil {
			n.log.Warn("failed to publish async question pending action",
				zap.String("task_id", change.TaskID),
				zap.String("session_id", change.SessionID),
				zap.Error(err))
		}
	}
}

// asyncQuestionSessionState reads the mutable destination state through the
// task repository so a cancelled session cannot strand a parked gate.
type asyncQuestionSessionState struct {
	sessions asyncQuestionSessionReader
}

func (a asyncQuestionSessionState) AsyncQuestionSessionState(ctx context.Context, sessionID string) (string, bool, bool, error) {
	if a.sessions == nil {
		return "", false, false, nil
	}
	session, err := a.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return "", false, false, err
	}
	if session == nil {
		return "", false, false, nil
	}
	task, err := a.sessions.GetTask(ctx, session.TaskID)
	if err != nil {
		return "", false, false, err
	}
	return string(session.State), task != nil && task.ArchivedAt != nil, true, nil
}
