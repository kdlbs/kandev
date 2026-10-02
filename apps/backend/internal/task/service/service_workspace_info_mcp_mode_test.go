package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/mcpmode"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/stretchr/testify/require"
)

// fakeMcpModeTaskRepo overrides only GetTask so resolveWorkspaceInfoMcpMode
// can be tested against read outcomes that don't correspond to a real row
// (BUILD DECISION F14, docs/specs/coordinator/system-design/copilot.md#fail-closed).
type fakeMcpModeTaskRepo struct {
	taskrepo.TaskRepository
	task *models.Task
	err  error
}

func (r *fakeMcpModeTaskRepo) GetTask(context.Context, string) (*models.Task, error) {
	return r.task, r.err
}

func TestResolveWorkspaceInfoMcpMode_EmptyTaskID(t *testing.T) {
	svc, _, _ := createTestService(t)
	mode, err := svc.resolveWorkspaceInfoMcpMode(context.Background(), "")
	require.NoError(t, err)
	require.Empty(t, mode)
}

func TestResolveWorkspaceInfoMcpMode_NilTaskIsEmptyWithNoError(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.tasks = &fakeMcpModeTaskRepo{task: nil, err: nil}
	mode, err := svc.resolveWorkspaceInfoMcpMode(context.Background(), "task-missing")
	require.NoError(t, err)
	require.Empty(t, mode)
}

func TestResolveWorkspaceInfoMcpMode_NotFoundIsEmptyWithNoError(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.tasks = &fakeMcpModeTaskRepo{err: fmt.Errorf("get task: %w", taskrepo.ErrTaskNotFound)}
	mode, err := svc.resolveWorkspaceInfoMcpMode(context.Background(), "task-missing")
	require.NoError(t, err)
	require.Empty(t, mode)
}

func TestResolveWorkspaceInfoMcpMode_OtherReadErrorFails(t *testing.T) {
	svc, _, _ := createTestService(t)
	readErr := errors.New("boom")
	svc.tasks = &fakeMcpModeTaskRepo{err: readErr}
	_, err := svc.resolveWorkspaceInfoMcpMode(context.Background(), "task-123")
	require.Error(t, err)
	require.ErrorIs(t, err, readErr)
}

func TestResolveWorkspaceInfoMcpMode_CoordinatorOriginTask(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.tasks = &fakeMcpModeTaskRepo{task: &models.Task{ID: "task-coordinator", Origin: models.TaskOriginCoordinator}}
	mode, err := svc.resolveWorkspaceInfoMcpMode(context.Background(), "task-coordinator")
	require.NoError(t, err)
	require.Equal(t, mcpmode.Coordinator, mode)
}

func TestResolveWorkspaceInfoMcpMode_KanbanOriginIsEmpty(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.tasks = &fakeMcpModeTaskRepo{task: &models.Task{ID: "task-kanban"}}
	mode, err := svc.resolveWorkspaceInfoMcpMode(context.Background(), "task-kanban")
	require.NoError(t, err)
	require.Empty(t, mode)
}

// TestGetWorkspaceInfoForSession_CoordinatorOriginTaskSetsMcpMode is the
// integration-level companion: a conversation-task session for a
// coordinator-origin task must surface McpMode on the full WorkspaceInfo.
func TestGetWorkspaceInfoForSession_CoordinatorOriginTaskSetsMcpMode(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-coordinator-convo", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123",
		Title: "Coordinator conversation", Priority: "medium", Origin: models.TaskOriginCoordinator,
	}))
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-coordinator-convo", TaskID: "task-coordinator-convo", AgentProfileID: "profile-1",
		State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now,
	}))

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-coordinator-convo", "session-coordinator-convo")
	require.NoError(t, err)
	require.Equal(t, mcpmode.Coordinator, info.McpMode)
}

// TestGetWorkspaceInfoForSession_KanbanTaskLeavesMcpModeEmpty pins the
// unchanged default: an ordinary Kanban-origin task never claims the
// coordinator MCP mode.
func TestGetWorkspaceInfoForSession_KanbanTaskLeavesMcpModeEmpty(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-kanban-mcp-mode", TaskID: "task-123", AgentProfileID: "profile-1",
		State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now,
	}))

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-kanban-mcp-mode")
	require.NoError(t, err)
	require.Empty(t, info.McpMode)
}

// TestGetWorkspaceInfoForSession_ConfiguredServiceStillFailsOnMissingTask
// pins populateWorkspaceRepositorySpecs's pre-existing behavior (BUILD
// DECISION F14): with taskRepos/repoEntities configured, a task read that
// fails with ErrTaskNotFound must still fail GetWorkspaceInfoForSession, and
// must never reach the new McpMode derivation. The session's task row is
// created for real (the FK constraint requires it); only the task lookup
// itself is swapped to observe a not-found read, mirroring a benign
// delete/lookup race rather than a schema violation.
func TestGetWorkspaceInfoForSession_ConfiguredServiceStillFailsOnMissingTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-orphan-task", TaskID: "task-123", AgentProfileID: "profile-1",
		State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now,
	}))
	svc.tasks = &fakeMcpModeTaskRepo{err: fmt.Errorf("get task: %w", taskrepo.ErrTaskNotFound)}

	_, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-orphan-task")
	require.Error(t, err)
	require.ErrorIs(t, err, taskrepo.ErrTaskNotFound)
}
