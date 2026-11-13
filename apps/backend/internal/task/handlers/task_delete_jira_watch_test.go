package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/jira"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const jiraWatchRecoveryWorkspaceID = "ws-jira-recovery"
const jiraWatchRecoveryWorkflowID = "wf-jira-recovery"
const jiraWatchRecoveryStepID = "step-jira-recovery"

type jiraWatchRecoverySecrets struct{}

func (jiraWatchRecoverySecrets) Reveal(context.Context, string) (string, error) { return "token", nil }
func (jiraWatchRecoverySecrets) Set(context.Context, string, string, string) error {
	return nil
}
func (jiraWatchRecoverySecrets) Delete(context.Context, string) error         { return nil }
func (jiraWatchRecoverySecrets) Exists(context.Context, string) (bool, error) { return true, nil }

type jiraWatchRecoveryStepGetter struct {
	workflowID string
	stepID     string
}

func (g jiraWatchRecoveryStepGetter) GetStep(context.Context, string) (*wfmodels.WorkflowStep, error) {
	return &wfmodels.WorkflowStep{ID: g.stepID, WorkflowID: g.workflowID}, nil
}

func (jiraWatchRecoveryStepGetter) GetNextStepByPosition(context.Context, string, int) (*wfmodels.WorkflowStep, error) {
	return nil, nil
}

type jiraWatchRecoveryClient struct {
	tickets []jira.JiraTicket
}

func (c *jiraWatchRecoveryClient) TestAuth(context.Context) (*jira.TestConnectionResult, error) {
	return &jira.TestConnectionResult{OK: true}, nil
}

func (c *jiraWatchRecoveryClient) GetTicket(context.Context, string) (*jira.JiraTicket, error) {
	return nil, nil
}

func (c *jiraWatchRecoveryClient) ListTransitions(context.Context, string) ([]jira.JiraTransition, error) {
	return nil, nil
}

func (c *jiraWatchRecoveryClient) DoTransition(context.Context, string, string) error { return nil }
func (c *jiraWatchRecoveryClient) ListProjects(context.Context) ([]jira.JiraProject, error) {
	return nil, nil
}
func (c *jiraWatchRecoveryClient) ListProjectStatuses(context.Context, string) ([]jira.JiraStatus, error) {
	return nil, nil
}
func (c *jiraWatchRecoveryClient) SearchTickets(context.Context, string, string, int) (*jira.SearchResult, error) {
	return &jira.SearchResult{Tickets: c.tickets, IsLast: true}, nil
}
func (c *jiraWatchRecoveryClient) SearchTicketsForWatch(context.Context, string, string, int) (*jira.SearchResult, error) {
	return &jira.SearchResult{Tickets: c.tickets, IsLast: true}, nil
}

type jiraWatchRecoveryFixture struct {
	router    *gin.Engine
	db        *sqlx.DB
	taskRepo  *tasksqlite.Repository
	taskSvc   *service.Service
	jiraSvc   *jira.Service
	client    *jiraWatchRecoveryClient
	events    <-chan *jira.NewJiraIssueEvent
	workspace string
	workflow  string
	step      string
}

func newJiraWatchRecoveryFixture(t *testing.T) *jiraWatchRecoveryFixture {
	t.Helper()
	connection, err := db.OpenSQLite(filepath.Join(t.TempDir(), "jira-watch-recovery.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(connection, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	taskRepo, cleanup, err := taskrepository.Provide(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })

	ctx := context.Background()
	require.NoError(t, taskRepo.CreateWorkspace(ctx, &taskmodels.Workspace{
		ID: jiraWatchRecoveryWorkspaceID, Name: "Jira recovery",
	}))
	require.NoError(t, taskRepo.CreateWorkflow(ctx, &taskmodels.Workflow{
		ID: jiraWatchRecoveryWorkflowID, WorkspaceID: jiraWatchRecoveryWorkspaceID, Name: "Workflow",
	}))
	now := time.Now().UTC()
	_, err = sqlxDB.ExecContext(ctx, `INSERT INTO workflow_steps
		(id, workflow_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		jiraWatchRecoveryStepID, jiraWatchRecoveryWorkflowID, "Step", 0, now, now)
	require.NoError(t, err)

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	jiraStore, err := jira.NewStore(sqlxDB, sqlxDB)
	require.NoError(t, err)
	require.NoError(t, jiraStore.UpsertConfigForWorkspace(ctx, jiraWatchRecoveryWorkspaceID, &jira.JiraConfig{
		SiteURL: "https://a.net", Email: "e", AuthMethod: jira.AuthMethodAPIToken,
		InstanceType: jira.InstanceTypeCloud,
	}))
	client := &jiraWatchRecoveryClient{}
	jiraSvc := jira.NewService(jiraStore, jiraWatchRecoverySecrets{}, func(_ *jira.JiraConfig, _ string) jira.Client {
		return client
	}, log)
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	jiraSvc.SetEventBus(eventBus)
	eventChannel := make(chan *jira.NewJiraIssueEvent, 4)
	_, err = eventBus.Subscribe(events.JiraNewIssue, func(_ context.Context, event *bus.Event) error {
		if payload, ok := event.Data.(*jira.NewJiraIssueEvent); ok {
			eventChannel <- payload
		}
		return nil
	})
	require.NoError(t, err)

	taskSvc := service.NewService(service.Repos{
		Workspaces: taskRepo, Tasks: taskRepo, TaskRepos: taskRepo,
		Workflows: taskRepo, Messages: taskRepo,
		Attachments: taskRepo, Turns: taskRepo, Sessions: taskRepo,
		GitSnapshots: taskRepo, RepoEntities: taskRepo, Executors: taskRepo,
		Environments: taskRepo, TaskEnvironments: taskRepo, Reviews: taskRepo,
		ResourceCleanups: taskRepo, StatusSummaries: taskRepo, TaskActivity: taskRepo,
		SubagentContexts: taskRepo, Usage: taskRepo, BackgroundWork: taskRepo,
		RecoveryOperations: taskRepo,
	}, eventBus, log, service.RepositoryDiscoveryConfig{})
	taskSvc.SetWorktreeCleanup(authzDeleteCleanup{})
	taskSvc.SetWorkflowStepGetter(jiraWatchRecoveryStepGetter{
		workflowID: jiraWatchRecoveryWorkflowID, stepID: jiraWatchRecoveryStepID,
	})

	router := gin.New()
	handler := &TaskHandlers{service: taskSvc, repo: taskRepo, logger: log}
	handler.registerHTTP(router)
	jira.RegisterRoutes(router, ws.NewDispatcher(), jiraSvc, log)
	return &jiraWatchRecoveryFixture{
		router: router, db: sqlxDB, taskRepo: taskRepo, taskSvc: taskSvc, jiraSvc: jiraSvc,
		client: client, events: eventChannel, workspace: jiraWatchRecoveryWorkspaceID,
		workflow: jiraWatchRecoveryWorkflowID, step: jiraWatchRecoveryStepID,
	}
}

func (f *jiraWatchRecoveryFixture) createTask(t *testing.T, id, title string) {
	t.Helper()
	require.NoError(t, f.taskRepo.CreateTask(context.Background(), &taskmodels.Task{
		ID: id, WorkspaceID: f.workspace, WorkflowID: f.workflow,
		WorkflowStepID: f.step, Title: title,
	}))
}

func (f *jiraWatchRecoveryFixture) createWatch(t *testing.T) *jira.IssueWatch {
	t.Helper()
	watch, err := f.jiraSvc.CreateIssueWatch(context.Background(), &jira.CreateIssueWatchRequest{
		WorkspaceID: f.workspace, WorkflowID: f.workflow, WorkflowStepID: f.step,
		JQL: "project = PROJ",
	})
	require.NoError(t, err)
	return watch
}

func (f *jiraWatchRecoveryFixture) mapTask(t *testing.T, watchID, key, taskID string) {
	t.Helper()
	ctx := context.Background()
	reserved, err := f.jiraSvc.Store().ReserveIssueWatchTask(ctx, watchID, key, "https://a.net/browse/"+key)
	require.NoError(t, err)
	require.True(t, reserved)
	require.NoError(t, f.jiraSvc.Store().AssignIssueWatchTaskID(ctx, watchID, key, taskID))
}

func (f *jiraWatchRecoveryFixture) mappedTaskID(t *testing.T, watchID, key string) string {
	t.Helper()
	var taskID string
	require.NoError(t, f.db.GetContext(context.Background(), &taskID, f.db.Rebind(`
		SELECT task_id FROM jira_issue_watch_tasks WHERE issue_watch_id = ? AND issue_key = ?`),
		watchID, key))
	return taskID
}

func (f *jiraWatchRecoveryFixture) taskCount(t *testing.T) int {
	t.Helper()
	var count int
	require.NoError(t, f.db.GetContext(context.Background(), &count, f.db.Rebind(
		`SELECT COUNT(*) FROM tasks WHERE workspace_id = ?`), f.workspace))
	return count
}

func (f *jiraWatchRecoveryFixture) serve(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, withTaskDeleteTestIdentity(request))
	return recorder
}

func (f *jiraWatchRecoveryFixture) trigger(t *testing.T, watchID string) int {
	t.Helper()
	recorder := f.serve(t, http.MethodPost,
		"/api/v1/jira/watches/issue/"+watchID+"/trigger?workspace_id="+f.workspace, "")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response struct {
		NewIssues int `json:"newIssues"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	return response.NewIssues
}

func (f *jiraWatchRecoveryFixture) deleteTask(t *testing.T, taskID string) {
	t.Helper()
	preflight := f.serve(t, http.MethodPost, "/api/v1/tasks/delete-preflight",
		`{"task_ids":["`+taskID+`"]}`)
	require.Equal(t, http.StatusOK, preflight.Code, preflight.Body.String())
	var preview service.TaskDeletePreflightResult
	require.NoError(t, json.Unmarshal(preflight.Body.Bytes(), &preview))
	require.NotEmpty(t, preview.ConfirmationID)

	request := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/"+taskID, nil)
	request.Header.Set(taskDeleteConfirmationHeader, preview.ConfirmationID)
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, withTaskDeleteTestIdentity(request))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
}

func (f *jiraWatchRecoveryFixture) awaitEvent(t *testing.T) *jira.NewJiraIssueEvent {
	t.Helper()
	select {
	case event := <-f.events:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Jira issue publication")
		return nil
	}
}

func TestHTTPDeleteTask_JiraWatchRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	f := newJiraWatchRecoveryFixture(t)
	ctx := context.Background()
	f.createTask(t, "task-live", "Live task")
	f.createTask(t, "task-archived", "Archived task")
	changed, err := f.taskRepo.ArchiveTaskIfActive(ctx, "task-archived", "")
	require.NoError(t, err)
	require.True(t, changed)

	watch := f.createWatch(t)
	f.mapTask(t, watch.ID, "PROJ-1", "task-live")
	f.mapTask(t, watch.ID, "PROJ-2", "task-archived")
	require.Equal(t, "task-live", f.mappedTaskID(t, watch.ID, "PROJ-1"))
	require.Equal(t, "task-archived", f.mappedTaskID(t, watch.ID, "PROJ-2"))
	f.client.tickets = []jira.JiraTicket{
		{Key: "PROJ-1", Summary: "live", URL: "https://a.net/browse/PROJ-1"},
		{Key: "PROJ-2", Summary: "archived", URL: "https://a.net/browse/PROJ-2"},
	}
	require.Equal(t, 0, f.trigger(t, watch.ID), "existing and archived tasks must suppress dispatch")

	f.deleteTask(t, "task-live")
	_, err = f.taskRepo.GetTask(ctx, "task-live")
	require.Error(t, err, "native delete must remove the task row")
	require.Equal(t, 1, f.trigger(t, watch.ID), "deleted task must become eligible")
	event := f.awaitEvent(t)
	require.Equal(t, "PROJ-1", event.Issue.Key)
	require.Equal(t, watch.ID, event.IssueWatchID)

	reserved, err := f.jiraSvc.Store().ReserveIssueWatchTask(ctx, event.IssueWatchID, event.Issue.Key, event.Issue.URL)
	require.NoError(t, err)
	require.True(t, reserved)
	created, err := f.taskSvc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: event.WorkspaceID, WorkflowID: event.WorkflowID,
		WorkflowStepID: event.WorkflowStepID, Title: "[PROJ-1] live",
	})
	require.NoError(t, err)
	require.NoError(t, f.jiraSvc.Store().AssignIssueWatchTaskID(
		ctx, event.IssueWatchID, event.Issue.Key, created.Task.ID))
	require.Equal(t, 0, f.trigger(t, watch.ID), "replacement task must suppress the next poll")
	require.Equal(t, created.Task.ID, f.mappedTaskID(t, watch.ID, "PROJ-1"))
	require.Equal(t, "task-archived", f.mappedTaskID(t, watch.ID, "PROJ-2"))
	require.Equal(t, 2, f.taskCount(t), "only the archived control and one replacement task must remain")

	archived, err := f.taskRepo.GetTask(ctx, "task-archived")
	require.NoError(t, err)
	require.NotNil(t, archived.ArchivedAt, "archived control must stay archived")
	replacement, err := f.taskRepo.GetTask(ctx, created.Task.ID)
	require.NoError(t, err)
	require.Nil(t, replacement.ArchivedAt)
}
