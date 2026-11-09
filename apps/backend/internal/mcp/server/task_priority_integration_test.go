package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	backendmcp "github.com/kandev/kandev/internal/mcp/handlers"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type externalTaskPriorityFixture struct {
	taskService *service.Service
	repository  *sqliterepo.Repository
	eventBus    *bus.MemoryEventBus
	logger      *logger.Logger
}

func newExternalTaskPriorityFixture(t *testing.T) *externalTaskPriorityFixture {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "mcp-task-priority.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	repo, cleanup, err := repository.Provide(database, database, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })
	_, err = worktree.NewSQLiteStore(database, database)
	require.NoError(t, err)

	log := newTestLogger(t)
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(func() { eventBus.Close() })
	taskService := service.NewService(service.Repos{
		Workspaces:       repo,
		Tasks:            repo,
		TaskRepos:        repo,
		WorkspaceFolders: repo,
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

	return &externalTaskPriorityFixture{taskService: taskService, repository: repo, eventBus: eventBus, logger: log}
}

func (f *externalTaskPriorityFixture) dispatcher() *ws.Dispatcher {
	dispatcher := ws.NewDispatcher()
	handlers := backendmcp.NewHandlers(
		f.taskService, nil, nil, nil, nil, f.repository, f.repository, f.eventBus,
		nil, nil, nil, nil, f.logger,
	)
	handlers.RegisterHandlers(dispatcher)
	return dispatcher
}

func (f *externalTaskPriorityFixture) httpClient(t *testing.T, identity authn.Identity) *externalTaskPriorityHTTPClient {
	t.Helper()
	server := NewExternal(NewExternalDispatcherBackendClient(f.dispatcher(), f.logger), f.logger, "")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, identity)
		c.Next()
	})
	server.RegisterBackendRoutes(router)
	transport := httptest.NewServer(router)
	t.Cleanup(transport.Close)
	return newExternalTaskPriorityHTTPClient(t, transport.URL+"/mcp")
}

func (f *externalTaskPriorityFixture) createWorkspace(t *testing.T, id, name, owner string) (*models.Workspace, string) {
	t.Helper()
	ctx := context.Background()
	workspace := &models.Workspace{ID: id, Name: name, OwnerID: owner}
	require.NoError(t, f.repository.CreateWorkspace(ctx, workspace))
	workflowID := id + "-workflow"
	require.NoError(t, f.repository.CreateWorkflow(ctx, &models.Workflow{
		ID: workflowID, WorkspaceID: id, Name: "Kanban",
	}))
	return workspace, workflowID
}

func TestExternalMCPTaskPriorityJourney(t *testing.T) {
	fixture := newExternalTaskPriorityFixture(t)
	ctx := context.Background()
	workspace, workflowID := fixture.createWorkspace(t, "priority-journey-workspace", "Priority journey", "priority-user")
	client := fixture.httpClient(t, authn.Identity{UserID: "priority-user", Role: authn.RoleMember})

	tools := client.request(t, "tools/list", map[string]interface{}{})
	listedTools := tools["result"].(map[string]interface{})["tools"].([]interface{})
	var names []string
	for _, item := range listedTools {
		tool := item.(map[string]interface{})
		names = append(names, tool["name"].(string))
	}
	require.Contains(t, names, "create_task_kandev")
	require.Contains(t, names, "update_task_kandev", "external MCP must expose the shared generic task update tool")

	eventsSeen := make(chan *bus.Event, 32)
	subscription, err := fixture.eventBus.Subscribe(">", func(_ context.Context, event *bus.Event) error {
		if event.Type == events.TaskCreated || event.Type == events.TaskUpdated {
			eventsSeen <- event
		}
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = subscription.Unsubscribe() })

	created := client.callTool(t, "create_task_kandev", map[string]interface{}{
		"workspace_id": workspace.ID, "workflow_id": workflowID,
		"title": "External priority journey", "agent_profile_id": "profile-1",
		"start_agent": false, "priority": models.TaskPriorityHigh, "external_id": "priority-journey",
	})
	require.False(t, created.isError, created.text)
	var task struct {
		ID       string `json:"id"`
		Priority string `json:"priority"`
	}
	require.NoError(t, json.Unmarshal([]byte(created.text), &task))
	require.NotEmpty(t, task.ID)
	assert.Equal(t, models.TaskPriorityHigh, task.Priority)
	assert.Equal(t, models.TaskPriorityHigh, externalTaskPriorityFromList(t, client, workflowID, task.ID))

	sessionID := "priority-journey-session"
	require.NoError(t, fixture.repository.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: task.ID, AgentProfileID: "profile-1",
		State: models.TaskSessionStateWaitingForInput, IsPrimary: true,
	}))
	baseline, err := fixture.taskService.GetTask(ctx, task.ID)
	require.NoError(t, err)

	updated := client.callTool(t, "update_task_kandev", map[string]interface{}{
		"task_id": task.ID, "priority": models.TaskPriorityCritical,
	})
	require.False(t, updated.isError, updated.text)
	var updatedTask struct {
		ID       string `json:"id"`
		Priority string `json:"priority"`
	}
	require.NoError(t, json.Unmarshal([]byte(updated.text), &updatedTask))
	assert.Equal(t, task.ID, updatedTask.ID)
	assert.Equal(t, models.TaskPriorityCritical, updatedTask.Priority)
	assert.Equal(t, models.TaskPriorityCritical, externalTaskPriorityFromList(t, client, workflowID, task.ID))

	omitted := client.callTool(t, "update_task_kandev", map[string]interface{}{"task_id": task.ID})
	require.False(t, omitted.isError, omitted.text)
	var omittedTask struct {
		Priority string `json:"priority"`
	}
	require.NoError(t, json.Unmarshal([]byte(omitted.text), &omittedTask))
	assert.Equal(t, models.TaskPriorityCritical, omittedTask.Priority)
	assert.Equal(t, models.TaskPriorityCritical, externalTaskPriorityFromList(t, client, workflowID, task.ID))

	persisted, err := fixture.taskService.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, models.TaskPriorityCritical, persisted.Priority)
	assert.Equal(t, baseline.Title, persisted.Title)
	assert.Equal(t, baseline.Description, persisted.Description)
	assert.Equal(t, baseline.State, persisted.State)
	assert.Equal(t, baseline.Position, persisted.Position)
	assert.Equal(t, baseline.WorkflowID, persisted.WorkflowID)
	assert.Equal(t, baseline.WorkflowStepID, persisted.WorkflowStepID)
	assert.Equal(t, baseline.Metadata, persisted.Metadata)
	session, err := fixture.repository.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	assert.Equal(t, models.TaskSessionStateWaitingForInput, session.State)

	assertExternalTaskPriorityEvent(t, eventsSeen, events.TaskCreated, task.ID, models.TaskPriorityHigh)
	assertExternalTaskPriorityEvent(t, eventsSeen, events.TaskUpdated, task.ID, models.TaskPriorityCritical)

	retry := client.callTool(t, "create_task_kandev", map[string]interface{}{
		"workspace_id": workspace.ID, "workflow_id": workflowID,
		"title": "Changed retry payload", "agent_profile_id": "profile-1",
		"start_agent": false, "priority": models.TaskPriorityLow, "external_id": "priority-journey",
	})
	require.False(t, retry.isError, retry.text)
	var retried struct {
		ID           string `json:"id"`
		Priority     string `json:"priority"`
		Deduplicated bool   `json:"deduplicated"`
	}
	require.NoError(t, json.Unmarshal([]byte(retry.text), &retried))
	assert.Equal(t, task.ID, retried.ID)
	assert.True(t, retried.Deduplicated)
	assert.Equal(t, models.TaskPriorityCritical, retried.Priority)

	priorities := []string{
		models.TaskPriorityCritical,
		models.TaskPriorityMedium,
		models.TaskPriorityLow,
	}
	updatedPriorities := []string{
		models.TaskPriorityLow,
		models.TaskPriorityHigh,
		models.TaskPriorityMedium,
	}
	for i, priority := range priorities {
		created := client.callTool(t, "create_task_kandev", map[string]interface{}{
			"workspace_id": workspace.ID, "workflow_id": workflowID,
			"title": "External priority " + priority, "agent_profile_id": "profile-1",
			"start_agent": false, "priority": priority,
		})
		require.False(t, created.isError, created.text)
		var createdTask struct {
			ID       string `json:"id"`
			Priority string `json:"priority"`
		}
		require.NoError(t, json.Unmarshal([]byte(created.text), &createdTask))
		assert.Equal(t, priority, createdTask.Priority)
		update := client.callTool(t, "update_task_kandev", map[string]interface{}{
			"task_id": createdTask.ID, "priority": updatedPriorities[i],
		})
		require.False(t, update.isError, update.text)
		var result struct {
			Priority string `json:"priority"`
		}
		require.NoError(t, json.Unmarshal([]byte(update.text), &result))
		assert.Equal(t, updatedPriorities[i], result.Priority)
	}

	defaulted := client.callTool(t, "create_task_kandev", map[string]interface{}{
		"workspace_id": workspace.ID, "workflow_id": workflowID,
		"title": "External default priority", "agent_profile_id": "profile-1", "start_agent": false,
	})
	require.False(t, defaulted.isError, defaulted.text)
	var defaultTask struct {
		Priority string `json:"priority"`
	}
	require.NoError(t, json.Unmarshal([]byte(defaulted.text), &defaultTask))
	assert.Equal(t, models.TaskPriorityMedium, defaultTask.Priority)
}

func TestExternalMCPTaskPriorityAuthorization(t *testing.T) {
	fixture := newExternalTaskPriorityFixture(t)
	ctx := context.Background()
	kanbanWorkspace, kanbanWorkflowID := fixture.createWorkspace(t, "priority-owned-kanban", "Owned Kanban", "priority-owner")
	officeWorkspace := &models.Workspace{ID: "priority-owned-office", Name: "Owned Office", OwnerID: "priority-owner"}
	require.NoError(t, fixture.repository.CreateWorkspace(ctx, officeWorkspace))
	officeWorkflowID, err := fixture.repository.EnsureOfficeWorkflow(ctx, officeWorkspace.ID)
	require.NoError(t, err)
	foreignWorkspace, foreignWorkflowID := fixture.createWorkspace(t, "priority-foreign-workspace", "Foreign", "another-user")

	kanbanTask := createExternalPriorityTask(t, fixture, kanbanWorkspace.ID, kanbanWorkflowID, "Owned Kanban task")
	officeTask := createExternalPriorityTask(t, fixture, officeWorkspace.ID, officeWorkflowID, "Owned Office task")
	foreignTask := createExternalPriorityTask(t, fixture, foreignWorkspace.ID, foreignWorkflowID, "Foreign task")
	client := fixture.httpClient(t, authn.Identity{UserID: "priority-owner", Role: authn.RoleMember})

	for _, target := range []struct {
		name   string
		taskID string
	}{
		{name: "kanban", taskID: kanbanTask.ID},
		{name: "office", taskID: officeTask.ID},
	} {
		t.Run(target.name, func(t *testing.T) {
			for _, priority := range taskPriorityValues {
				result := client.callTool(t, "update_task_kandev", map[string]interface{}{
					"task_id": target.taskID, "priority": priority,
				})
				require.False(t, result.isError, result.text)
				var updated struct {
					Priority string `json:"priority"`
				}
				require.NoError(t, json.Unmarshal([]byte(result.text), &updated))
				assert.Equal(t, priority, updated.Priority)
				stored, err := fixture.taskService.GetTask(ctx, target.taskID)
				require.NoError(t, err)
				assert.Equal(t, priority, stored.Priority)
			}
		})
	}

	denied := client.callTool(t, "update_task_kandev", map[string]interface{}{
		"task_id": foreignTask.ID, "priority": models.TaskPriorityCritical,
	})
	assert.True(t, denied.isError, "foreign workspace update must be denied")
	unchanged, err := fixture.taskService.GetTask(ctx, foreignTask.ID)
	require.NoError(t, err)
	assert.Equal(t, models.TaskPriorityMedium, unchanged.Priority)
}

func TestExternalMCPViewerCannotChangeDeferredLaunchPrompt(t *testing.T) {
	fixture := newExternalTaskPriorityFixture(t)
	ctx := context.Background()
	workspace, workflowID := fixture.createWorkspace(t, "priority-viewer-workspace", "Viewer workspace", "priority-owner")
	require.NoError(t, fixture.repository.UpsertWorkspaceMember(ctx, &models.WorkspaceMember{
		WorkspaceID: workspace.ID,
		UserID:      "priority-viewer",
		Role:        string(authz.WorkspaceRoleViewer),
	}))
	created, err := fixture.taskService.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: workspace.ID,
		WorkflowID:  workflowID,
		Title:       "Deferred viewer task",
		StartAgent:  true,
		DeferredLaunch: map[string]interface{}{
			"intent":           "start",
			"agent_profile_id": "profile-1",
			"prompt":           "owner supplied brief",
		},
	})
	require.NoError(t, err)
	owner := fixture.httpClient(t, authn.Identity{UserID: "priority-owner", Role: authn.RoleMember})
	ownerUpdate := owner.callTool(t, "update_task_kandev", map[string]interface{}{
		"task_id":                created.Task.ID,
		"deferred_launch_prompt": "owner supplied updated brief",
	})
	require.False(t, ownerUpdate.isError, ownerUpdate.text)

	viewer := fixture.httpClient(t, authn.Identity{UserID: "priority-viewer", Role: authn.RoleMember})
	result := viewer.callTool(t, "update_task_kandev", map[string]interface{}{
		"task_id":                created.Task.ID,
		"deferred_launch_prompt": "viewer supplied brief",
	})
	assert.True(t, result.isError, "viewer task update must be denied")

	stored, err := fixture.repository.GetTask(ctx, created.Task.ID)
	require.NoError(t, err)
	launch := stored.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	assert.Equal(t, "owner supplied updated brief", launch["prompt"], "denial must not change the queued prompt")
}

func createExternalPriorityTask(
	t *testing.T,
	fixture *externalTaskPriorityFixture,
	workspaceID, workflowID, title string,
) *models.Task {
	t.Helper()
	created, err := fixture.taskService.CreateTask(context.Background(), &service.CreateTaskRequest{
		WorkspaceID: workspaceID,
		WorkflowID:  workflowID,
		Title:       title,
		Priority:    models.TaskPriorityMedium,
	})
	require.NoError(t, err)
	return created.Task
}

func externalTaskPriorityFromList(t *testing.T, client *externalTaskPriorityHTTPClient, workflowID, taskID string) string {
	t.Helper()
	result := client.callTool(t, "list_tasks_kandev", map[string]interface{}{"workflow_id": workflowID})
	require.False(t, result.isError, result.text)
	var listed struct {
		Tasks []struct {
			ID       string `json:"id"`
			Priority string `json:"priority"`
		} `json:"tasks"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.text), &listed))
	for _, task := range listed.Tasks {
		if task.ID == taskID {
			return task.Priority
		}
	}
	t.Fatalf("task %q was not returned by list_tasks_kandev", taskID)
	return ""
}

func assertExternalTaskPriorityEvent(t *testing.T, eventsSeen <-chan *bus.Event, eventType, taskID, priority string) {
	t.Helper()
	for {
		select {
		case event := <-eventsSeen:
			data, ok := event.Data.(map[string]interface{})
			if event.Type == eventType && ok && data["task_id"] == taskID {
				assert.Equal(t, priority, data["priority"])
				return
			}
		default:
			t.Fatalf("event %q for task %q was not published", eventType, taskID)
			return
		}
	}
}

type externalTaskPriorityHTTPClient struct {
	url       string
	sessionID string
	nextID    int
}

type externalTaskPriorityToolResult struct {
	isError bool
	text    string
}

func newExternalTaskPriorityHTTPClient(t *testing.T, url string) *externalTaskPriorityHTTPClient {
	t.Helper()
	client := &externalTaskPriorityHTTPClient{url: url, nextID: 1}
	response := postJSONRPC(t, url, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"priority-test","version":"1.0.0"}}}`, "")
	require.Equal(t, http.StatusOK, response.statusCode, response.body)
	client.sessionID = response.sessionID
	return client
}

func (c *externalTaskPriorityHTTPClient) request(t *testing.T, method string, params map[string]interface{}) map[string]interface{} {
	t.Helper()
	c.nextID++
	body, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "id": c.nextID, "method": method, "params": params,
	})
	require.NoError(t, err)
	response := postJSONRPC(t, c.url, string(body), c.sessionID)
	require.Equal(t, http.StatusOK, response.statusCode, response.body)
	line := extractDataLine(response.body)
	require.NotEmpty(t, line, "no data line in response: %s", response.body)
	var rpc map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(line), &rpc))
	return rpc
}

func (c *externalTaskPriorityHTTPClient) callTool(t *testing.T, name string, arguments map[string]interface{}) externalTaskPriorityToolResult {
	t.Helper()
	rpc := c.request(t, "tools/call", map[string]interface{}{"name": name, "arguments": arguments})
	if rpc["error"] != nil {
		return externalTaskPriorityToolResult{isError: true, text: fmt.Sprint(rpc["error"])}
	}
	result, ok := rpc["result"].(map[string]interface{})
	require.True(t, ok, "missing JSON-RPC result: %v", rpc)
	content, ok := result["content"].([]interface{})
	require.True(t, ok, "missing tool content: %v", result)
	require.NotEmpty(t, content)
	text, _ := content[0].(map[string]interface{})["text"].(string)
	isError, _ := result["isError"].(bool)
	return externalTaskPriorityToolResult{isError: isError, text: text}
}
