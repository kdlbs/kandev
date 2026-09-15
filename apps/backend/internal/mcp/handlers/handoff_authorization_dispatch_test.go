package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestListRelatedTasksDispatcherEnforcesRelatedReadAuthorization(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	log := testLogger(t)
	sharedDB := sqlx.NewDb(repo.DB(), "sqlite3")
	officeRepo, err := officesqlite.NewWithDB(sharedDB, sharedDB, log)
	require.NoError(t, err)
	docs := service.NewDocumentService(repo, log)
	handoff := service.NewHandoffService(repo, repo, docs, officeRepo, nil, log)
	handlers := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, log)
	handlers.SetHandoffService(handoff)
	dispatcher := ws.NewDispatcher()
	handlers.RegisterHandlers(dispatcher)

	seedRelatedReadTasks(t, repo)
	_, err = docs.CreateOrUpdateDocument(ctx, "parent", "spec", "custom", "Spec", "parent body", "agent", "Agent")
	require.NoError(t, err)
	_, err = docs.CreateOrUpdateDocument(ctx, "stranger", "secret", "custom", "Secret", "stranger body", "agent", "Agent")
	require.NoError(t, err)
	require.NoError(t, officeRepo.CreateTaskBlocker(ctx, &officemodels.TaskBlocker{
		TaskID: "stranger", BlockerTaskID: "stranger-blocker",
	}))
	require.NoError(t, officeRepo.CreateTaskBlocker(ctx, &officemodels.TaskBlocker{
		TaskID: "stranger-dependent", BlockerTaskID: "stranger",
	}))
	require.NoError(t, officeRepo.CreateActivityEntry(ctx, &officemodels.ActivityEntry{
		ID: "activity-before-read", WorkspaceID: "ws-a", ActorType: officemodels.ActivityActorAgent,
		ActorID: "agent", Action: officemodels.ActivityAction("sentinel"),
		TargetType: officemodels.ActivityTargetTask, TargetID: "stranger", Details: `{}`,
	}))

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
	require.Len(t, compactPayload.Blockers, 1)
	assert.Equal(t, "stranger-blocker", compactPayload.Blockers[0].ID)
	require.Len(t, compactPayload.BlockedBy, 1)
	assert.Equal(t, "stranger-dependent", compactPayload.BlockedBy[0].ID)

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

func TestListRelatedTasksDispatcherRejectsExplicitTargetWithoutAttestedIdentity(t *testing.T) {
	svc, repo := newTestTaskService(t)
	log := testLogger(t)
	handoff := service.NewHandoffService(repo, repo, service.NewDocumentService(repo, log), nil, nil, log)
	handlers := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, log)
	handlers.SetHandoffService(handoff)
	dispatcher := ws.NewDispatcher()
	handlers.RegisterHandlers(dispatcher)
	seedRelatedReadTasks(t, repo)

	response := dispatchRelatedRead(t, dispatcher, map[string]any{
		"task_id": "stranger", "verbose": true,
	})

	assertRelatedReadDenied(t, response, "related_task_scope_required")
	assert.NotContains(t, string(response.Payload), "stranger")
	assert.NotContains(t, string(response.Payload), "Unrelated")
}

func TestListRelatedTasksAuditMarksInternalFailuresAsErrors(t *testing.T) {
	svc, repo := newTestTaskService(t)
	core, observed := observer.New(zap.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	handoff := service.NewHandoffService(repo, repo, service.NewDocumentService(repo, log), nil, nil, log)
	handlers := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, log)
	handlers.SetHandoffService(handoff)

	_, err = repo.DB().ExecContext(context.Background(), "DROP TABLE tasks")
	require.NoError(t, err)
	response, err := handlers.handleListRelatedTasks(context.Background(), makeWSMessage(t, ws.ActionMCPListRelatedTasks, map[string]any{
		"task_id": "task-a", "caller_task_id": "task-a", "caller_session_id": "session-a",
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)

	entries := observed.FilterMessage("mcp.related_task_read.authorization").All()
	require.Len(t, entries, 1)
	assert.Equal(t, "error", entries[0].ContextMap()["outcome"])
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
		{ID: "stranger-blocker", WorkspaceID: "ws-a", WorkflowID: "wf-a", Title: "Unrelated blocker", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now},
		{ID: "stranger-dependent", WorkspaceID: "ws-a", WorkflowID: "wf-a", Title: "Unrelated dependent", State: v1.TaskStateCreated, CreatedAt: now, UpdatedAt: now},
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
	tasks             [][]string
	workflows         [][]string
	sessions          [][]string
	documents         [][]string
	documentRevisions [][]string
	blockers          [][]string
	activities        [][]string
}

func snapshotRelatedReadTables(t *testing.T, repo relatedReadSnapshotRepo) relatedReadTableSnapshot {
	t.Helper()
	db := repo.DB()
	return relatedReadTableSnapshot{
		tasks:             snapshotSQLRows(t, db, "SELECT * FROM tasks ORDER BY id"),
		workflows:         snapshotSQLRows(t, db, "SELECT * FROM workflows ORDER BY id"),
		sessions:          snapshotSQLRows(t, db, "SELECT * FROM task_sessions ORDER BY id"),
		documents:         snapshotSQLRows(t, db, "SELECT * FROM task_documents ORDER BY task_id, key"),
		documentRevisions: snapshotSQLRows(t, db, "SELECT * FROM task_document_revisions ORDER BY task_id, document_key, revision_number"),
		blockers:          snapshotSQLRows(t, db, "SELECT * FROM task_blockers ORDER BY task_id, blocker_task_id"),
		activities:        snapshotSQLRows(t, db, "SELECT * FROM office_activity_log ORDER BY id"),
	}
}

type relatedReadSnapshotRepo interface {
	DB() *sql.DB
}

func snapshotSQLRows(t *testing.T, db *sql.DB, query string) [][]string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), query)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()

	columns, err := rows.Columns()
	require.NoError(t, err)
	result := make([][]string, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for i := range values {
			destinations[i] = &values[i]
		}
		require.NoError(t, rows.Scan(destinations...))
		serialized := make([]string, len(values))
		for i, value := range values {
			if bytes, ok := value.([]byte); ok {
				value = string(bytes)
			}
			serialized[i] = fmt.Sprintf("%T:%v", value, value)
		}
		result = append(result, serialized)
	}
	require.NoError(t, rows.Err())
	return result
}
