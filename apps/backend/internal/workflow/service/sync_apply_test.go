package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
)

// mockSyncOps implements SyncWorkflowOps on top of a mockWorkflowProvider.
type mockSyncOps struct {
	provider         *mockWorkflowProvider
	tasksPerWorkflow map[string]int
	tasksPerStep     map[string]int
	deleted          []string
}

func newMockSyncOps(provider *mockWorkflowProvider) *mockSyncOps {
	return &mockSyncOps{
		provider:         provider,
		tasksPerWorkflow: map[string]int{},
		tasksPerStep:     map[string]int{},
	}
}

func (m *mockSyncOps) DeleteWorkflow(_ context.Context, id string) error {
	for i, wf := range m.provider.workflows {
		if wf.ID == id {
			m.provider.workflows = append(m.provider.workflows[:i], m.provider.workflows[i+1:]...)
			m.deleted = append(m.deleted, id)
			return nil
		}
	}
	return fmt.Errorf("workflow %s not found", id)
}

func (m *mockSyncOps) CountTasksByWorkflow(_ context.Context, workflowID string) (int, error) {
	return m.tasksPerWorkflow[workflowID], nil
}

func (m *mockSyncOps) CountTasksByWorkflowStep(_ context.Context, stepID string) (int, error) {
	return m.tasksPerStep[stepID], nil
}

func (m *mockSyncOps) SetWorkflowSource(ctx context.Context, id, source, sourcePath string) error {
	wf, err := m.provider.GetWorkflow(ctx, id)
	if err != nil {
		return err
	}
	wf.Source = source
	wf.SourcePath = sourcePath
	return nil
}

func setupSyncService(t *testing.T) (*Service, *mockWorkflowProvider, *mockSyncOps) {
	svc, _ := setupTestService(t)
	provider := &mockWorkflowProvider{}
	svc.SetWorkflowProvider(provider)
	ops := newMockSyncOps(provider)
	svc.SetSyncWorkflowOps(ops)
	return svc, provider, ops
}

func portableWorkflow(name string, stepNames ...string) models.WorkflowPortable {
	steps := make([]models.StepPortable, 0, len(stepNames))
	for i, sn := range stepNames {
		steps = append(steps, models.StepPortable{
			Name:        sn,
			Position:    i,
			Color:       "#aabbcc",
			IsStartStep: i == 0,
		})
	}
	return models.WorkflowPortable{Name: name, Description: "synced " + name, Steps: steps}
}

func exportOf(workflows ...models.WorkflowPortable) *models.WorkflowExport {
	return &models.WorkflowExport{
		Version:   models.ExportVersion,
		Type:      models.ExportType,
		Workflows: workflows,
	}
}

func addSyncedWorkflow(provider *mockWorkflowProvider, id, workspaceID, name, sourcePath string) *taskmodels.Workflow {
	provider.addWorkflow(id, workspaceID, name)
	wf := provider.workflows[len(provider.workflows)-1]
	wf.Source = taskmodels.WorkflowSourceGitHub
	wf.SourcePath = sourcePath
	return wf
}

func TestApplySyncedWorkflows_CreatesNewWorkflows(t *testing.T) {
	svc, provider, _ := setupSyncService(t)
	ctx := context.Background()

	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", []SyncFileExport{
		{Path: "flows/dev.yml", Export: exportOf(portableWorkflow("Dev Flow", "Todo", "Doing", "Done"))},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"Dev Flow"}, result.Created)
	assert.Empty(t, result.Warnings)

	wf, err := provider.GetWorkflow(ctx, "imported-Dev Flow")
	require.NoError(t, err)
	assert.Equal(t, taskmodels.WorkflowSourceGitHub, wf.Source)
	assert.Equal(t, "flows/dev.yml", wf.SourcePath)

	steps, err := svc.ListStepsByWorkflow(ctx, wf.ID)
	require.NoError(t, err)
	require.Len(t, steps, 3)
	assert.Equal(t, "Todo", steps[0].Name)
	assert.True(t, steps[0].IsStartStep)
}

func TestApplySyncedWorkflows_UpdatesMatchedWorkflowPreservingStepIDs(t *testing.T) {
	svc, provider, _ := setupSyncService(t)
	ctx := context.Background()
	wf := addSyncedWorkflow(provider, "wf-1", "ws-1", "Dev Flow", "flows/dev.yml")

	createStep(t, svc, &models.WorkflowStep{ID: "step-todo", WorkflowID: wf.ID, Name: "Todo", Position: 0, IsStartStep: true})
	createStep(t, svc, &models.WorkflowStep{ID: "step-done", WorkflowID: wf.ID, Name: "Done", Position: 1})

	// New definition: Todo keeps its identity but moves color/position,
	// "Review" is inserted, "Done" survives at position 2.
	pw := portableWorkflow("Dev Flow", "Todo", "Review", "Done")
	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", []SyncFileExport{
		{Path: "flows/dev.yml", Export: exportOf(pw)},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"Dev Flow"}, result.Updated)
	assert.Empty(t, result.Created)
	assert.Empty(t, result.Warnings)
	assert.Equal(t, "synced Dev Flow", wf.Description)

	steps, err := svc.ListStepsByWorkflow(ctx, wf.ID)
	require.NoError(t, err)
	require.Len(t, steps, 3)
	byName := map[string]*models.WorkflowStep{}
	for _, st := range steps {
		byName[st.Name] = st
	}
	assert.Equal(t, "step-todo", byName["Todo"].ID, "matched step keeps its ID")
	assert.Equal(t, "step-done", byName["Done"].ID, "matched step keeps its ID")
	assert.Equal(t, 1, byName["Review"].Position)
	assert.Equal(t, 2, byName["Done"].Position)
}

func TestApplySyncedWorkflows_RemovedStepWithTasksBlocksUpdate(t *testing.T) {
	svc, provider, ops := setupSyncService(t)
	ctx := context.Background()
	wf := addSyncedWorkflow(provider, "wf-1", "ws-1", "Dev Flow", "flows/dev.yml")
	createStep(t, svc, &models.WorkflowStep{ID: "step-todo", WorkflowID: wf.ID, Name: "Todo", Position: 0, IsStartStep: true})
	createStep(t, svc, &models.WorkflowStep{ID: "step-legacy", WorkflowID: wf.ID, Name: "Legacy", Position: 1})
	ops.tasksPerStep["step-legacy"] = 2

	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", []SyncFileExport{
		{Path: "flows/dev.yml", Export: exportOf(portableWorkflow("Dev Flow", "Todo"))},
	})
	require.NoError(t, err)
	assert.Empty(t, result.Updated)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], "Legacy")
	assert.Contains(t, result.Warnings[0], "2 task(s)")

	steps, err := svc.ListStepsByWorkflow(ctx, wf.ID)
	require.NoError(t, err)
	assert.Len(t, steps, 2, "workflow left untouched")
}

func TestApplySyncedWorkflows_RemovedStepWithoutTasksIsDeleted(t *testing.T) {
	svc, provider, _ := setupSyncService(t)
	ctx := context.Background()
	wf := addSyncedWorkflow(provider, "wf-1", "ws-1", "Dev Flow", "flows/dev.yml")
	createStep(t, svc, &models.WorkflowStep{ID: "step-todo", WorkflowID: wf.ID, Name: "Todo", Position: 0, IsStartStep: true})
	createStep(t, svc, &models.WorkflowStep{ID: "step-legacy", WorkflowID: wf.ID, Name: "Legacy", Position: 1})

	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", []SyncFileExport{
		{Path: "flows/dev.yml", Export: exportOf(portableWorkflow("Dev Flow", "Todo"))},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"Dev Flow"}, result.Updated)
	assert.Empty(t, result.Warnings)

	steps, err := svc.ListStepsByWorkflow(ctx, wf.ID)
	require.NoError(t, err)
	require.Len(t, steps, 1)
	assert.Equal(t, "Todo", steps[0].Name)
}

func TestApplySyncedWorkflows_DeletesVanishedWorkflowWithoutTasks(t *testing.T) {
	svc, provider, ops := setupSyncService(t)
	ctx := context.Background()
	addSyncedWorkflow(provider, "wf-gone", "ws-1", "Old Flow", "flows/old.yml")

	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"Old Flow"}, result.Deleted)
	assert.Equal(t, []string{"wf-gone"}, ops.deleted)
}

func TestApplySyncedWorkflows_KeepsVanishedWorkflowWithTasks(t *testing.T) {
	svc, provider, ops := setupSyncService(t)
	ctx := context.Background()
	addSyncedWorkflow(provider, "wf-gone", "ws-1", "Old Flow", "flows/old.yml")
	ops.tasksPerWorkflow["wf-gone"] = 3

	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", nil)
	require.NoError(t, err)
	assert.Empty(t, result.Deleted)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], "Old Flow")
	assert.Empty(t, ops.deleted)
}

func TestApplySyncedWorkflows_BrokenFileFreezesItsWorkflows(t *testing.T) {
	svc, provider, ops := setupSyncService(t)
	ctx := context.Background()
	addSyncedWorkflow(provider, "wf-frozen", "ws-1", "Frozen Flow", "flows/broken.yml")

	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", []SyncFileExport{
		{Path: "flows/broken.yml", Export: nil},
	})
	require.NoError(t, err)
	assert.Empty(t, result.Deleted)
	assert.Empty(t, ops.deleted, "workflows from unparseable files must not be deleted")
}

func TestApplySyncedWorkflows_LeavesManualWorkflowsAlone(t *testing.T) {
	svc, provider, ops := setupSyncService(t)
	ctx := context.Background()
	provider.addWorkflow("wf-manual", "ws-1", "Dev Flow") // manual workflow, same name

	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", []SyncFileExport{
		{Path: "flows/dev.yml", Export: exportOf(portableWorkflow("Dev Flow", "Todo"))},
	})
	require.NoError(t, err)
	// A synced sibling is created; the manual workflow is neither updated nor deleted.
	assert.Equal(t, []string{"Dev Flow"}, result.Created)
	assert.Empty(t, ops.deleted)

	manual, err := provider.GetWorkflow(ctx, "wf-manual")
	require.NoError(t, err)
	assert.Equal(t, "", manual.Source)
	assert.Equal(t, "", manual.Description)
}

func TestApplySyncedWorkflows_DuplicateStepNamesInDefinitionWarn(t *testing.T) {
	svc, provider, _ := setupSyncService(t)
	ctx := context.Background()
	wf := addSyncedWorkflow(provider, "wf-1", "ws-1", "Dev Flow", "flows/dev.yml")
	createStep(t, svc, &models.WorkflowStep{ID: "step-todo", WorkflowID: wf.ID, Name: "Todo", Position: 0, IsStartStep: true})

	pw := models.WorkflowPortable{
		Name: "Dev Flow",
		Steps: []models.StepPortable{
			{Name: "Todo", Position: 0},
			{Name: "Todo", Position: 1},
		},
	}
	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", []SyncFileExport{
		{Path: "flows/dev.yml", Export: exportOf(pw)},
	})
	require.NoError(t, err)
	assert.Empty(t, result.Updated)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], "must be unique")
}

func TestApplySyncedWorkflows_RemapsStepEventPositions(t *testing.T) {
	svc, _, _ := setupSyncService(t)
	ctx := context.Background()

	pw := models.WorkflowPortable{
		Name: "Flow",
		Steps: []models.StepPortable{
			{
				Name: "Start", Position: 0, IsStartStep: true,
				Events: models.StepEvents{
					OnTurnComplete: []models.OnTurnCompleteAction{
						{Type: models.OnTurnCompleteMoveToStep, Config: map[string]any{"step_position": 1}},
					},
				},
			},
			{Name: "End", Position: 1},
		},
	}
	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", []SyncFileExport{
		{Path: "flows/flow.yml", Export: exportOf(pw)},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"Flow"}, result.Created)

	steps, err := svc.ListStepsByWorkflow(ctx, "imported-Flow")
	require.NoError(t, err)
	require.Len(t, steps, 2)
	require.Len(t, steps[0].Events.OnTurnComplete, 1)
	assert.Equal(t, steps[1].ID, steps[0].Events.OnTurnComplete[0].Config["step_id"])
}

func TestApplySyncedWorkflows_SecondApplyIsNoOp(t *testing.T) {
	svc, _, _ := setupSyncService(t)
	ctx := context.Background()
	files := []SyncFileExport{
		{Path: "flows/dev.yml", Export: exportOf(portableWorkflow("Dev Flow", "Todo", "Done"))},
	}

	first, err := svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)
	require.Equal(t, []string{"Dev Flow"}, first.Created)

	second, err := svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)
	assert.Empty(t, second.Created)
	assert.Empty(t, second.Updated, "no drift means nothing is rewritten or broadcast")
	assert.Empty(t, second.Deleted)
	assert.Empty(t, second.Warnings)
}

func TestApplySyncedWorkflows_RepairsLocalStepEdit(t *testing.T) {
	svc, _, _ := setupSyncService(t)
	ctx := context.Background()
	files := []SyncFileExport{
		{Path: "flows/dev.yml", Export: exportOf(portableWorkflow("Dev Flow", "Todo", "Done"))},
	}
	_, err := svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)

	// A user recolors a synced step in the UI.
	steps, err := svc.ListStepsByWorkflow(ctx, "imported-Dev Flow")
	require.NoError(t, err)
	steps[0].Color = "#123456"
	require.NoError(t, svc.repo.UpdateStep(ctx, steps[0]))

	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)
	assert.Equal(t, []string{"Dev Flow"}, result.Updated, "local drift is repaired")

	steps, err = svc.ListStepsByWorkflow(ctx, "imported-Dev Flow")
	require.NoError(t, err)
	assert.Equal(t, "#aabbcc", steps[0].Color, "definition wins over the local edit")
}

func TestApplySyncedWorkflows_RecreatesLocallyDeletedStep(t *testing.T) {
	svc, _, _ := setupSyncService(t)
	ctx := context.Background()
	files := []SyncFileExport{
		{Path: "flows/dev.yml", Export: exportOf(portableWorkflow("Dev Flow", "Todo", "Done"))},
	}
	_, err := svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)

	steps, err := svc.ListStepsByWorkflow(ctx, "imported-Dev Flow")
	require.NoError(t, err)
	require.Len(t, steps, 2)
	require.NoError(t, svc.DeleteStep(ctx, steps[1].ID))

	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)
	assert.Equal(t, []string{"Dev Flow"}, result.Updated)

	steps, err = svc.ListStepsByWorkflow(ctx, "imported-Dev Flow")
	require.NoError(t, err)
	require.Len(t, steps, 2, "locally deleted step is recreated from the definition")
	assert.Equal(t, "Done", steps[1].Name)
}

func TestEnsureWorkflowMutable(t *testing.T) {
	svc, provider, _ := setupSyncService(t)
	ctx := context.Background()
	addSyncedWorkflow(provider, "wf-synced", "ws-1", "Synced Flow", "flows/dev.yml")
	provider.addWorkflow("wf-manual", "ws-1", "Manual Flow")

	assert.ErrorIs(t, svc.EnsureWorkflowMutable(ctx, "wf-synced"), ErrWorkflowReadOnly)
	assert.NoError(t, svc.EnsureWorkflowMutable(ctx, "wf-manual"))
	assert.Error(t, svc.EnsureWorkflowMutable(ctx, "wf-missing"), "lookup errors propagate")
}
