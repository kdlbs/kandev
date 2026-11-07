package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type mcpCreateRelationRepo struct {
	added [][2]string
}

func (r *mcpCreateRelationRepo) AddRelation(_ context.Context, taskA, taskB string) error {
	r.added = append(r.added, [2]string{taskA, taskB})
	return nil
}

func (*mcpCreateRelationRepo) ListRelationsForTask(context.Context, string) ([]string, error) {
	return nil, nil
}

func (*mcpCreateRelationRepo) ListRelationsForTasks(context.Context, []string) (map[string][]string, error) {
	return nil, nil
}

func (*mcpCreateRelationRepo) RemoveRelationsForTask(context.Context, string) error { return nil }

var _ service.RelationRepository = (*mcpCreateRelationRepo)(nil)

func TestHandleCreateTaskPassesRelatedTaskIDsToService(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, workspaces)
	workflows, err := svc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, err)
	require.NotEmpty(t, workflows)
	peer, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: workspaces[0].ID,
		WorkflowID:  workflows[0].ID,
		Title:       "MCP relation peer",
	})
	require.NoError(t, err)

	relations := &mcpCreateRelationRepo{}
	svc.SetRelationRepository(relations)
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	resp, err := h.handleCreateTask(mcpTestExternalContext(ctx), makeWSMessage(t, ws.ActionMCPCreateTask, map[string]any{
		"workspace_id":     workspaces[0].ID,
		"workflow_id":      workflows[0].ID,
		"title":            "MCP task with relation",
		"agent_profile_id": "profile-1",
		"start_agent":      false,
		"related_task_ids": []string{peer.Task.ID},
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &created))
	require.NotEmpty(t, created.ID)
	require.Equal(t, [][2]string{{created.ID, peer.Task.ID}}, relations.added)
}
