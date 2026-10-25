package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type policyPromptMessageRepo struct {
	*messageAddSwitchRepo
	taskRepositories []*models.TaskRepository
	repositories     map[string]*models.Repository
}

func (r *policyPromptMessageRepo) ListTaskRepositories(
	_ context.Context,
	_ string,
) ([]*models.TaskRepository, error) {
	return r.taskRepositories, nil
}

func (r *policyPromptMessageRepo) GetRepository(
	_ context.Context,
	id string,
) (*models.Repository, error) {
	return r.repositories[id], nil
}

// @covers AC-WORKSPACES-BRANCH-POLICIES-004.6
func TestWSAddMessage_PersistsSnapshottedPullRequestTargetContext(t *testing.T) {
	now := time.Now().UTC()
	base := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{"t1": {
			ID: "t1", WorkspaceID: "ws1", State: v1.TaskStateInProgress, UpdatedAt: now,
		}},
		sessions: map[string]*models.TaskSession{"s1": {
			ID: "s1", TaskID: "t1", State: models.TaskSessionStateCreated,
			AgentProfileID: "profile-1", UpdatedAt: now,
		}},
		primaryID: "s1",
	}
	repo := &policyPromptMessageRepo{
		messageAddSwitchRepo: base,
		taskRepositories: []*models.TaskRepository{{
			ID:                            "task-repo-1",
			TaskID:                        "t1",
			RepositoryID:                  "repo-1",
			BaseBranch:                    "develop",
			CheckoutBranch:                "release/next",
			BranchPolicyID:                "policy-release",
			BranchPolicyPullRequestTarget: "main",
		}},
		repositories: map[string]*models.Repository{
			"repo-1": {ID: "repo-1", WorkspaceID: "ws1", Name: "kandev"},
		},
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
	orch := &firstTurnCaptureOrchestrator{started: make(chan capturedFirstTurn, 1)}
	handler := NewMessageHandlers(svc, orch, log)

	req, err := ws.NewRequest("req-policy-target", ws.ActionMessageAdd, map[string]any{
		"task_id": "t1", "session_id": "s1", "content": "Create the release",
	})
	require.NoError(t, err)
	response, err := handler.wsAddMessage(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Len(t, base.messages, 1)
	require.Contains(t, base.messages[0].Content, `gh pr create --base "main"`)
}
