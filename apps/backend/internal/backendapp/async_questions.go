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
	if err := a.validateDestination(ctx, delivery); err != nil {
		return err
	}
	identity, err := a.queue.ResolveSessionIdentity(ctx, delivery.DestinationTaskID, delivery.DestinationSession)
	if err != nil {
		if errors.Is(err, messagequeue.ErrSessionIdentityMismatch) {
			return questions.ErrDestinationTerminal
		}
		return err
	}
	if delivery.DestinationIncarnationID != "" && identity.SessionIncarnationID != delivery.DestinationIncarnationID {
		return questions.ErrDestinationTerminal
	}
	metadata := make(map[string]interface{}, len(delivery.Metadata)+2)
	for key, value := range delivery.Metadata {
		metadata[key] = value
	}
	metadata[questions.MetadataAsyncQuestionNotification] = true
	_, _, err = a.queue.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, delivery.AdmissionID, delivery.Content, "", asyncQuestionQueueActor,
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

func (a *asyncQuestionQueueAdapter) validateDestination(ctx context.Context, delivery *models.AsyncQuestionDelivery) error {
	if a.sessions == nil {
		return nil
	}
	session, err := a.sessions.GetTaskSession(ctx, delivery.DestinationSession)
	if err != nil {
		return err
	}
	if session == nil || session.TaskID != delivery.DestinationTaskID || terminalAsyncQuestionSessionState(string(session.State)) {
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
	bus bus.EventBus
}

func (n asyncQuestionNotifier) AsyncQuestionChanged(ctx context.Context, workspaceID, kind, questionID string) {
	if n.bus == nil {
		return
	}
	event := bus.NewEvent("task.question."+kind, "async-questions", map[string]interface{}{
		"workspace_id": workspaceID,
		"question_id":  questionID,
		"kind":         kind,
	})
	_ = n.bus.Publish(ctx, "task.question."+kind, event)
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
