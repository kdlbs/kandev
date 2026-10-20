package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
)

type stepTransitionsFixture struct {
	handlers *TaskHandlers
	countRow func(t *testing.T) int
}

func newStepTransitionsFixture(t *testing.T) stepTransitionsFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	database, _ := workflowHandlerTemplate.Open(t)
	repo := sqliterepo.NewWithInitializedDB(database, database, nil)
	ownerCtx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "owner", Role: authn.RoleMember, OrgID: "owner-org"})
	require.NoError(t, repo.CreateWorkspace(ownerCtx, &models.Workspace{ID: "ws-private", OwnerID: "owner", OrgID: "owner-org", Name: "Private"}))
	_, err := database.Exec(database.Rebind(`INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, ?, ?), (?, ?, ?, ?)`),
		"step-todo", "wf-1", "Todo", 0, "step-approval", "wf-1", "Approval", 1)
	require.NoError(t, err)
	require.NoError(t, repo.CreateTask(ownerCtx, &models.Task{
		ID: "task-private", WorkspaceID: "ws-private", WorkflowID: "wf-1", WorkflowStepID: "step-todo", Title: "Private", Priority: "medium",
	}))
	require.NoError(t, repo.CreateTask(ownerCtx, &models.Task{ID: "task-other", WorkspaceID: "ws-private", Title: "Other", Priority: "medium"}))
	_, err = database.Exec(database.Rebind(`INSERT INTO task_step_transitions
		(task_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id, trigger, actor_kind, actor_id, contract_version, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		"task-private", "wf-1", "step-todo", "wf-1", "step-approval", "mcp_move", "agent", "session-1", 1, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)

	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo,
		TaskEnvironments: repo, Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return stepTransitionsFixture{
		handlers: &TaskHandlers{service: svc, repo: repo, logger: log},
		countRow: func(t *testing.T) int {
			var count int
			require.NoError(t, database.Get(&count, `SELECT COUNT(*) FROM task_step_transitions`))
			return count
		},
	}
}

func getStepTransitions(t *testing.T, h *TaskHandlers, identity *authn.Identity, taskID string, query url.Values) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := context.Background()
	if identity != nil {
		ctx = authn.WithIdentity(ctx, *identity)
	}
	target := "/api/v1/tasks/" + taskID + "/step-transitions"
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	c.Request = httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx)
	c.Params = gin.Params{{Key: "id", Value: taskID}}
	h.httpListTaskStepTransitions(c)
	return recorder
}

func decodeStepTransitionPage(t *testing.T, recorder *httptest.ResponseRecorder) stepTransitionPageResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var page stepTransitionPageResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &page))
	return page
}

// @covers AC-TASKS-STEP-TRANSITION-READ-001.1 AC-TASKS-STEP-TRANSITION-READ-001.4
func TestHTTPListTaskStepTransitionsReturnsAttributedMovesOldestFirst(t *testing.T) {
	fixture := newStepTransitionsFixture(t)
	before := fixture.countRow(t)
	owner := &authn.Identity{UserID: "owner", Role: authn.RoleMember, OrgID: "owner-org"}

	page := decodeStepTransitionPage(t, getStepTransitions(t, fixture.handlers, owner, "task-private", nil))

	require.Len(t, page.Items, 2)
	assert.False(t, page.HasMore)
	assert.Empty(t, page.NextCursor)
	genesis, move := page.Items[0], page.Items[1]
	assert.Equal(t, "task_created", genesis.Trigger)
	assert.Equal(t, "human", genesis.ActorKind)
	require.NotNil(t, genesis.ActorID)
	assert.Equal(t, "owner", *genesis.ActorID)
	assert.Nil(t, genesis.FromWorkflowStepID)
	require.NotNil(t, genesis.ToWorkflowStepName)
	assert.Equal(t, "Todo", *genesis.ToWorkflowStepName)

	assert.Equal(t, "mcp_move", move.Trigger)
	assert.Equal(t, "agent", move.ActorKind)
	require.NotNil(t, move.ActorID)
	assert.Equal(t, "session-1", *move.ActorID)
	require.NotNil(t, move.FromWorkflowStepID)
	assert.Equal(t, "step-todo", *move.FromWorkflowStepID)
	require.NotNil(t, move.ToWorkflowStepName)
	assert.Equal(t, "Approval", *move.ToWorkflowStepName)
	require.NotNil(t, move.ToWorkflowID)
	assert.Equal(t, "wf-1", *move.ToWorkflowID)
	occurredAt, err := time.Parse(time.RFC3339Nano, move.OccurredAt)
	require.NoError(t, err)
	assert.False(t, occurredAt.IsZero())

	assert.Equal(t, before, fixture.countRow(t), "a read must not write ledger rows")
}

// @covers AC-TASKS-STEP-TRANSITION-READ-001.1
func TestHTTPListTaskStepTransitionsPaginatesWithCursor(t *testing.T) {
	fixture := newStepTransitionsFixture(t)
	owner := &authn.Identity{UserID: "owner", Role: authn.RoleMember, OrgID: "owner-org"}

	first := decodeStepTransitionPage(t, getStepTransitions(t, fixture.handlers, owner, "task-private", url.Values{"limit": {"1"}}))
	require.Len(t, first.Items, 1)
	require.True(t, first.HasMore)
	assert.Equal(t, "task_created", first.Items[0].Trigger)

	second := decodeStepTransitionPage(t, getStepTransitions(t, fixture.handlers, owner, "task-private",
		url.Values{"limit": {"1"}, "cursor": {first.NextCursor}}))
	require.Len(t, second.Items, 1)
	assert.False(t, second.HasMore)
	assert.Equal(t, "mcp_move", second.Items[0].Trigger)
}

// @covers AC-TASKS-STEP-TRANSITION-READ-001.2
func TestHTTPListTaskStepTransitionsHidesUnreachableAndUnknownTasks(t *testing.T) {
	fixture := newStepTransitionsFixture(t)
	for _, tc := range []struct {
		name     string
		identity *authn.Identity
		taskID   string
	}{
		{"foreign workspace", &authn.Identity{UserID: "outsider", Role: authn.RoleMember, OrgID: "owner-org"}, "task-private"},
		{"owner from another organization", &authn.Identity{UserID: "owner", Role: authn.RoleMember, OrgID: "foreign-org"}, "task-private"},
		{"unknown task for owner", &authn.Identity{UserID: "owner", Role: authn.RoleMember, OrgID: "owner-org"}, "task-missing"},
		{"unknown task with auth disabled", &authn.Identity{UserID: "default-user", Role: authn.RoleAdmin, Synthetic: true}, "task-missing"},
		{"unknown task without identity", nil, "task-missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := getStepTransitions(t, fixture.handlers, tc.identity, tc.taskID, nil)
			assert.Equal(t, http.StatusNotFound, recorder.Code)
			assert.JSONEq(t, `{"error":"task not found"}`, recorder.Body.String())
		})
	}
}

// @covers AC-TASKS-STEP-TRANSITION-READ-001.2
func TestHTTPListTaskStepTransitionsAllowsUnscopedCallers(t *testing.T) {
	fixture := newStepTransitionsFixture(t)
	synthetic := &authn.Identity{UserID: "default-user", Role: authn.RoleAdmin, Synthetic: true}
	assert.Len(t, decodeStepTransitionPage(t, getStepTransitions(t, fixture.handlers, synthetic, "task-private", nil)).Items, 2)
	assert.Len(t, decodeStepTransitionPage(t, getStepTransitions(t, fixture.handlers, nil, "task-private", nil)).Items, 2)
}

// @covers AC-TASKS-STEP-TRANSITION-READ-001.5
func TestHTTPListTaskStepTransitionsRejectsInvalidPaging(t *testing.T) {
	fixture := newStepTransitionsFixture(t)
	owner := &authn.Identity{UserID: "owner", Role: authn.RoleMember, OrgID: "owner-org"}
	first := decodeStepTransitionPage(t, getStepTransitions(t, fixture.handlers, owner, "task-private", url.Values{"limit": {"1"}}))
	for _, tc := range []struct {
		name, taskID string
		query        url.Values
	}{
		{"negative limit", "task-private", url.Values{"limit": {"-1"}}},
		{"non-numeric limit", "task-private", url.Values{"limit": {"ten"}}},
		{"malformed cursor", "task-private", url.Values{"cursor": {"not-a-cursor"}}},
		{"cursor of another task", "task-other", url.Values{"cursor": {first.NextCursor}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := getStepTransitions(t, fixture.handlers, owner, tc.taskID, tc.query)
			assert.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
		})
	}
}
