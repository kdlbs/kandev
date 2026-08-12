package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	usermodels "github.com/kandev/kandev/internal/user/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestHandleCreateTask_UsesVerifiedCreatorSessionProfileAndRuntime(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	workflow, err := svc.CreateWorkflow(ctx, &service.CreateWorkflowRequest{
		WorkspaceID: workspaces[0].ID,
		Name:        "Creator session workflow",
	})
	require.NoError(t, err)

	sourceResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: workspaces[0].ID,
		WorkflowID:  workflow.ID,
		Title:       "Source task",
		Metadata: map[string]interface{}{
			models.MetaKeyAgentProfileID: "source-task-profile",
		},
	})
	require.NoError(t, err)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "creator-session",
		TaskID:            sourceResult.Task.ID,
		AgentProfileID:    "creator-session-profile",
		ExecutorProfileID: "source-executor-profile",
		State:             models.TaskSessionStateWaitingForInput,
		IsPrimary:         true,
		AgentProfileSnapshot: map[string]interface{}{
			"model": "profile-model",
			"mode":  "profile-mode",
			"config_options": map[string]string{
				"reasoning_effort": "medium",
			},
		},
		Metadata: map[string]interface{}{
			models.SessionMetaKeyRuntimeConfig: models.SessionRuntimeConfig{
				Model: "runtime-model",
				ConfigOptions: map[string]string{
					"reasoning_effort": "high",
				},
			},
			models.SessionMetaKeySessionMode: "acceptEdits",
			models.SessionMetaKeyRuntimeConfigOverrides: models.SessionRuntimeConfig{
				ConfigOptions: map[string]string{
					"reasoning_effort": "low",
				},
			},
		},
	}))

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	resp, err := h.handleCreateTask(ctx, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"source_task_id":    sourceResult.Task.ID,
		"source_session_id": "creator-session",
		"workspace_id":      workspaces[0].ID,
		"workflow_id":       workflow.ID,
		"title":             "Creator session child",
		"start_agent":       false,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, "payload: %s", string(resp.Payload))

	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &created))
	task, err := svc.GetTask(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "creator-session-profile", task.Metadata[models.MetaKeyAgentProfileID])
	require.Equal(t, "source-executor-profile", task.Metadata[models.MetaKeyExecutorProfileID])

	seed, ok := models.LoadInitialSessionRuntimeConfig(task.Metadata)
	require.True(t, ok)
	require.Equal(t, "runtime-model", seed.Model)
	require.Equal(t, "acceptEdits", seed.Mode)
	require.Equal(t, map[string]string{"reasoning_effort": "low"}, seed.ConfigOptions)
}

func TestHandleCreateTask_RejectsCreatorSessionFromAnotherTaskBeforePersistence(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	workflow, err := svc.CreateWorkflow(ctx, &service.CreateWorkflowRequest{
		WorkspaceID: workspaces[0].ID,
		Name:        "Creator attribution workflow",
	})
	require.NoError(t, err)

	sourceResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: workspaces[0].ID,
		WorkflowID:  workflow.ID,
		Title:       "Source task",
	})
	require.NoError(t, err)
	foreignResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: workspaces[0].ID,
		WorkflowID:  workflow.ID,
		Title:       "Foreign task",
	})
	require.NoError(t, err)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:             "foreign-session",
		TaskID:         foreignResult.Task.ID,
		AgentProfileID: "foreign-profile",
		State:          models.TaskSessionStateWaitingForInput,
	}))

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	resp, err := h.handleCreateTask(ctx, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"source_task_id":    sourceResult.Task.ID,
		"source_session_id": "foreign-session",
		"workspace_id":      workspaces[0].ID,
		"workflow_id":       workflow.ID,
		"title":             "Should not persist",
		"start_agent":       false,
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)

	tasks, err := svc.ListTasks(ctx, workflow.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 2)
}

func TestHandleCreateTask_SubtaskKeepsParentExecutorWhenCreatorProfileWins(t *testing.T) {
	ctx := context.Background()
	svc, workspace, workflow, source := newCreatorSessionTask(t)
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}

	resp, err := h.handleCreateTask(ctx, makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"parent_id":         source.ID,
		"source_task_id":    source.ID,
		"source_session_id": "creator-session",
		"title":             "Creator session subtask",
		"start_agent":       false,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, "payload: %s", string(resp.Payload))

	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &created))
	task, err := svc.GetTask(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, workspace.ID, task.WorkspaceID)
	require.Equal(t, workflow.ID, task.WorkflowID)
	require.Equal(t, "creator-session-profile", task.Metadata[models.MetaKeyAgentProfileID])
	require.Equal(t, "parent-executor-profile", task.Metadata[models.MetaKeyExecutorProfileID])
	seed, ok := models.LoadInitialSessionRuntimeConfig(task.Metadata)
	require.True(t, ok)
	require.Equal(t, "creator-runtime-model", seed.Model)
}

func TestResolveMCPAutoStartConfig_ExplicitAndWorkspaceProfilesSuppressCreatorRuntime(t *testing.T) {
	ctx := context.Background()
	svc, workspace, workflow, source := newCreatorSessionTask(t)

	explicitProvider := &mcpUserSettingsProvider{err: context.Canceled}
	explicitHandler := &Handlers{
		taskSvc:              svc,
		userSettingsProvider: explicitProvider,
		logger:               testLogger(t).WithFields(),
	}
	explicit, err := explicitHandler.resolveMCPAutoStartConfigWithSource(
		ctx,
		&models.Task{WorkspaceID: workspace.ID, WorkflowID: workflow.ID},
		"explicit-profile", "", source.ID, "creator-session",
	)
	require.NoError(t, err)
	require.Equal(t, "explicit-profile", explicit.AgentProfileID)
	require.Nil(t, explicit.InitialRuntimeConfig)
	require.Zero(t, explicitProvider.calls)

	workspaceProfile := "workspace-profile"
	_, err = svc.UpdateWorkspace(ctx, workspace.ID, &service.UpdateWorkspaceRequest{
		DefaultAgentProfileID: &workspaceProfile,
	})
	require.NoError(t, err)
	workspaceHandler := &Handlers{
		taskSvc: svc,
		userSettingsProvider: &mcpUserSettingsProvider{settings: &usermodels.UserSettings{
			MCPTaskAgentProfileDefault: usermodels.MCPTaskAgentProfileDefaultWorkspaceDefault,
		}},
		logger: testLogger(t).WithFields(),
	}
	workspaceConfig, err := workspaceHandler.resolveMCPAutoStartConfigWithSource(
		ctx,
		&models.Task{WorkspaceID: workspace.ID, WorkflowID: workflow.ID},
		"", "", source.ID, "creator-session",
	)
	require.NoError(t, err)
	require.Equal(t, workspaceProfile, workspaceConfig.AgentProfileID)
	require.Nil(t, workspaceConfig.InitialRuntimeConfig)
}

func TestResolveMCPAutoStartConfig_WorkflowProfileSuppressesCreatorRuntime(t *testing.T) {
	ctx := context.Background()
	svc, repo, workflowCtrl, workflowRepo := newTestTaskServiceWithWorkflow(t)
	workspace, workflow := defaultWorkspaceAndWorkflow(t, ctx, svc)
	step := seedWorkflowStep(t, ctx, workflowRepo, &workflowmodels.WorkflowStep{
		WorkflowID:      workflow.ID,
		Name:            "Workflow profile",
		Position:        1,
		AgentProfileID:  "workflow-profile",
		AllowManualMove: true,
	})
	sourceResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: workspace.ID,
		WorkflowID:  workflow.ID,
		Title:       "Workflow source",
	})
	require.NoError(t, err)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:             "workflow-creator-session",
		TaskID:         sourceResult.Task.ID,
		AgentProfileID: "creator-session-profile",
		State:          models.TaskSessionStateWaitingForInput,
		AgentProfileSnapshot: map[string]interface{}{
			"model": "creator-runtime-model",
		},
	}))

	h := &Handlers{taskSvc: svc, workflowCtrl: workflowCtrl, logger: testLogger(t).WithFields()}
	config, err := h.resolveMCPAutoStartConfigWithSource(
		ctx,
		&models.Task{WorkspaceID: workspace.ID, WorkflowID: workflow.ID, WorkflowStepID: step.ID},
		"", "", sourceResult.Task.ID, "workflow-creator-session",
	)
	require.NoError(t, err)
	require.Equal(t, "workflow-profile", config.AgentProfileID)
	require.Nil(t, config.InitialRuntimeConfig)
}

func newCreatorSessionTask(t *testing.T) (*service.Service, *models.Workspace, *models.Workflow, *models.Task) {
	t.Helper()
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	workflow, err := svc.CreateWorkflow(ctx, &service.CreateWorkflowRequest{
		WorkspaceID: workspaces[0].ID,
		Name:        "Creator session workflow",
	})
	require.NoError(t, err)
	sourceResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: workspaces[0].ID,
		WorkflowID:  workflow.ID,
		Title:       "Source task",
		Metadata: map[string]interface{}{
			models.MetaKeyAgentProfileID:    "source-task-profile",
			models.MetaKeyExecutorProfileID: "parent-executor-profile",
		},
	})
	require.NoError(t, err)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "creator-session",
		TaskID:            sourceResult.Task.ID,
		AgentProfileID:    "creator-session-profile",
		ExecutorProfileID: "parent-executor-profile",
		State:             models.TaskSessionStateWaitingForInput,
		IsPrimary:         true,
		AgentProfileSnapshot: map[string]interface{}{
			"model": "creator-runtime-model",
		},
	}))
	return svc, workspaces[0], workflow, sourceResult.Task
}
