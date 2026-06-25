package backendapp

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
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	taskdto "github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	usercontroller "github.com/kandev/kandev/internal/user/controller"
	userdto "github.com/kandev/kandev/internal/user/dto"
	usermodels "github.com/kandev/kandev/internal/user/models"
	userservice "github.com/kandev/kandev/internal/user/service"
	userstore "github.com/kandev/kandev/internal/user/store"
	"github.com/kandev/kandev/internal/webapp"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	"github.com/kandev/kandev/internal/worktree"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func decodePayload(t *testing.T, raw json.RawMessage) map[string]interface{} {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	return payload
}

func TestMapUserSettingsStateIncludesTaskCreateLastUsed(t *testing.T) {
	state := mapUserSettingsState(userdto.UserSettingsResponse{
		Settings: userdto.UserSettingsDTO{
			TaskCreateLastUsed: usermodels.TaskCreateLastUsed{
				RepositoryID:      "repo-1",
				Branch:            "main",
				AgentProfileID:    "agent-profile-1",
				ExecutorProfileID: "executor-profile-1",
			},
		},
	}, "")

	lastUsed, ok := state["taskCreateLastUsed"].(map[string]any)
	if !ok {
		t.Fatalf("taskCreateLastUsed missing or wrong type: %#v", state["taskCreateLastUsed"])
	}
	if lastUsed["repositoryId"] != "repo-1" {
		t.Fatalf("repositoryId = %#v, want repo-1", lastUsed["repositoryId"])
	}
	if lastUsed["branch"] != "main" {
		t.Fatalf("branch = %#v, want main", lastUsed["branch"])
	}
	if lastUsed["agentProfileId"] != "agent-profile-1" {
		t.Fatalf("agentProfileId = %#v, want agent-profile-1", lastUsed["agentProfileId"])
	}
	if lastUsed["executorProfileId"] != "executor-profile-1" {
		t.Fatalf("executorProfileId = %#v, want executor-profile-1", lastUsed["executorProfileId"])
	}
}

// TestAppendSessionStateMessage_IncludesTaskEnvironmentID asserts the snapshot
// the WS hub sends on `session.subscribe` carries `task_environment_id`.
//
// Why this matters: PR #758 routes shell terminals by environment, and the
// frontend reads `environmentIdBySessionId` from `session.state_changed`
// payloads to populate that map. If the subscribe snapshot omits it,
// late-subscribing clients (page reload, task switch, WS reconnect) leave
// `environmentId=null` for the active session and the terminal panel hangs
// on "Connecting terminal..." forever.
func TestAppendSessionStateMessage_IncludesTaskEnvironmentID(t *testing.T) {
	session := &models.TaskSession{
		ID:                "sess-1",
		TaskID:            "task-1",
		State:             models.TaskSessionStateRunning,
		TaskEnvironmentID: "env-42",
	}

	msgs := appendSessionStateMessage(session.ID, session, nil)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Action != ws.ActionSessionStateChanged {
		t.Fatalf("expected action %q, got %q", ws.ActionSessionStateChanged, msgs[0].Action)
	}

	payload := decodePayload(t, msgs[0].Payload)
	got, present := payload["task_environment_id"]
	if !present {
		t.Fatalf("payload missing task_environment_id key — frontend env map will not be seeded")
	}
	if got != "env-42" {
		t.Fatalf("expected task_environment_id=env-42, got %v", got)
	}
}

func TestAppendSessionStateMessage_IncludesRoutingMetadata(t *testing.T) {
	session := &models.TaskSession{
		ID:             "sess-1",
		TaskID:         "task-1",
		State:          models.TaskSessionStateRunning,
		AgentProfileID: "profile-1",
		AgentProfileSnapshot: map[string]interface{}{
			"name":  "Codex",
			"model": "gpt-5",
		},
		IsPassthrough: true,
	}

	msgs := appendSessionStateMessage(session.ID, session, nil)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	payload := decodePayload(t, msgs[0].Payload)
	if got := payload["agent_profile_id"]; got != "profile-1" {
		t.Fatalf("expected agent_profile_id=profile-1, got %v", got)
	}
	snapshot, ok := payload["agent_profile_snapshot"].(map[string]interface{})
	if !ok {
		t.Fatalf("payload missing agent_profile_snapshot: %#v", payload["agent_profile_snapshot"])
	}
	if snapshot["name"] != "Codex" || snapshot["model"] != "gpt-5" {
		t.Fatalf("unexpected agent_profile_snapshot: %#v", snapshot)
	}
	if got := payload["is_passthrough"]; got != true {
		t.Fatalf("expected is_passthrough=true, got %v", got)
	}
}

func TestAppendAgentctlStatusMessage_IncludesWorkspacePathForReload(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "error",
		Format:     "console",
		OutputPath: "stdout",
	})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	mgr := lifecycle.NewManager(nil, nil, nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	workspacePath := filepath.Join(t.TempDir(), "scratch")
	if err := mgr.ExecutionStoreForTesting().Add(&lifecycle.AgentExecution{
		ID:                "exec-1",
		TaskID:            "task-1",
		SessionID:         "sess-1",
		TaskEnvironmentID: "env-1",
		WorkspacePath:     workspacePath,
	}); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	msgs := appendAgentctlStatusMessage(context.Background(), mgr, "sess-1", nil, log)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Action != ws.ActionSessionAgentctlStarting {
		t.Fatalf("expected action %q, got %q", ws.ActionSessionAgentctlStarting, msgs[0].Action)
	}

	payload := decodePayload(t, msgs[0].Payload)
	got, present := payload["worktree_path"]
	if !present {
		t.Fatalf("payload missing worktree_path — reload cannot hydrate repo-less scratch workspace path")
	}
	if got != workspacePath {
		t.Fatalf("expected worktree_path=%q, got %v", workspacePath, got)
	}
}

func TestStopLifecycleManagerAllowsAgentctlInstanceCleanupWindow(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "error",
		Format:     "console",
		OutputPath: "stdout",
	})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	stopper := &shutdownDeadlineExecutor{}
	execRegistry := lifecycle.NewExecutorRegistry(log)
	execRegistry.Register(stopper)
	mgr := lifecycle.NewManager(nil, nil, execRegistry, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	if err := mgr.ExecutionStoreForTesting().Add(&lifecycle.AgentExecution{
		ID:          "exec-1",
		TaskID:      "task-1",
		SessionID:   "sess-1",
		RuntimeName: executor.NameStandalone,
	}); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	startedAt := time.Now()
	_ = stopLifecycleManager(mgr, log)

	if stopper.deadline.IsZero() {
		t.Fatal("StopInstance was not called with a deadline")
	}
	if got := stopper.deadline.Sub(startedAt); got < 15*time.Second {
		t.Fatalf("agent shutdown timeout = %v, want at least 15s for agentctl instance cleanup", got)
	}
}

func TestBootInitialStateIncludesFeatureFlags(t *testing.T) {
	state := bootInitialState(
		context.Background(),
		nil,
		routeParams{features: config.FeaturesConfig{Office: true}},
		webapp.ClassifyRoute("/"),
	)

	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal state: %v", err)
	}
	var decoded struct {
		Features config.FeaturesConfig `json:"features"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal state: %v", err)
	}
	if !decoded.Features.Office {
		t.Fatal("features.office should hydrate true from the backend boot payload")
	}
}

type shutdownDeadlineExecutor struct {
	deadline time.Time
}

func (s *shutdownDeadlineExecutor) Name() executor.Name { return executor.NameStandalone }

func (s *shutdownDeadlineExecutor) HealthCheck(context.Context) error { return nil }

func (s *shutdownDeadlineExecutor) CreateInstance(
	context.Context,
	*lifecycle.ExecutorCreateRequest,
) (*lifecycle.ExecutorInstance, error) {
	return nil, nil
}

func (s *shutdownDeadlineExecutor) StopInstance(
	ctx context.Context,
	_ *lifecycle.ExecutorInstance,
	_ bool,
) error {
	if deadline, ok := ctx.Deadline(); ok {
		s.deadline = deadline
	}
	return nil
}

func (s *shutdownDeadlineExecutor) RecoverInstances(context.Context) ([]*lifecycle.ExecutorInstance, error) {
	return nil, nil
}

func (s *shutdownDeadlineExecutor) GetInteractiveRunner() *process.InteractiveRunner { return nil }

func (s *shutdownDeadlineExecutor) RequiresCloneURL() bool { return false }

func (s *shutdownDeadlineExecutor) ShouldApplyPreferredShell() bool { return true }

func (s *shutdownDeadlineExecutor) IsAlwaysResumable() bool { return false }

func TestBootInitialStateHomeIncludesKanbanFirstPaintState(t *testing.T) {
	taskSvc, workflowSvc := newBootStateTestServices(t)
	ctx := context.Background()

	workspaces, err := taskSvc.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(workspaces) == 0 {
		t.Fatal("expected seeded default workspace")
	}
	workflows, err := taskSvc.ListWorkflows(ctx, workspaces[0].ID, true)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(workflows) == 0 {
		t.Fatal("expected seeded default workflow")
	}

	state := bootInitialState(
		ctx,
		nil,
		routeParams{
			taskSvc: taskSvc,
			services: &Services{
				Workflow: workflowSvc,
			},
		},
		webapp.ClassifyRoute("/"),
	)

	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal state: %v", err)
	}
	var decoded struct {
		Workspaces struct {
			Items    []taskdto.WorkspaceDTO `json:"items"`
			ActiveID string                 `json:"activeId"`
		} `json:"workspaces"`
		Workflows struct {
			Items []struct {
				ID          string `json:"id"`
				WorkspaceID string `json:"workspaceId"`
				Name        string `json:"name"`
			} `json:"items"`
			ActiveID string `json:"activeId"`
		} `json:"workflows"`
		KanbanMulti struct {
			Snapshots map[string]struct {
				WorkflowID   string `json:"workflowId"`
				WorkflowName string `json:"workflowName"`
				Steps        []struct {
					ID    string `json:"id"`
					Title string `json:"title"`
				} `json:"steps"`
				Tasks []struct {
					ID             string `json:"id"`
					WorkflowStepID string `json:"workflowStepId"`
				} `json:"tasks"`
			} `json:"snapshots"`
			IsLoading bool `json:"isLoading"`
		} `json:"kanbanMulti"`
		Kanban struct {
			WorkflowID string `json:"workflowId"`
			Steps      []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"steps"`
			Tasks []struct {
				ID             string `json:"id"`
				WorkflowStepID string `json:"workflowStepId"`
			} `json:"tasks"`
			IsLoading bool `json:"isLoading"`
		} `json:"kanban"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal state: %v", err)
	}

	if decoded.Workspaces.ActiveID != workspaces[0].ID {
		t.Fatalf("active workspace = %q, want %q", decoded.Workspaces.ActiveID, workspaces[0].ID)
	}
	if len(decoded.Workflows.Items) == 0 || decoded.Workflows.ActiveID == "" {
		t.Fatalf("expected workflows to hydrate first paint, got %+v", decoded.Workflows)
	}
	if _, ok := decoded.KanbanMulti.Snapshots[workflows[0].ID]; !ok {
		t.Fatalf("expected snapshot for workflow %q in boot payload", workflows[0].ID)
	}
	if decoded.Kanban.WorkflowID != workflows[0].ID {
		t.Fatalf("active kanban workflow = %q, want %q", decoded.Kanban.WorkflowID, workflows[0].ID)
	}
	if decoded.KanbanMulti.IsLoading || decoded.Kanban.IsLoading {
		t.Fatal("boot payload should mark kanban data loaded")
	}
}

func TestBootInitialStateHomePreservesSavedAllWorkflowsFilter(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()

	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(workspaces) == 0 {
		t.Fatal("expected seeded default workspace")
	}
	workflowB, err := harness.taskSvc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{
		WorkspaceID: workspaces[0].ID,
		Name:        "Workflow B",
	})
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	allWorkflows := ""
	repositoryIDs := []string{}
	if _, err := harness.userSvc.UpdateUserSettings(ctx, &userservice.UpdateUserSettingsRequest{
		WorkspaceID:      &workspaces[0].ID,
		WorkflowFilterID: &allWorkflows,
		RepositoryIDs:    &repositoryIDs,
	}); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}

	state := bootInitialState(
		ctx,
		nil,
		routeParams{
			taskSvc:  harness.taskSvc,
			userCtrl: harness.userCtrl,
			services: &Services{
				Workflow: harness.workflowSvc,
			},
		},
		webapp.ClassifyRoute("/"),
	)

	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal state: %v", err)
	}
	var decoded struct {
		Workflows struct {
			ActiveID *string `json:"activeId"`
		} `json:"workflows"`
		UserSettings struct {
			WorkflowID *string `json:"workflowId"`
		} `json:"userSettings"`
		KanbanMulti struct {
			Snapshots map[string]json.RawMessage `json:"snapshots"`
		} `json:"kanbanMulti"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal state: %v", err)
	}

	if decoded.Workflows.ActiveID != nil {
		t.Fatalf("active workflow = %q, want nil for All Workflows", *decoded.Workflows.ActiveID)
	}
	if decoded.UserSettings.WorkflowID != nil {
		t.Fatalf("user settings workflow = %q, want nil for All Workflows", *decoded.UserSettings.WorkflowID)
	}
	if _, ok := decoded.KanbanMulti.Snapshots[workflowB.ID]; !ok {
		t.Fatalf("expected boot snapshots to include second workflow %q", workflowB.ID)
	}
}

func TestBootRouteDataTaskDetailIncludesTaskPageData(t *testing.T) {
	taskSvc, workflowSvc := newBootStateTestServices(t)
	ctx := context.Background()
	workspaces, err := taskSvc.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	workflows, err := taskSvc.ListWorkflows(ctx, workspaces[0].ID, true)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	steps, err := workflowSvc.ListStepsByWorkflow(ctx, workflows[0].ID)
	if err != nil {
		t.Fatalf("ListStepsByWorkflow: %v", err)
	}
	task, err := taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID:    workspaces[0].ID,
		WorkflowID:     workflows[0].ID,
		WorkflowStepID: steps[0].ID,
		Title:          "Boot detail task",
		Description:    "Should be present before React mounts",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	payload := webapp.NewBootPayload(
		webapp.ClassifyRoute("/t/"+task.ID),
		webapp.RuntimeConfig{APIPrefix: "/api/v1", WebSocketPath: "/ws"},
		bootInitialState(ctx, nil, routeParams{
			taskSvc: taskSvc,
			services: &Services{
				Workflow: workflowSvc,
			},
		}, webapp.ClassifyRoute("/t/"+task.ID)),
	)
	payload.RouteData = bootRouteData(ctx, nil, routeParams{
		taskSvc: taskSvc,
		services: &Services{
			Workflow: workflowSvc,
		},
	}, webapp.ClassifyRoute("/t/"+task.ID))

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}
	var decoded struct {
		RouteData struct {
			TaskDetail struct {
				Task struct {
					ID    string `json:"id"`
					Title string `json:"title"`
				} `json:"task"`
				SessionID    *string                `json:"sessionId"`
				InitialState map[string]interface{} `json:"initialState"`
			} `json:"taskDetail"`
		} `json:"routeData"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal payload: %v", err)
	}

	if decoded.RouteData.TaskDetail.Task.ID != task.ID {
		t.Fatalf("route task id = %q, want %q", decoded.RouteData.TaskDetail.Task.ID, task.ID)
	}
	if decoded.RouteData.TaskDetail.Task.Title != "Boot detail task" {
		t.Fatalf("route task title = %q", decoded.RouteData.TaskDetail.Task.Title)
	}
	if decoded.RouteData.TaskDetail.SessionID != nil {
		t.Fatalf("new task should not have a boot session id, got %q", *decoded.RouteData.TaskDetail.SessionID)
	}
	if _, ok := decoded.RouteData.TaskDetail.InitialState["kanban"]; !ok {
		t.Fatal("task detail route data should include initialState.kanban")
	}
}

func TestBootRouteDataTasksIncludesFirstPageRows(t *testing.T) {
	taskSvc, workflowSvc := newBootStateTestServices(t)
	ctx := context.Background()
	workspaces, err := taskSvc.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	workflows, err := taskSvc.ListWorkflows(ctx, workspaces[0].ID, true)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	steps, err := workflowSvc.ListStepsByWorkflow(ctx, workflows[0].ID)
	if err != nil {
		t.Fatalf("ListStepsByWorkflow: %v", err)
	}
	created, err := taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID:    workspaces[0].ID,
		WorkflowID:     workflows[0].ID,
		WorkflowStepID: steps[0].ID,
		Title:          "Tasks table row",
		Description:    "Visible on first paint",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	route := webapp.ClassifyRoute("/tasks")
	payload := bootPayload(ctx, nil, routeParams{
		taskSvc: taskSvc,
		services: &Services{
			Workflow: workflowSvc,
		},
	}, route)

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}
	var decoded struct {
		InitialState struct {
			Workspaces struct {
				ActiveID string `json:"activeId"`
			} `json:"workspaces"`
			Workflows struct {
				ActiveID *string `json:"activeId"`
				Items    []struct {
					ID string `json:"id"`
				} `json:"items"`
			} `json:"workflows"`
		} `json:"initialState"`
		RouteData struct {
			TasksPage struct {
				ActiveWorkspaceID string `json:"activeWorkspaceId"`
				Tasks             []struct {
					ID    string `json:"id"`
					Title string `json:"title"`
				} `json:"tasks"`
				Total int `json:"total"`
			} `json:"tasksPage"`
		} `json:"routeData"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal payload: %v", err)
	}

	if decoded.InitialState.Workspaces.ActiveID != workspaces[0].ID {
		t.Fatalf("active workspace = %q, want %q", decoded.InitialState.Workspaces.ActiveID, workspaces[0].ID)
	}
	if len(decoded.InitialState.Workflows.Items) == 0 {
		t.Fatal("tasks route should hydrate workflow items")
	}
	if decoded.RouteData.TasksPage.ActiveWorkspaceID != workspaces[0].ID {
		t.Fatalf("route active workspace = %q, want %q", decoded.RouteData.TasksPage.ActiveWorkspaceID, workspaces[0].ID)
	}
	if decoded.RouteData.TasksPage.Total != 1 {
		t.Fatalf("tasks total = %d, want 1", decoded.RouteData.TasksPage.Total)
	}
	if len(decoded.RouteData.TasksPage.Tasks) != 1 || decoded.RouteData.TasksPage.Tasks[0].ID != created.ID {
		t.Fatalf("route tasks = %+v, want task %q", decoded.RouteData.TasksPage.Tasks, created.ID)
	}
}

func TestBootRouteDataTasksUsesActiveWorkspaceCookie(t *testing.T) {
	taskSvc, workflowSvc := newBootStateTestServices(t)
	ctx := context.Background()
	if _, err := taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{
		Name: "Cookie Workspace",
	}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	workspace := nonFallbackWorkspace(t, ctx, taskSvc)
	if _, err := taskSvc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{
		WorkspaceID: workspace.ID,
		Name:        "Cookie Workflow",
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-state?path=%2Ftasks", nil)
	req.AddCookie(&http.Cookie{Name: "kandev-active-workspace", Value: workspace.ID})
	payload := bootPayload(ctx, req, routeParams{
		taskSvc: taskSvc,
		services: &Services{
			Workflow: workflowSvc,
		},
	}, webapp.ClassifyRoute("/tasks"))

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}
	var decoded struct {
		InitialState struct {
			Workspaces struct {
				ActiveID string `json:"activeId"`
			} `json:"workspaces"`
		} `json:"initialState"`
		RouteData struct {
			TasksPage struct {
				ActiveWorkspaceID string `json:"activeWorkspaceId"`
			} `json:"tasksPage"`
		} `json:"routeData"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal payload: %v", err)
	}
	if decoded.InitialState.Workspaces.ActiveID != workspace.ID {
		t.Fatalf("initial active workspace = %q, want %q", decoded.InitialState.Workspaces.ActiveID, workspace.ID)
	}
	if decoded.RouteData.TasksPage.ActiveWorkspaceID != workspace.ID {
		t.Fatalf("route active workspace = %q, want %q", decoded.RouteData.TasksPage.ActiveWorkspaceID, workspace.ID)
	}
}

func TestBootPayloadIncludesDebugRuntimeWhenDevMode(t *testing.T) {
	t.Parallel()

	payload := bootPayload(context.Background(), nil, routeParams{devMode: true}, webapp.ClassifyRoute("/"))
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}
	var decoded struct {
		Runtime struct {
			Debug bool `json:"debug"`
		} `json:"runtime"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal payload: %v", err)
	}
	if !decoded.Runtime.Debug {
		t.Fatal("runtime.debug = false, want true when backend devMode is enabled")
	}
}

func TestBootRouteDataIntegrationRouteIncludesOnlyLocalContext(t *testing.T) {
	taskSvc, workflowSvc := newBootStateTestServices(t)
	ctx := context.Background()
	workspaces, err := taskSvc.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}

	payload := bootPayload(ctx, nil, routeParams{
		taskSvc: taskSvc,
		services: &Services{
			Workflow: workflowSvc,
		},
	}, webapp.ClassifyRoute("/github"))

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}
	var decoded struct {
		RouteData map[string]json.RawMessage `json:"routeData"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal payload: %v", err)
	}
	if _, ok := decoded.RouteData["github"]; ok {
		t.Fatal("integration boot data must not include provider GitHub results")
	}
	var contextData struct {
		ActiveWorkspaceID string `json:"activeWorkspaceId"`
		Workflows         []struct {
			ID string `json:"id"`
		} `json:"workflows"`
		Steps []struct {
			ID string `json:"id"`
		} `json:"steps"`
		Repositories []struct {
			ID string `json:"id"`
		} `json:"repositories"`
	}
	if err := json.Unmarshal(decoded.RouteData["routeContext"], &contextData); err != nil {
		t.Fatalf("Unmarshal routeContext: %v", err)
	}
	if contextData.ActiveWorkspaceID != workspaces[0].ID {
		t.Fatalf("active workspace = %q, want %q", contextData.ActiveWorkspaceID, workspaces[0].ID)
	}
	if len(contextData.Workflows) == 0 || len(contextData.Steps) == 0 {
		t.Fatalf("expected local workflow context, got %+v", contextData)
	}
}

func TestBootRouteDataIntegrationRouteUsesActiveWorkspaceCookie(t *testing.T) {
	taskSvc, workflowSvc := newBootStateTestServices(t)
	ctx := context.Background()
	if _, err := taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{
		Name: "Cookie Workspace",
	}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	workspace := nonFallbackWorkspace(t, ctx, taskSvc)
	if _, err := taskSvc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{
		WorkspaceID: workspace.ID,
		Name:        "Cookie Workflow",
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-state?path=%2Fgithub", nil)
	req.AddCookie(&http.Cookie{Name: "kandev-active-workspace", Value: workspace.ID})
	payload := bootPayload(ctx, req, routeParams{
		taskSvc: taskSvc,
		services: &Services{
			Workflow: workflowSvc,
		},
	}, webapp.ClassifyRoute("/github"))

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}
	var decoded struct {
		RouteData struct {
			RouteContext struct {
				ActiveWorkspaceID string `json:"activeWorkspaceId"`
			} `json:"routeContext"`
		} `json:"routeData"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal payload: %v", err)
	}
	if decoded.RouteData.RouteContext.ActiveWorkspaceID != workspace.ID {
		t.Fatalf("route active workspace = %q, want %q", decoded.RouteData.RouteContext.ActiveWorkspaceID, workspace.ID)
	}
}

func TestBootRouteDataIntegrationRouteCookieWinsOverSettingsWorkspace(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(workspaces) == 0 {
		t.Fatal("expected seeded default workspace")
	}
	settingsWorkspaceID := workspaces[0].ID
	cookieWorkspace, err := harness.taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{
		Name: "Cookie Workspace",
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if _, err := harness.taskSvc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{
		WorkspaceID: cookieWorkspace.ID,
		Name:        "Cookie Workflow",
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if _, err := harness.userSvc.UpdateUserSettings(ctx, &userservice.UpdateUserSettingsRequest{
		WorkspaceID: &settingsWorkspaceID,
	}); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-state?path=%2Fgithub", nil)
	req.AddCookie(&http.Cookie{Name: "kandev-active-workspace", Value: cookieWorkspace.ID})
	payload := bootPayload(ctx, req, routeParams{
		taskSvc:  harness.taskSvc,
		userCtrl: harness.userCtrl,
		services: &Services{
			Workflow: harness.workflowSvc,
		},
	}, webapp.ClassifyRoute("/github"))

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}
	var decoded struct {
		InitialState struct {
			Workspaces struct {
				ActiveID string `json:"activeId"`
			} `json:"workspaces"`
			UserSettings struct {
				WorkspaceID string `json:"workspaceId"`
			} `json:"userSettings"`
		} `json:"initialState"`
		RouteData struct {
			RouteContext struct {
				ActiveWorkspaceID string `json:"activeWorkspaceId"`
			} `json:"routeContext"`
		} `json:"routeData"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal payload: %v", err)
	}
	if decoded.InitialState.Workspaces.ActiveID != cookieWorkspace.ID {
		t.Fatalf("initial active workspace = %q, want %q", decoded.InitialState.Workspaces.ActiveID, cookieWorkspace.ID)
	}
	if decoded.InitialState.UserSettings.WorkspaceID != cookieWorkspace.ID {
		t.Fatalf("user settings workspace = %q, want %q", decoded.InitialState.UserSettings.WorkspaceID, cookieWorkspace.ID)
	}
	if decoded.RouteData.RouteContext.ActiveWorkspaceID != cookieWorkspace.ID {
		t.Fatalf("route active workspace = %q, want %q", decoded.RouteData.RouteContext.ActiveWorkspaceID, cookieWorkspace.ID)
	}
}

func nonFallbackWorkspace(t *testing.T, ctx context.Context, taskSvc *taskservice.Service) *models.Workspace {
	t.Helper()
	workspaces, err := taskSvc.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(workspaces) < 2 {
		t.Fatalf("expected at least two workspaces, got %d", len(workspaces))
	}
	fallbackID := workspaces[0].ID
	for _, workspace := range workspaces {
		if workspace != nil && workspace.ID != fallbackID {
			return workspace
		}
	}
	t.Fatal("expected a workspace different from the default fallback")
	return nil
}

func TestBootInitialStateSettingsWithNilServicesDoesNotPanic(t *testing.T) {
	state := bootInitialState(
		context.Background(),
		nil,
		routeParams{features: config.FeaturesConfig{Office: true}},
		webapp.ClassifyRoute("/settings/prompts"),
	)

	if _, ok := state["features"]; !ok {
		t.Fatal("features should always be present even when optional services are unavailable")
	}
	if _, ok := state["prompts"]; ok {
		t.Fatal("prompts should not be marked loaded when the prompts controller is unavailable")
	}
}

func TestQueryValueReadsRouteQueryFromAppStatePath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/app-state?path=%2F%3FworkspaceId%3Dws-1%26workflowId%3Dwf-1", nil)

	if got := queryValue(req, "workspaceId"); got != "ws-1" {
		t.Fatalf("workspaceId = %q, want ws-1", got)
	}
	if got := queryValue(req, "workflowId"); got != "wf-1" {
		t.Fatalf("workflowId = %q, want wf-1", got)
	}
}

type bootStateTestHarness struct {
	taskSvc     *taskservice.Service
	workflowSvc *workflowservice.Service
	userCtrl    *usercontroller.Controller
	userSvc     *userservice.Service
}

func newBootStateTestServices(t *testing.T) (*taskservice.Service, *workflowservice.Service) {
	harness := newBootStateTestHarness(t)
	return harness.taskSvc, harness.workflowSvc
}

func newBootStateTestHarness(t *testing.T) bootStateTestHarness {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	taskRepo, cleanup, err := taskrepo.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("task repository: %v", err)
	}
	if _, err := worktree.NewSQLiteStore(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("worktree store: %v", err)
	}
	workflowRepo, err := workflowrepo.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("workflow repository: %v", err)
	}
	userRepo, userCleanup, err := userstore.Provide(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("user repository: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlxDB.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
		if err := cleanup(); err != nil {
			t.Errorf("close task repo: %v", err)
		}
		if err := userCleanup(); err != nil {
			t.Errorf("close user repo: %v", err)
		}
	})

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	eventBus := bus.NewMemoryEventBus(log)
	userSvc := userservice.NewService(userRepo, eventBus, log)
	workflowSvc := workflowservice.NewService(workflowRepo, log)
	taskSvc := taskservice.NewService(
		taskservice.Repos{
			Workspaces:       taskRepo,
			Tasks:            taskRepo,
			TaskRepos:        taskRepo,
			Workflows:        taskRepo,
			Messages:         taskRepo,
			Turns:            taskRepo,
			Sessions:         taskRepo,
			GitSnapshots:     taskRepo,
			RepoEntities:     taskRepo,
			Executors:        taskRepo,
			Environments:     taskRepo,
			TaskEnvironments: taskRepo,
			Reviews:          taskRepo,
		},
		eventBus,
		log,
		taskservice.RepositoryDiscoveryConfig{},
	)
	taskSvc.SetWorkflowStepCreator(workflowSvc)
	taskSvc.SetWorkflowStepGetter(&workflowStepGetterAdapter{svc: workflowSvc})
	taskSvc.SetStartStepResolver(&startStepResolverAdapter{svc: workflowSvc})
	workflowSvc.SetWorkflowProvider(&workflowProviderAdapter{svc: taskSvc})
	return bootStateTestHarness{
		taskSvc:     taskSvc,
		workflowSvc: workflowSvc,
		userCtrl:    usercontroller.NewController(userSvc),
		userSvc:     userSvc,
	}
}

func TestNewWebAppHandlerUsesEmbeddedAssetsWithoutDistDir(t *testing.T) {
	t.Setenv("KANDEV_WEB_DIST_DIR", "")
	t.Chdir(t.TempDir())

	handler, source, ok := newWebAppHandler(routeParams{})
	if !ok {
		t.Fatal("expected embedded web app handler when no filesystem dist dir exists")
	}
	if source != "embedded" {
		t.Fatalf("source = %q, want embedded", source)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "window.__KANDEV_BOOT_PAYLOAD__") {
		t.Fatal("embedded shell should receive boot payload injection")
	}
}

func TestResolveActiveOfficeWorkspaceIDPrefersCookie(t *testing.T) {
	workspaces := []taskdto.WorkspaceDTO{
		{ID: "ws-a", OfficeWorkflowID: "office-a"},
		{ID: "ws-b", OfficeWorkflowID: "office-b"},
	}

	got := resolveActiveOfficeWorkspaceID(workspaces, "ws-b")
	if got != "ws-b" {
		t.Fatalf("expected cookie workspace to win, got %q", got)
	}
}

func TestAppendSessionStateMessage_IncludesUpdatedAt(t *testing.T) {
	updatedAt := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	session := &models.TaskSession{
		ID:        "sess-3",
		TaskID:    "task-1",
		State:     models.TaskSessionStateWaitingForInput,
		UpdatedAt: updatedAt,
	}

	msgs := appendSessionStateMessage(session.ID, session, nil)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	payload := decodePayload(t, msgs[0].Payload)
	got, present := payload["updated_at"]
	if !present {
		t.Fatal("payload missing updated_at — stale subscribe snapshots cannot be ignored")
	}
	if got != updatedAt.Format(time.RFC3339Nano) {
		t.Fatalf("expected updated_at=%q, got %v", updatedAt.Format(time.RFC3339Nano), got)
	}
}

func TestAppendContextWindowMessage_DoesNotEmitStateSnapshot(t *testing.T) {
	session := &models.TaskSession{
		ID:        "sess-4",
		TaskID:    "task-1",
		State:     models.TaskSessionStateRunning,
		UpdatedAt: time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC),
		Metadata: map[string]interface{}{
			"context_window": map[string]interface{}{"size": 100},
		},
	}

	msgs := appendContextWindowMessage(session.ID, session, nil)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	payload := decodePayload(t, msgs[0].Payload)
	if _, present := payload["new_state"]; present {
		t.Fatal("context-window snapshot must not carry new_state and overwrite fresher session state")
	}
}

// TestAppendSessionStateMessage_OmitsEmptyTaskEnvironmentID — sessions without
// an environment (legacy rows, archived sessions) must not emit an empty
// task_environment_id field that would clobber a populated frontend map.
func TestAppendSessionStateMessage_OmitsEmptyTaskEnvironmentID(t *testing.T) {
	session := &models.TaskSession{
		ID:     "sess-2",
		TaskID: "task-1",
		State:  models.TaskSessionStateCompleted,
	}

	msgs := appendSessionStateMessage(session.ID, session, nil)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	payload := decodePayload(t, msgs[0].Payload)
	if _, present := payload["task_environment_id"]; present {
		t.Fatalf("payload should not include task_environment_id when session has none")
	}
}

func TestExternalMCPOpenMiddleware_AllowsLoopbackAndRemote(t *testing.T) {
	r := setupExternalMCPAccessRouter()

	for _, tc := range []struct{ name, remoteAddr string }{
		{"loopback", "127.0.0.1:4321"},
		{"remote", "203.0.113.10:4321"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/mcp", nil)
			req.RemoteAddr = tc.remoteAddr
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("request = %d, want %d", w.Code, http.StatusOK)
			}
		})
	}
}

func setupExternalMCPAccessRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(externalMCPOpenMiddleware())
	r.GET("/mcp", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}
