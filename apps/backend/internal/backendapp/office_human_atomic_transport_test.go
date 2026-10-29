package backendapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/dashboard"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/hierarchy"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type officeAtomicFixture struct {
	h         *pluginMoveErrorBindingHarness
	svc       *taskservice.Service
	office    *officesqlite.Repository
	workflow  *workflowrepo.Repository
	dashboard *dashboard.DashboardService
	log       *logger.Logger
}

func newOfficeAtomicFixture(t *testing.T, wrap func(*tasksqlite.Repository) taskrepo.TaskRepository) officeAtomicFixture {
	gin.SetMode(gin.TestMode)
	t.Helper()
	h := newOfficeAtomicHarness(t)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	office, err := officesqlite.NewWithDB(h.db, h.db, log)
	require.NoError(t, err)
	workflow, err := workflowrepo.NewWithDB(h.db, h.db, log)
	require.NoError(t, err)
	tasks := taskrepo.TaskRepository(h.repo)
	if wrap != nil {
		tasks = wrap(h.repo)
	}
	svc := taskservice.NewService(taskservice.Repos{Tasks: tasks, Workspaces: h.repo, Workflows: h.repo, Sessions: h.repo, Reviews: h.repo, Messages: h.repo, Turns: h.repo, TaskRepos: h.repo, GitSnapshots: h.repo, RepoEntities: h.repo, Executors: h.repo, Environments: h.repo, TaskEnvironments: h.repo}, h.eventBus, log, taskservice.RepositoryDiscoveryConfig{})
	svc.SetWorkflowStepGetter(&workflowStepGetterAdapter{svc: h.workflowSvc})
	svc.SetStepHistoryRecorder(h.workflowSvc)
	dash := dashboard.NewDashboardService(office, log, nil, nil, nil)
	dash.SetDecisionStore(workflow)
	dash.SetTaskFinalStepMover(officeFinalStepMoverAdapter{tasks: svc, steps: h.workflowSvc})
	dash.SetTaskHumanDoneCompleter(officeHumanDoneCompleterAdapter{tasks: svc})
	dash.SetEventBus(h.eventBus)
	svc.SetOfficeApprovalGuard(officeApprovalGuardAdapter{reader: dash})
	configureOfficeHumanCompletionGuard(h.repo, office, workflow)
	require.NoError(t, h.workflowSvc.CreateStep(context.Background(), &wfmodels.WorkflowStep{ID: "atomic-done", WorkflowID: "wf-home", Name: "Done", Position: 10, CompleteTaskOnEnter: true}))
	h.createTask(t, "atomic-task", false)
	_, err = h.db.Exec(`UPDATE workspaces SET office_workflow_id='wf-home' WHERE id='ws-1'`)
	require.NoError(t, err)
	return officeAtomicFixture{h, svc, office, workflow, dash, log}
}

// @covers AC-OFFICE-HUMAN-DONE-002.1 AC-OFFICE-HUMAN-DONE-002.2
func TestOfficeHumanCompletionRegisteredRefusalAndSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, route := range []string{"HTTP state", "WS state", "WS direct state", "HTTP approve", "HTTP bulk selected", "HTTP bulk workflow", "Office current final", "Office final move"} {
		t.Run(route, func(t *testing.T) {
			f := newOfficeAtomicFixture(t, nil)
			ctx := context.Background()
			if route == "Office current final" {
				task, err := f.h.repo.GetTask(ctx, "atomic-task")
				require.NoError(t, err)
				task.WorkflowStepID = "atomic-done"
				require.NoError(t, f.h.repo.UpdateTask(ctx, task))
			}
			require.NoError(t, f.h.repo.CreateTaskSession(ctx, &models.TaskSession{ID: "atomic-session", TaskID: "atomic-task", State: models.TaskSessionStateWaitingForInput, IsPrimary: true, ReviewStatus: models.ReviewStatusPending}))
			_, err := f.workflow.UpsertTaskParticipant(ctx, "step-home", "atomic-task", "approver", "pending-agent")
			require.NoError(t, err)
			router, dispatcher := gin.New(), ws.NewDispatcher()
			taskhandlers.RegisterTaskRoutes(router, dispatcher, f.svc, nil, f.h.repo, nil, f.log)
			dashboard.RegisterRoutes(router.Group("/api/v1/office"), f.dashboard, nil, nil, nil, nil, f.log)
			invoke := func() (int, map[string]any) {
				if strings.HasPrefix(route, "WS ") {
					action := ws.ActionTaskUpdate
					if route == "WS direct state" {
						action = ws.ActionTaskState
					}
					req, err := ws.NewRequest("atomic-state", action, map[string]any{"id": "atomic-task", "state": "COMPLETED"})
					require.NoError(t, err)
					response, err := dispatcher.Dispatch(ctx, req)
					require.NoError(t, err)
					var body map[string]any
					require.NoError(t, response.ParsePayload(&body))
					if response.Type == ws.MessageTypeResponse {
						return http.StatusOK, body
					}
					if body["code"] == string(ws.ErrorCodeConflict) {
						return http.StatusConflict, body
					}
					return http.StatusInternalServerError, body
				}
				method, path, payload := http.MethodPatch, "/api/v1/tasks/atomic-task", `{"state":"COMPLETED"}`
				if strings.HasPrefix(route, "Office") {
					path = "/api/v1/office/tasks/atomic-task"
					payload = `{"status":"done"}`
				}
				if strings.HasPrefix(route, "HTTP bulk") {
					method = http.MethodPost
					path = "/api/v1/tasks/bulk-move"
					if route == "HTTP bulk selected" {
						payload = `{"task_ids":["atomic-task"],"target_workflow_id":"wf-home","target_step_id":"atomic-done"}`
					} else {
						payload = `{"source_workflow_id":"wf-home","source_step_id":"step-home","target_workflow_id":"wf-home","target_step_id":"atomic-done"}`
					}
				}
				if route == "HTTP approve" {
					method = http.MethodPost
					path = "/api/v1/sessions/atomic-session/approve"
					payload = `{}`
				}
				request := httptest.NewRequest(method, path, strings.NewReader(payload)).WithContext(ctx)
				request.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, request)
				var body map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				return rec.Code, body
			}
			before, err := f.h.repo.GetTask(ctx, "atomic-task")
			require.NoError(t, err)
			session, err := f.h.repo.GetTaskSession(ctx, "atomic-session")
			require.NoError(t, err)
			beforeLedger, err := f.h.repo.GetLatestTaskStepTransitionID(ctx, "atomic-task")
			require.NoError(t, err)
			effects := officeAtomicDurableEffects(t, f)
			published := officeAtomicPublishedEvents(t, f)
			status, body := invoke()
			require.Equal(t, http.StatusConflict, status, "body=%v", body)
			require.Equal(t, effects, officeAtomicDurableEffects(t, f), "refusal has no persisted effects")
			require.Empty(t, published(), "refusal publishes no effects")
			after, err := f.h.repo.GetTask(ctx, "atomic-task")
			require.NoError(t, err)
			require.Equal(t, before, after)
			afterSession, err := f.h.repo.GetTaskSession(ctx, "atomic-session")
			require.NoError(t, err)
			require.Equal(t, session, afterSession)
			afterLedger, err := f.h.repo.GetLatestTaskStepTransitionID(ctx, "atomic-task")
			require.NoError(t, err)
			require.Equal(t, beforeLedger, afterLedger)
			if strings.HasPrefix(route, "Office") {
				require.NotContains(t, body, "status")
			} else if !strings.HasPrefix(route, "WS ") {
				require.Equal(t, "task_approvals_pending", body["code"])
			}
			require.NoError(t, f.workflow.DeleteTaskParticipant(ctx, "step-home", "atomic-task", "approver", "pending-agent"))
			status, body = invoke()
			require.Equal(t, http.StatusOK, status, "same-fixture control body=%v", body)
			completed, err := f.h.repo.GetTask(ctx, "atomic-task")
			require.NoError(t, err)
			require.Equal(t, v1.TaskStateCompleted, completed.State)
			require.NotEmpty(t, published(), "same-fixture success reaches event publication")
		})
	}
}

type officeFieldAdmissionWindow struct {
	*tasksqlite.Repository
	before func()
}

func (r *officeFieldAdmissionWindow) UpdateTaskFieldsWithParentAdmission(ctx context.Context, id string, update models.TaskFieldUpdate, validate hierarchy.TaskParentValidator) (*models.TaskFieldUpdateResult, error) {
	r.before()
	return r.Repository.UpdateTaskFieldsWithParentAdmission(ctx, id, update, validate)
}
func TestOfficeHumanCompletionRegisteredScopeConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, transport := range []string{"HTTP", "WS"} {
		t.Run(transport, func(t *testing.T) {
			var window *officeFieldAdmissionWindow
			f := newOfficeAtomicFixture(t, func(r *tasksqlite.Repository) taskrepo.TaskRepository {
				window = &officeFieldAdmissionWindow{Repository: r}
				return window
			})
			window.before = func() {
				task, err := f.h.repo.GetTask(context.Background(), "atomic-task")
				require.NoError(t, err)
				task.WorkflowStepID = "atomic-done"
				require.NoError(t, f.h.repo.UpdateTask(context.Background(), task))
			}
			router, dispatcher := gin.New(), ws.NewDispatcher()
			taskhandlers.RegisterTaskRoutes(router, dispatcher, f.svc, nil, f.h.repo, nil, f.log)
			if transport == "HTTP" {
				request := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/atomic-task", strings.NewReader(`{"state":"COMPLETED"}`))
				request.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, request)
				require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
			} else {
				request, err := ws.NewRequest("scope", ws.ActionTaskUpdate, map[string]any{"id": "atomic-task", "state": "COMPLETED"})
				require.NoError(t, err)
				response, err := dispatcher.Dispatch(context.Background(), request)
				require.NoError(t, err)
				var payload ws.ErrorPayload
				require.NoError(t, response.ParsePayload(&payload))
				require.Equal(t, ws.ErrorCodeConflict, payload.Code)
				require.Equal(t, "workflow_change_conflict", payload.Details["error_code"])
			}
			task, err := f.h.repo.GetTask(context.Background(), "atomic-task")
			require.NoError(t, err)
			require.Equal(t, "atomic-done", task.WorkflowStepID)
			require.Equal(t, v1.TaskStateTODO, task.State)
		})
	}
}

// @covers AC-OFFICE-HUMAN-DONE-002.5
func TestOfficeHumanCompletionFreshProjectMembership(t *testing.T) {
	for _, initial := range []string{"", "old-project"} {
		t.Run("initial="+initial, func(t *testing.T) {
			var window *officeAdmissionWindow
			f := newOfficeAtomicFixture(t, func(r *tasksqlite.Repository) taskrepo.TaskRepository {
				window = &officeAdmissionWindow{Repository: r}
				return window
			})
			ctx := context.Background()
			_, err := f.h.db.Exec(`UPDATE workspaces SET office_workflow_id='' WHERE id='ws-1'`)
			require.NoError(t, err)
			require.NoError(t, f.office.UpdateTaskProjectID(ctx, "atomic-task", initial))
			next := "new-project"
			if initial != "" {
				next = ""
			}
			window.before = func() { require.NoError(t, f.office.UpdateTaskProjectID(ctx, "atomic-task", next)) }
			_, err = f.svc.MoveTaskWithOptions(ctx, "atomic-task", "wf-home", "atomic-done", 0, taskservice.MoveTaskOptions{StepHistoryActor: wfmodels.StepTransitionActorHuman})
			require.NoError(t, err)
			after, err := f.h.repo.GetTask(ctx, "atomic-task")
			require.NoError(t, err)
			require.Equal(t, next, after.ProjectID, "completion must preserve the legitimate concurrent membership write")
			require.Equal(t, v1.TaskStateCompleted, after.State)
		})
	}
}

func TestOfficeHumanCompletionMissingWorkflowRefused(t *testing.T) {
	f := newOfficeAtomicFixture(t, nil)
	ctx := context.Background()
	require.NoError(t, f.h.repo.DeleteWorkflow(ctx, "wf-home"))
	before, err := f.h.repo.GetTask(ctx, "atomic-task")
	require.NoError(t, err)
	state := v1.TaskStateCompleted
	_, err = f.svc.UpdateTaskAsHuman(ctx, "atomic-task", &taskservice.UpdateTaskRequest{State: &state})
	require.ErrorIs(t, err, repoerrors.ErrWorkflowChangeConflict)
	after, err := f.h.repo.GetTask(ctx, "atomic-task")
	require.NoError(t, err)
	require.Equal(t, before, after)
}

// Ordinary field edits apply to the locked row without owning its placement.
func TestOfficeHumanContentEditPreservesConcurrentMove(t *testing.T) {
	for _, transport := range []string{"HTTP", "WS"} {
		t.Run(transport, func(t *testing.T) {
			var window *officeFieldAdmissionWindow
			f := newOfficeAtomicFixture(t, func(r *tasksqlite.Repository) taskrepo.TaskRepository {
				window = &officeFieldAdmissionWindow{Repository: r}
				return window
			})
			ctx := context.Background()
			require.NoError(t, f.h.workflowSvc.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "content-destination", WorkflowID: "wf-home", Name: "Working", Position: 5}))
			var moved *models.Task
			var transition int64
			window.before = func() {
				var err error
				_, err = f.svc.MoveTaskWithOptions(ctx, "atomic-task", "wf-home", "content-destination", 0, taskservice.MoveTaskOptions{StepHistoryActor: wfmodels.StepTransitionActorSystem})
				require.NoError(t, err)
				moved, err = f.h.repo.GetTask(ctx, "atomic-task")
				require.NoError(t, err)
				transition, err = f.h.repo.GetLatestTaskStepTransitionID(ctx, "atomic-task")
				require.NoError(t, err)
			}
			router, dispatcher := gin.New(), ws.NewDispatcher()
			taskhandlers.RegisterTaskRoutes(router, dispatcher, f.svc, nil, f.h.repo, nil, f.log)
			if transport == "HTTP" {
				req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/atomic-task", strings.NewReader(`{"title":"Renamed","description":"Revised"}`))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			} else {
				req, err := ws.NewRequest("content", ws.ActionTaskUpdate, map[string]any{"id": "atomic-task", "title": "Renamed", "description": "Revised"})
				require.NoError(t, err)
				response, err := dispatcher.Dispatch(ctx, req)
				require.NoError(t, err)
				require.Equal(t, ws.MessageTypeResponse, response.Type, "%+v", response)
			}
			task, err := f.h.repo.GetTask(ctx, "atomic-task")
			require.NoError(t, err)
			require.Equal(t, "Renamed", task.Title)
			require.Equal(t, "Revised", task.Description)
			require.Equal(t, moved.WorkflowStepID, task.WorkflowStepID)
			require.Equal(t, moved.State, task.State)
			after, err := f.h.repo.GetLatestTaskStepTransitionID(ctx, "atomic-task")
			require.NoError(t, err)
			require.Equal(t, transition, after)
			var completions int
			require.NoError(t, f.h.db.QueryRowContext(ctx, `SELECT count(*) FROM task_completion_gate_history WHERE task_id='atomic-task'`).Scan(&completions))
			require.Zero(t, completions)
		})
	}
}

func TestOfficeHumanContentEditRejectsForeignWorkspace(t *testing.T) {
	for _, transport := range []string{"HTTP", "WS"} {
		t.Run(transport, func(t *testing.T) {
			var window *officeFieldAdmissionWindow
			f := newOfficeAtomicFixture(t, func(r *tasksqlite.Repository) taskrepo.TaskRepository {
				window = &officeFieldAdmissionWindow{Repository: r}
				return window
			})
			ctx := context.Background()
			var moved *models.Task
			window.before = func() {
				var err error
				moved, err = f.h.repo.GetTask(ctx, "atomic-task")
				require.NoError(t, err)
				moved.WorkspaceID = "ws-2"
				moved.WorkflowID = "wf-other-workspace"
				moved.WorkflowStepID = ""
				require.NoError(t, f.h.repo.UpdateTask(ctx, moved))
				moved, err = f.h.repo.GetTask(ctx, "atomic-task")
				require.NoError(t, err)
			}
			router, dispatcher := gin.New(), ws.NewDispatcher()
			taskhandlers.RegisterTaskRoutes(router, dispatcher, f.svc, nil, f.h.repo, nil, f.log)
			if transport == "HTTP" {
				req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/atomic-task", strings.NewReader(`{"title":"Forbidden edit"}`))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
			} else {
				req, err := ws.NewRequest("foreign-content", ws.ActionTaskUpdate, map[string]any{"id": "atomic-task", "title": "Forbidden edit"})
				require.NoError(t, err)
				response, err := dispatcher.Dispatch(ctx, req)
				require.NoError(t, err)
				var payload ws.ErrorPayload
				require.NoError(t, response.ParsePayload(&payload))
				require.Equal(t, ws.ErrorCodeConflict, payload.Code)
			}
			task, err := f.h.repo.GetTask(ctx, "atomic-task")
			require.NoError(t, err)
			require.Equal(t, moved, task)
		})
	}
}
