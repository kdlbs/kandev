package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orgunit"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type workspaceDeleteObservedWorkspaceRepo struct {
	repository.WorkspaceRepository
	getCalls atomic.Int64
}

func (r *workspaceDeleteObservedWorkspaceRepo) GetWorkspace(ctx context.Context, id string) (*models.Workspace, error) {
	r.getCalls.Add(1)
	return r.WorkspaceRepository.GetWorkspace(ctx, id)
}

type workspaceDeleteObservedTaskRepo struct {
	repository.TaskRepository
	listCalls atomic.Int64
}

func (r *workspaceDeleteObservedTaskRepo) ListTasksByWorkspace(
	ctx context.Context,
	workspaceID, workflowID, repositoryID, query string,
	page, pageSize int,
	sort string,
	includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool,
) ([]*models.Task, int, error) {
	r.listCalls.Add(1)
	return r.TaskRepository.ListTasksByWorkspace(
		ctx, workspaceID, workflowID, repositoryID, query, page, pageSize, sort,
		includeArchived, includeEphemeral, onlyEphemeral, excludeConfig,
	)
}

type workspaceDeleteCanvasRecorder struct {
	mu           sync.Mutex
	workspaceIDs []string
	taskIDs      []string
}

func (r *workspaceDeleteCanvasRecorder) CleanupWorkspaceCanvases(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workspaceIDs = append(r.workspaceIDs, workspaceID)
	return nil
}

func (r *workspaceDeleteCanvasRecorder) CleanupTaskCanvases(_ context.Context, taskID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.taskIDs = append(r.taskIDs, taskID)
	return nil
}

func (r *workspaceDeleteCanvasRecorder) calls() (workspaceIDs, taskIDs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.workspaceIDs...), append([]string(nil), r.taskIDs...)
}

type workspaceDeleteSecretRecorder struct {
	mu          sync.Mutex
	workspaceID []string
}

func (r *workspaceDeleteSecretRecorder) DeleteWorkspaceSecrets(_ context.Context, workspaceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workspaceID = append(r.workspaceID, workspaceID)
	return nil
}

func (r *workspaceDeleteSecretRecorder) calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.workspaceID...)
}

type workspaceDeleteCleanupJobRow struct {
	ID          string    `db:"id"`
	OperationID string    `db:"operation_id"`
	TaskID      string    `db:"task_id"`
	Trigger     string    `db:"trigger"`
	State       string    `db:"state"`
	Attempts    int       `db:"attempts"`
	LastError   string    `db:"last_error"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

type workspaceDeleteFixture struct {
	database    *sqlx.DB
	repo        *sqliterepo.Repository
	workspaces  *workspaceDeleteObservedWorkspaceRepo
	tasks       *workspaceDeleteObservedTaskRepo
	service     *service.Service
	router      *gin.Engine
	dispatch    *ws.Dispatcher
	canvas      *workspaceDeleteCanvasRecorder
	secrets     *workspaceDeleteSecretRecorder
	units       *orgunit.Service
	eventMu     sync.Mutex
	observed    []*bus.Event
	cleanupJobs []workspaceDeleteCleanupJobRow
}

func newWorkspaceDeleteFixture(t *testing.T) *workspaceDeleteFixture {
	t.Helper()
	database, _ := workflowHandlerTemplate.Open(t)
	repo := sqliterepo.NewWithInitializedDB(database, database, nil)
	log := newTestLogger(t)
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)

	f := &workspaceDeleteFixture{
		database: database,
		repo:     repo,
		workspaces: &workspaceDeleteObservedWorkspaceRepo{
			WorkspaceRepository: repo,
		},
		tasks:    &workspaceDeleteObservedTaskRepo{TaskRepository: repo},
		router:   gin.New(),
		dispatch: ws.NewDispatcher(),
		canvas:   &workspaceDeleteCanvasRecorder{},
		secrets:  &workspaceDeleteSecretRecorder{},
	}
	f.service = service.NewService(service.Repos{
		Workspaces:       f.workspaces,
		Tasks:            f.tasks,
		TaskRepos:        repo,
		Workflows:        repo,
		Messages:         repo,
		Turns:            repo,
		Sessions:         repo,
		GitSnapshots:     repo,
		RepoEntities:     repo,
		Executors:        repo,
		Environments:     repo,
		TaskEnvironments: repo,
		Reviews:          repo,
	}, eventBus, log, service.RepositoryDiscoveryConfig{})

	unitStore, err := orgunit.NewStore(internaldb.NewPool(database, database))
	require.NoError(t, err)
	f.units = orgunit.NewService(unitStore, nil)
	f.units.SetWorkspaceCounter(repo)
	f.service.SetUnitPlacer(f.units)
	f.service.SetUnitReach(f.units)
	f.service.SetCanvasCleanup(f.canvas)
	f.service.SetWorkspaceSecretDeleter(f.secrets)

	_, err = eventBus.Subscribe(">", func(_ context.Context, event *bus.Event) error {
		f.eventMu.Lock()
		defer f.eventMu.Unlock()
		f.observed = append(f.observed, event)
		return nil
	})
	require.NoError(t, err)
	RegisterWorkspaceRoutes(f.router, f.dispatch, f.service, log)
	f.cleanupJobs = f.snapshotCleanupJobs(t)
	return f
}

func deleteIdentityContext(userID string, role authn.Role) context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: userID, Role: role})
}

func syntheticDeleteIdentityContext() context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "synthetic-admin", Role: authn.RoleAdmin, Synthetic: true,
	})
}

func (f *workspaceDeleteFixture) createWorkspace(t *testing.T, id, name, ownerID, unitID, orgID string) *models.Workspace {
	t.Helper()
	workspace := &models.Workspace{ID: id, Name: name, OwnerID: ownerID, UnitID: unitID, OrgID: orgID}
	require.NoError(t, f.repo.CreateWorkspace(t.Context(), workspace))
	return workspace
}

func (f *workspaceDeleteFixture) addMember(t *testing.T, workspaceID, userID, role string) {
	t.Helper()
	require.NoError(t, f.repo.UpsertWorkspaceMember(t.Context(), &models.WorkspaceMember{
		WorkspaceID: workspaceID, UserID: userID, Role: role, AddedBy: "user-owner",
	}))
}

func (f *workspaceDeleteFixture) seedDependentRows(t *testing.T, workspaceID string) (workflowID, taskID, sessionID string) {
	t.Helper()
	workflowID = "workflow-" + workspaceID
	taskID = "task-" + workspaceID
	sessionID = "session-" + workspaceID
	now := time.Now().UTC()
	require.NoError(t, f.repo.CreateWorkflow(t.Context(), &models.Workflow{
		ID: workflowID, WorkspaceID: workspaceID, Name: "Board",
	}))
	require.NoError(t, f.repo.CreateTask(t.Context(), &models.Task{
		ID: taskID, WorkspaceID: workspaceID, WorkflowID: workflowID, WorkflowStepID: "step-1",
		Title: "Task", State: v1.TaskStateCreated, Priority: "medium",
	}))
	require.NoError(t, f.repo.CreateTaskSession(t.Context(), &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
		StartedAt: now, UpdatedAt: now,
	}))
	return workflowID, taskID, sessionID
}

func (f *workspaceDeleteFixture) requireDecision(t *testing.T, ctx context.Context, workspaceID string, canRead, canManage bool) {
	t.Helper()
	decision, err := f.service.WorkspaceDecisionByID(ctx, workspaceID)
	require.NoError(t, err)
	require.Equal(t, canRead, decision.CanRead(), "workspace.read decision")
	require.Equal(t, canManage, decision.Has(authz.ScopeWorkspaceManage), "workspace.manage decision")
}

func (f *workspaceDeleteFixture) resetCounters() {
	f.workspaces.getCalls.Store(0)
	f.tasks.listCalls.Store(0)
}

func (f *workspaceDeleteFixture) countRows(t *testing.T, table, column, value string) int {
	t.Helper()
	var count int
	require.NoError(t, f.database.GetContext(t.Context(), &count,
		fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", table, column), value))
	return count
}

func (f *workspaceDeleteFixture) snapshotCleanupJobs(t *testing.T) []workspaceDeleteCleanupJobRow {
	t.Helper()
	var jobs []workspaceDeleteCleanupJobRow
	require.NoError(t, f.database.SelectContext(t.Context(), &jobs, `
		SELECT id, operation_id, task_id, trigger, state, attempts, last_error, created_at, updated_at
		FROM task_resource_cleanup_jobs
		ORDER BY id
	`))
	return jobs
}

type workspaceDeleteResult struct {
	status      int
	body        string
	frame       ws.MessageType
	payload     ws.ErrorPayload
	dispatchErr error
}

func (f *workspaceDeleteFixture) deleteHTTPPayload(ctx context.Context, id, body string) workspaceDeleteResult {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/workspaces/"+id, strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	f.router.ServeHTTP(recorder, request)
	return workspaceDeleteResult{status: recorder.Code, body: recorder.Body.String()}
}

func (f *workspaceDeleteFixture) deleteWSRequest(t *testing.T, ctx context.Context, action string, payload map[string]string) workspaceDeleteResult {
	t.Helper()
	request, err := ws.NewRequest("delete-request", action, payload)
	require.NoError(t, err)
	response, dispatchErr := f.dispatch.Dispatch(ctx, request)
	result := workspaceDeleteResult{dispatchErr: dispatchErr}
	if response == nil {
		return result
	}
	result.frame = response.Type
	if response.Type == ws.MessageTypeError {
		_ = json.Unmarshal(response.Payload, &result.payload)
	}
	return result
}

func (f *workspaceDeleteFixture) delete(t *testing.T, ctx context.Context, transport, id, confirmName string) workspaceDeleteResult {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"confirm_name": confirmName})
	require.NoError(t, err)
	if transport == "HTTP" {
		return f.deleteHTTPPayload(ctx, id, string(payload))
	}
	return f.deleteWSRequest(t, ctx, ws.ActionWorkspaceDelete, map[string]string{
		"id": id, "confirm_name": confirmName,
	})
}

func requireDeleteDenied(t *testing.T, transport string, result workspaceDeleteResult) {
	t.Helper()
	if transport == "HTTP" {
		require.Equal(t, http.StatusForbidden, result.status)
		require.JSONEq(t, `{"error":"insufficient permissions for this action"}`, result.body)
		return
	}
	require.NoError(t, result.dispatchErr)
	require.Equal(t, ws.MessageTypeError, result.frame)
	require.Equal(t, ws.ErrorCodeForbidden, result.payload.Code)
	require.Equal(t, "insufficient permissions for this action", result.payload.Message)
}

func requireDeleteNotFound(t *testing.T, transport string, result workspaceDeleteResult) {
	t.Helper()
	if transport == "HTTP" {
		require.Equal(t, http.StatusNotFound, result.status)
		require.JSONEq(t, `{"error":"workspace not found"}`, result.body)
		return
	}
	require.NoError(t, result.dispatchErr)
	require.Equal(t, ws.MessageTypeError, result.frame)
	require.Equal(t, ws.ErrorCodeNotFound, result.payload.Code)
	require.Equal(t, "Workspace not found", result.payload.Message)
}

func requireDeleteValidation(t *testing.T, transport string, result workspaceDeleteResult, message string) {
	t.Helper()
	if transport == "HTTP" {
		require.Equal(t, http.StatusBadRequest, result.status)
		require.Contains(t, result.body, message)
		return
	}
	require.NoError(t, result.dispatchErr)
	require.Equal(t, ws.MessageTypeError, result.frame)
	require.Equal(t, ws.ErrorCodeValidation, result.payload.Code)
	require.Equal(t, message, result.payload.Message)
}

func (f *workspaceDeleteFixture) requireNoDeleteProgress(t *testing.T, workspaceID string) {
	t.Helper()
	require.Zero(t, f.tasks.listCalls.Load(), "denied or invalid delete must not enter deletion")
	canvasWorkspaceIDs, canvasTaskIDs := f.canvas.calls()
	require.Empty(t, canvasWorkspaceIDs, "denied or invalid delete must not clean canvases")
	require.Empty(t, canvasTaskIDs, "denied or invalid delete must not clean task canvases")
	require.Empty(t, f.secrets.calls(), "denied or invalid delete must not delete secrets")
	require.Equal(t, f.cleanupJobs, f.snapshotCleanupJobs(t), "denied or invalid delete must not create cleanup jobs")
	require.Empty(t, f.eventsOfType(events.WorkspaceDeleted), "denied or invalid delete must not publish workspace.deleted")
	require.Empty(t, f.eventsOfType(events.WorkflowDeleted), "denied or invalid delete must not publish workflow.deleted")
	require.Empty(t, f.eventsOfType(events.TaskDeleted), "denied or invalid delete must not publish task.deleted")
	if workspaceID != "" {
		require.Equal(t, 1, f.countRows(t, "workspaces", "id", workspaceID))
	}
}

func (f *workspaceDeleteFixture) requireDependentRowsRetained(t *testing.T, workspaceID string) {
	t.Helper()
	require.Equal(t, 1, f.countRows(t, "workspaces", "id", workspaceID))
	require.Greater(t, f.countRows(t, "workflows", "workspace_id", workspaceID), 0)
	require.Greater(t, f.countRows(t, "tasks", "workspace_id", workspaceID), 0)
	require.Greater(t, f.countRows(t, "task_sessions", "task_id", "task-"+workspaceID), 0)
	require.Greater(t, f.countRows(t, "workspace_members", "workspace_id", workspaceID), 0)
}

func (f *workspaceDeleteFixture) eventsOfType(eventType string) []*bus.Event {
	f.eventMu.Lock()
	defer f.eventMu.Unlock()
	matched := make([]*bus.Event, 0, 1)
	for _, event := range f.observed {
		if event.Type == eventType {
			matched = append(matched, event)
		}
	}
	return matched
}

func (f *workspaceDeleteFixture) requireDeleteEvent(t *testing.T, eventType, id string) {
	t.Helper()
	require.Eventually(t, func() bool {
		for _, event := range f.eventsOfType(eventType) {
			data, ok := event.Data.(map[string]interface{})
			if ok && (data["id"] == id || data["task_id"] == id) {
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond, "expected %s for %s", eventType, id)
}

func requireDeleteSuccess(t *testing.T, transport string, result workspaceDeleteResult) {
	t.Helper()
	if transport == "HTTP" {
		require.Equal(t, http.StatusOK, result.status)
		require.JSONEq(t, `{"success":true}`, result.body)
		return
	}
	require.NoError(t, result.dispatchErr)
	require.Equal(t, ws.MessageTypeResponse, result.frame)
}

// @covers AC-WORKSPACES-DELETION-001.9, AC-WORKSPACES-DELETION-001.10
func TestWorkspaceDeleteAuthorizationIntegration(t *testing.T) {
	for _, transport := range []string{"HTTP", "WS"} {
		t.Run("direct collaborator/"+transport, func(t *testing.T) {
			f := newWorkspaceDeleteFixture(t)
			workspace := f.createWorkspace(t, "ws-direct", "Direct", "user-owner", "", "")
			f.addMember(t, workspace.ID, "user-collab", string(authz.WorkspaceRoleCollaborator))
			ctx := deleteIdentityContext("user-collab", authn.RoleMember)
			f.requireDecision(t, ctx, workspace.ID, true, false)
			f.resetCounters()

			result := f.delete(t, ctx, transport, workspace.ID, workspace.Name)

			requireDeleteDenied(t, transport, result)
			require.Equal(t, int64(1), f.workspaces.getCalls.Load())
			f.requireNoDeleteProgress(t, workspace.ID)
			require.Equal(t, 1, f.countRows(t, "workspace_members", "workspace_id", workspace.ID))
		})

		t.Run("inherited collaborator/"+transport, func(t *testing.T) {
			f := newWorkspaceDeleteFixture(t)
			root, err := f.units.EnsureRoot(t.Context(), "", "Test org")
			require.NoError(t, err)
			team, err := f.units.Create(t.Context(), root.ID, "Platform")
			require.NoError(t, err)
			require.NoError(t, f.units.SetMember(t.Context(), team.ID, "user-unit", "collaborator", "user-owner"))
			workspace := f.createWorkspace(t, "ws-unit", "Unit", "user-owner", team.ID, "")
			ctx := deleteIdentityContext("user-unit", authn.RoleMember)
			f.requireDecision(t, ctx, workspace.ID, true, false)
			f.resetCounters()

			result := f.delete(t, ctx, transport, workspace.ID, workspace.Name)

			requireDeleteDenied(t, transport, result)
			f.requireNoDeleteProgress(t, workspace.ID)
		})

		t.Run("viewer wrong name with dependents/"+transport, func(t *testing.T) {
			f := newWorkspaceDeleteFixture(t)
			workspace := f.createWorkspace(t, "ws-viewer", "Viewer", "user-owner", "", "")
			f.seedDependentRows(t, workspace.ID)
			f.addMember(t, workspace.ID, "user-viewer", string(authz.WorkspaceRoleViewer))
			ctx := deleteIdentityContext("user-viewer", authn.RoleMember)
			f.requireDecision(t, ctx, workspace.ID, true, false)
			f.resetCounters()

			result := f.delete(t, ctx, transport, workspace.ID, "Wrong Name")

			requireDeleteDenied(t, transport, result)
			f.requireNoDeleteProgress(t, workspace.ID)
			f.requireDependentRowsRetained(t, workspace.ID)
		})

		t.Run("hidden and missing parity/"+transport, func(t *testing.T) {
			f := newWorkspaceDeleteFixture(t)
			workspace := f.createWorkspace(t, "ws-private", "Secret Name", "user-owner", "", "")
			ctx := deleteIdentityContext("user-outsider", authn.RoleMember)
			f.requireDecision(t, ctx, workspace.ID, false, false)
			f.resetCounters()

			hidden := f.delete(t, ctx, transport, workspace.ID, workspace.Name)
			missing := f.delete(t, ctx, transport, "ws-missing", "Secret Name")

			requireDeleteNotFound(t, transport, hidden)
			requireDeleteNotFound(t, transport, missing)
			require.NotContains(t, hidden.body, workspace.Name)
			require.NotContains(t, hidden.body, workspace.ID)
			wsPayload, err := json.Marshal(hidden.payload)
			require.NoError(t, err)
			require.NotContains(t, string(wsPayload), workspace.Name)
			require.NotContains(t, string(wsPayload), workspace.ID)
			f.requireNoDeleteProgress(t, workspace.ID)
		})

		t.Run("admin nonmember/"+transport, func(t *testing.T) {
			f := newWorkspaceDeleteFixture(t)
			workspace := f.createWorkspace(t, "ws-admin-private", "Private", "user-owner", "", "")
			ctx := deleteIdentityContext("user-admin", authn.RoleAdmin)
			f.requireDecision(t, ctx, workspace.ID, false, false)
			f.resetCounters()

			result := f.delete(t, ctx, transport, workspace.ID, workspace.Name)

			requireDeleteNotFound(t, transport, result)
			f.requireNoDeleteProgress(t, workspace.ID)
		})

		t.Run("foreign organization/"+transport, func(t *testing.T) {
			f := newWorkspaceDeleteFixture(t)
			workspace := f.createWorkspace(t, "ws-foreign-org", "Foreign", "user-owner", "", "org-a")
			ctx := authn.WithIdentity(context.Background(), authn.Identity{
				UserID: "user-outsider", Role: authn.RoleMember, OrgID: "org-b",
			})
			f.requireDecision(t, ctx, workspace.ID, false, false)
			f.resetCounters()

			result := f.delete(t, ctx, transport, workspace.ID, workspace.Name)

			requireDeleteNotFound(t, transport, result)
			f.requireNoDeleteProgress(t, workspace.ID)
		})
	}
}

// @covers AC-WORKSPACES-DELETION-001.2, AC-WORKSPACES-DELETION-001.9, AC-WORKSPACES-DELETION-001.10
func TestWorkspaceDeleteValidationIntegration(t *testing.T) {
	f := newWorkspaceDeleteFixture(t)
	workspace := f.createWorkspace(t, "ws-validation", "Validation", "user-owner", "", "")
	f.addMember(t, workspace.ID, "user-collab", string(authz.WorkspaceRoleCollaborator))
	owner := deleteIdentityContext("user-owner", authn.RoleMember)
	collaborator := deleteIdentityContext("user-collab", authn.RoleMember)
	outsider := deleteIdentityContext("user-outsider", authn.RoleMember)

	for _, tc := range []struct {
		name      string
		transport string
		run       func() workspaceDeleteResult
		message   string
	}{
		{"HTTP malformed payload", "HTTP", func() workspaceDeleteResult {
			return f.deleteHTTPPayload(owner, workspace.ID, `{`)
		}, "invalid payload"},
		{"HTTP empty confirm name", "HTTP", func() workspaceDeleteResult {
			return f.deleteHTTPPayload(owner, workspace.ID, `{"confirm_name":""}`)
		}, "confirm_name is required"},
		{"WS empty id", "WS", func() workspaceDeleteResult {
			return f.deleteWSRequest(t, owner, ws.ActionWorkspaceDelete, map[string]string{"confirm_name": workspace.Name})
		}, "id is required"},
		{"WS empty confirm name", "WS", func() workspaceDeleteResult {
			return f.deleteWSRequest(t, owner, ws.ActionWorkspaceDelete, map[string]string{"id": workspace.ID})
		}, "confirm_name is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.resetCounters()
			result := tc.run()
			requireDeleteValidation(t, tc.transport, result, tc.message)
			require.Zero(t, f.workspaces.getCalls.Load(), "payload validation must precede the workspace read")
			f.requireNoDeleteProgress(t, workspace.ID)
		})
	}

	t.Run("authorized wrong name", func(t *testing.T) {
		f.resetCounters()
		result := f.deleteHTTPPayload(owner, workspace.ID, `{"confirm_name":"Wrong"}`)
		requireDeleteValidation(t, "HTTP", result, service.ErrWorkspaceConfirmNameMismatch.Error())
		require.Zero(t, f.tasks.listCalls.Load(), "name comparison must precede deletion")
		f.requireNoDeleteProgress(t, workspace.ID)
	})

	t.Run("readable non-manager wrong name", func(t *testing.T) {
		f.resetCounters()
		for _, transport := range []string{"HTTP", "WS"} {
			result := f.delete(t, collaborator, transport, workspace.ID, "Wrong")
			requireDeleteDenied(t, transport, result)
		}
		require.Zero(t, f.tasks.listCalls.Load(), "management denial must precede name comparison")
		f.requireNoDeleteProgress(t, workspace.ID)
	})

	t.Run("unreadable wrong name", func(t *testing.T) {
		f.resetCounters()
		for _, transport := range []string{"HTTP", "WS"} {
			result := f.delete(t, outsider, transport, workspace.ID, "Wrong")
			requireDeleteNotFound(t, transport, result)
		}
		require.Zero(t, f.tasks.listCalls.Load(), "reach denial must precede name comparison")
		f.requireNoDeleteProgress(t, workspace.ID)
	})
}

// @covers AC-WORKSPACES-DELETION-001.2, AC-WORKSPACES-DELETION-001.3, AC-WORKSPACES-DELETION-001.4, AC-WORKSPACES-DELETION-001.5
func TestWorkspaceDeleteSuccessIntegration(t *testing.T) {
	t.Run("HTTP owner with dependents", func(t *testing.T) {
		f := newWorkspaceDeleteFixture(t)
		workspace := f.createWorkspace(t, "ws-owner", "Owned", "user-owner", "", "")
		workflowID, taskID, sessionID := f.seedDependentRows(t, workspace.ID)
		ctx := deleteIdentityContext("user-owner", authn.RoleMember)
		f.requireDecision(t, ctx, workspace.ID, true, true)
		f.resetCounters()

		result := f.delete(t, ctx, "HTTP", workspace.ID, workspace.Name)

		requireDeleteSuccess(t, "HTTP", result)
		require.Equal(t, 0, f.countRows(t, "workspaces", "id", workspace.ID))
		require.Equal(t, 0, f.countRows(t, "workflows", "id", workflowID))
		require.Equal(t, 0, f.countRows(t, "tasks", "id", taskID))
		require.Equal(t, 0, f.countRows(t, "task_sessions", "id", sessionID))
		canvasWorkspaceIDs, _ := f.canvas.calls()
		require.Equal(t, []string{workspace.ID}, canvasWorkspaceIDs)
		require.Equal(t, []string{workspace.ID}, f.secrets.calls())
		f.requireDeleteEvent(t, events.WorkspaceDeleted, workspace.ID)
		f.requireDeleteEvent(t, events.WorkflowDeleted, workflowID)
		f.requireDeleteEvent(t, events.TaskDeleted, taskID)
	})

	t.Run("WS admin owner", func(t *testing.T) {
		f := newWorkspaceDeleteFixture(t)
		workspace := f.createWorkspace(t, "ws-admin-owner", "Admin Owned", "user-admin", "", "")
		f.addMember(t, workspace.ID, "user-admin", string(authz.WorkspaceRoleOwner))
		ctx := deleteIdentityContext("user-admin", authn.RoleAdmin)
		f.requireDecision(t, ctx, workspace.ID, true, true)
		f.resetCounters()

		result := f.delete(t, ctx, "WS", workspace.ID, workspace.Name)

		requireDeleteSuccess(t, "WS", result)
		require.Equal(t, 0, f.countRows(t, "workspaces", "id", workspace.ID))
		require.Equal(t, []string{workspace.ID}, f.secrets.calls())
		f.requireDeleteEvent(t, events.WorkspaceDeleted, workspace.ID)
	})

	t.Run("internal synthetic identity", func(t *testing.T) {
		f := newWorkspaceDeleteFixture(t)
		workspace := f.createWorkspace(t, "ws-synthetic", "Synthetic", "", "", "")
		ctx := syntheticDeleteIdentityContext()
		f.requireDecision(t, ctx, workspace.ID, true, true)
		f.resetCounters()

		result := f.delete(t, ctx, "HTTP", workspace.ID, workspace.Name)

		requireDeleteSuccess(t, "HTTP", result)
		require.Equal(t, 0, f.countRows(t, "workspaces", "id", workspace.ID))
		f.requireDeleteEvent(t, events.WorkspaceDeleted, workspace.ID)
	})
}

// @covers AC-WORKSPACES-DELETION-001.11
func TestWorkspaceDeleteStorageFailureIntegration(t *testing.T) {
	f := newWorkspaceDeleteFixture(t)
	workspace := f.createWorkspace(t, "ws-storage", "Storage", "user-owner", "", "")
	workflowID, taskID, sessionID := f.seedDependentRows(t, workspace.ID)
	ctx := deleteIdentityContext("user-owner", authn.RoleMember)
	f.requireDecision(t, ctx, workspace.ID, true, true)
	_, err := f.database.ExecContext(t.Context(), `
		CREATE TRIGGER reject_workspace_delete
		BEFORE DELETE ON workspaces
		BEGIN
			SELECT RAISE(ABORT, 'cascade failure: workspace metadata not found');
		END
	`)
	require.NoError(t, err)

	for _, transport := range []string{"HTTP", "WS"} {
		t.Run(transport, func(t *testing.T) {
			f.resetCounters()
			result := f.delete(t, ctx, transport, workspace.ID, workspace.Name)

			if transport == "HTTP" {
				require.Equal(t, http.StatusInternalServerError, result.status)
				require.JSONEq(t, `{"error":"workspace not deleted"}`, result.body)
			} else {
				require.NoError(t, result.dispatchErr)
				require.Equal(t, ws.MessageTypeError, result.frame)
				require.Equal(t, ws.ErrorCodeInternalError, result.payload.Code)
				require.Equal(t, "failed to delete workspace", result.payload.Message)
			}
			require.Equal(t, 1, f.countRows(t, "workspaces", "id", workspace.ID))
			require.Equal(t, 1, f.countRows(t, "workflows", "id", workflowID))
			require.Equal(t, 1, f.countRows(t, "tasks", "id", taskID))
			require.Equal(t, 1, f.countRows(t, "task_sessions", "id", sessionID))
			require.Empty(t, f.secrets.calls(), "secret cleanup runs only after the cascade commits")
			require.Empty(t, f.eventsOfType(events.WorkspaceDeleted))
			require.Empty(t, f.eventsOfType(events.WorkflowDeleted))
			require.Empty(t, f.eventsOfType(events.TaskDeleted))
		})
	}

	_, err = f.database.ExecContext(t.Context(), `DROP TRIGGER reject_workspace_delete`)
	require.NoError(t, err)
	result := f.delete(t, ctx, "HTTP", workspace.ID, workspace.Name)
	requireDeleteSuccess(t, "HTTP", result)
	require.Equal(t, 0, f.countRows(t, "workspaces", "id", workspace.ID))
	require.Equal(t, []string{workspace.ID}, f.secrets.calls())
}
