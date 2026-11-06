package handlers

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

const (
	feedbackTaskID    = "feedback-task"
	feedbackSessionID = "feedback-session"
	feedbackPlanID    = "feedback-plan"
	feedbackCommentID = "feedback-comment"
	feedbackMessageID = "feedback-message"
)

type feedbackAdmissionContextKey struct{}

type feedbackAdmissionCoordinator struct {
	firstTurnCaptureOrchestrator
	queue                 *messagequeue.Service
	maxQueuedPrompts      int
	beforeAdmission       func(context.Context)
	initialBriefPending   bool
	notifications         int
	notifiedDuringGate    bool
	notifiedWithGateToken bool
}

func (o *feedbackAdmissionCoordinator) WithInitialTaskBriefAdmission(
	ctx context.Context,
	sessionID string,
	fn func(context.Context) error,
) error {
	return o.queue.WithPromptAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		if o.beforeAdmission != nil {
			o.beforeAdmission(admittedCtx)
		}
		return fn(context.WithValue(admittedCtx, feedbackAdmissionContextKey{}, true))
	})
}

func (o *feedbackAdmissionCoordinator) MarkInitialTaskBriefDispatchPending(string) {
	o.initialBriefPending = true
}

func (o *feedbackAdmissionCoordinator) InitialTaskBriefDispatchPending(string) bool {
	return o.initialBriefPending
}

func (o *feedbackAdmissionCoordinator) CompleteInitialTaskBriefDispatch(context.Context, string, string) {
	o.initialBriefPending = false
}

func (o *feedbackAdmissionCoordinator) NotifyQueuedUserPrompt(ctx context.Context, _, sessionID string) {
	o.notifications++
	o.notifiedDuringGate = o.notifiedDuringGate || o.queue.SessionAdmissionActive(sessionID)
	held, _ := ctx.Value(feedbackAdmissionContextKey{}).(bool)
	o.notifiedWithGateToken = o.notifiedWithGateToken || held
}

func (o *feedbackAdmissionCoordinator) MaxQueuedPromptsPerSession() int {
	return o.maxQueuedPrompts
}

type feedbackAdmissionFixture struct {
	handler     *MessageHandlers
	repo        *sqliterepo.Repository
	queueRepo   messagequeue.Repository
	coordinator *feedbackAdmissionCoordinator
}

func newFeedbackAdmissionFixture(t *testing.T, description string) feedbackAdmissionFixture {
	t.Helper()
	h, repo, _, _ := newCreationAttachmentHandler(t)
	database := sqlx.NewDb(repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(database, database)
	require.NoError(t, err)
	ctx := t.Context()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: feedbackTaskID, WorkspaceID: "ws-http-creation", WorkflowID: "wf-http-creation",
		Title: "Plan feedback", Description: description, State: "IN_PROGRESS",
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: feedbackSessionID, TaskID: feedbackTaskID, State: models.TaskSessionStateWaitingForInput,
	}))
	require.NoError(t, repo.SetSessionPrimary(ctx, feedbackSessionID))
	require.NoError(t, repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: feedbackPlanID, TaskID: feedbackTaskID, Content: "A plan",
	}))
	_, err = repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: feedbackCommentID, TaskID: feedbackTaskID, PlanID: feedbackPlanID,
		Body: "Apply this change", SelectedText: "A plan", AnchorFrom: 1, AnchorTo: 6,
	})
	require.NoError(t, err)
	coordinator := &feedbackAdmissionCoordinator{
		queue: messagequeue.NewService(queueRepo, 10, h.logger), maxQueuedPrompts: 10,
	}
	return feedbackAdmissionFixture{
		handler: NewMessageHandlers(h.service, coordinator, h.logger), repo: repo, queueRepo: queueRepo, coordinator: coordinator,
	}
}

func feedbackAdmissionRequest(t *testing.T, version int64) *ws.Message {
	t.Helper()
	request, err := ws.NewRequest("feedback-request", ws.ActionMessageAdd, map[string]interface{}{
		"task_id": feedbackTaskID, "session_id": feedbackSessionID,
		"client_message_id": feedbackMessageID, "content": "Apply the feedback", "plan_mode": true,
		"plan_comment_refs":       []models.TaskPlanCommentRef{{ID: feedbackCommentID, Version: version}},
		"require_primary_session": true,
	})
	require.NoError(t, err)
	return request
}

// @covers AC-TASKS-PLAN-COMMENTS-002.2, AC-TASKS-PLAN-COMMENTS-002.5
func TestWSAddMessagePlanCommentFeedbackAdmissionReleasesGateBeforeNotification(t *testing.T) {
	for _, description := range []string{"", "Implement the original task brief"} {
		t.Run(description, func(t *testing.T) {
			fixture := newFeedbackAdmissionFixture(t, description)
			response, err := fixture.handler.wsAddMessage(t.Context(), feedbackAdmissionRequest(t, 1))
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
			require.Equal(t, 1, fixture.coordinator.notifications)
			require.False(t, fixture.coordinator.notifiedDuringGate, "notification must not hold session admission")
			require.False(t, fixture.coordinator.notifiedWithGateToken, "notification must not reuse an admission callback context")

			message, err := fixture.repo.GetMessage(t.Context(), feedbackMessageID)
			require.NoError(t, err)
			require.Contains(t, message.Content, "Apply this change")
			require.Contains(t, message.Content, "Apply the feedback")
			if description != "" {
				require.Contains(t, message.Content, description)
			}
			status := fixture.coordinator.queue.GetStatus(t.Context(), feedbackSessionID)
			require.Equal(t, 1, status.Count)
			require.Equal(t, message.ID, status.Entries[0].ID)
			require.Equal(t, message.Content, status.Entries[0].Content)
			pending, err := fixture.repo.ListTaskPlanComments(t.Context(), feedbackTaskID)
			require.NoError(t, err)
			require.Empty(t, pending.Comments)
		})
	}
}

func TestWSAddMessagePreviewFeedbackAdmissionReleasesGateBeforeNotification(t *testing.T) {
	fixture := newFeedbackAdmissionFixture(t, "")
	item := handlerPreviewFeedbackSnapshot().Items[0]
	item.TaskID = feedbackTaskID
	_, err := fixture.repo.CreateTaskPreviewFeedback(t.Context(), item, "", "")
	require.NoError(t, err)
	request, err := ws.NewRequest("preview-request", ws.ActionMessageAdd, map[string]interface{}{
		"task_id": feedbackTaskID, "session_id": feedbackSessionID,
		"client_message_id": feedbackMessageID, "content": "Apply the preview feedback",
		"preview_feedback_refs":   []models.TaskPreviewFeedbackRef{{ID: item.ID, Version: 1}},
		"require_primary_session": true,
	})
	require.NoError(t, err)

	response, err := fixture.handler.wsAddMessage(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
	require.Equal(t, 1, fixture.coordinator.notifications)
	require.False(t, fixture.coordinator.notifiedDuringGate)
	require.False(t, fixture.coordinator.notifiedWithGateToken)
	message, err := fixture.repo.GetMessage(t.Context(), feedbackMessageID)
	require.NoError(t, err)
	require.Contains(t, message.Content, item.Comment)
	pending, err := fixture.repo.ListTaskPreviewFeedback(t.Context(), feedbackTaskID)
	require.NoError(t, err)
	require.Empty(t, pending.Items)
	comments, err := fixture.repo.ListTaskPlanComments(t.Context(), feedbackTaskID)
	require.NoError(t, err)
	require.Len(t, comments.Comments, 1)
}

// @covers AC-TASKS-PLAN-COMMENTS-002.6
func TestWSAddMessagePlanCommentFeedbackAdmissionPreservesRejectedFeedback(t *testing.T) {
	fixture := newFeedbackAdmissionFixture(t, "")
	response, err := fixture.handler.wsAddMessage(t.Context(), feedbackAdmissionRequest(t, 2))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type, string(response.Payload))
	require.Zero(t, fixture.coordinator.notifications)
	require.Zero(t, fixture.coordinator.queue.GetStatus(t.Context(), feedbackSessionID).Count)
	messages, err := fixture.repo.ListMessages(t.Context(), feedbackSessionID)
	require.NoError(t, err)
	require.Empty(t, messages)
	pending, err := fixture.repo.ListTaskPlanComments(t.Context(), feedbackTaskID)
	require.NoError(t, err)
	require.Len(t, pending.Comments, 1)
	require.Equal(t, feedbackCommentID, pending.Comments[0].ID)
}

func TestWSAddMessagePlanCommentFeedbackAdmissionPreservesFeedbackWhenQueueIsFull(t *testing.T) {
	fixture := newFeedbackAdmissionFixture(t, "")
	fixture.coordinator.maxQueuedPrompts = 1
	require.NoError(t, fixture.queueRepo.Insert(t.Context(), &messagequeue.QueuedMessage{
		ID: "existing-queued-prompt", TaskID: feedbackTaskID, SessionID: feedbackSessionID,
		Content: "Keep this queued instruction", QueuedBy: messagequeue.QueuedByUser,
	}, 1))

	response, err := fixture.handler.wsAddMessage(t.Context(), feedbackAdmissionRequest(t, 1))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type, string(response.Payload))
	var failure ws.ErrorPayload
	require.NoError(t, response.ParsePayload(&failure))
	require.Equal(t, messagequeue.QueueFullErrorCode, failure.Code)
	require.Zero(t, fixture.coordinator.notifications)
	messages, err := fixture.repo.ListMessages(t.Context(), feedbackSessionID)
	require.NoError(t, err)
	require.Empty(t, messages)
	pending, err := fixture.repo.ListTaskPlanComments(t.Context(), feedbackTaskID)
	require.NoError(t, err)
	require.Len(t, pending.Comments, 1)
	status := fixture.coordinator.queue.GetStatus(t.Context(), feedbackSessionID)
	require.Equal(t, 1, status.Count)
	require.Equal(t, "existing-queued-prompt", status.Entries[0].ID)
}

// @covers AC-TASKS-PLAN-COMMENTS-002.5, AC-TASKS-PLAN-COMMENTS-002.6
func TestWSAddMessagePlanCommentFeedbackAdmissionReplayWakesOnlyExistingReceipt(t *testing.T) {
	fixture := newFeedbackAdmissionFixture(t, "")
	request := feedbackAdmissionRequest(t, 1)
	first, err := fixture.handler.wsAddMessage(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, first.Type, string(first.Payload))
	_, err = fixture.repo.CreateTaskPlanComment(t.Context(), &models.TaskPlanComment{
		ID: "later-feedback", TaskID: feedbackTaskID, PlanID: feedbackPlanID,
		Body: "Keep this pending", SelectedText: "A plan", AnchorFrom: 1, AnchorTo: 6,
	})
	require.NoError(t, err)

	replayed, err := fixture.handler.wsAddMessage(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, replayed.Type, string(replayed.Payload))
	require.JSONEq(t, string(first.Payload), string(replayed.Payload))
	require.Equal(t, 2, fixture.coordinator.notifications)
	require.Equal(t, 1, fixture.coordinator.onTurnStartCount())
	require.False(t, fixture.coordinator.notifiedDuringGate)
	require.False(t, fixture.coordinator.notifiedWithGateToken)
	messages, err := fixture.repo.ListMessages(t.Context(), feedbackSessionID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, 1, fixture.coordinator.queue.GetStatus(t.Context(), feedbackSessionID).Count)
	pending, err := fixture.repo.ListTaskPlanComments(t.Context(), feedbackTaskID)
	require.NoError(t, err)
	require.Len(t, pending.Comments, 1)
	require.Equal(t, "later-feedback", pending.Comments[0].ID)
}

func TestWSAddMessagePlanCommentFeedbackAdmissionDefersInitialBriefContender(t *testing.T) {
	fixture := newFeedbackAdmissionFixture(t, "The original task brief")
	fixture.coordinator.initialBriefPending = true
	fixture.coordinator.beforeAdmission = func(ctx context.Context) {
		_, err := fixture.handler.service.CreateMessageWithID(ctx, "first-prompt", &service.CreateMessageRequest{
			TaskID: feedbackTaskID, TaskSessionID: feedbackSessionID,
			Content: "The winning first prompt", AuthorType: "user",
		})
		require.NoError(t, err)
	}

	response, err := fixture.handler.wsAddMessage(t.Context(), feedbackAdmissionRequest(t, 1))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
	require.Zero(t, fixture.coordinator.notifications)
	require.True(t, fixture.coordinator.initialBriefPending)
	message, err := fixture.repo.GetMessageWithPromptIndex(t.Context(), feedbackMessageID)
	require.NoError(t, err)
	require.Equal(t, 2, message.PromptIndex)
	require.NotContains(t, message.Content, "The original task brief")
	status := fixture.coordinator.queue.GetStatus(t.Context(), feedbackSessionID)
	require.Equal(t, 1, status.Count)
	require.Equal(t, feedbackMessageID, status.Entries[0].ID)
}

func TestWSAddMessagePlanCommentFeedbackAdmissionRetriesStaleBriefBeforeNotification(t *testing.T) {
	fixture := newFeedbackAdmissionFixture(t, "The old task brief")
	admissionAttempts := 0
	fixture.coordinator.beforeAdmission = func(ctx context.Context) {
		admissionAttempts++
		if admissionAttempts != 1 {
			return
		}
		task, err := fixture.repo.GetTask(ctx, feedbackTaskID)
		require.NoError(t, err)
		task.Description = "The current task brief"
		require.NoError(t, fixture.repo.UpdateTask(ctx, task))
	}

	response, err := fixture.handler.wsAddMessage(t.Context(), feedbackAdmissionRequest(t, 1))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
	require.Equal(t, 2, admissionAttempts)
	require.Equal(t, 1, fixture.coordinator.notifications)
	require.False(t, fixture.coordinator.notifiedDuringGate)
	require.False(t, fixture.coordinator.notifiedWithGateToken)
	messages, err := fixture.repo.ListMessages(t.Context(), feedbackSessionID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Contains(t, messages[0].Content, "The current task brief")
	require.NotContains(t, messages[0].Content, "The old task brief")
	require.Equal(t, 1, fixture.coordinator.queue.GetStatus(t.Context(), feedbackSessionID).Count)
}
