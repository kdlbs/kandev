package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type wsCreateRelationRepo struct {
	added [][2]string
}

func (r *wsCreateRelationRepo) AddRelation(_ context.Context, taskA, taskB string) error {
	r.added = append(r.added, [2]string{taskA, taskB})
	return nil
}

func (*wsCreateRelationRepo) ListRelationsForTask(context.Context, string) ([]string, error) {
	return nil, nil
}

func (*wsCreateRelationRepo) ListRelationsForTasks(context.Context, []string) (map[string][]string, error) {
	return nil, nil
}

func (*wsCreateRelationRepo) RemoveRelationsForTask(context.Context, string) error { return nil }

var _ service.RelationRepository = (*wsCreateRelationRepo)(nil)

func TestWSCreateTaskPassesRelatedTaskIDsToService(t *testing.T) {
	repo := &wsTaskRepo{}
	h := newWSTaskHandlers(t, repo)
	relations := &wsCreateRelationRepo{}
	h.service.SetRelationRepository(relations)

	resp, err := h.wsCreateTask(asUser("user-b"), wsWorkflowRequest(t, ws.ActionTaskCreate, map[string]any{
		"workspace_id":     "ws-b",
		"workflow_id":      "wf-b",
		"title":            "Related WS task",
		"start_agent":      false,
		"related_task_ids": []string{"task-b"},
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))
	require.Len(t, repo.created, 1)
	require.Equal(t, [][2]string{{repo.created[0].ID, "task-b"}}, relations.added)
}
