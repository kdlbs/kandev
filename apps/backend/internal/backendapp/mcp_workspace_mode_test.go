package backendapp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestExternalMCPTaskModesReachPersistenceAndManagement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	harness := newBootStateTestHarness(t)
	ctx := context.Background()

	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, workspaces)
	kanbanWorkspace := workspaces[0]
	kanbanWorkflows, err := harness.taskSvc.ListWorkflows(ctx, kanbanWorkspace.ID, false)
	require.NoError(t, err)
	require.NotEmpty(t, kanbanWorkflows)

	officeWorkspace := &models.Workspace{
		ID:   "external-composition-office",
		Name: "External composition Office",
	}
	require.NoError(t, harness.taskRepo.CreateWorkspace(ctx, officeWorkspace))
	officeWorkflowID, err := harness.taskRepo.EnsureOfficeWorkflow(ctx, officeWorkspace.ID)
	require.NoError(t, err)
	kanbanMoveStepID := "external-composition-kanban-target"
	officeMoveStepID := "external-composition-office-target"
	require.NoError(t, harness.workflowSvc.CreateStep(ctx, &wfmodels.WorkflowStep{
		ID: kanbanMoveStepID, WorkflowID: kanbanWorkflows[0].ID, Name: "External Kanban target", Position: 100,
	}))
	require.NoError(t, harness.workflowSvc.CreateStep(ctx, &wfmodels.WorkflowStep{
		ID: officeMoveStepID, WorkflowID: officeWorkflowID, Name: "External Office target", Position: 100,
	}))

	log, err := logger.NewLogger(logger.LoggingConfig{
		Level: "error", Format: "console", OutputPath: "stdout",
	})
	require.NoError(t, err)
	eventBus := bus.NewMemoryEventBus(log)
	taskRepoAdapter := &taskRepositoryAdapter{repo: harness.taskRepo, svc: harness.taskSvc}
	orchestratorSvc := orchestrator.NewService(
		orchestrator.DefaultServiceConfig(), eventBus, nil,
		taskRepoAdapter, harness.taskRepo, nil, nil, nil, log,
	)
	lifecycleMgr := lifecycle.NewManager(
		nil, eventBus, nil, nil, nil, nil,
		lifecycle.ExecutorFallbackDeny, t.TempDir(), log,
	)
	registerMCPStopTestCleanup(t, "lifecycle manager", lifecycleMgr.Stop)

	router := gin.New()
	gateway := gateways.NewGateway(log)
	registerMCPAndDebugRoutes(routeParams{
		router:          router,
		gateway:         gateway,
		taskSvc:         harness.taskSvc,
		taskRepo:        harness.taskRepo,
		orchestratorSvc: orchestratorSvc,
		lifecycleMgr:    lifecycleMgr,
		eventBus:        eventBus,
		services:        &Services{Workflow: harness.workflowSvc},
		devMode:         true,
		log:             log,
		addCleanup: func(cleanup func() error) {
			registerMCPStopTestCleanup(t, "MCP composition service", cleanup)
		},
	}, nil, nil, nil, nil, nil, nil)

	httpServer := httptest.NewServer(router)
	t.Cleanup(httpServer.Close)

	created := make(map[string]string, 2)
	for _, tc := range []struct {
		name, workspaceID, workflowID, title, externalID string
		wantOffice                                       bool
	}{
		{
			name:        "kanban",
			workspaceID: kanbanWorkspace.ID,
			workflowID:  kanbanWorkflows[0].ID,
			title:       "HTTP external Kanban",
			externalID:  "http-external-kanban",
			wantOffice:  false,
		},
		{
			name:        "office",
			workspaceID: officeWorkspace.ID,
			workflowID:  officeWorkflowID,
			title:       "HTTP external Office",
			externalID:  "http-external-office",
			wantOffice:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := externalWorkspaceModeToolCall(t, httpServer.URL+"/mcp", 1, "create_task_kandev", map[string]any{
				"workspace_id":     tc.workspaceID,
				"workflow_id":      tc.workflowID,
				"title":            tc.title,
				"agent_profile_id": "profile-1",
				"external_id":      tc.externalID,
				"start_agent":      false,
			})
			id, ok := payload["id"].(string)
			require.True(t, ok)
			require.NotEmpty(t, id)
			created[tc.name] = id

			task, err := harness.taskSvc.GetTask(ctx, id)
			require.NoError(t, err)
			require.Equal(t, tc.title, task.Title)
			require.Equal(t, tc.workspaceID, task.WorkspaceID)
			require.Equal(t, tc.wantOffice, task.IsFromOffice)

			retry := externalWorkspaceModeToolCall(t, httpServer.URL+"/mcp", 2, "create_task_kandev", map[string]any{
				"workspace_id":     tc.workspaceID,
				"workflow_id":      tc.workflowID,
				"title":            tc.title + " retry",
				"agent_profile_id": "profile-1",
				"external_id":      tc.externalID,
				"start_agent":      false,
			})
			require.Equal(t, id, retry["id"])
			require.Equal(t, true, retry["deduplicated"])
		})
	}

	for _, tc := range []struct {
		name, workflowID, moveStepID string
	}{
		{name: "kanban", workflowID: kanbanWorkflows[0].ID, moveStepID: kanbanMoveStepID},
		{name: "office", workflowID: officeWorkflowID, moveStepID: officeMoveStepID},
	} {
		t.Run(tc.name+" management", func(t *testing.T) {
			listPayload := externalWorkspaceModeToolCall(t, httpServer.URL+"/mcp", 10, "list_tasks_kandev", map[string]any{
				"workflow_id": tc.workflowID,
			})
			listData, err := json.Marshal(listPayload)
			require.NoError(t, err)
			require.Contains(t, string(listData), created[tc.name])

			externalWorkspaceModeToolCall(t, httpServer.URL+"/mcp", 20, "update_task_state_kandev", map[string]any{
				"task_id": created[tc.name],
				"state":   "IN_PROGRESS",
			})
			task, err := harness.taskSvc.GetTask(ctx, created[tc.name])
			require.NoError(t, err)
			require.Equal(t, v1.TaskStateInProgress, task.State)

			externalWorkspaceModeToolCall(t, httpServer.URL+"/mcp", 30, "move_task_kandev", map[string]any{
				"task_id":          created[tc.name],
				"workflow_id":      tc.workflowID,
				"workflow_step_id": tc.moveStepID,
			})
			task, err = harness.taskSvc.GetTask(ctx, created[tc.name])
			require.NoError(t, err)
			require.Equal(t, tc.moveStepID, task.WorkflowStepID)

			archived := externalWorkspaceModeToolCall(t, httpServer.URL+"/mcp", 40, "archive_task_kandev", map[string]any{
				"task_id": created[tc.name],
			})
			require.Equal(t, true, archived["success"])
			task, err = harness.taskSvc.GetTask(ctx, created[tc.name])
			require.NoError(t, err)
			require.NotNil(t, task.ArchivedAt)

			deleted := externalWorkspaceModeToolCall(t, httpServer.URL+"/mcp", 50, "delete_task_kandev", map[string]any{
				"task_id": created[tc.name],
			})
			require.Equal(t, true, deleted["success"])
			_, err = harness.taskSvc.GetTask(ctx, created[tc.name])
			require.Error(t, err)
		})
	}
}

func externalWorkspaceModeToolCall(
	t *testing.T,
	url string,
	id int,
	name string,
	arguments map[string]any,
) map[string]any {
	t.Helper()
	response := postExternalWorkspaceModeMCPRequest(t, url, id, name, arguments)
	require.Equal(t, http.StatusOK, response.StatusCode, "MCP %s response: %s", name, response.Body)
	message := decodeExternalMCPResponse(t, response.Body)
	require.NotContains(t, message, "error", "MCP %s response: %s", name, response.Body)
	result, ok := message["result"].(map[string]any)
	require.True(t, ok, "MCP %s result: %s", name, response.Body)
	require.NotEqual(t, true, result["isError"], "MCP %s result: %s", name, response.Body)
	content, ok := result["content"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, content)
	first, ok := content[0].(map[string]any)
	require.True(t, ok)
	text, ok := first["text"].(string)
	require.True(t, ok)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &payload), "MCP %s tool payload: %s", name, text)
	return payload
}

func postExternalWorkspaceModeMCPRequest(
	t *testing.T,
	url string,
	id int,
	name string,
	arguments map[string]any,
) externalMCPHTTPResponse {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      name,
			"arguments": arguments,
			"_meta":     externalModernRequestMeta(),
		},
	})
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Mcp-Protocol-Version", externalModernProtocolVersion)
	request.Header.Set("Mcp-Method", "tools/call")
	request.Header.Set("Mcp-Name", name)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	var responseBody bytes.Buffer
	_, err = responseBody.ReadFrom(response.Body)
	require.NoError(t, err)
	return externalMCPHTTPResponse{Response: response, Body: strings.TrimSpace(responseBody.String())}
}
