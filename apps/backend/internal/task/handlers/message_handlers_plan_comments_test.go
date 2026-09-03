package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

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
	r.messagesMu.Unlock()
	return &models.TaskPlanCommentSnapshot{TaskID: message.TaskID, PlanID: "plan-handler", Revision: 4}, nil
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
