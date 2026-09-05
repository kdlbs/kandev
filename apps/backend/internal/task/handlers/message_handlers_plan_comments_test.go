package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func (r *messageAddSwitchRepo) ValidateMessagePlanComments(
	_ context.Context,
	_, _, content string,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
) error {
	if r.preflightErr != nil {
		return r.preflightErr
	}
	if !requirePrimary || len(refs) != 1 || refs[0].ID != "comment-handler" || refs[0].Version != 3 ||
		!plancomments.ContainsReservedPlaceholder(content) {
		return errors.New("plan comment preflight was not forwarded")
	}
	return nil
}

func (r *messageAddSwitchRepo) CreateMessageWithPlanComments(
	_ context.Context,
	message *models.Message,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
) (*models.TaskPlanCommentSnapshot, error) {
	if !requirePrimary || len(refs) != 1 || refs[0].ID != "comment-handler" || refs[0].Version != 3 {
		return nil, errors.New("plan comment request was not forwarded")
	}
	content, err := plancomments.ResolvePlaceholder(message.Content, []*models.TaskPlanComment{{
		ID: "comment-handler", Body: "stored feedback", SelectedText: "selection",
	}})
	if err != nil {
		return nil, err
	}
	message.Content = content
	message.PromptIndex = 1
	r.messagesMu.Lock()
	r.messages = append(r.messages, message)
	r.idempotentMessage = message
	r.messagesMu.Unlock()
	return &models.TaskPlanCommentSnapshot{TaskID: message.TaskID, PlanID: "plan-handler", Revision: 4}, nil
}

func (r *messageAddSwitchRepo) CreateMessageWithPlanCommentsAndQueue(
	ctx context.Context,
	message *models.Message,
	queued *messagequeue.QueuedMessage,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
	_ int,
) (*models.TaskPlanCommentSnapshot, error) {
	snapshot, err := r.CreateMessageWithPlanComments(ctx, message, refs, requirePrimary)
	if err != nil {
		return nil, err
	}
	queued.Content = message.Content
	copy := *queued
	r.queuedMessage = &copy
	return snapshot, nil
}

func TestWSAddMessageAcceptsEmptyBodyWithPlanCommentsAndUsesResolvedContent(t *testing.T) {
	now := time.Now().UTC()
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{
			"t1": {ID: "t1", State: "IN_PROGRESS", UpdatedAt: now},
		},
		sessions: map[string]*models.TaskSession{
			"s1": {ID: "s1", TaskID: "t1", State: models.TaskSessionStateWaitingForInput, UpdatedAt: now},
		},
		primaryID: "s1",
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	h := NewMessageHandlers(svc, nil, log)
	req, err := ws.NewRequest("req-plan-comments", ws.ActionMessageAdd, map[string]interface{}{
		"task_id": "t1", "session_id": "s1", "content": "",
		"client_message_id":       "message-handler-plan-comments",
		"plan_comment_refs":       []map[string]interface{}{{"id": "comment-handler", "version": 3}},
		"require_primary_session": true,
	})
	require.NoError(t, err)

	response, err := h.wsAddMessage(t.Context(), req)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
	stored := repo.firstMessageContent()
	require.Contains(t, stored, "### Plan Comments")
	require.Contains(t, stored, "> stored feedback")
	require.False(t, strings.ContainsRune(stored, '\x00'))
}

func TestWSAddMessageRunRejectsPrimaryReplacedByOnTurnStart(t *testing.T) {
	now := time.Now().UTC()
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{
			"t1": {ID: "t1", State: "IN_PROGRESS", UpdatedAt: now},
		},
		sessions: map[string]*models.TaskSession{
			"s1": {ID: "s1", TaskID: "t1", State: models.TaskSessionStateWaitingForInput, UpdatedAt: now},
			"s2": {ID: "s2", TaskID: "t1", State: models.TaskSessionStateCreated, UpdatedAt: now},
		},
		primaryID: "s1",
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	orch := &switchingTurnStartOrchestrator{repo: repo, switchPrimary: true}
	handler := NewMessageHandlers(svc, orch, log)
	req, err := ws.NewRequest("req-run-primary", ws.ActionMessageAdd, map[string]interface{}{
		"task_id": "t1", "session_id": "s1", "client_message_id": "message-run-primary",
		"plan_comment_refs":       []map[string]interface{}{{"id": "comment-handler", "version": 3}},
		"require_primary_session": true,
	})
	require.NoError(t, err)

	response, err := handler.wsAddMessage(t.Context(), req)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type, string(response.Payload))
	var errorPayload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &errorPayload))
	require.Equal(t, ws.ErrorCodePrimarySessionChanged, errorPayload.Code)
	require.Equal(t, "s2", errorPayload.Details["primary_session_id"])
	require.Empty(t, repo.messages)
}

func TestWSAddMessageRejectsStaleCommentsBeforeTurnStartSideEffects(t *testing.T) {
	now := time.Now().UTC()
	snapshot := &models.TaskPlanCommentSnapshot{TaskID: "t1", PlanID: "plan", Revision: 4}
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{"t1": {ID: "t1", State: "REVIEW", UpdatedAt: now}},
		sessions: map[string]*models.TaskSession{
			"s1": {ID: "s1", TaskID: "t1", State: models.TaskSessionStateWaitingForInput, UpdatedAt: now},
		},
		primaryID:    "s1",
		preflightErr: &plancommenttx.CommentsChangedError{Snapshot: snapshot},
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo,
		TaskEnvironments: repo, Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	orch := &switchingTurnStartOrchestrator{repo: repo}
	handler := NewMessageHandlers(svc, orch, log)
	req, err := ws.NewRequest("req-stale", ws.ActionMessageAdd, map[string]interface{}{
		"task_id": "t1", "session_id": "s1", "client_message_id": "message-stale",
		"plan_comment_refs":       []map[string]interface{}{{"id": "comment-handler", "version": 3}},
		"require_primary_session": true,
	})
	require.NoError(t, err)

	response, err := handler.wsAddMessage(t.Context(), req)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	require.Equal(t, ws.ErrorCodePlanCommentsChanged, payload.Code)
	require.Zero(t, orch.turnStartCalls)
	require.Empty(t, repo.messages)
	require.Empty(t, repo.turns)
	require.Equal(t, "REVIEW", string(repo.tasks["t1"].State))
}

func TestValidateAddMessageRequestRejectsReservedPlanCommentMarker(t *testing.T) {
	errMessage := validateAddMessageRequest(wsAddMessageRequest{
		TaskID: "task", TaskSessionID: "session", ClientMessageID: "message",
		Content:         plancomments.WithPlaceholder("typed"),
		PlanCommentRefs: []models.TaskPlanCommentRef{{ID: "comment", Version: 1}},
	})
	require.Equal(t, "content contains a reserved plan comment marker", errMessage)
}

func TestWSAddMessageRejectsChangedPlanCommentReplayBeforeSessionHooks(t *testing.T) {
	now := time.Now().UTC()
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{
			"t1": {ID: "t1", State: "IN_PROGRESS", UpdatedAt: now},
		},
		sessions: map[string]*models.TaskSession{
			"s1": {ID: "s1", TaskID: "t1", State: models.TaskSessionStateWaitingForInput, UpdatedAt: now},
		},
		primaryID: "s1",
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	h := NewMessageHandlers(svc, nil, log)
	request := func(content string, requirePrimary bool) *ws.Message {
		message, requestErr := ws.NewRequest("req-replay", ws.ActionMessageAdd, map[string]interface{}{
			"task_id": "t1", "session_id": "s1", "content": content,
			"client_message_id":       "message-handler-replay",
			"plan_comment_refs":       []map[string]interface{}{{"id": "comment-handler", "version": 3}},
			"require_primary_session": requirePrimary,
		})
		require.NoError(t, requestErr)
		return message
	}

	first, err := h.wsAddMessage(t.Context(), request("original", true))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, first.Type, string(first.Payload))
	replayed, err := h.wsAddMessage(t.Context(), request("changed", false))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, replayed.Type, string(replayed.Payload))
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(replayed.Payload, &payload))
	require.Equal(t, ws.ErrorCodeValidation, payload.Code)
	require.Len(t, repo.messages, 1)
}
