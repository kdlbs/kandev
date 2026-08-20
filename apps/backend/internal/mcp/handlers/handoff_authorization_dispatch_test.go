package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListRelatedTasksDispatcherEnforcesRelatedReadAuthorization(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	log := testLogger(t)
	docs := service.NewDocumentService(repo, log)
	handoff := service.NewHandoffService(repo, repo, docs, nil, nil, log)
	handlers := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, log)
	handlers.SetHandoffService(handoff)
	dispatcher := ws.NewDispatcher()
	handlers.RegisterHandlers(dispatcher)

	seedRelatedReadTasks(t, repo)
	_, err := docs.CreateOrUpdateDocument(ctx, "parent", "spec", "custom", "Spec", "parent body", "agent", "Agent")
	require.NoError(t, err)
	_, err = docs.CreateOrUpdateDocument(ctx, "stranger", "secret", "custom", "Secret", "stranger body", "agent", "Agent")
	require.NoError(t, err)

	before := snapshotRelatedReadTables(t, repo)

	self := dispatchRelatedRead(t, dispatcher, map[string]any{
		"task_id": "child-a", "caller_task_id": "child-a", "caller_session_id": "session-caller",
	})
	require.Equal(t, ws.MessageTypeResponse, self.Type)
	var selfPayload service.RelatedTasks
	require.NoError(t, json.Unmarshal(self.Payload, &selfPayload))
	assert.Equal(t, "child-a", selfPayload.Task.ID)
	assertRelatedTasksOmitDescriptions(t, &selfPayload)

	relation := dispatchRelatedRead(t, dispatcher, map[string]any{
		"task_id": "parent", "caller_task_id": "child-a", "caller_session_id": "session-caller", "verbose": true,
	})
	require.Equal(t, ws.MessageTypeResponse, relation.Type)
	var relationPayload service.RelatedTasks
	require.NoError(t, json.Unmarshal(relation.Payload, &relationPayload))
	assert.Equal(t, "parent description", relationPayload.Task.Description)
	assert.Equal(t, []string{"spec"}, relationPayload.Task.DocumentKeys)
	require.Len(t, relationPayload.Children, 2)

	coordinatorCompact := dispatchRelatedRead(t, dispatcher, map[string]any{
		"task_id": "stranger", "caller_task_id": "child-a", "caller_session_id": "session-caller",
		"related_read_scope": "workspace-task-tree",
	})
	require.Equal(t, ws.MessageTypeResponse, coordinatorCompact.Type)
	var compactPayload service.RelatedTasks
	require.NoError(t, json.Unmarshal(coordinatorCompact.Payload, &compactPayload))
	assert.Equal(t, "stranger", compactPayload.Task.ID)
	assertRelatedTasksOmitDescriptions(t, &compactPayload)
	assert.Empty(t, compactPayload.Task.DocumentKeys)

	ordinary := dispatchRelatedRead(t, dispatcher, map[string]any{
		"task_id": "stranger", "caller_task_id": "child-a", "caller_session_id": "session-caller",
	})
	assertRelatedReadDenied(t, ordinary, "related_task_scope_required")

	coordinatorVerbose := dispatchRelatedRead(t, dispatcher, map[string]any{
		"task_id": "stranger", "caller_task_id": "child-a", "caller_session_id": "session-caller",
		"related_read_scope": "workspace-task-tree", "verbose": true,
	})
	assertRelatedReadDenied(t, coordinatorVerbose, "verbose_document_scope_required")

	foreign := dispatchRelatedRead(t, dispatcher, map[string]any{
		"task_id": "foreign", "caller_task_id": "child-a", "caller_session_id": "session-caller",
		"related_read_scope": "workspace-task-tree",
	})
	unknown := dispatchRelatedRead(t, dispatcher, map[string]any{
		"task_id": "missing", "caller_task_id": "child-a", "caller_session_id": "session-caller",
		"related_read_scope": "workspace-task-tree",
	})
	assertRelatedReadDenied(t, foreign, "target_unavailable")
	assertRelatedReadDenied(t, unknown, "target_unavailable")
	assert.Equal(t, string(foreign.Payload), string(unknown.Payload), "unknown and foreign targets must be indistinguishable")
	assert.NotContains(t, string(foreign.Payload), "Foreign secret")

	assert.Equal(t, before, snapshotRelatedReadTables(t, repo))
}

func seedRelatedReadTasks(t *testing.T, repo relatedReadSeedRepo) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	for _, workspace := range []*models.Workspace{
		{ID: "ws-a", Name: "Workspace A", CreatedAt: now, UpdatedAt: now},
		{ID: "ws-b", Name: "Workspace B", CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, repo.CreateWorkspace(ctx, workspace))
	}
	for _, workflow := range []*models.Workflow{
		{ID: "wf-a", WorkspaceID: "ws-a", Name: "Flow A", CreatedAt: now, UpdatedAt: now},
		{ID: "wf-b", WorkspaceID: "ws-b", Name: "Flow B", CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, repo.CreateWorkflow(ctx, workflow))
	}
	for _, task := range []*models.Task{
		{ID: "parent", WorkspaceID: "ws-a", WorkflowID: "wf-a", Title: "Parent", Description: "parent description", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now},
		{ID: "child-a", WorkspaceID: "ws-a", WorkflowID: "wf-a", Title: "Child A", ParentID: "parent", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now},
		{ID: "child-b", WorkspaceID: "ws-a", WorkflowID: "wf-a", Title: "Child B", ParentID: "parent", State: v1.TaskStateCreated, CreatedAt: now, UpdatedAt: now},
		{ID: "stranger", WorkspaceID: "ws-a", WorkflowID: "wf-a", Title: "Unrelated", Description: "stranger description", State: v1.TaskStateReview, CreatedAt: now, UpdatedAt: now},
		{ID: "foreign", WorkspaceID: "ws-b", WorkflowID: "wf-b", Title: "Foreign secret", Description: "Foreign secret description", State: v1.TaskStateCreated, CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, repo.CreateTask(ctx, task))
	}
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-caller", TaskID: "child-a", State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
	}))
}

type relatedReadSeedRepo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
	CreateTaskSession(context.Context, *models.TaskSession) error
}

func dispatchRelatedRead(t *testing.T, dispatcher *ws.Dispatcher, payload map[string]any) *ws.Message {
	t.Helper()
	resp, err := dispatcher.Dispatch(context.Background(), makeWSMessage(t, ws.ActionMCPListRelatedTasks, payload))
	require.NoError(t, err)
	require.NotNil(t, resp)
	return resp
}

func assertRelatedReadDenied(t *testing.T, resp *ws.Message, reason string) {
	t.Helper()
	require.Equal(t, ws.MessageTypeError, resp.Type)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, ws.ErrorCodeForbidden, payload.Code)
	assert.Equal(t, "related task access denied", payload.Message)
	assert.Equal(t, reason, payload.Details["reason"])
}

func assertRelatedTasksOmitDescriptions(t *testing.T, related *service.RelatedTasks) {
	t.Helper()
	require.NotNil(t, related)
	for _, task := range []*service.RelatedTask{
		&related.Task,
		related.Parent,
	} {
		assertRelatedTaskOmitsDescription(t, task)
	}
	for _, group := range [][]*service.RelatedTask{
		related.Children,
		related.Siblings,
		related.Blockers,
		related.BlockedBy,
	} {
		for _, task := range group {
			assertRelatedTaskOmitsDescription(t, task)
		}
	}
}

func assertRelatedTaskOmitsDescription(t *testing.T, task *service.RelatedTask) {
	t.Helper()
	if task == nil {
		return
	}
	assert.Empty(t, task.Description)
}

type relatedReadTableSnapshot struct {
	tasksA       int
	tasksB       int
	callerSess   int
	parentDocs   int
	strangerDocs int
	foreignDocs  int
}

func snapshotRelatedReadTables(t *testing.T, repo relatedReadSnapshotRepo) relatedReadTableSnapshot {
	t.Helper()
	ctx := context.Background()
	tasksA, err := repo.CountTasksByWorkflow(ctx, "wf-a")
	require.NoError(t, err)
	tasksB, err := repo.CountTasksByWorkflow(ctx, "wf-b")
	require.NoError(t, err)
	sessionCounts, err := repo.GetSessionCountsByTaskIDs(ctx, []string{"child-a"})
	require.NoError(t, err)
	parentDocs, err := repo.ListDocuments(ctx, "parent")
	require.NoError(t, err)
	strangerDocs, err := repo.ListDocuments(ctx, "stranger")
	require.NoError(t, err)
	foreignDocs, err := repo.ListDocuments(ctx, "foreign")
	require.NoError(t, err)
	return relatedReadTableSnapshot{
		tasksA:       tasksA,
		tasksB:       tasksB,
		callerSess:   sessionCounts["child-a"],
		parentDocs:   len(parentDocs),
		strangerDocs: len(strangerDocs),
		foreignDocs:  len(foreignDocs),
	}
}

type relatedReadSnapshotRepo interface {
	CountTasksByWorkflow(context.Context, string) (int, error)
	GetSessionCountsByTaskIDs(context.Context, []string) (map[string]int, error)
	ListDocuments(context.Context, string) ([]*models.TaskDocument, error)
}
