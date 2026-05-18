package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kandev/kandev/internal/github"
	officeconfig "github.com/kandev/kandev/internal/office/config"
	"github.com/kandev/kandev/internal/office/configloader"
	officedashboard "github.com/kandev/kandev/internal/office/dashboard"
	officeengineadapters "github.com/kandev/kandev/internal/office/engine_adapters"
	officeonboarding "github.com/kandev/kandev/internal/office/onboarding"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	officeroutines "github.com/kandev/kandev/internal/office/routines"
	officeservice "github.com/kandev/kandev/internal/office/service"
	officewakeup "github.com/kandev/kandev/internal/office/wakeup"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// taskWorkspaceCreatorAdapter adapts the task service to the office
// WorkspaceCreator interface for dual workspace creation.
type taskWorkspaceCreatorAdapter struct {
	taskSvc *taskservice.Service
}

func (a *taskWorkspaceCreatorAdapter) CreateWorkspace(ctx context.Context, name, description string) error {
	_, err := a.taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{
		Name:        name,
		Description: description,
	})
	return err
}

func (a *taskWorkspaceCreatorAdapter) FindWorkspaceIDByName(ctx context.Context, name string) (string, error) {
	workspaces, err := a.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		return "", err
	}
	for _, ws := range workspaces {
		if ws.Name == name {
			return ws.ID, nil
		}
	}
	return "", nil
}

func (a *taskWorkspaceCreatorAdapter) ListWorkspaceNames(ctx context.Context) ([]string, error) {
	workspaces, err := a.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(workspaces))
	for i, ws := range workspaces {
		names[i] = ws.Name
	}
	return names, nil
}

// taskPRListerAdapter adapts *github.Service to the office TaskPRLister
// interface so the children-completed payload can carry per-child PR URLs.
// Returns an empty map (no PRs) when the github service is nil.
type taskPRListerAdapter struct {
	gh *github.Service
}

func (a *taskPRListerAdapter) ListTaskPRsByTaskIDs(
	ctx context.Context, taskIDs []string,
) (map[string][]officeservice.TaskPRLink, error) {
	out := make(map[string][]officeservice.TaskPRLink)
	if a.gh == nil || len(taskIDs) == 0 {
		return out, nil
	}
	prs, err := a.gh.ListTaskPRsByTaskIDs(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	for taskID, list := range prs {
		links := make([]officeservice.TaskPRLink, 0, len(list))
		for _, pr := range list {
			if pr == nil {
				continue
			}
			links = append(links, officeservice.TaskPRLink{
				URL:    pr.PRURL,
				Title:  pr.PRTitle,
				Number: pr.PRNumber,
				State:  pr.State,
			})
		}
		if len(links) > 0 {
			out[taskID] = links
		}
	}
	return out, nil
}

// childTaskCreatorAdapter bridges *taskservice.Service to the
// engine_adapters.ChildTaskCreator interface. The two structs use
// identical fields but live in different packages — this adapter copies
// the fields field-by-field to keep the dependency graph acyclic.
type childTaskCreatorAdapter struct {
	taskSvc *taskservice.Service
}

func (a *childTaskCreatorAdapter) CreateChildTask(
	ctx context.Context, parent *models.Task, spec officeengineadapters.ChildTaskCreateSpec,
) (string, error) {
	return a.taskSvc.CreateChildTask(ctx, parent, taskservice.ChildTaskSpec{
		Title:          spec.Title,
		Description:    spec.Description,
		WorkflowID:     spec.WorkflowID,
		StepID:         spec.StepID,
		AgentProfileID: spec.AgentProfileID,
	})
}

// taskCreatorAdapter adapts the task service to the office TaskCreator interface.
type taskCreatorAdapter struct {
	taskSvc *taskservice.Service
}

func (a *taskCreatorAdapter) CreateOfficeTask(ctx context.Context, workspaceID, projectID, assigneeAgentID, title, description string) (string, error) {
	task, err := a.taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{ //nolint:exhaustruct
		WorkspaceID:            workspaceID,
		Title:                  title,
		Description:            description,
		ProjectID:              projectID,
		AssigneeAgentProfileID: assigneeAgentID,
		Origin:                 models.TaskOriginOnboarding,
	})
	if err != nil {
		return "", err
	}
	return task.ID, nil
}

// CreateOfficeTaskInWorkflow creates an office task pinned to a specific
// workflow id, bypassing the workspace's default office_workflow_id. Used
// for the standing coordination task that lives on the dedicated
// Coordination workflow.
func (a *taskCreatorAdapter) CreateOfficeTaskInWorkflow(
	ctx context.Context, workspaceID, projectID, assigneeAgentID, workflowID, title, description string,
) (string, error) {
	task, err := a.taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{ //nolint:exhaustruct
		WorkspaceID:            workspaceID,
		WorkflowID:             workflowID,
		Title:                  title,
		Description:            description,
		ProjectID:              projectID,
		AssigneeAgentProfileID: assigneeAgentID,
		Origin:                 models.TaskOriginOnboarding,
	})
	if err != nil {
		return "", err
	}
	return task.ID, nil
}

func (a *taskCreatorAdapter) CreateOfficeSubtask(
	ctx context.Context,
	parentTaskID, assigneeAgentID, title, description string,
) (string, error) {
	parent, err := a.taskSvc.GetTask(ctx, parentTaskID)
	if err != nil {
		return "", err
	}
	return a.taskSvc.CreateChildTask(ctx, parent, taskservice.ChildTaskSpec{
		Title:          title,
		Description:    description,
		AgentProfileID: assigneeAgentID,
	})
}

// routineWakeupAdapter bridges the routines package's WakeupEnqueuer
// interface to the concrete office sqlite repo + wakeup dispatcher.
// Defined here (not in the routines package) so the routines package
// stays free of office/repository/sqlite imports — keeping the
// dependency direction routines → wakeup (via interface) and never the
// reverse.
type routineWakeupAdapter struct {
	repo       *officesqlite.Repository
	dispatcher *officewakeup.Dispatcher
}

func (a *routineWakeupAdapter) CreateWakeupRequest(
	ctx context.Context, req *officeroutines.WakeupRequest,
) error {
	row := &officesqlite.WakeupRequest{
		ID:             req.ID,
		AgentProfileID: req.AgentProfileID,
		Source:         req.Source,
		Reason:         req.Reason,
		Payload:        req.Payload,
		RequestedAt:    req.RequestedAt,
	}
	if req.IdempotencyKey != "" {
		row.IdempotencyKey = sql.NullString{String: req.IdempotencyKey, Valid: true}
	}
	return a.repo.CreateWakeupRequest(ctx, row)
}

func (a *routineWakeupAdapter) Dispatch(ctx context.Context, requestID string) error {
	return a.dispatcher.Dispatch(ctx, requestID)
}

// configSyncerAdapter bridges config.ConfigService to the onboarding.ConfigSyncer
// interface by converting config.ImportResult to onboarding.ApplyResult.
type configSyncerAdapter struct {
	svc *officeconfig.ConfigService
}

func (a *configSyncerAdapter) ApplyIncoming(ctx context.Context, workspaceID string) (*officeonboarding.ApplyResult, error) {
	result, err := a.svc.ApplyIncoming(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return &officeonboarding.ApplyResult{
		CreatedCount: result.CreatedCount,
		UpdatedCount: result.UpdatedCount,
	}, nil
}

// workflowEnsurerAdapter wraps the task repo to satisfy onboarding.WorkflowEnsurer.
type workflowEnsurerAdapter struct {
	repo *tasksqlite.Repository
}

func (a *workflowEnsurerAdapter) EnsureOfficeWorkflow(ctx context.Context, workspaceID string) (string, error) {
	return a.repo.EnsureOfficeWorkflow(ctx, workspaceID)
}

func (a *workflowEnsurerAdapter) EnsureOfficeDefaultWorkflow(ctx context.Context, workspaceID string) (string, error) {
	return a.repo.EnsureOfficeDefaultWorkflow(ctx, workspaceID)
}

func (a *workflowEnsurerAdapter) EnsureRoutineWorkflow(ctx context.Context, workspaceID string) (string, error) {
	return a.repo.EnsureRoutineWorkflow(ctx, workspaceID)
}

// workspaceSettingsProviderAdapter implements dashboard.SettingsProvider using
// the filesystem-based config loader and writer.
type workspaceSettingsProviderAdapter struct {
	loader *configloader.ConfigLoader
	writer *configloader.FileWriter
}

func (a *workspaceSettingsProviderAdapter) GetSettings(workspaceName string) (*officedashboard.WorkspaceSettings, error) {
	ws, err := a.loader.GetWorkspace(workspaceName)
	if err != nil {
		// Workspace not found in config loader — return defaults.
		return &officedashboard.WorkspaceSettings{
			Name:                   workspaceName,
			PermissionHandlingMode: "human",
			RecoveryLookbackHours:  24,
		}, nil
	}
	mode := string(ws.Settings.PermissionHandlingMode)
	if mode == "" {
		mode = "human"
	}
	lookback := ws.Settings.RecoveryLookbackHours
	if lookback == 0 {
		lookback = 24
	}
	return &officedashboard.WorkspaceSettings{
		Name:                   ws.Settings.Name,
		Description:            ws.Settings.Description,
		PermissionHandlingMode: mode,
		RecoveryLookbackHours:  lookback,
	}, nil
}

func (a *workspaceSettingsProviderAdapter) UpdatePermissionHandlingMode(workspaceName, mode string) error {
	ws, err := a.loader.GetWorkspace(workspaceName)
	if err != nil {
		// Workspace not in loader — write a minimal kandev.yml.
		ws = &configloader.WorkspaceConfig{
			Name: workspaceName,
			Settings: configloader.WorkspaceSettings{
				Name: workspaceName,
			},
		}
	}
	settings := ws.Settings
	settings.PermissionHandlingMode = configloader.PermissionHandlingMode(mode)
	data, err := configloader.MarshalSettings(settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	return a.writer.WriteRawSettings(workspaceName, data)
}

func (a *workspaceSettingsProviderAdapter) UpdateRecoveryLookbackHours(workspaceName string, hours int) error {
	ws, err := a.loader.GetWorkspace(workspaceName)
	if err != nil {
		ws = &configloader.WorkspaceConfig{
			Name: workspaceName,
			Settings: configloader.WorkspaceSettings{
				Name: workspaceName,
			},
		}
	}
	settings := ws.Settings
	settings.RecoveryLookbackHours = hours
	data, err := configloader.MarshalSettings(settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	return a.writer.WriteRawSettings(workspaceName, data)
}
