package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type planCommentQueueServiceStub struct {
	*messagequeue.Service
	request *messagequeue.PlanCommentQueueRequest
	result  *messagequeue.PlanCommentQueueResult
	err     error
}

func (s *planCommentQueueServiceStub) QueueMessageWithPlanComments(
	_ context.Context,
	req messagequeue.PlanCommentQueueRequest,
) (*messagequeue.PlanCommentQueueResult, error) {
	s.request = &req
	return s.result, s.err
}

type recordingPlanCommentQueueBus struct {
	subjects []string
	events   []*bus.Event
}

func (b *recordingPlanCommentQueueBus) Publish(_ context.Context, subject string, event *bus.Event) error {
	b.subjects = append(b.subjects, subject)
	b.events = append(b.events, event)
	return nil
}

func (*recordingPlanCommentQueueBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (*recordingPlanCommentQueueBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (*recordingPlanCommentQueueBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (*recordingPlanCommentQueueBus) Close()            {}
func (*recordingPlanCommentQueueBus) IsConnected() bool { return true }

func TestWsQueueMessageAdmitsPlanCommentsAndPublishesSnapshot(t *testing.T) {
	handlers, service, eventBus := newPlanCommentQueueHandlers(t)
	refs := []models.TaskPlanCommentRef{{ID: "comment", Version: 2}}
	snapshot := &models.TaskPlanCommentSnapshot{TaskID: "task", PlanID: "plan", Revision: 5}
	service.result = &messagequeue.PlanCommentQueueResult{
		Message: &messagequeue.QueuedMessage{
			ID: "client-queue", SessionID: "session", TaskID: "task",
			Content: "resolved comments", QueuedBy: messagequeue.QueuedByUser,
		},
		Snapshot: snapshot,
	}

	response, err := handlers.wsQueueMessage(context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
		"session_id":              "session",
		"task_id":                 "task",
		"client_queue_id":         "client-queue",
		"plan_comment_refs":       refs,
		"require_primary_session": true,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.NotNil(t, service.request)
	require.Equal(t, "client-queue", service.request.ClientQueueID)
	require.Equal(t, refs, service.request.PlanCommentRefs)
	require.True(t, service.request.RequirePrimarySession)
	require.Equal(t, messagequeue.QueuedByUser, service.request.UserID)
	require.Equal(t, []string{events.TaskPlanCommentsChanged, events.MessageQueueStatusChanged}, eventBus.subjects)
	require.Same(t, snapshot, eventBus.events[0].Data)
}

func TestWsQueueMessageValidatesPlanCommentAdmission(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]interface{}
	}{
		{
			name: "requires client queue id",
			payload: map[string]interface{}{
				"session_id": "session", "task_id": "task",
				"plan_comment_refs": []models.TaskPlanCommentRef{{ID: "comment", Version: 1}},
			},
		},
		{
			name: "rejects duplicate refs",
			payload: map[string]interface{}{
				"session_id": "session", "task_id": "task", "client_queue_id": "queue",
				"plan_comment_refs": []models.TaskPlanCommentRef{{ID: "comment", Version: 1}, {ID: "comment", Version: 1}},
			},
		},
		{
			name: "rejects invalid ref version",
			payload: map[string]interface{}{
				"session_id": "session", "task_id": "task", "client_queue_id": "queue",
				"plan_comment_refs": []models.TaskPlanCommentRef{{ID: "comment"}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handlers, service, eventBus := newPlanCommentQueueHandlers(t)
			response, err := handlers.wsQueueMessage(
				context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, test.payload),
			)
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeError, response.Type)
			require.Equal(t, ws.ErrorCodeValidation, parseError(t, response).Code)
			require.Nil(t, service.request)
			require.Empty(t, eventBus.subjects)
		})
	}
}

func TestWsQueueMessageMapsPlanCommentConflicts(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{
			name: "comments changed",
			err: &plancommenttx.CommentsChangedError{Snapshot: &models.TaskPlanCommentSnapshot{
				TaskID: "task", PlanID: "plan", Revision: 8,
			}},
			code: ws.ErrorCodePlanCommentsChanged,
		},
		{
			name: "primary changed",
			err: &plancommenttx.PrimarySessionChangedError{
				SessionID: "new-primary", State: models.TaskSessionStateWaitingForInput,
			},
			code: ws.ErrorCodePrimarySessionChanged,
		},
		{name: "queue id reused", err: messagequeue.ErrQueueIDConflict, code: ws.ErrorCodeValidation},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handlers, service, eventBus := newPlanCommentQueueHandlers(t)
			service.err = test.err
			response, err := handlers.wsQueueMessage(context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
				"session_id": "session", "task_id": "task", "client_queue_id": "queue",
				"plan_comment_refs": []models.TaskPlanCommentRef{{ID: "comment", Version: 1}},
			}))
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeError, response.Type)
			require.Equal(t, test.code, parseError(t, response).Code)
			require.Empty(t, eventBus.subjects)
		})
	}
}

func TestWsQueueMessageReleasesPreclaimedAttachmentsWhenCommentAdmissionFails(t *testing.T) {
	handlers, service, eventBus := newPlanCommentQueueHandlers(t)
	service.err = messagequeue.ErrQueueFull
	attachments := &recordingQueueAttachmentClaimer{}
	handlers.SetAttachmentClaimer(attachments)

	response, err := handlers.wsQueueMessage(context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
		"session_id": "session", "task_id": "task", "client_queue_id": "queue",
		"plan_comment_refs": []models.TaskPlanCommentRef{{ID: "comment", Version: 1}},
		"attachments": []messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment", MimeType: "text/plain", Name: "note.txt", SizeBytes: 4,
		}},
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Equal(t, []string{"attachment"}, attachments.claims)
	require.Equal(t, []string{"attachment"}, attachments.releases)
	require.Empty(t, eventBus.subjects)
}

func newPlanCommentQueueHandlers(
	t *testing.T,
) (*QueueHandlers, *planCommentQueueServiceStub, *recordingPlanCommentQueueBus) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	service := &planCommentQueueServiceStub{Service: messagequeue.NewServiceMemory(log)}
	eventBus := &recordingPlanCommentQueueBus{}
	handlers := NewQueueHandlers(service, eventBus, log, nil, allowQueueAccess{}, nil)
	return handlers, service, eventBus
}

var _ QueueService = (*planCommentQueueServiceStub)(nil)
var _ bus.EventBus = (*recordingPlanCommentQueueBus)(nil)
var _ QueueAttachmentReleaser = (*recordingQueueAttachmentClaimer)(nil)
