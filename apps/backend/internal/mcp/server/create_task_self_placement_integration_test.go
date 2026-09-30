package mcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	backendmcp "github.com/kandev/kandev/internal/mcp/handlers"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	workflowcontroller "github.com/kandev/kandev/internal/workflow/controller"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type selfPlacementScopedBackend struct {
	client            BackendClient
	resolver          *mcpscope.Resolver
	taskID, sessionID string
}

type selfPlacementWorkspacePolicyAttacher struct{}

func (selfPlacementWorkspacePolicyAttacher) AttachWorkspacePolicy(context.Context, string, string, service.WorkspacePolicy) error {
	return nil
}

func (b selfPlacementScopedBackend) RequestPayload(ctx context.Context, action string, payload, result interface{}) error {
	scoped, err := b.resolver.ScopePrincipal(ctx, b.taskID, b.sessionID)
	if err != nil {
		return err
	}
	return b.client.RequestPayload(scoped, action, payload, result)
}

type selfPlacementLaunchSpy struct {
	backendmcp.SessionLauncher
	requests chan *orchestrator.LaunchSessionRequest
}

func (s *selfPlacementLaunchSpy) LaunchSession(_ context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
	s.requests <- req
	return &orchestrator.LaunchSessionResponse{Success: true, TaskID: req.TaskID, SessionID: "launched-session"}, nil
}

func newSelfPlacementJourney(t *testing.T) (*Server, *service.Service, *sqlite.Repository, *selfPlacementLaunchSpy) {
	t.Helper()
	log := newTestLogger(t)
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "self-placement.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	repo, err := sqlite.NewWithDB(database, database, log)
	require.NoError(t, err)
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(func() { eventBus.Close() })
	taskSvc := service.NewService(service.Repos{
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
	taskSvc.SetWorkspacePolicyAttacher(selfPlacementWorkspacePolicyAttacher{})

	ctx := context.Background()
	const (
		workspaceID = "workspace-self-placement"
		workflowID  = "workflow-self-placement"
		parentID    = "parent-self-placement"
		childID     = "child-self-placement"
		sessionID   = "session-self-placement"
	)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Self placement"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "Kanban"}))
	now := time.Now().UTC()
	for _, task := range []*models.Task{
		{ID: parentID, WorkspaceID: workspaceID, WorkflowID: workflowID, Title: "Parent", State: v1.TaskStateCreated, Priority: "medium", CreatedAt: now, UpdatedAt: now},
		{ID: childID, WorkspaceID: workspaceID, WorkflowID: workflowID, ParentID: parentID, Title: "Child", State: v1.TaskStateCreated, Priority: "medium", CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, repo.CreateTask(ctx, task))
	}
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: childID, State: models.TaskSessionStateWaitingForInput, IsPrimary: true, AgentProfileID: "creator-profile",
	}))

	steps, err := workflowrepo.NewWithDB(database, database, log)
	require.NoError(t, err)
	wfSvc := workflowservice.NewService(steps, log)
	t.Cleanup(func() { require.NoError(t, wfSvc.Close()) })
	step := &workflowmodels.WorkflowStep{ID: "start-self-placement", WorkflowID: workflowID, Name: "Start", IsStartStep: true,
		Events: workflowmodels.StepEvents{OnEnter: []workflowmodels.OnEnterAction{{Type: workflowmodels.OnEnterAutoStartAgent}}},
	}
	require.NoError(t, steps.CreateStep(ctx, step))
	taskSvc.SetWorkflowStepGetter(wfSvc)
	launcher := &selfPlacementLaunchSpy{requests: make(chan *orchestrator.LaunchSessionRequest, 4)}
	backendHandlers := backendmcp.NewHandlers(
		taskSvc, workflowcontroller.NewController(wfSvc), nil, nil, nil, repo, repo, nil, nil, nil, launcher, nil, log,
	)
	dispatcher := ws.NewDispatcher()
	backendHandlers.RegisterHandlers(dispatcher)
	server := New(
		selfPlacementScopedBackend{client: NewDispatcherBackendClient(dispatcher, log),
			resolver: mcpscope.NewResolver(repo, nil, func() bool { return false }, log), taskID: childID, sessionID: sessionID},
		sessionID, childID, 10005, log, "", false, ModeTask,
	)
	return server, taskSvc, repo, launcher
}

// @covers AC-TASKS-SELF-SIBLING-001.1 AC-TASKS-SELF-SIBLING-001.2 AC-TASKS-SELF-SIBLING-001.4
// @covers AC-TASKS-SELF-SIBLING-002.1 AC-TASKS-SELF-SIBLING-002.2 AC-TASKS-SELF-SIBLING-002.3 AC-TASKS-SELF-SIBLING-002.4
func TestCreateTaskSelfPlacementJourney(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, svc, repo, launcher := newSelfPlacementJourney(t)
		args := map[string]interface{}{
			"title": "Sibling from MCP", "parent_id": "self", "prompt": "Complete this follow-up",
			"external_id": "mcp-journey", "start_agent": true,
		}
		created := callTool(t, server, "create_task_kandev", args)
		require.False(t, created.IsError, firstText(t, created))
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(firstText(t, created)), &result))
		require.Equal(t, "parent-self-placement", result["parent_id"])
		require.Equal(t, false, result["deduplicated"])
		require.Equal(t, true, result["creation_complete"])
		resolution := result["parent_resolution"].(map[string]interface{})
		require.Equal(t, "child-self-placement", resolution["requested_parent_id"])
		require.Equal(t, "parent-self-placement", resolution["resolved_parent_id"])
		require.Equal(t, "kanban_depth_limit", resolution["reason"])
		require.Contains(t, resolution["message"], "Created a sibling")
		require.Contains(t, resolution["message"], "coordination")
		require.NotContains(t, result, "description")

		ctx := context.Background()
		id := result["id"].(string)
		task, err := svc.GetTask(ctx, id)
		require.NoError(t, err)
		require.Equal(t, "workspace-self-placement", task.WorkspaceID)
		require.Equal(t, "workflow-self-placement", task.WorkflowID)
		require.Equal(t, "creator-profile", task.Metadata[models.MetaKeyAgentProfileID])
		var actorKind, actorID, sessionID string
		require.NoError(t, repo.DB().QueryRowContext(ctx,
			"SELECT actor_kind, actor_id, session_id FROM task_step_transitions WHERE task_id = ?", id,
		).Scan(&actorKind, &actorID, &sessionID))
		require.Equal(t, "agent", actorKind)
		require.Equal(t, "session-self-placement", actorID)
		require.Equal(t, actorID, sessionID)
		synctest.Wait()
		require.Len(t, launcher.requests, 1)
		launch := <-launcher.requests
		require.Equal(t, id, launch.TaskID)
		require.Equal(t, "creator-profile", launch.AgentProfileID)
		require.Equal(t, "Complete this follow-up", launch.Prompt)

		retry := callTool(t, server, "create_task_kandev", args)
		require.False(t, retry.IsError, firstText(t, retry))
		var found map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(firstText(t, retry)), &found))
		require.Equal(t, id, found["id"])
		require.Equal(t, true, found["deduplicated"])
		require.Equal(t, true, found["creation_complete"])
		require.Contains(t, found["parent_resolution"].(map[string]interface{})["message"], "without creation or reparenting")

		args["parent_id"], args["external_id"] = "child-self-placement", "explicit-child"
		rejected := callTool(t, server, "create_task_kandev", args)
		require.True(t, rejected.IsError)
		require.Contains(t, firstText(t, rejected), "subtask")
		require.NotContains(t, firstText(t, rejected), "parent_resolution")
		tasks, err := svc.ListTasks(ctx, "workflow-self-placement")
		require.NoError(t, err)
		require.Len(t, tasks, 3)
		synctest.Wait()
		require.Empty(t, launcher.requests)
	})
}
