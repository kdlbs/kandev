package backendapp

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestPluginsWorkspaceAdminPreservesNativeStepActions(t *testing.T) {
	ctx := context.Background()
	services, _, _ := provideTestServices(t, "workspace-admin-native-actions")
	workspace, err := services.Task.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{Name: "Workspace Admin Test"})
	require.NoError(t, err)
	workflow, err := services.Task.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{
		ID: "workspace-admin-workflow", WorkspaceID: workspace.ID, Name: "Workflow",
	})
	require.NoError(t, err)
	step := &workflowmodels.WorkflowStep{
		ID: "workspace-admin-step", WorkflowID: workflow.ID, Name: "Review", Position: 0,
		StageType: workflowmodels.StageTypeReview,
		Events: workflowmodels.StepEvents{
			OnEnter: []workflowmodels.OnEnterAction{{
				Type: workflowmodels.OnEnterSetSessionMode, Config: map[string]interface{}{"mode": "acceptEdits"},
			}},
			OnTurnComplete: []workflowmodels.OnTurnCompleteAction{{
				Type: workflowmodels.OnTurnCompleteMoveToNext, Config: map[string]interface{}{"native_marker": "keep"},
			}},
		},
	}
	_, err = services.Workflow.CreateStepWithStartStepUpdates(ctx, step)
	require.NoError(t, err)
	stored, err := services.Workflow.GetStep(ctx, step.ID)
	require.NoError(t, err)

	name := "Review renamed"
	_, err = (pluginsWorkspaceAdminAdapter{tasks: services.Task, workflows: services.Workflow}).updateWorkflowStep(ctx, workspace.ID, &pluginsdk.WorkspaceWorkflowStepUpdate{
		WorkflowID: workflow.ID, StepID: stored.ID,
		ExpectedWorkflowResourceVersion: workflow.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ExpectedStepResourceVersion:     stored.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Configuration: pluginsdk.WorkflowStepConfiguration{
			ID: stored.ID, WorkflowID: stored.WorkflowID, Name: name, Position: int32(stored.Position),
			StageType: string(stored.StageType), Color: stored.Color, Prompt: stored.Prompt,
			AllowManualMove: stored.AllowManualMove, IsStartStep: stored.IsStartStep,
			ShowInCommandPanel: stored.ShowInCommandPanel, AutoArchiveAfterHours: int32(stored.AutoArchiveAfterHours),
			AgentProfileID: stored.AgentProfileID, ProfileSessionStartPolicy: string(stored.ProfileSessionStartPolicy),
			ProfileSessionEndPolicy: string(stored.ProfileSessionEndPolicy), WIPLimit: int32(stored.WIPLimit),
			PullFromStepID: stored.PullFromStepID, AutoAdvanceRequiresSignal: stored.AutoAdvanceRequiresSignal,
			CancelTriggersTurnComplete: stored.CancelTriggersTurnComplete, CompleteTaskOnEnter: stored.CompleteTaskOnEnter,
			OnTurnCompleteActions: []pluginsdk.WorkflowTransitionAction{{Type: string(workflowmodels.OnTurnCompleteMoveToNext)}},
		},
	})
	require.NoError(t, err)
	updated, err := services.Workflow.GetStep(ctx, step.ID)
	require.NoError(t, err)
	require.Equal(t, name, updated.Name)
	require.Equal(t, stored.Events.OnEnter, updated.Events.OnEnter,
		"an update preserves native on-enter behavior that the plugin DTO cannot represent")
	require.Equal(t, stored.Events.OnTurnComplete, updated.Events.OnTurnComplete,
		"an update preserves action config the plugin DTO cannot represent")
}

func TestPluginsWorkspaceAdminUsesValidatedVersionedDomainOperations(t *testing.T) {
	ctx := context.Background()
	services, _, _ := provideTestServices(t, "workspace-admin-domain-operations")
	workspace, err := services.Task.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{Name: "Coordinator Workspace"})
	require.NoError(t, err)
	adapter := pluginsWorkspaceAdminAdapter{tasks: services.Task, workflows: services.Workflow}

	initialWorkspaceVersion := workspace.UpdatedAt.UTC().Format(time.RFC3339Nano)
	workspaceName := "Coordinator Workspace Updated"
	updatedWorkspace, err := adapter.updateWorkspaceDefaults(ctx, workspace.ID, workspace.ID, &pluginsdk.WorkspaceDefaultsUpdate{
		ExpectedResourceVersion: initialWorkspaceVersion, Name: &workspaceName,
	})
	require.NoError(t, err)
	require.Equal(t, workspace.ID, updatedWorkspace.TargetID)
	updatedWorkspaceModel, err := services.Task.GetWorkspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.Equal(t, workspaceName, updatedWorkspaceModel.Name)

	staleWorkspaceName := "Stale workspace name"
	_, err = adapter.updateWorkspaceDefaults(ctx, workspace.ID, workspace.ID, &pluginsdk.WorkspaceDefaultsUpdate{
		ExpectedResourceVersion: initialWorkspaceVersion, Name: &staleWorkspaceName,
	})
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict)
	_, err = services.Task.UpdateWorkspace(ctx, workspace.ID, &taskservice.UpdateWorkspaceRequest{
		ExpectedUpdatedAt: &workspace.UpdatedAt, Name: &staleWorkspaceName,
	})
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict,
		"the shared task service must preserve the version fence between adapter validation and persistence")
	_, err = services.Task.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{
		ID: "stale-workflow-create", WorkspaceID: workspace.ID, Name: "Stale workflow",
		ExpectedWorkspaceUpdatedAt: &workspace.UpdatedAt,
	})
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict,
		"workflow creation must bind its insert to the workspace version observed by the plugin")

	workflowA, err := adapter.createWorkflow(ctx, workspace.ID, "workspace-admin-workflow-a", &pluginsdk.WorkspaceWorkflowCreate{
		ExpectedWorkspaceResourceVersion: updatedWorkspaceModel.UpdatedAt.UTC().Format(time.RFC3339Nano), Name: "Workflow A",
	})
	require.NoError(t, err)
	workflowB, err := adapter.createWorkflow(ctx, workspace.ID, "workspace-admin-workflow-b", &pluginsdk.WorkspaceWorkflowCreate{
		ExpectedWorkspaceResourceVersion: updatedWorkspaceModel.UpdatedAt.UTC().Format(time.RFC3339Nano), Name: "Workflow B",
	})
	require.NoError(t, err)
	workflowAModel, err := services.Task.GetWorkflow(ctx, workflowA.TargetID)
	require.NoError(t, err)
	workflowBModel, err := services.Task.GetWorkflow(ctx, workflowB.TargetID)
	require.NoError(t, err)

	workflowName := "Workflow A Updated"
	staleWorkflowVersion := workflowAModel.UpdatedAt.Add(-time.Second)
	_, err = services.Task.UpdateWorkflow(ctx, workflowAModel.ID, &taskservice.UpdateWorkflowRequest{
		ExpectedUpdatedAt: &staleWorkflowVersion, Name: &workflowName,
	})
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict,
		"workflow updates must carry the plugin's observed resource version to storage")
	updatedWorkflow, err := adapter.updateWorkflow(ctx, workspace.ID, &pluginsdk.WorkspaceWorkflowUpdate{
		WorkflowID: workflowAModel.ID, ExpectedResourceVersion: workflowAModel.UpdatedAt.UTC().Format(time.RFC3339Nano), Name: &workflowName,
	})
	require.NoError(t, err)
	staleWorkflowName := "Stale workflow update"
	_, err = adapter.updateWorkflow(ctx, workspace.ID, &pluginsdk.WorkspaceWorkflowUpdate{
		WorkflowID: workflowAModel.ID, ExpectedResourceVersion: workflowAModel.UpdatedAt.UTC().Format(time.RFC3339Nano), Name: &staleWorkflowName,
	})
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict)

	workspaceForOrder, err := services.Task.GetWorkspace(ctx, workspace.ID)
	require.NoError(t, err)
	updatedWorkflowVersion, err := parseAdminVersion(updatedWorkflow.ResourceVersion)
	require.NoError(t, err)
	err = services.Task.ReorderWorkflowsIfUnchanged(ctx, workspace.ID,
		[]string{workflowBModel.ID, workflowAModel.ID}, workspaceForOrder.UpdatedAt,
		map[string]time.Time{workflowBModel.ID: workflowBModel.UpdatedAt, workflowAModel.ID: updatedWorkflowVersion.Add(-time.Second)},
	)
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict,
		"workflow ordering must bind every row update to the observed versions")
	ordered, err := adapter.reorderWorkflows(ctx, workspace.ID, &pluginsdk.WorkspaceWorkflowReorder{
		ExpectedWorkspaceResourceVersion: workspaceForOrder.UpdatedAt.UTC().Format(time.RFC3339Nano),
		WorkflowIDs:                      []string{workflowBModel.ID, workflowAModel.ID},
		WorkflowResourceVersions: []string{
			workflowBModel.UpdatedAt.UTC().Format(time.RFC3339Nano), updatedWorkflow.ResourceVersion,
		},
	})
	require.NoError(t, err)
	require.False(t, ordered.AlreadyApplied)
	workflows, err := services.Task.ListWorkflows(ctx, workspace.ID, true)
	require.NoError(t, err)
	require.Equal(t, []string{workflowBModel.ID, workflowAModel.ID}, []string{workflows[0].ID, workflows[1].ID})

	workflowAModel, err = services.Task.GetWorkflow(ctx, workflowAModel.ID)
	require.NoError(t, err)
	staleStepWorkflowVersion := workflowAModel.UpdatedAt.Add(-time.Second)
	_, err = services.Workflow.CreateStepWithStartStepUpdatesIfWorkflowUnchanged(ctx, &workflowmodels.WorkflowStep{
		ID: "workspace-admin-stale-step", WorkflowID: workflowAModel.ID, Name: "Stale step", Position: 0,
	}, staleStepWorkflowVersion)
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict,
		"step creation must bind its insert to the observed workflow version")
	first, err := adapter.createWorkflowStep(ctx, workspace.ID, "workspace-admin-step-a", &pluginsdk.WorkspaceWorkflowStepCreate{
		WorkflowID: workflowAModel.ID, ExpectedWorkflowResourceVersion: workflowAModel.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Configuration: pluginsdk.WorkflowStepConfiguration{WorkflowID: workflowAModel.ID, Name: "Implement", Position: 0, StageType: "work", IsStartStep: true},
	})
	require.NoError(t, err)
	second, err := adapter.createWorkflowStep(ctx, workspace.ID, "workspace-admin-step-b", &pluginsdk.WorkspaceWorkflowStepCreate{
		WorkflowID: workflowAModel.ID, ExpectedWorkflowResourceVersion: workflowAModel.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Configuration: pluginsdk.WorkflowStepConfiguration{WorkflowID: workflowAModel.ID, Name: "Review", Position: 1, StageType: "review"},
	})
	require.NoError(t, err)
	foreignWorkflow, err := services.Task.GetWorkflow(ctx, workflowBModel.ID)
	require.NoError(t, err)
	foreignTarget := &workflowmodels.WorkflowStep{
		ID: "workspace-admin-foreign-step", WorkflowID: foreignWorkflow.ID, Name: "Foreign target", Position: 0,
	}
	_, err = services.Workflow.CreateStepWithStartStepUpdates(ctx, foreignTarget)
	require.NoError(t, err)
	_, err = adapter.createWorkflowStep(ctx, workspace.ID, "workspace-admin-invalid-step", &pluginsdk.WorkspaceWorkflowStepCreate{
		WorkflowID: workflowAModel.ID, ExpectedWorkflowResourceVersion: workflowAModel.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Configuration: pluginsdk.WorkflowStepConfiguration{
			WorkflowID: workflowAModel.ID, Name: "Invalid target", Position: 2,
			OnTurnStartActions: []pluginsdk.WorkflowTransitionAction{{Type: "move_to_step", TargetStepID: foreignTarget.ID}},
		},
	})
	require.ErrorContains(t, err, "same workflow",
		"typed Host step changes must preserve native same-workflow target validation")
	firstStep, err := services.Workflow.GetStep(ctx, first.TargetID)
	require.NoError(t, err)
	secondStep, err := services.Workflow.GetStep(ctx, second.TargetID)
	require.NoError(t, err)
	stepName := "Implementation"
	stepConfig := adminStepConfig(firstStep)
	stepConfig.Name = stepName
	updatedStep, err := adapter.updateWorkflowStep(ctx, workspace.ID, &pluginsdk.WorkspaceWorkflowStepUpdate{
		WorkflowID: workflowAModel.ID, StepID: firstStep.ID,
		ExpectedWorkflowResourceVersion: workflowAModel.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ExpectedStepResourceVersion:     firstStep.UpdatedAt.UTC().Format(time.RFC3339Nano), Configuration: stepConfig,
	})
	require.NoError(t, err)
	currentFirstStep, err := services.Workflow.GetStep(ctx, firstStep.ID)
	require.NoError(t, err)
	staleStepUpdate := workflowStepFromAdminConfig(adminStepConfig(currentFirstStep), currentFirstStep)
	_, err = services.Workflow.UpdateStepWithStartStepUpdatesIfUnchanged(ctx, staleStepUpdate,
		workflowAModel.UpdatedAt, firstStep.UpdatedAt)
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict,
		"step updates must bind their write to the observed step version")
	updatedStepVersion, err := parseAdminVersion(updatedStep.ResourceVersion)
	require.NoError(t, err)
	err = services.Workflow.ReorderStepsIfUnchanged(ctx, workflowAModel.ID,
		[]string{secondStep.ID, firstStep.ID}, workflowAModel.UpdatedAt,
		map[string]time.Time{secondStep.ID: secondStep.UpdatedAt.Add(-time.Second), firstStep.ID: updatedStepVersion},
	)
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict,
		"step ordering must reject any stale step version")
	_, err = adapter.reorderWorkflowSteps(ctx, workspace.ID, &pluginsdk.WorkspaceWorkflowStepReorder{
		WorkflowID: workflowAModel.ID, ExpectedWorkflowResourceVersion: workflowAModel.UpdatedAt.UTC().Format(time.RFC3339Nano),
		StepIDs:              []string{secondStep.ID, firstStep.ID},
		StepResourceVersions: []string{secondStep.UpdatedAt.UTC().Format(time.RFC3339Nano), updatedStep.ResourceVersion},
	})
	require.NoError(t, err)
	steps, err := services.Workflow.ListStepsByWorkflow(ctx, workflowAModel.ID)
	require.NoError(t, err)
	require.Equal(t, []string{secondStep.ID, firstStep.ID}, []string{steps[0].ID, steps[1].ID})

	badPrefix := "not a valid prefix"
	_, err = adapter.registerRepository(ctx, workspace.ID, "workspace-admin-repository", &pluginsdk.WorkspaceRepositoryRegister{
		ExpectedWorkspaceResourceVersion: workspaceForOrder.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Name:                             "Repository", SourceType: "remote", WorktreeBranchPrefix: badPrefix,
	})
	require.Error(t, err, "repository configuration uses native worktree validation")
	_, err = services.Task.CreateRepository(ctx, &taskservice.CreateRepositoryRequest{
		ID: "stale-repository-create", WorkspaceID: workspace.ID, Name: "Stale repository", SourceType: "remote",
		ExpectedWorkspaceUpdatedAt: &workspace.UpdatedAt,
	})
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict,
		"repository registration must bind its insert to the workspace version observed by the plugin")
	pullBeforeWorktree := false
	registeredRepository, err := adapter.registerRepository(ctx, workspace.ID, "workspace-admin-repository", &pluginsdk.WorkspaceRepositoryRegister{
		ExpectedWorkspaceResourceVersion: workspaceForOrder.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Name:                             "Repository", SourceType: "remote", DefaultBranch: "main", WorktreeBranchPrefix: "coordinator/",
		PullBeforeWorktree: &pullBeforeWorktree,
	})
	require.NoError(t, err)
	repo, err := services.Task.GetRepository(ctx, registeredRepository.TargetID)
	require.NoError(t, err)
	require.False(t, repo.PullBeforeWorktree)
	newBranch := "develop"
	staleRepositoryVersion := repo.UpdatedAt.Add(-time.Second)
	_, err = services.Task.UpdateRepository(ctx, repo.ID, &taskservice.UpdateRepositoryRequest{
		ExpectedUpdatedAt: &staleRepositoryVersion, DefaultBranch: &newBranch,
	})
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict,
		"repository updates must carry the plugin's observed resource version to storage")
	updatedRepository, err := adapter.updateRepository(ctx, workspace.ID, &pluginsdk.WorkspaceRepositoryUpdate{
		RepositoryID: repo.ID, ExpectedResourceVersion: repo.UpdatedAt.UTC().Format(time.RFC3339Nano), DefaultBranch: &newBranch,
	})
	require.NoError(t, err)
	staleBranch := "release"
	_, err = adapter.updateRepository(ctx, workspace.ID, &pluginsdk.WorkspaceRepositoryUpdate{
		RepositoryID: repo.ID, ExpectedResourceVersion: repo.UpdatedAt.UTC().Format(time.RFC3339Nano), DefaultBranch: &staleBranch,
	})
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict)
	require.NotEmpty(t, updatedRepository.ResourceVersion)

	foreignWorkspace, err := services.Task.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{Name: "Foreign Workspace"})
	require.NoError(t, err)
	_, err = adapter.updateRepository(ctx, foreignWorkspace.ID, &pluginsdk.WorkspaceRepositoryUpdate{
		RepositoryID: repo.ID, ExpectedResourceVersion: repo.UpdatedAt.UTC().Format(time.RFC3339Nano), DefaultBranch: &staleBranch,
	})
	require.ErrorIs(t, err, repoerrors.ErrRepositoryNotFound)
}

func TestPluginsWorkspaceAdminCannotEditImproveKandevResources(t *testing.T) {
	ctx := context.Background()
	services, _, _ := provideTestServices(t, "workspace-admin-improve-guard")
	workspace, err := services.Task.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{Name: taskmodels.WorkspaceNameImproveKandev})
	require.NoError(t, err)
	workflow, err := services.Task.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{
		ID: "improve-workflow", WorkspaceID: workspace.ID, Name: "Workflow",
	})
	require.NoError(t, err)
	steps := []*workflowmodels.WorkflowStep{
		{ID: "improve-step-a", WorkflowID: workflow.ID, Name: "First", Position: 0, StageType: workflowmodels.StageTypeWork},
		{ID: "improve-step-b", WorkflowID: workflow.ID, Name: "Second", Position: 1, StageType: workflowmodels.StageTypeReview},
	}
	for _, step := range steps {
		_, err := services.Workflow.CreateStepWithStartStepUpdates(ctx, step)
		require.NoError(t, err)
	}
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	require.NoError(t, exec.Command("git", "init", "-q", repositoryPath).Run())
	repository, err := services.Task.CreateRepository(ctx, &taskservice.CreateRepositoryRequest{
		WorkspaceID: workspace.ID, Name: "Improve repository", SourceType: "local", LocalPath: repositoryPath,
	})
	require.NoError(t, err)
	adapter := pluginsWorkspaceAdminAdapter{tasks: services.Task, workflows: services.Workflow}

	workflowName := "Changed workflow"
	_, err = adapter.updateWorkflow(ctx, workspace.ID, &pluginsdk.WorkspaceWorkflowUpdate{
		WorkflowID: workflow.ID, ExpectedResourceVersion: workflow.UpdatedAt.UTC().Format(time.RFC3339Nano), Name: &workflowName,
	})
	require.Error(t, err, "reserved workspace workflow updates are denied")

	currentWorkflow, err := services.Task.GetWorkflow(ctx, workflow.ID)
	require.NoError(t, err)
	currentStep, err := services.Workflow.GetStep(ctx, steps[0].ID)
	require.NoError(t, err)
	_, err = adapter.updateWorkflowStep(ctx, workspace.ID, &pluginsdk.WorkspaceWorkflowStepUpdate{
		WorkflowID: workflow.ID, StepID: currentStep.ID,
		ExpectedWorkflowResourceVersion: currentWorkflow.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ExpectedStepResourceVersion:     currentStep.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Configuration:                   adminStepConfig(currentStep),
	})
	require.Error(t, err, "reserved workspace step updates are denied")

	currentSteps, err := services.Workflow.ListStepsByWorkflow(ctx, workflow.ID)
	require.NoError(t, err)
	_, err = adapter.reorderWorkflowSteps(ctx, workspace.ID, &pluginsdk.WorkspaceWorkflowStepReorder{
		WorkflowID: workflow.ID, ExpectedWorkflowResourceVersion: currentWorkflow.UpdatedAt.UTC().Format(time.RFC3339Nano),
		StepIDs: []string{currentSteps[1].ID, currentSteps[0].ID},
		StepResourceVersions: []string{
			currentSteps[1].UpdatedAt.UTC().Format(time.RFC3339Nano), currentSteps[0].UpdatedAt.UTC().Format(time.RFC3339Nano),
		},
	})
	require.Error(t, err, "reserved workspace step reorders are denied")

	repositoryName := "Changed repository"
	_, err = adapter.updateRepository(ctx, workspace.ID, &pluginsdk.WorkspaceRepositoryUpdate{
		RepositoryID: repository.ID, ExpectedResourceVersion: repository.UpdatedAt.UTC().Format(time.RFC3339Nano), Name: &repositoryName,
	})
	require.Error(t, err, "reserved workspace repository updates are denied")
}

func adminStepConfig(step *workflowmodels.WorkflowStep) pluginsdk.WorkflowStepConfiguration {
	return pluginsdk.WorkflowStepConfiguration{
		ID: step.ID, WorkflowID: step.WorkflowID, Name: step.Name, Position: int32(step.Position), StageType: string(step.StageType),
		Color: step.Color, Prompt: step.Prompt, AllowManualMove: step.AllowManualMove, IsStartStep: step.IsStartStep,
		ShowInCommandPanel: step.ShowInCommandPanel, AutoArchiveAfterHours: int32(step.AutoArchiveAfterHours),
		AgentProfileID: step.AgentProfileID, ProfileSessionStartPolicy: string(step.ProfileSessionStartPolicy),
		ProfileSessionEndPolicy: string(step.ProfileSessionEndPolicy), WIPLimit: int32(step.WIPLimit),
		PullFromStepID: step.PullFromStepID, AutoAdvanceRequiresSignal: step.AutoAdvanceRequiresSignal,
		CancelTriggersTurnComplete: step.CancelTriggersTurnComplete, CompleteTaskOnEnter: step.CompleteTaskOnEnter,
	}
}

func TestPluginsWorkspaceAdminRepositoryReplayMatchesPullSetting(t *testing.T) {
	current := &taskmodels.Repository{
		ID: "repository-1", WorkspaceID: "workspace-1", Name: "repo", SourceType: "remote",
		PullBeforeWorktree: true,
	}
	requested := &pluginsdk.WorkspaceRepositoryRegister{Name: "repo", SourceType: "remote", PullBeforeWorktree: workspaceAdminBoolPointer(false)}
	require.False(t, repositoryRegistrationMatches(current, "workspace-1", requested),
		"a retry with a different pull policy must not claim that repository registration already applied")
	requested.PullBeforeWorktree = nil
	require.True(t, repositoryRegistrationMatches(current, "workspace-1", requested),
		"an omitted pull policy uses the native default of true")
}

func workspaceAdminBoolPointer(value bool) *bool { return &value }
