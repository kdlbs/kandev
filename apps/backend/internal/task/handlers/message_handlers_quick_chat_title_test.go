package handlers

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestWSAddMessagePendingQuickChatOwnerReceivesTitleContext(t *testing.T) {
	tests := []struct {
		name             string
		passthrough      bool
		config           bool
		owner            string
		wantTitleTool    bool
		wantStructured   bool
		wantPassthrough  bool
		wantPlainMessage bool
	}{
		{
			name:           "structured owner",
			owner:          "s1",
			wantTitleTool:  true,
			wantStructured: true,
		},
		{
			name:            "passthrough owner",
			passthrough:     true,
			owner:           "s1",
			wantTitleTool:   true,
			wantPassthrough: true,
		},
		{
			name:             "different session",
			owner:            "s2",
			wantPlainMessage: true,
		},
		{
			name:             "configuration session",
			config:           true,
			owner:            "s1",
			wantPlainMessage: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &messageAddSwitchRepo{
				tasks: map[string]*models.Task{
					"task-quick": {
						ID:          "task-quick",
						WorkspaceID: "ws-quick",
						State:       v1.TaskStateInProgress,
						IsEphemeral: true,
						Metadata: map[string]interface{}{
							models.MetaKeyAgentTitlePending:        true,
							models.MetaKeyAgentTitleOwnerSessionID: tt.owner,
						},
					},
				},
				sessions: map[string]*models.TaskSession{
					"s1": {
						ID:            "s1",
						TaskID:        "task-quick",
						State:         models.TaskSessionStateWaitingForInput,
						IsPassthrough: tt.passthrough,
						Metadata:      map[string]interface{}{},
						UpdatedAt:     time.Now().UTC(),
					},
				},
				primaryID: "s1",
			}
			if tt.config {
				repo.sessions["s1"].Metadata["config_mode"] = true
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
			req, err := ws.NewRequest("quick-title-message", ws.ActionMessageAdd, map[string]interface{}{
				"task_id": "task-quick", "session_id": "s1", "content": "Organize this chat",
			})
			require.NoError(t, err)

			resp, err := h.wsAddMessage(context.Background(), req)
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, resp.Type)
			require.Len(t, repo.messages, 1)
			content := repo.messages[0].Content

			switch {
			case tt.wantStructured:
				require.Contains(t, content, "Kandev Task ID: task-quick")
				require.Contains(t, content, "Session ID: s1")
				require.Contains(t, content, "set_task_title_kandev")
			case tt.wantPassthrough:
				require.Contains(t, content, "Before doing any other work, call set_task_title_kandev")
				require.NotContains(t, content, "<kandev-system>")
			case tt.wantPlainMessage:
				require.Equal(t, "Organize this chat", content)
			}

			if !tt.wantTitleTool {
				require.NotContains(t, content, "set_task_title_kandev")
			}
		})
	}
}

func TestWSAddMessagePendingQuickChatOwnerGetsTitleContextAgainWhilePending(t *testing.T) {
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{"task-quick": {
			ID:          "task-quick",
			WorkspaceID: "ws-quick",
			State:       v1.TaskStateInProgress,
			IsEphemeral: true,
			Metadata: map[string]interface{}{
				models.MetaKeyAgentTitlePending:        true,
				models.MetaKeyAgentTitleOwnerSessionID: "s1",
			},
		}},
		sessions: map[string]*models.TaskSession{"s1": {
			ID:        "s1",
			TaskID:    "task-quick",
			State:     models.TaskSessionStateWaitingForInput,
			UpdatedAt: time.Now().UTC(),
		}},
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

	for index, prompt := range []string{"First request", "Try again"} {
		req, err := ws.NewRequest(
			"quick-title-retry-"+string(rune('a'+index)),
			ws.ActionMessageAdd,
			map[string]interface{}{"task_id": "task-quick", "session_id": "s1", "content": prompt},
		)
		require.NoError(t, err)
		resp, err := h.wsAddMessage(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, resp.Type)
	}

	require.Len(t, repo.messages, 2)
	for _, message := range repo.messages {
		require.Equal(t, 1, strings.Count(message.Content, "set_task_title_kandev"))
	}
	require.Equal(t, 2, repo.taskGetCalls, "the task fetched for state transition is reused for title injection")
}

func TestWSAddMessagePendingQuickChatPassthroughCreatedSessionKeepsFirstPromptVerbatim(t *testing.T) {
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{"task-quick": {
			ID:          "task-quick",
			WorkspaceID: "ws-quick",
			State:       v1.TaskStateInProgress,
			IsEphemeral: true,
			Metadata: map[string]interface{}{
				models.MetaKeyAgentTitlePending:        true,
				models.MetaKeyAgentTitleOwnerSessionID: "s1",
			},
		}},
		sessions: map[string]*models.TaskSession{"s1": {
			ID:             "s1",
			TaskID:         "task-quick",
			State:          models.TaskSessionStateCreated,
			IsPassthrough:  true,
			AgentProfileID: "profile-1",
			UpdatedAt:      time.Now().UTC(),
		}},
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

	firstReq, err := ws.NewRequest("passthrough-created", ws.ActionMessageAdd, map[string]interface{}{
		"task_id": "task-quick", "session_id": "s1", "content": "hello from passthrough",
	})
	require.NoError(t, err)
	_, err = h.wsAddMessage(context.Background(), firstReq)
	require.NoError(t, err)
	require.Len(t, repo.messages, 1)
	require.Equal(t, "hello from passthrough", repo.messages[0].Content)

	// Once the eager session is waiting, the title owner receives the
	// instruction on a later prompt while the first prompt remains verbatim.
	repo.sessions["s1"].State = models.TaskSessionStateWaitingForInput
	secondReq, err := ws.NewRequest("passthrough-follow-up", ws.ActionMessageAdd, map[string]interface{}{
		"task_id": "task-quick", "session_id": "s1", "content": "follow up",
	})
	require.NoError(t, err)
	_, err = h.wsAddMessage(context.Background(), secondReq)
	require.NoError(t, err)
	require.Len(t, repo.messages, 2)
	require.Contains(t, repo.messages[1].Content, "Before doing any other work, call set_task_title_kandev")
}

func TestWSAddMessagePendingQuickChatDoesNotAssignTitleOwner(t *testing.T) {
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{"task-quick": {
			ID:          "task-quick",
			WorkspaceID: "ws-quick",
			State:       v1.TaskStateInProgress,
			IsEphemeral: true,
			Metadata: map[string]interface{}{
				models.MetaKeyAgentTitlePending: true,
			},
		}},
		sessions: map[string]*models.TaskSession{"s1": {
			ID:        "s1",
			TaskID:    "task-quick",
			State:     models.TaskSessionStateWaitingForInput,
			UpdatedAt: time.Now().UTC(),
		}},
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
	req, err := ws.NewRequest("quick-title-no-owner", ws.ActionMessageAdd, map[string]interface{}{
		"task_id": "task-quick", "session_id": "s1", "content": "No owner yet",
	})
	require.NoError(t, err)

	resp, err := h.wsAddMessage(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	require.Len(t, repo.messages, 1)
	require.Equal(t, "No owner yet", repo.messages[0].Content)
}

func TestWSAddMessagePendingNonEphemeralOwnerIsNotRewrapped(t *testing.T) {
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{"task-normal": {
			ID:          "task-normal",
			WorkspaceID: "ws-normal",
			State:       v1.TaskStateInProgress,
			IsEphemeral: false,
			Metadata: map[string]interface{}{
				models.MetaKeyAgentTitlePending:        true,
				models.MetaKeyAgentTitleOwnerSessionID: "s1",
			},
		}},
		sessions: map[string]*models.TaskSession{"s1": {
			ID:        "s1",
			TaskID:    "task-normal",
			State:     models.TaskSessionStateWaitingForInput,
			UpdatedAt: time.Now().UTC(),
		}},
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
	req, err := ws.NewRequest("normal-title-retry", ws.ActionMessageAdd, map[string]interface{}{
		"task_id": "task-normal", "session_id": "s1", "content": "Keep this ordinary task prompt unchanged",
	})
	require.NoError(t, err)

	resp, err := h.wsAddMessage(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	require.Len(t, repo.messages, 1)
	require.Equal(t, "Keep this ordinary task prompt unchanged", repo.messages[0].Content)
}
