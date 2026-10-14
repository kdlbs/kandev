package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"time"

	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/plugins"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowcontroller "github.com/kandev/kandev/internal/workflow/controller"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type pluginsWorkspaceAdminAdapter struct {
	tasks     *taskservice.Service
	workflows *workflowservice.Service
	agents    *agentsettingscontroller.Controller
}

func (a pluginsWorkspaceAdminAdapter) ApplyWorkspaceAdministrationExact(ctx context.Context, input plugins.ExactWorkspaceAdminInput) (plugins.ExactWorkspaceAdminResult, error) {
	command := input.Command
	switch input.OperationKind {
	case pluginsdk.WorkspaceAdminUpdateDefaults:
		return a.updateWorkspaceDefaults(ctx, command.WorkspaceID, input.TargetID, command.UpdateDefaults)
	case pluginsdk.WorkspaceAdminCreateWorkflow:
		return a.createWorkflow(ctx, command.WorkspaceID, input.TargetID, command.CreateWorkflow)
	case pluginsdk.WorkspaceAdminUpdateWorkflow:
		return a.updateWorkflow(ctx, command.WorkspaceID, command.UpdateWorkflow)
	case pluginsdk.WorkspaceAdminReorderWorkflows:
		return a.reorderWorkflows(ctx, command.WorkspaceID, command.ReorderWorkflows)
	case pluginsdk.WorkspaceAdminCreateWorkflowStep:
		return a.createWorkflowStep(ctx, command.WorkspaceID, input.TargetID, command.CreateStep)
	case pluginsdk.WorkspaceAdminUpdateWorkflowStep:
		return a.updateWorkflowStep(ctx, command.WorkspaceID, command.UpdateStep)
	case pluginsdk.WorkspaceAdminReorderWorkflowStep:
		return a.reorderWorkflowSteps(ctx, command.WorkspaceID, command.ReorderSteps)
	case pluginsdk.WorkspaceAdminRegisterRepository:
		return a.registerRepository(ctx, command.WorkspaceID, input.TargetID, command.RegisterRepo)
	case pluginsdk.WorkspaceAdminUpdateRepository:
		return a.updateRepository(ctx, command.WorkspaceID, command.UpdateRepo)
	default:
		return plugins.ExactWorkspaceAdminResult{}, errors.New("unsupported workspace administration operation")
	}
}

func (a pluginsWorkspaceAdminAdapter) updateWorkspaceDefaults(ctx context.Context, workspaceID, targetID string, input *pluginsdk.WorkspaceDefaultsUpdate) (plugins.ExactWorkspaceAdminResult, error) {
	expectedVersion, err := parseAdminVersion(input.ExpectedResourceVersion)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrTaskVersionConflict
	}
	workspace, err := a.tasks.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if workspace == nil || workspace.IsImproveKandev() {
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrWorkspaceNotFound
	}
	if !adminVersionMatches(workspace.UpdatedAt, input.ExpectedResourceVersion) {
		if workspaceDefaultsMatch(workspace, input) {
			return adminResult(targetID, workspace.UpdatedAt, true), nil
		}
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrTaskVersionConflict
	}
	if err := a.validateWorkspaceDefaults(ctx, input); err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	updated, err := a.tasks.UpdateWorkspace(ctx, workspaceID, &taskservice.UpdateWorkspaceRequest{
		ExpectedUpdatedAt: &expectedVersion,
		Name:              input.Name, Description: input.Description,
		DefaultExecutorID: input.DefaultExecutorID, DefaultEnvironmentID: input.DefaultEnvironmentID,
		DefaultAgentProfileID: input.DefaultAgentProfileID, DefaultConfigAgentProfileID: input.DefaultConfigAgentProfile,
	})
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	return adminResult(targetID, updated.UpdatedAt, false), nil
}

func (a pluginsWorkspaceAdminAdapter) validateWorkspaceDefaults(ctx context.Context, input *pluginsdk.WorkspaceDefaultsUpdate) error {
	if input.DefaultExecutorID != nil && *input.DefaultExecutorID != "" {
		if _, err := a.tasks.GetExecutor(ctx, *input.DefaultExecutorID); err != nil {
			return fmt.Errorf("default executor is unavailable: %w", err)
		}
	}
	if input.DefaultEnvironmentID != nil && *input.DefaultEnvironmentID != "" {
		if _, err := a.tasks.GetEnvironment(ctx, *input.DefaultEnvironmentID); err != nil {
			return fmt.Errorf("default environment is unavailable: %w", err)
		}
	}
	if input.DefaultAgentProfileID != nil && *input.DefaultAgentProfileID != "" {
		if err := a.validateAgentProfile(ctx, *input.DefaultAgentProfileID); err != nil {
			return err
		}
	}
	if input.DefaultConfigAgentProfile != nil && *input.DefaultConfigAgentProfile != "" {
		if err := a.validateAgentProfile(ctx, *input.DefaultConfigAgentProfile); err != nil {
			return err
		}
	}
	return nil
}

func (a pluginsWorkspaceAdminAdapter) validateAgentProfile(ctx context.Context, profileID string) error {
	if a.agents == nil {
		return errors.New("agent profile catalog is unavailable")
	}
	listed, err := a.agents.ListAgents(ctx)
	if err != nil {
		return err
	}
	for _, agent := range listed.Agents {
		for _, profile := range agent.Profiles {
			if profile.ID == profileID && profile.Enabled {
				return nil
			}
		}
	}
	return fmt.Errorf("agent profile is not available")
}

func (a pluginsWorkspaceAdminAdapter) requireMutableWorkspace(ctx context.Context, workspaceID string) error {
	workspace, err := a.tasks.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	if workspace == nil || workspace.IsImproveKandev() {
		return repoerrors.ErrWorkspaceNotFound
	}
	return nil
}

//nolint:cyclop // Keep resource-version admission and workflow creation checks together.
func (a pluginsWorkspaceAdminAdapter) createWorkflow(ctx context.Context, workspaceID, id string, input *pluginsdk.WorkspaceWorkflowCreate) (plugins.ExactWorkspaceAdminResult, error) {
	expectedVersion, err := parseAdminVersion(input.ExpectedWorkspaceResourceVersion)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	workspace, err := a.tasks.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if workspace == nil || workspace.IsImproveKandev() || !adminVersionMatches(workspace.UpdatedAt, input.ExpectedWorkspaceResourceVersion) {
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrTaskVersionConflict
	}
	if existing, getErr := a.tasks.GetWorkflow(ctx, id); getErr == nil && existing != nil {
		if existing.WorkspaceID == workspaceID && existing.Name == input.Name && existing.Description == input.Description && existing.Prompt == input.Prompt {
			return adminResult(id, existing.UpdatedAt, true), nil
		}
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrTaskOperationConflict
	} else if getErr != nil && !errors.Is(getErr, repoerrors.ErrWorkflowNotFound) && !errors.Is(getErr, sql.ErrNoRows) {
		return plugins.ExactWorkspaceAdminResult{}, getErr
	}
	created, err := a.tasks.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{
		ID: id, ExpectedWorkspaceUpdatedAt: &expectedVersion,
		WorkspaceID: workspaceID, Name: input.Name, Description: input.Description, Prompt: input.Prompt,
	})
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	return adminResult(created.ID, created.UpdatedAt, false), nil
}

func (a pluginsWorkspaceAdminAdapter) updateWorkflow(ctx context.Context, workspaceID string, input *pluginsdk.WorkspaceWorkflowUpdate) (plugins.ExactWorkspaceAdminResult, error) {
	expectedVersion, err := parseAdminVersion(input.ExpectedResourceVersion)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	workflow, err := a.tasks.GetWorkflow(ctx, input.WorkflowID)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if workflow == nil || workflow.WorkspaceID != workspaceID || workflow.Hidden || workflow.Source != "" && workflow.Source != taskmodels.WorkflowSourceManual {
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrWorkflowNotFound
	}
	if err := a.requireMutableWorkspace(ctx, workspaceID); err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if input.AgentProfileID != nil && *input.AgentProfileID != "" {
		if err := a.validateAgentProfile(ctx, *input.AgentProfileID); err != nil {
			return plugins.ExactWorkspaceAdminResult{}, err
		}
	}
	if !adminVersionMatches(workflow.UpdatedAt, input.ExpectedResourceVersion) {
		if workflowUpdateMatches(workflow, input) {
			return adminResult(workflow.ID, workflow.UpdatedAt, true), nil
		}
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrTaskVersionConflict
	}
	updated, err := a.tasks.UpdateWorkflow(ctx, workflow.ID, &taskservice.UpdateWorkflowRequest{
		ExpectedUpdatedAt: &expectedVersion,
		Name:              input.Name, Description: input.Description, Prompt: input.Prompt, AgentProfileID: input.AgentProfileID,
	})
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	return adminResult(updated.ID, updated.UpdatedAt, false), nil
}

//nolint:cyclop // The operation validates the complete ordered set before applying it.
func (a pluginsWorkspaceAdminAdapter) reorderWorkflows(ctx context.Context, workspaceID string, input *pluginsdk.WorkspaceWorkflowReorder) (plugins.ExactWorkspaceAdminResult, error) {
	expectedWorkspace, err := parseAdminVersion(input.ExpectedWorkspaceResourceVersion)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	workspace, err := a.tasks.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if workspace == nil || workspace.IsImproveKandev() {
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrWorkspaceNotFound
	}
	workflows, err := a.tasks.ListWorkflows(ctx, workspaceID, true)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if exactWorkflowOrderMatches(workflows, input.WorkflowIDs) {
		return adminResult(workspaceID, workspace.UpdatedAt, true), nil
	}
	if !adminVersionMatches(workspace.UpdatedAt, input.ExpectedWorkspaceResourceVersion) || len(workflows) != len(input.WorkflowIDs) {
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrTaskVersionConflict
	}
	if len(input.WorkflowResourceVersions) != len(input.WorkflowIDs) {
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrTaskVersionConflict
	}
	versionByID := make(map[string]string, len(input.WorkflowIDs))
	expectedByID := make(map[string]time.Time, len(input.WorkflowIDs))
	for index, id := range input.WorkflowIDs {
		versionByID[id] = input.WorkflowResourceVersions[index]
		version, parseErr := parseAdminVersion(input.WorkflowResourceVersions[index])
		if parseErr != nil {
			return plugins.ExactWorkspaceAdminResult{}, parseErr
		}
		expectedByID[id] = version
	}
	for _, workflow := range workflows {
		if !adminVersionMatches(workflow.UpdatedAt, versionByID[workflow.ID]) {
			return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrTaskVersionConflict
		}
		if workflow.Hidden || (workflow.Source != "" && workflow.Source != taskmodels.WorkflowSourceManual) {
			return plugins.ExactWorkspaceAdminResult{}, errors.New("workflow list contains a synchronized resource")
		}
	}
	if err := a.tasks.ReorderWorkflowsIfUnchanged(ctx, workspaceID, input.WorkflowIDs, expectedWorkspace, expectedByID); err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	latest, err := a.tasks.ListWorkflows(ctx, workspaceID, true)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	return adminResult(workspaceID, latestAdminWorkflowVersion(latest), false), nil
}

//nolint:cyclop // The operation validates step configuration and workflow admission together.
func (a pluginsWorkspaceAdminAdapter) createWorkflowStep(ctx context.Context, workspaceID, id string, input *pluginsdk.WorkspaceWorkflowStepCreate) (plugins.ExactWorkspaceAdminResult, error) {
	expectedWorkflow, err := parseAdminVersion(input.ExpectedWorkflowResourceVersion)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	workflow, err := a.tasks.GetWorkflow(ctx, input.WorkflowID)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if workflow == nil || workflow.WorkspaceID != workspaceID || workflow.Hidden || workflow.Source != "" && workflow.Source != taskmodels.WorkflowSourceManual {
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrWorkflowNotFound
	}
	if err := a.requireMutableWorkspace(ctx, workspaceID); err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if err := a.workflows.EnsureWorkflowMutable(ctx, workflow.ID); err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if !adminVersionMatches(workflow.UpdatedAt, input.ExpectedWorkflowResourceVersion) {
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrTaskVersionConflict
	}
	configuration := input.Configuration
	configuration.ID = id
	configuration.WorkflowID = input.WorkflowID
	if configuration.AgentProfileID != "" {
		if err := a.validateAgentProfile(ctx, configuration.AgentProfileID); err != nil {
			return plugins.ExactWorkspaceAdminResult{}, err
		}
	}
	step := workflowStepFromAdminConfig(configuration, nil)
	if existing, getErr := a.workflows.GetStep(ctx, id); getErr == nil && existing != nil {
		if adminWorkflowStepMatches(existing, configuration) {
			return adminResult(id, existing.UpdatedAt, true), nil
		}
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrTaskOperationConflict
	} else if getErr != nil && !errors.Is(getErr, sql.ErrNoRows) && !errors.Is(getErr, workflowmodels.ErrWorkflowStepNotFound) {
		return plugins.ExactWorkspaceAdminResult{}, getErr
	}
	if err := workflowcontroller.NewController(a.workflows).ValidateStepReferences(ctx, step); err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if _, err := a.workflows.CreateStepWithStartStepUpdatesIfWorkflowUnchanged(ctx, step, expectedWorkflow); err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	created, err := a.workflows.GetStep(ctx, id)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	return adminResult(id, created.UpdatedAt, false), nil
}

//nolint:cyclop // The operation checks both observed versions before replacing step configuration.
func (a pluginsWorkspaceAdminAdapter) updateWorkflowStep(ctx context.Context, workspaceID string, input *pluginsdk.WorkspaceWorkflowStepUpdate) (plugins.ExactWorkspaceAdminResult, error) {
	expectedWorkflow, err := parseAdminVersion(input.ExpectedWorkflowResourceVersion)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	expectedStep, err := parseAdminVersion(input.ExpectedStepResourceVersion)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	workflow, err := a.tasks.GetWorkflow(ctx, input.WorkflowID)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if workflow == nil || workflow.WorkspaceID != workspaceID || workflow.Hidden || workflow.Source != "" && workflow.Source != taskmodels.WorkflowSourceManual {
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrWorkflowNotFound
	}
	if err := a.requireMutableWorkspace(ctx, workspaceID); err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if err := a.workflows.EnsureWorkflowMutable(ctx, workflow.ID); err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	step, err := a.workflows.GetStep(ctx, input.StepID)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if step == nil || step.WorkflowID != input.WorkflowID {
		return plugins.ExactWorkspaceAdminResult{}, sql.ErrNoRows
	}
	if !adminVersionMatches(workflow.UpdatedAt, input.ExpectedWorkflowResourceVersion) || !adminVersionMatches(step.UpdatedAt, input.ExpectedStepResourceVersion) {
		if adminWorkflowStepMatches(step, input.Configuration) {
			return adminResult(step.ID, step.UpdatedAt, true), nil
		}
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrTaskVersionConflict
	}
	configuration := input.Configuration
	configuration.ID, configuration.WorkflowID = input.StepID, input.WorkflowID
	if configuration.AgentProfileID != "" {
		if err := a.validateAgentProfile(ctx, configuration.AgentProfileID); err != nil {
			return plugins.ExactWorkspaceAdminResult{}, err
		}
	}
	updated := workflowStepFromAdminConfig(configuration, step)
	if err := workflowcontroller.NewController(a.workflows).ValidateStepReferences(ctx, updated); err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if _, err := a.workflows.UpdateStepWithStartStepUpdatesIfUnchanged(ctx, updated, expectedWorkflow, expectedStep); err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	current, err := a.workflows.GetStep(ctx, input.StepID)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	return adminResult(current.ID, current.UpdatedAt, false), nil
}

//nolint:cyclop // The operation validates the complete ordered set before applying it.
func (a pluginsWorkspaceAdminAdapter) reorderWorkflowSteps(ctx context.Context, workspaceID string, input *pluginsdk.WorkspaceWorkflowStepReorder) (plugins.ExactWorkspaceAdminResult, error) {
	expectedWorkflow, err := parseAdminVersion(input.ExpectedWorkflowResourceVersion)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	workflow, err := a.tasks.GetWorkflow(ctx, input.WorkflowID)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if workflow == nil || workflow.WorkspaceID != workspaceID || workflow.Hidden || workflow.Source != "" && workflow.Source != taskmodels.WorkflowSourceManual {
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrWorkflowNotFound
	}
	if err := a.requireMutableWorkspace(ctx, workspaceID); err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if err := a.workflows.EnsureWorkflowMutable(ctx, workflow.ID); err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	steps, err := a.workflows.ListStepsByWorkflow(ctx, input.WorkflowID)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if exactWorkflowStepOrderMatches(steps, input.StepIDs) {
		return adminResult(input.WorkflowID, latestAdminStepVersion(steps), true), nil
	}
	if !adminVersionMatches(workflow.UpdatedAt, input.ExpectedWorkflowResourceVersion) || len(steps) != len(input.StepIDs) {
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrTaskVersionConflict
	}
	if len(input.StepResourceVersions) != len(input.StepIDs) {
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrTaskVersionConflict
	}
	versionByID := make(map[string]string, len(input.StepIDs))
	expectedByID := make(map[string]time.Time, len(input.StepIDs))
	for index, id := range input.StepIDs {
		versionByID[id] = input.StepResourceVersions[index]
		version, parseErr := parseAdminVersion(input.StepResourceVersions[index])
		if parseErr != nil {
			return plugins.ExactWorkspaceAdminResult{}, parseErr
		}
		expectedByID[id] = version
	}
	for _, step := range steps {
		if !adminVersionMatches(step.UpdatedAt, versionByID[step.ID]) {
			return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrTaskVersionConflict
		}
	}
	if err := workflowcontroller.NewController(a.workflows).ValidateStepOrder(ctx, input.WorkflowID, input.StepIDs); err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if err := a.workflows.ReorderStepsIfUnchanged(ctx, input.WorkflowID, input.StepIDs, expectedWorkflow, expectedByID); err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	latest, err := a.workflows.ListStepsByWorkflow(ctx, input.WorkflowID)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	return adminResult(input.WorkflowID, latestAdminStepVersion(latest), false), nil
}

func (a pluginsWorkspaceAdminAdapter) registerRepository(ctx context.Context, workspaceID, id string, input *pluginsdk.WorkspaceRepositoryRegister) (plugins.ExactWorkspaceAdminResult, error) {
	expectedWorkspace, err := parseAdminVersion(input.ExpectedWorkspaceResourceVersion)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	workspace, err := a.tasks.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if workspace == nil || workspace.IsImproveKandev() || !adminVersionMatches(workspace.UpdatedAt, input.ExpectedWorkspaceResourceVersion) {
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrTaskVersionConflict
	}
	if existing, getErr := a.tasks.GetRepository(ctx, id); getErr == nil && existing != nil {
		if repositoryRegistrationMatches(existing, workspaceID, input) {
			return adminResult(id, existing.UpdatedAt, true), nil
		}
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrTaskOperationConflict
	} else if getErr != nil && !errors.Is(getErr, repoerrors.ErrRepositoryNotFound) && !errors.Is(getErr, sql.ErrNoRows) {
		return plugins.ExactWorkspaceAdminResult{}, getErr
	}
	created, err := a.tasks.CreateRepository(ctx, &taskservice.CreateRepositoryRequest{
		ID: id, ExpectedWorkspaceUpdatedAt: &expectedWorkspace, WorkspaceID: workspaceID, Name: input.Name, SourceType: input.SourceType,
		LocalPath: input.LocalPath, Provider: input.Provider, ProviderRepoID: input.ProviderRepoID,
		ProviderHost: input.ProviderHost, ProviderScope: input.ProviderScope, ProviderOwner: input.ProviderOwner,
		ProviderName: input.ProviderName, RemoteURL: input.RemoteURL, DefaultBranch: input.DefaultBranch,
		WorktreeBranchPrefix: input.WorktreeBranchPrefix, WorktreeBranchTemplate: input.WorktreeBranchTemplate,
		PullBeforeWorktree: input.PullBeforeWorktree,
	})
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	return adminResult(created.ID, created.UpdatedAt, false), nil
}

func (a pluginsWorkspaceAdminAdapter) updateRepository(ctx context.Context, workspaceID string, input *pluginsdk.WorkspaceRepositoryUpdate) (plugins.ExactWorkspaceAdminResult, error) {
	expectedVersion, err := parseAdminVersion(input.ExpectedResourceVersion)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	repository, err := a.tasks.GetRepository(ctx, input.RepositoryID)
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if repository == nil || repository.WorkspaceID != workspaceID {
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrRepositoryNotFound
	}
	if err := a.requireMutableWorkspace(ctx, workspaceID); err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	if !adminVersionMatches(repository.UpdatedAt, input.ExpectedResourceVersion) {
		if repositoryUpdateMatches(repository, input) {
			return adminResult(repository.ID, repository.UpdatedAt, true), nil
		}
		return plugins.ExactWorkspaceAdminResult{}, repoerrors.ErrTaskVersionConflict
	}
	updated, err := a.tasks.UpdateRepository(ctx, repository.ID, &taskservice.UpdateRepositoryRequest{
		ExpectedUpdatedAt: &expectedVersion,
		Name:              input.Name, DefaultBranch: input.DefaultBranch, WorktreeBranchPrefix: input.WorktreeBranchPrefix,
		WorktreeBranchTemplate: input.WorktreeBranchTemplate, PullBeforeWorktree: input.PullBeforeWorktree,
	})
	if err != nil {
		return plugins.ExactWorkspaceAdminResult{}, err
	}
	return adminResult(updated.ID, updated.UpdatedAt, false), nil
}

func workflowStepFromAdminConfig(value pluginsdk.WorkflowStepConfiguration, current *workflowmodels.WorkflowStep) *workflowmodels.WorkflowStep {
	step := &workflowmodels.WorkflowStep{
		ID: value.ID, WorkflowID: value.WorkflowID, Name: value.Name, Position: int(value.Position), StageType: workflowmodels.StageType(value.StageType),
		Color: value.Color, Prompt: value.Prompt, AllowManualMove: value.AllowManualMove, IsStartStep: value.IsStartStep,
		ShowInCommandPanel: value.ShowInCommandPanel, AutoArchiveAfterHours: int(value.AutoArchiveAfterHours),
		AgentProfileID: value.AgentProfileID, ProfileSessionStartPolicy: taskmodels.WorkflowProfileSessionStartPolicy(value.ProfileSessionStartPolicy),
		ProfileSessionEndPolicy: taskmodels.WorkflowProfileSessionEndPolicy(value.ProfileSessionEndPolicy), WIPLimit: int(value.WIPLimit),
		PullFromStepID: value.PullFromStepID, AutoAdvanceRequiresSignal: value.AutoAdvanceRequiresSignal,
		CancelTriggersTurnComplete: value.CancelTriggersTurnComplete, CompleteTaskOnEnter: value.CompleteTaskOnEnter,
	}
	if current != nil {
		step.SessionTarget = current.SessionTarget
		step.Events.OnExit = append([]workflowmodels.OnExitAction(nil), current.Events.OnExit...)
		step.Events.OnComment = append([]workflowmodels.GenericAction(nil), current.Events.OnComment...)
		step.Events.OnBlockerResolved = append([]workflowmodels.GenericAction(nil), current.Events.OnBlockerResolved...)
		step.Events.OnChildrenCompleted = append([]workflowmodels.GenericAction(nil), current.Events.OnChildrenCompleted...)
		step.Events.OnApprovalResolved = append([]workflowmodels.GenericAction(nil), current.Events.OnApprovalResolved...)
		step.Events.OnHeartbeat = append([]workflowmodels.GenericAction(nil), current.Events.OnHeartbeat...)
		step.Events.OnBudgetAlert = append([]workflowmodels.GenericAction(nil), current.Events.OnBudgetAlert...)
		step.Events.OnAgentError = append([]workflowmodels.GenericAction(nil), current.Events.OnAgentError...)
	}
	if current != nil {
		step.Events.OnEnter = mergeAdminOnEnterActions(current.Events.OnEnter, value.OnEnterActionTypes)
		step.Events.OnTurnStart = mergeAdminOnTurnStartActions(current.Events.OnTurnStart, value.OnTurnStartActions)
		step.Events.OnTurnComplete = mergeAdminOnTurnCompleteActions(current.Events.OnTurnComplete, value.OnTurnCompleteActions)
	} else {
		step.Events.OnEnter = makeAdminOnEnterActions(value.OnEnterActionTypes)
		step.Events.OnTurnStart = makeAdminOnTurnStartActions(value.OnTurnStartActions)
		step.Events.OnTurnComplete = makeAdminOnTurnCompleteActions(value.OnTurnCompleteActions)
	}
	return step
}

func mergeAdminOnEnterActions(current []workflowmodels.OnEnterAction, requested []string) []workflowmodels.OnEnterAction {
	remaining := makeAdminOnEnterActions(requested)
	merged := make([]workflowmodels.OnEnterAction, 0, len(current)+len(remaining))
	for _, action := range current {
		if !supportedAdminOnEnterAction(action.Type) {
			merged = append(merged, workflowmodels.OnEnterAction{Type: action.Type, Config: maps.Clone(action.Config)})
			continue
		}
		if len(remaining) == 0 {
			continue
		}
		replacement := remaining[0]
		remaining = remaining[1:]
		if replacement.Type == action.Type {
			replacement.Config = maps.Clone(action.Config)
		}
		merged = append(merged, replacement)
	}
	return append(merged, remaining...)
}

func supportedAdminOnEnterAction(action workflowmodels.OnEnterActionType) bool {
	switch action {
	case workflowmodels.OnEnterEnablePlanMode, workflowmodels.OnEnterAutoStartAgent,
		workflowmodels.OnEnterResetAgentContext, workflowmodels.OnEnterClearDecisions:
		return true
	default:
		return false
	}
}

func makeAdminOnEnterActions(requested []string) []workflowmodels.OnEnterAction {
	actions := make([]workflowmodels.OnEnterAction, 0, len(requested))
	for _, action := range requested {
		actions = append(actions, workflowmodels.OnEnterAction{Type: workflowmodels.OnEnterActionType(action)})
	}
	return actions
}

//nolint:dupl // This typed adapter preserves start-action ordering for the exact workflow contract.
func mergeAdminOnTurnStartActions(current []workflowmodels.OnTurnStartAction, requested []pluginsdk.WorkflowTransitionAction) []workflowmodels.OnTurnStartAction {
	remaining := append([]pluginsdk.WorkflowTransitionAction(nil), requested...)
	merged := make([]workflowmodels.OnTurnStartAction, 0, len(current)+len(remaining))
	for _, action := range current {
		if !supportedAdminTurnStartAction(action.Type) {
			action.Config = maps.Clone(action.Config)
			merged = append(merged, action)
			continue
		}
		match := adminTransitionMatch(remaining, string(action.Type))
		if match < 0 {
			continue
		}
		candidate := remaining[match]
		remaining = append(remaining[:match], remaining[match+1:]...)
		config := maps.Clone(action.Config)
		config = setAdminTransitionTarget(config, candidate)
		merged = append(merged, workflowmodels.OnTurnStartAction{Type: workflowmodels.OnTurnStartActionType(candidate.Type), Config: config})
	}
	for _, action := range remaining {
		merged = append(merged, workflowmodels.OnTurnStartAction{Type: workflowmodels.OnTurnStartActionType(action.Type), Config: adminTransitionConfig(action)})
	}
	return merged
}

//nolint:dupl // This typed adapter preserves completion-action ordering for the exact workflow contract.
func mergeAdminOnTurnCompleteActions(current []workflowmodels.OnTurnCompleteAction, requested []pluginsdk.WorkflowTransitionAction) []workflowmodels.OnTurnCompleteAction {
	remaining := append([]pluginsdk.WorkflowTransitionAction(nil), requested...)
	merged := make([]workflowmodels.OnTurnCompleteAction, 0, len(current)+len(remaining))
	for _, action := range current {
		if !supportedAdminTurnCompleteAction(action.Type) {
			action.Config = maps.Clone(action.Config)
			merged = append(merged, action)
			continue
		}
		match := adminTransitionMatch(remaining, string(action.Type))
		if match < 0 {
			continue
		}
		candidate := remaining[match]
		remaining = append(remaining[:match], remaining[match+1:]...)
		config := maps.Clone(action.Config)
		config = setAdminTransitionTarget(config, candidate)
		merged = append(merged, workflowmodels.OnTurnCompleteAction{Type: workflowmodels.OnTurnCompleteActionType(candidate.Type), Config: config})
	}
	for _, action := range remaining {
		merged = append(merged, workflowmodels.OnTurnCompleteAction{Type: workflowmodels.OnTurnCompleteActionType(action.Type), Config: adminTransitionConfig(action)})
	}
	return merged
}

func supportedAdminTurnStartAction(action workflowmodels.OnTurnStartActionType) bool {
	return action == workflowmodels.OnTurnStartMoveToNext || action == workflowmodels.OnTurnStartMoveToPrevious || action == workflowmodels.OnTurnStartMoveToStep
}

func supportedAdminTurnCompleteAction(action workflowmodels.OnTurnCompleteActionType) bool {
	return action == workflowmodels.OnTurnCompleteMoveToNext || action == workflowmodels.OnTurnCompleteMoveToPrevious ||
		action == workflowmodels.OnTurnCompleteMoveToStep || action == workflowmodels.OnTurnCompleteDisablePlanMode
}

func adminTransitionMatch(requested []pluginsdk.WorkflowTransitionAction, actionType string) int {
	for index, action := range requested {
		if action.Type == actionType {
			return index
		}
	}
	return -1
}

func adminTransitionConfig(action pluginsdk.WorkflowTransitionAction) map[string]interface{} {
	var config map[string]interface{}
	return setAdminTransitionTarget(config, action)
}

func setAdminTransitionTarget(config map[string]interface{}, action pluginsdk.WorkflowTransitionAction) map[string]interface{} {
	if action.TargetStepID == "" {
		delete(config, "step_id")
		return config
	}
	if config == nil {
		config = make(map[string]interface{})
	}
	config["step_id"] = action.TargetStepID
	return config
}

func makeAdminOnTurnStartActions(requested []pluginsdk.WorkflowTransitionAction) []workflowmodels.OnTurnStartAction {
	actions := make([]workflowmodels.OnTurnStartAction, 0, len(requested))
	for _, action := range requested {
		config := make(map[string]interface{})
		setAdminTransitionTarget(config, action)
		actions = append(actions, workflowmodels.OnTurnStartAction{Type: workflowmodels.OnTurnStartActionType(action.Type), Config: config})
	}
	return actions
}

func makeAdminOnTurnCompleteActions(requested []pluginsdk.WorkflowTransitionAction) []workflowmodels.OnTurnCompleteAction {
	actions := make([]workflowmodels.OnTurnCompleteAction, 0, len(requested))
	for _, action := range requested {
		config := make(map[string]interface{})
		setAdminTransitionTarget(config, action)
		actions = append(actions, workflowmodels.OnTurnCompleteAction{Type: workflowmodels.OnTurnCompleteActionType(action.Type), Config: config})
	}
	return actions
}

func adminWorkflowStepMatches(current *workflowmodels.WorkflowStep, requested pluginsdk.WorkflowStepConfiguration) bool {
	if current == nil {
		return false
	}
	actual := pluginsdk.WorkflowStepConfiguration{
		ID: current.ID, WorkflowID: current.WorkflowID, Name: current.Name, Position: int32(current.Position),
		StageType: string(current.StageType), Color: current.Color, Prompt: current.Prompt,
		AllowManualMove: current.AllowManualMove, IsStartStep: current.IsStartStep, ShowInCommandPanel: current.ShowInCommandPanel,
		AutoArchiveAfterHours: int32(current.AutoArchiveAfterHours), AgentProfileID: current.AgentProfileID,
		ProfileSessionStartPolicy: string(current.ProfileSessionStartPolicy), ProfileSessionEndPolicy: string(current.ProfileSessionEndPolicy),
		WIPLimit: int32(current.WIPLimit), PullFromStepID: current.PullFromStepID, AutoAdvanceRequiresSignal: current.AutoAdvanceRequiresSignal,
		CancelTriggersTurnComplete: current.CancelTriggersTurnComplete, CompleteTaskOnEnter: current.CompleteTaskOnEnter,
	}
	for _, action := range current.Events.OnEnter {
		if supportedAdminOnEnterAction(action.Type) {
			actual.OnEnterActionTypes = append(actual.OnEnterActionTypes, string(action.Type))
		}
	}
	for _, action := range current.Events.OnTurnStart {
		if supportedAdminTurnStartAction(action.Type) {
			target, _ := action.Config["step_id"].(string)
			actual.OnTurnStartActions = append(actual.OnTurnStartActions, pluginsdk.WorkflowTransitionAction{Type: string(action.Type), TargetStepID: target})
		}
	}
	for _, action := range current.Events.OnTurnComplete {
		if supportedAdminTurnCompleteAction(action.Type) {
			target, _ := action.Config["step_id"].(string)
			actual.OnTurnCompleteActions = append(actual.OnTurnCompleteActions, pluginsdk.WorkflowTransitionAction{Type: string(action.Type), TargetStepID: target})
		}
	}
	return reflect.DeepEqual(actual, requested)
}

func exactWorkflowOrderMatches(current []*taskmodels.Workflow, requested []string) bool {
	if len(current) != len(requested) {
		return false
	}
	for index := range current {
		if current[index].ID != requested[index] {
			return false
		}
	}
	return true
}

func exactWorkflowStepOrderMatches(current []*workflowmodels.WorkflowStep, requested []string) bool {
	if len(current) != len(requested) {
		return false
	}
	for index := range current {
		if current[index].ID != requested[index] {
			return false
		}
	}
	return true
}

func workflowDefaultsValueMatches(current *string, desired *string) bool {
	if desired == nil {
		return true
	}
	if current == nil {
		return *desired == ""
	}
	return strings.TrimSpace(*current) == strings.TrimSpace(*desired)
}

func workspaceDefaultsMatch(current *taskmodels.Workspace, requested *pluginsdk.WorkspaceDefaultsUpdate) bool {
	return (requested.Name == nil || current.Name == *requested.Name) &&
		(requested.Description == nil || current.Description == *requested.Description) &&
		workflowDefaultsValueMatches(current.DefaultExecutorID, requested.DefaultExecutorID) &&
		workflowDefaultsValueMatches(current.DefaultEnvironmentID, requested.DefaultEnvironmentID) &&
		workflowDefaultsValueMatches(current.DefaultAgentProfileID, requested.DefaultAgentProfileID) &&
		workflowDefaultsValueMatches(current.DefaultConfigAgentProfileID, requested.DefaultConfigAgentProfile)
}

func workflowUpdateMatches(current *taskmodels.Workflow, requested *pluginsdk.WorkspaceWorkflowUpdate) bool {
	return (requested.Name == nil || current.Name == *requested.Name) &&
		(requested.Description == nil || current.Description == *requested.Description) &&
		(requested.Prompt == nil || current.Prompt == *requested.Prompt) &&
		(requested.AgentProfileID == nil || current.AgentProfileID == strings.TrimSpace(*requested.AgentProfileID))
}

//nolint:cyclop // Every registered repository field participates in the identity comparison.
func repositoryRegistrationMatches(current *taskmodels.Repository, workspaceID string, requested *pluginsdk.WorkspaceRepositoryRegister) bool {
	pullBeforeWorktree := true
	if requested.PullBeforeWorktree != nil {
		pullBeforeWorktree = *requested.PullBeforeWorktree
	}
	return current.WorkspaceID == workspaceID && current.Name == requested.Name && current.SourceType == requested.SourceType &&
		current.LocalPath == requested.LocalPath && current.Provider == requested.Provider && current.ProviderRepoID == requested.ProviderRepoID &&
		current.ProviderHost == requested.ProviderHost && current.ProviderScope == requested.ProviderScope && current.ProviderOwner == requested.ProviderOwner &&
		current.ProviderName == requested.ProviderName && current.RemoteURL == requested.RemoteURL && current.DefaultBranch == requested.DefaultBranch &&
		current.WorktreeBranchPrefix == requested.WorktreeBranchPrefix && current.WorktreeBranchTemplate == requested.WorktreeBranchTemplate &&
		current.PullBeforeWorktree == pullBeforeWorktree
}

func repositoryUpdateMatches(current *taskmodels.Repository, requested *pluginsdk.WorkspaceRepositoryUpdate) bool {
	return (requested.Name == nil || current.Name == *requested.Name) &&
		(requested.DefaultBranch == nil || current.DefaultBranch == *requested.DefaultBranch) &&
		(requested.WorktreeBranchPrefix == nil || current.WorktreeBranchPrefix == *requested.WorktreeBranchPrefix) &&
		(requested.WorktreeBranchTemplate == nil || current.WorktreeBranchTemplate == *requested.WorktreeBranchTemplate) &&
		(requested.PullBeforeWorktree == nil || current.PullBeforeWorktree == *requested.PullBeforeWorktree)
}

func adminVersionMatches(current time.Time, expected string) bool {
	parsed, err := parseAdminVersion(expected)
	return err == nil && current.UTC().Equal(parsed.UTC())
}

func parseAdminVersion(expected string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, expected)
	if err != nil {
		return time.Time{}, repoerrors.ErrTaskVersionConflict
	}
	return parsed.UTC(), nil
}

func adminResult(targetID string, updatedAt time.Time, alreadyApplied bool) plugins.ExactWorkspaceAdminResult {
	return plugins.ExactWorkspaceAdminResult{TargetID: targetID, ResourceVersion: updatedAt.UTC().Format(time.RFC3339Nano), AlreadyApplied: alreadyApplied}
}

func latestAdminWorkflowVersion(workflows []*taskmodels.Workflow) time.Time {
	var latest time.Time
	for _, workflow := range workflows {
		if workflow.UpdatedAt.After(latest) {
			latest = workflow.UpdatedAt
		}
	}
	return latest
}

func latestAdminStepVersion(steps []*workflowmodels.WorkflowStep) time.Time {
	var latest time.Time
	for _, step := range steps {
		if step.UpdatedAt.After(latest) {
			latest = step.UpdatedAt
		}
	}
	return latest
}
