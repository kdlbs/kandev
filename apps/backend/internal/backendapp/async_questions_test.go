package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/questions"
)

func TestAsyncQuestionNotificationSkipsUnforwardedParent(t *testing.T) {
	base := map[string]interface{}{
		"task_id": "child-task", "session_id": "child-session", "question_id": "question-1",
		"recipient": string(models.AsyncQuestionRecipientParent), "kind": "created",
	}
	_, _, _, eligible := asyncQuestionOperatorNotification(base)
	require.False(t, eligible)

	base["kind"] = "forwarded"
	taskID, sessionID, questionID, eligible := asyncQuestionOperatorNotification(base)
	require.True(t, eligible)
	require.Equal(t, "child-task", taskID)
	require.Equal(t, "child-session", sessionID)
	require.Equal(t, "question-1", questionID)
}

type asyncQuestionRoutingSessions struct {
	active *models.TaskSession
}

func (s *asyncQuestionRoutingSessions) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	if s.active != nil && s.active.ID == id {
		return s.active, nil
	}
	if id == "original-parent-session" {
		return &models.TaskSession{ID: id, TaskID: "parent-task", State: models.TaskSessionStateCompleted}, nil
	}
	return nil, nil
}

func (s *asyncQuestionRoutingSessions) GetTask(_ context.Context, id string) (*models.Task, error) {
	if id != "parent-task" {
		return nil, nil
	}
	return &models.Task{ID: id}, nil
}

func (s *asyncQuestionRoutingSessions) GetActiveTaskSessionByTaskID(_ context.Context, taskID string) (*models.TaskSession, error) {
	if taskID != "parent-task" {
		return nil, errors.New("unexpected task")
	}
	if s.active == nil {
		return nil, models.ErrTaskSessionNotFound
	}
	return s.active, nil
}

func newParentQuestionDelivery() *models.AsyncQuestionDelivery {
	return &models.AsyncQuestionDelivery{
		AdmissionID: "aq-stable-parent-admission", QuestionID: "question-parent-routing",
		Kind: models.AsyncQuestionDeliveryParentQuestion, DestinationTaskID: "parent-task",
		DestinationSession: "original-parent-session", DestinationIncarnationID: "inc-original-parent",
		Content: "child asks a question", Metadata: map[string]interface{}{"async_question_id": "question-parent-routing"},
	}
}

type recordingReadiness struct {
	calls int
}

func (r *recordingReadiness) CheckQueueAdmissionReadiness(context.Context, messagequeue.QueueSessionIdentity) {
	r.calls++
}

func TestAsyncQuestionQueueAdapterWakesAdmittedDestination(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	queue := messagequeue.NewServiceMemory(log)
	readiness := &recordingReadiness{}
	adapter := &asyncQuestionQueueAdapter{queue: queue, readiness: readiness}

	delivery := &models.AsyncQuestionDelivery{
		AdmissionID:        "aq-adapter-wake",
		QuestionID:         "question-1",
		Kind:               models.AsyncQuestionDeliveryOwner,
		DestinationTaskID:  "task-1",
		DestinationSession: "session-1",
		Content:            "correlated answer",
		Metadata:           map[string]interface{}{"async_question_id": "question-1"},
	}
	ctx := context.Background()
	require.NoError(t, adapter.AdmitAsyncQuestionDelivery(ctx, delivery))
	require.Equal(t, 1, readiness.calls, "admission must wake the destination")
	require.Equal(t, 1, queue.GetStatus(ctx, "session-1").Count)

	// Replaying the exact admission identity produces no second queue row but
	// still rechecks readiness.
	require.NoError(t, adapter.AdmitAsyncQuestionDelivery(ctx, delivery))
	require.Equal(t, 2, readiness.calls)
	require.Equal(t, 1, queue.GetStatus(ctx, "session-1").Count)
}

func TestAsyncQuestionQueueAdapterRemovesUndispatchedParentPrompt(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	queue := messagequeue.NewServiceMemory(log)
	adapter := &asyncQuestionQueueAdapter{queue: queue}
	delivery := &models.AsyncQuestionDelivery{
		AdmissionID:        "aq-parent-withdrawn",
		QuestionID:         "question-parent-withdrawn",
		Kind:               models.AsyncQuestionDeliveryParentQuestion,
		DestinationTaskID:  "task-parent",
		DestinationSession: "session-parent",
		Content:            "async child question",
		Metadata:           map[string]interface{}{"async_question_id": "question-parent-withdrawn"},
	}
	ctx := context.Background()
	require.NoError(t, adapter.AdmitAsyncQuestionDelivery(ctx, delivery))
	require.Equal(t, 1, queue.GetStatus(ctx, delivery.DestinationSession).Count)
	retirer, ok := interface{}(adapter).(questions.DeliveryQueueRetirer)
	require.True(t, ok, "the queue adapter must support exact parent prompt removal")
	require.NoError(t, retirer.RemoveAsyncQuestionDelivery(ctx, delivery))
	require.Equal(t, 0, queue.GetStatus(ctx, delivery.DestinationSession).Count)
}

func TestAsyncQuestionParentDeliveryTargetsActiveSession(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	ctx := context.Background()
	queue := messagequeue.NewServiceMemory(log)
	sessions := &asyncQuestionRoutingSessions{active: &models.TaskSession{
		ID: "replacement-parent-session", TaskID: "parent-task", State: models.TaskSessionStateRunning,
		QueueIncarnationID: "memory:replacement-parent-session",
	}}
	adapter := &asyncQuestionQueueAdapter{queue: queue, sessions: sessions}
	delivery := newParentQuestionDelivery()

	require.NoError(t, adapter.AdmitAsyncQuestionDelivery(ctx, delivery))
	require.Equal(t, 0, queue.GetStatus(ctx, delivery.DestinationSession).Count)
	require.Equal(t, 1, queue.GetStatus(ctx, sessions.active.ID).Count)
	require.NoError(t, adapter.AdmitAsyncQuestionDelivery(ctx, delivery))
	require.Equal(t, 1, queue.GetStatus(ctx, sessions.active.ID).Count, "retry must reuse the stored admission identity")
}

func TestAsyncQuestionParentDeliveryWaitsForSession(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	ctx := context.Background()
	queue := messagequeue.NewServiceMemory(log)
	sessions := &asyncQuestionRoutingSessions{}
	adapter := &asyncQuestionQueueAdapter{queue: queue, sessions: sessions}
	delivery := newParentQuestionDelivery()

	err = adapter.AdmitAsyncQuestionDelivery(ctx, delivery)
	require.Error(t, err)
	require.NotErrorIs(t, err, questions.ErrDestinationTerminal, "no active parent session must leave the outbox intent retryable")
	require.Equal(t, 0, queue.GetStatus(ctx, delivery.DestinationSession).Count)

	sessions.active = &models.TaskSession{
		ID: "replacement-parent-session", TaskID: "parent-task", State: models.TaskSessionStateRunning,
		QueueIncarnationID: "memory:replacement-parent-session",
	}
	require.NoError(t, adapter.AdmitAsyncQuestionDelivery(ctx, delivery))
	require.Equal(t, 1, queue.GetStatus(ctx, sessions.active.ID).Count)
}
