package executor

import (
	"context"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestResolveTaskSessionMCPProfile_SelectsSurfaceAndQuestionCapability(t *testing.T) {
	tests := []struct {
		name               string
		task               *models.Task
		session            *models.TaskSession
		allowTitle         bool
		wantSurface        mcpprofile.Surface
		wantUserQuestion   bool
		wantParentQuestion bool
		wantTaskTitle      bool
	}{
		{
			name:               "normal kanban task",
			task:               &models.Task{ID: "task"},
			session:            &models.TaskSession{ID: "session", TaskID: "task"},
			wantSurface:        mcpprofile.SurfaceKanbanTask,
			wantUserQuestion:   true,
			wantParentQuestion: false,
		},
		{
			name:               "normal passthrough task",
			task:               &models.Task{ID: "task"},
			session:            &models.TaskSession{ID: "session", TaskID: "task", IsPassthrough: true},
			wantSurface:        mcpprofile.SurfaceKanbanTask,
			wantUserQuestion:   false,
			wantParentQuestion: false,
		},
		{
			name:               "autopilot root",
			task:               &models.Task{ID: "task", Autopilot: true},
			session:            &models.TaskSession{ID: "session", TaskID: "task"},
			wantSurface:        mcpprofile.SurfaceKanbanTask,
			wantUserQuestion:   false,
			wantParentQuestion: false,
		},
		{
			name:               "autopilot child",
			task:               &models.Task{ID: "task", ParentID: "parent", Autopilot: true},
			session:            &models.TaskSession{ID: "session", TaskID: "task"},
			wantSurface:        mcpprofile.SurfaceKanbanTask,
			wantUserQuestion:   false,
			wantParentQuestion: true,
		},
		{
			name:               "autopilot passthrough child",
			task:               &models.Task{ID: "task", ParentID: "parent", Autopilot: true},
			session:            &models.TaskSession{ID: "session", TaskID: "task", IsPassthrough: true},
			wantSurface:        mcpprofile.SurfaceKanbanTask,
			wantUserQuestion:   false,
			wantParentQuestion: false,
		},
		{
			name:               "office task",
			task:               &models.Task{ID: "task", IsFromOffice: true},
			session:            &models.TaskSession{ID: "session", TaskID: "task"},
			wantSurface:        mcpprofile.SurfaceOfficeTask,
			wantUserQuestion:   true,
			wantParentQuestion: false,
		},
		{
			name:               "automation task",
			task:               &models.Task{ID: "task", Origin: models.TaskOriginAutomationRun},
			session:            &models.TaskSession{ID: "session", TaskID: "task"},
			wantSurface:        mcpprofile.SurfaceAutomation,
			wantUserQuestion:   false,
			wantParentQuestion: false,
		},
		{
			name:               "configuration session",
			task:               &models.Task{ID: "task"},
			session:            &models.TaskSession{ID: "session", TaskID: "task", Metadata: map[string]interface{}{"config_mode": true}},
			wantSurface:        mcpprofile.SurfaceConfiguration,
			wantUserQuestion:   true,
			wantParentQuestion: false,
		},
		{
			name:               "title owner adds title capability",
			task:               &models.Task{ID: "task", Metadata: map[string]interface{}{models.MetaKeyAgentTitlePending: true, models.MetaKeyAgentTitleOwnerSessionID: "session"}},
			session:            &models.TaskSession{ID: "session", TaskID: "task"},
			allowTitle:         true,
			wantSurface:        mcpprofile.SurfaceKanbanTask,
			wantUserQuestion:   true,
			wantParentQuestion: false,
			wantTaskTitle:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepository()
			repo.tasks[tt.task.ID] = tt.task
			repo.sessions[tt.session.ID] = tt.session
			exec := newTestExecutor(t, &mockAgentManager{}, repo)

			profile, err := exec.resolveTaskSessionMCPProfile(context.Background(), tt.task.ID, tt.session, tt.allowTitle)
			require.NoError(t, err)
			require.Equal(t, tt.wantSurface, profile.Surface)
			require.Equal(t, tt.wantUserQuestion, profile.HasCapability(mcpprofile.CapabilityUserQuestion))
			require.Equal(t, tt.wantParentQuestion, profile.HasCapability(mcpprofile.CapabilityParentQuestion))
			require.Equal(t, tt.wantTaskTitle, profile.HasCapability(mcpprofile.CapabilityTaskTitle))
		})
	}
}

func TestResolveTaskSessionMCPMode_TitlePendingIsTaskModeVariant(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	repo.tasks["task-pending"] = &models.Task{
		ID:       "task-pending",
		Metadata: map[string]interface{}{models.MetaKeyAgentTitlePending: true, models.MetaKeyAgentTitleOwnerSessionID: "session-pending"},
	}
	repo.sessions["session-pending"] = &models.TaskSession{ID: "session-pending", TaskID: "task-pending"}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	mode, err := exec.resolveTaskSessionMCPMode(ctx, "task-pending", repo.sessions["session-pending"], true)
	require.NoError(t, err)
	require.Equal(t, McpModeTaskTitlePending, mode)

	repo.sessions["session-other"] = &models.TaskSession{ID: "session-other", TaskID: "task-pending"}
	mode, err = exec.resolveTaskSessionMCPMode(ctx, "task-pending", repo.sessions["session-other"], true)
	require.NoError(t, err)
	require.Empty(t, mode)

	mode, err = exec.resolveTaskSessionMCPMode(ctx, "task-pending", repo.sessions["session-pending"], false)
	require.NoError(t, err)
	require.Empty(t, mode)
}

func TestResolveTaskSessionMCPMode_TitlePendingDoesNotOverrideRestrictedModes(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	repo.tasks["task-pending"] = &models.Task{
		ID:           "task-pending",
		IsFromOffice: true,
		Metadata:     map[string]interface{}{models.MetaKeyAgentTitlePending: true},
	}
	repo.sessions["session-config"] = &models.TaskSession{
		TaskID:   "task-pending",
		Metadata: map[string]interface{}{"config_mode": true},
	}
	repo.sessions["session-office"] = &models.TaskSession{TaskID: "task-pending"}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	mode, err := exec.resolveTaskSessionMCPMode(ctx, "task-pending", repo.sessions["session-config"], true)
	require.NoError(t, err)
	require.Equal(t, McpModeConfig, mode)

	mode, err = exec.resolveTaskSessionMCPMode(ctx, "task-pending", repo.sessions["session-office"], true)
	require.NoError(t, err)
	require.Equal(t, McpModeOffice, mode)
}
