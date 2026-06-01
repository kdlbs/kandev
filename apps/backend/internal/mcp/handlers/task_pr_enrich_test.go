package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTaskPRLister struct {
	byTask map[string][]TaskPRInfo
	err    error
	gotIDs []string
}

func (f *fakeTaskPRLister) ListTaskPRsByTaskIDs(_ context.Context, taskIDs []string) (map[string][]TaskPRInfo, error) {
	f.gotIDs = taskIDs
	if f.err != nil {
		return nil, f.err
	}
	return f.byTask, nil
}

func TestEnrichTasksWithPRs(t *testing.T) {
	lister := &fakeTaskPRLister{
		byTask: map[string][]TaskPRInfo{
			"task-1": {
				{Number: 42, URL: "https://github.com/o/r/pull/42", Title: "Fix bug", State: "merged"},
				{Number: 43, URL: "https://github.com/o/r/pull/43", Title: "Add feature", State: "open"},
			},
		},
	}
	h := &Handlers{taskPRLister: lister, logger: testLogger(t).WithFields()}

	dtos := []dto.TaskDTO{{ID: "task-1"}, {ID: "task-2"}}
	h.enrichTasksWithPRs(context.Background(), dtos)

	assert.ElementsMatch(t, []string{"task-1", "task-2"}, lister.gotIDs)
	require.Len(t, dtos[0].PRs, 2)
	assert.Equal(t, v1.TaskPRSummary{
		Number: 42, URL: "https://github.com/o/r/pull/42", Title: "Fix bug", State: "merged",
	}, dtos[0].PRs[0])
	assert.Equal(t, "open", dtos[0].PRs[1].State)
	assert.Nil(t, dtos[1].PRs, "tasks without PRs stay nil")
}

func TestEnrichTasksWithPRs_NilListerIsNoop(t *testing.T) {
	h := &Handlers{logger: testLogger(t).WithFields()}
	dtos := []dto.TaskDTO{{ID: "task-1"}}
	h.enrichTasksWithPRs(context.Background(), dtos)
	assert.Nil(t, dtos[0].PRs)
}

func TestHandleListTasks_IncludesAssociatedPRs(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-pr", Name: "PR WS", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-pr", WorkspaceID: "ws-pr", WorkflowID: "wf-pr",
		Title: "Has a PR", State: v1.TaskStateReview, CreatedAt: now, UpdatedAt: now,
	}))

	h := &Handlers{
		taskSvc: svc,
		logger:  testLogger(t).WithFields(),
		taskPRLister: &fakeTaskPRLister{byTask: map[string][]TaskPRInfo{
			"task-pr": {{Number: 7, URL: "https://github.com/o/r/pull/7", Title: "Ship it", State: "merged"}},
		}},
	}

	msg := makeWSMessage(t, ws.ActionMCPListTasks, map[string]any{"workflow_id": "wf-pr"})
	resp, err := h.handleListTasks(ctx, msg)
	require.NoError(t, err)

	var payload dto.ListTasksResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	require.Len(t, payload.Tasks, 1)
	require.Len(t, payload.Tasks[0].PRs, 1)
	assert.Equal(t, "merged", payload.Tasks[0].PRs[0].State)
	assert.Equal(t, 7, payload.Tasks[0].PRs[0].Number)
	assert.Equal(t, "https://github.com/o/r/pull/7", payload.Tasks[0].PRs[0].URL)
}

func TestEnrichRelatedTasksWithPRs(t *testing.T) {
	lister := &fakeTaskPRLister{byTask: map[string][]TaskPRInfo{
		"self":   {{Number: 1, URL: "u1", State: "open"}},
		"parent": {{Number: 3, URL: "u3", State: "closed"}},
		"child":  {{Number: 2, URL: "u2", State: "merged"}},
	}}
	h := &Handlers{taskPRLister: lister, logger: testLogger(t).WithFields()}

	related := &service.RelatedTasks{
		Task:      service.RelatedTask{ID: "self"},
		Parent:    &service.RelatedTask{ID: "parent"},
		Children:  []*service.RelatedTask{{ID: "child"}, {ID: "child-no-pr"}},
		Siblings:  []*service.RelatedTask{{ID: "sibling"}},
		BlockedBy: []*service.RelatedTask{{ID: "blocker"}},
	}
	h.enrichRelatedTasksWithPRs(context.Background(), related)

	assert.ElementsMatch(t,
		[]string{"self", "parent", "child", "child-no-pr", "sibling", "blocker"},
		lister.gotIDs)
	require.Len(t, related.Task.PRs, 1)
	assert.Equal(t, "open", related.Task.PRs[0].State)
	require.Len(t, related.Parent.PRs, 1)
	assert.Equal(t, "closed", related.Parent.PRs[0].State)
	require.Len(t, related.Children[0].PRs, 1)
	assert.Equal(t, "merged", related.Children[0].PRs[0].State)
	assert.Nil(t, related.Children[1].PRs, "child without PRs stays nil")
	assert.Nil(t, related.Siblings[0].PRs)
}

func TestEnrichRelatedTasksWithPRs_NilSafe(t *testing.T) {
	// nil lister: no-op, no panic.
	h := &Handlers{logger: testLogger(t).WithFields()}
	h.enrichRelatedTasksWithPRs(context.Background(), &service.RelatedTasks{})
	// lister set but nil related: no panic.
	h2 := &Handlers{taskPRLister: &fakeTaskPRLister{}, logger: testLogger(t).WithFields()}
	h2.enrichRelatedTasksWithPRs(context.Background(), nil)
}

func TestEnrichTasksWithPRs_ListerErrorIsSwallowed(t *testing.T) {
	h := &Handlers{
		taskPRLister: &fakeTaskPRLister{err: errors.New("boom")},
		logger:       testLogger(t).WithFields(),
	}
	dtos := []dto.TaskDTO{{ID: "task-1"}}
	h.enrichTasksWithPRs(context.Background(), dtos)
	assert.Nil(t, dtos[0].PRs, "PRs left empty when the lister errors")
}
