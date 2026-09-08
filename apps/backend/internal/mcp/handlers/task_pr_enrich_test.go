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

type fakeTaskMRLister struct {
	byTask map[string][]TaskMRInfo
	err    error
	gotIDs []string
}

func (f *fakeTaskMRLister) ListTaskMRsByTaskIDs(_ context.Context, taskIDs []string) (map[string][]TaskMRInfo, error) {
	f.gotIDs = taskIDs
	if f.err != nil {
		return nil, f.err
	}
	return f.byTask, nil
}

func TestEnrichTasksWithPRs(t *testing.T) {
	draft := false
	lister := &fakeTaskPRLister{
		byTask: map[string][]TaskPRInfo{
			"task-1": {
				{RepositoryID: "repo-gh", Number: 42, URL: "https://github.com/o/r/pull/42", Title: "Fix bug", State: "merged", Draft: &draft, BaseRef: "main", HeadRef: "fix"},
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
	require.Len(t, dtos[0].ChangeRequests, 2)
	assert.Equal(t, v1.TaskChangeRequestSummary{
		Provider: "github", RepositoryID: "repo-gh", Number: 42,
		URL: "https://github.com/o/r/pull/42", Title: "Fix bug", State: "merged",
		Draft: &draft, BaseRef: "main", HeadRef: "fix",
	}, dtos[0].ChangeRequests[0])
	assert.Nil(t, dtos[1].PRs, "tasks without PRs stay nil")
}

func TestEnrichTasksWithGitLabMRsAddsProviderNeutralChangeRequests(t *testing.T) {
	merged := time.Now().UTC()
	lister := &fakeTaskMRLister{byTask: map[string][]TaskMRInfo{
		"task-1": {{
			RepositoryID: "repo-gl", Number: 224, URL: "https://gitlab.example.test/group/project/-/merge_requests/224",
			Title: "Ship MR", State: "merged", Draft: true, BaseRef: "main", HeadRef: "feature", MergedAt: &merged,
		}},
	}}
	h := &Handlers{taskMRLister: lister, logger: testLogger(t).WithFields()}

	dtos := []dto.TaskDTO{{ID: "task-1"}, {ID: "task-2"}}
	h.enrichTasksWithPRs(context.Background(), dtos)

	assert.ElementsMatch(t, []string{"task-1", "task-2"}, lister.gotIDs)
	assert.Nil(t, dtos[0].PRs, "GitLab MRs do not populate the legacy GitHub-only prs field")
	require.Len(t, dtos[0].ChangeRequests, 1)
	got := dtos[0].ChangeRequests[0]
	assert.Equal(t, "gitlab", got.Provider)
	assert.Equal(t, "repo-gl", got.RepositoryID)
	assert.Equal(t, 224, got.Number)
	assert.Equal(t, "merged", got.State)
	require.NotNil(t, got.Draft)
	assert.True(t, *got.Draft)
	assert.Equal(t, "main", got.BaseRef)
	assert.Equal(t, "feature", got.HeadRef)
	require.NotNil(t, got.MergedAt)
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
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-pr", WorkspaceID: "ws-pr", Name: "PR Workflow", CreatedAt: now, UpdatedAt: now,
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

func TestHandleListTasks_IncludesProviderNeutralGitLabMRs(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-mr", Name: "MR WS", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-mr", WorkspaceID: "ws-mr", Name: "MR Workflow", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-mr", WorkspaceID: "ws-mr", WorkflowID: "wf-mr",
		Title: "Has an MR", State: v1.TaskStateReview, CreatedAt: now, UpdatedAt: now,
	}))

	h := &Handlers{
		taskSvc: svc,
		logger:  testLogger(t).WithFields(),
		taskMRLister: &fakeTaskMRLister{byTask: map[string][]TaskMRInfo{
			"task-mr": {{RepositoryID: "repo-gl", Number: 224, URL: "https://gitlab.example.test/g/p/-/merge_requests/224", Title: "Ship it", State: "open"}},
		}},
	}

	msg := makeWSMessage(t, ws.ActionMCPListTasks, map[string]any{"workflow_id": "wf-mr"})
	resp, err := h.handleListTasks(ctx, msg)
	require.NoError(t, err)

	var payload dto.ListTasksResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	require.Len(t, payload.Tasks, 1)
	require.Empty(t, payload.Tasks[0].PRs)
	require.Len(t, payload.Tasks[0].ChangeRequests, 1)
	assert.Equal(t, "gitlab", payload.Tasks[0].ChangeRequests[0].Provider)
	assert.Equal(t, 224, payload.Tasks[0].ChangeRequests[0].Number)
}

func TestHandleListRelatedTasks_IncludesAssociatedPRs(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-rel", Name: "Rel WS", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-rel", WorkspaceID: "ws-rel", WorkflowID: "wf-rel",
		Title: "Self", State: v1.TaskStateReview, CreatedAt: now, UpdatedAt: now,
	}))

	merged := now
	h := &Handlers{
		taskSvc:    svc,
		handoffSvc: service.NewHandoffService(repo, nil, nil, nil, nil, testLogger(t)),
		logger:     testLogger(t).WithFields(),
		taskPRLister: &fakeTaskPRLister{byTask: map[string][]TaskPRInfo{
			"task-rel": {{Number: 99, URL: "https://github.com/o/r/pull/99", Title: "Fix", State: "merged", MergedAt: &merged}},
		}},
	}

	msg := makeWSMessage(t, ws.ActionMCPListRelatedTasks, map[string]any{"task_id": "task-rel"})
	resp, err := h.handleListRelatedTasks(ctx, msg)
	require.NoError(t, err)

	var payload service.RelatedTasks
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	require.Len(t, payload.Task.PRs, 1)
	assert.Equal(t, 99, payload.Task.PRs[0].Number)
	assert.Equal(t, "merged", payload.Task.PRs[0].State)
	require.NotNil(t, payload.Task.PRs[0].MergedAt, "merged_at should be surfaced")
}

func TestHandleListRelatedTasks_IncludesProviderNeutralGitLabMRs(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-rel-mr", Name: "Rel MR WS", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-rel-mr", WorkspaceID: "ws-rel-mr", WorkflowID: "wf-rel-mr",
		Title: "Self", State: v1.TaskStateReview, CreatedAt: now, UpdatedAt: now,
	}))

	h := &Handlers{
		taskSvc:    svc,
		handoffSvc: service.NewHandoffService(repo, nil, nil, nil, nil, testLogger(t)),
		logger:     testLogger(t).WithFields(),
		taskMRLister: &fakeTaskMRLister{byTask: map[string][]TaskMRInfo{
			"task-rel-mr": {{RepositoryID: "repo-gl", Number: 224, URL: "https://gitlab.example.test/g/p/-/merge_requests/224", Title: "Fix", State: "open"}},
		}},
	}

	msg := makeWSMessage(t, ws.ActionMCPListRelatedTasks, map[string]any{"task_id": "task-rel-mr"})
	resp, err := h.handleListRelatedTasks(ctx, msg)
	require.NoError(t, err)

	var payload service.RelatedTasks
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	require.Empty(t, payload.Task.PRs)
	require.Len(t, payload.Task.ChangeRequests, 1)
	assert.Equal(t, "gitlab", payload.Task.ChangeRequests[0].Provider)
	assert.Equal(t, 224, payload.Task.ChangeRequests[0].Number)
}

func TestEnrichRelatedTasksWithPRs(t *testing.T) {
	draft := false
	lister := &fakeTaskPRLister{byTask: map[string][]TaskPRInfo{
		"self":   {{RepositoryID: "repo-gh", Number: 1, URL: "u1", State: "open", Draft: &draft}},
		"parent": {{Number: 3, URL: "u3", State: "closed"}},
		"child":  {{Number: 2, URL: "u2", State: "merged"}},
	}}
	mrLister := &fakeTaskMRLister{byTask: map[string][]TaskMRInfo{
		"self": {{RepositoryID: "repo-gl", Number: 4, URL: "m1", State: "open"}},
	}}
	h := &Handlers{taskPRLister: lister, taskMRLister: mrLister, logger: testLogger(t).WithFields()}

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
	require.Len(t, related.Task.ChangeRequests, 2)
	assert.Equal(t, "github", related.Task.ChangeRequests[0].Provider)
	assert.Equal(t, "gitlab", related.Task.ChangeRequests[1].Provider)
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
