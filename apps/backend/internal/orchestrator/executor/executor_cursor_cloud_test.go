package executor

import (
	"context"
	"testing"
	"time"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestCursorCloudLaunchSkipsLocalWorkspacePreparation(t *testing.T) {
	repo := newMockRepository()
	task := &models.Task{ID: "task-cloud", WorkspaceID: "workspace-cloud", Title: "Cloud task"}
	session := &models.TaskSession{
		ID: "session-cloud", TaskID: task.ID, ExecutorID: "executor-cloud", ExecutorProfileID: "executor-profile-cloud",
		AgentProfileID: "agent-profile-cloud", State: models.TaskSessionStateCreated,
		StartedAt: time.Now().UTC(), AgentProfileSnapshot: map[string]interface{}{"model": "cursor-model"},
	}
	repo.tasks[task.ID] = task
	repo.sessions[session.ID] = session
	repo.executors[session.ExecutorID] = &models.Executor{ID: session.ExecutorID, Type: models.ExecutorTypeCursorCloud}
	repo.executorProfiles[session.ExecutorProfileID] = &models.ExecutorProfile{ID: session.ExecutorProfileID, ExecutorID: session.ExecutorID}

	started := make(chan struct{}, 1)
	manager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, request *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			if request.TurnID != "turn-cloud-launch" {
				t.Fatalf("cloud turn ID = %q, want durable launch turn", request.TurnID)
			}
			if request.TaskDescription != "composed prompt" {
				t.Fatalf("cloud prompt = %q, want composed prompt", request.TaskDescription)
			}
			if request.TaskEnvironmentID != "" || request.WorkspacePath != "" || request.RepositoryPath != "" || len(request.Env) != 0 {
				t.Fatalf("cloud launch included local workspace state: %#v", request)
			}
			if request.ExecutorType != string(models.ExecutorTypeCursorCloud) || request.ModelOverride != "cursor-model" {
				t.Fatalf("cloud executor selection = %#v", request)
			}
			return &LaunchAgentResponse{AgentExecutionID: "managed-execution", Status: v1.AgentStatusRunning}, nil
		},
		startAgentProcessFunc: func(context.Context, string) error { started <- struct{}{}; return nil },
	}
	exec := newTestExecutor(t, manager, repo)
	got, err := exec.LaunchPreparedSession(context.Background(), &v1.Task{ID: task.ID, WorkspaceID: task.WorkspaceID, Title: task.Title}, session.ID, LaunchOptions{
		AgentProfileID: session.AgentProfileID, ExecutorID: session.ExecutorID,
		Prompt: "composed prompt", StartAgent: true, McpMode: "task", TurnID: "turn-cloud-launch",
		McpProfile: ptrMCPProfile(mcpprofile.New(mcpprofile.SurfaceKanbanTask, nil, nil)),
	})
	if err != nil {
		t.Fatalf("LaunchPreparedSession: %v", err)
	}
	if got.AgentExecutionID != "managed-execution" {
		t.Fatalf("execution ID = %q", got.AgentExecutionID)
	}
	if len(repo.createTaskEnvironmentCalls) != 0 || len(repo.finalizeTaskEnvironmentCalls) != 0 {
		t.Fatalf("Cursor Cloud prepared local environments: creates=%d finalizes=%d", len(repo.createTaskEnvironmentCalls), len(repo.finalizeTaskEnvironmentCalls))
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Cursor Cloud start dispatch was not scheduled")
	}
}

func TestCursorCloudPreparedSessionPersistsExecutionCapabilities(t *testing.T) {
	repo := newMockRepository()
	session := &models.TaskSession{
		ID: "session-cloud-capabilities", TaskID: "task-cloud-capabilities",
		ExecutorID: "executor-cloud", ExecutorProfileID: "executor-profile-cloud",
		State: models.TaskSessionStateCreated, StartedAt: time.Now().UTC(),
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	if err := exec.createPreparedSession(context.Background(), session, nil, false, executorConfig{
		ExecutorID: "executor-cloud", ExecutorType: string(models.ExecutorTypeCursorCloud),
	}, nil); err != nil {
		t.Fatalf("createPreparedSession: %v", err)
	}
	stored, err := repo.GetTaskSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	capabilities := models.SessionExecutionCapabilities(stored)
	if capabilities == nil {
		t.Fatal("Cursor Cloud session did not persist its execution capability snapshot")
	}
	if !capabilities.Chat || !capabilities.Stop || !capabilities.FollowUp || !capabilities.RemoteResults {
		t.Fatalf("Cursor Cloud capabilities = %+v, want chat, stop, follow-up, and remote results", capabilities)
	}
	if capabilities.WorkspaceFiles || capabilities.ModelSwitch || capabilities.PermissionModeSwitch || capabilities.PlanModeSwitch {
		t.Fatalf("Cursor Cloud exposed unsupported capabilities: %+v", capabilities)
	}
}

func TestCursorCloudLaunchRejectsAttachmentsBeforeManagerCall(t *testing.T) {
	repo := newMockRepository()
	task := &models.Task{ID: "task-cloud", WorkspaceID: "workspace-cloud", Title: "Cloud task"}
	session := &models.TaskSession{
		ID: "session-cloud", TaskID: task.ID, ExecutorID: "executor-cloud", ExecutorProfileID: "executor-profile-cloud",
		AgentProfileID: "agent-profile-cloud", State: models.TaskSessionStateCreated,
		StartedAt: time.Now().UTC(),
	}
	repo.tasks[task.ID] = task
	repo.sessions[session.ID] = session
	repo.executors[session.ExecutorID] = &models.Executor{ID: session.ExecutorID, Type: models.ExecutorTypeCursorCloud}
	repo.executorProfiles[session.ExecutorProfileID] = &models.ExecutorProfile{ID: session.ExecutorProfileID, ExecutorID: session.ExecutorID}
	manager := &mockAgentManager{}
	exec := newTestExecutor(t, manager, repo)
	_, err := exec.LaunchPreparedSession(context.Background(), &v1.Task{ID: task.ID, WorkspaceID: task.WorkspaceID, Title: task.Title}, session.ID, LaunchOptions{
		AgentProfileID: session.AgentProfileID, ExecutorID: session.ExecutorID,
		Prompt: "composed prompt", StartAgent: true, Attachments: []v1.MessageAttachment{{Name: "image.png"}},
		McpMode: "task", McpProfile: ptrMCPProfile(mcpprofile.New(mcpprofile.SurfaceKanbanTask, nil, nil)),
	})
	if err == nil {
		t.Fatal("LaunchPreparedSession accepted unsupported attachments")
	}
	if manager.launchAgentCallCount != 0 {
		t.Fatalf("LaunchAgent called %d times for rejected attachments", manager.launchAgentCallCount)
	}
}

func ptrMCPProfile(value mcpprofile.Context) *mcpprofile.Context { return &value }
