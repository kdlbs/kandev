package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestWSAddMessageWIPQueuePreservesInboxTransitionID(t *testing.T) {
	now := time.Now().UTC()
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{
			"t1": {ID: "t1", State: v1.TaskStateInProgress, UpdatedAt: now},
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
	orch := &firstTurnCaptureOrchestrator{
		turnStartResult: orchestrator.ProcessOnTurnStartResult{Queued: true},
	}
	h := NewMessageHandlers(svc, orch, log)

	req, err := ws.NewRequest("req-inbox-transition", ws.ActionMessageAdd, map[string]interface{}{
		"task_id":    "t1",
		"session_id": "s1",
		"content":    "wait for admission",
	})
	require.NoError(t, err)
	resp, err := h.wsAddMessage(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	require.Len(t, repo.messages, 1)
	require.NotNil(t, orch.queuedPromptCall)

	deliveredID, ok := repo.messages[0].Metadata[messagequeue.MetadataInboxTransitionID].(string)
	require.True(t, ok)
	queuedID, ok := orch.queuedPromptCall.metadata[messagequeue.MetadataInboxTransitionID].(string)
	require.True(t, ok)
	require.NotEmpty(t, deliveredID)
	require.Equal(t, deliveredID, queuedID)
}
