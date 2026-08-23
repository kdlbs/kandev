package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
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

// setupSyncServiceWithLogger is a variant of setupSyncService for tests that
// need to observe log output (e.g. via a zaptest observer core).
func setupSyncServiceWithLogger(t *testing.T, log *logger.Logger) (*Service, *mockWorkflowProvider, *mockSyncOps) {
	svc, _ := setupTestService(t, log)
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

func TestApplySyncedWorkflows_UpdatesAndClearsWorkflowPrompt(t *testing.T) {
	svc, provider, _ := setupSyncService(t)
	ctx := context.Background()
	wf := addSyncedWorkflow(provider, "wf-1", "ws-1", "Dev Flow", "flows/dev.yml")
	wf.Prompt = "Old shared instructions."
	createStep(t, svc, &models.WorkflowStep{ID: "step-todo", WorkflowID: wf.ID, Name: "Todo", Position: 0, IsStartStep: true})

	// Update prompt from portable definition.
	updated := portableWorkflow("Dev Flow", "Todo")
	updated.Prompt = "If the PR is merged or closed, move the Task to Done."
	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", []SyncFileExport{
		{Path: "flows/dev.yml", Export: exportOf(updated)},
	})
	require.NoError(t, err)
	assert.Contains(t, result.Updated, "Dev Flow")

	got, err := provider.GetWorkflow(ctx, wf.ID)
	require.NoError(t, err)
	assert.Equal(t, "If the PR is merged or closed, move the Task to Done.", got.Prompt)

	// Omitting prompt in the portable file clears it on sync.
	cleared := portableWorkflow("Dev Flow", "Todo")
	cleared.Prompt = ""
	result, err = svc.ApplySyncedWorkflows(ctx, "ws-1", []SyncFileExport{
		{Path: "flows/dev.yml", Export: exportOf(cleared)},
	})
	require.NoError(t, err)
	assert.Contains(t, result.Updated, "Dev Flow")

	got, err = provider.GetWorkflow(ctx, wf.ID)
	require.NoError(t, err)
	assert.Equal(t, "", got.Prompt)
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

func TestApplySyncedWorkflows_PreservesCancelTriggersTurnComplete(t *testing.T) {
	svc, _, _ := setupSyncService(t)
	ctx := context.Background()
	first := models.WorkflowPortable{
		Name: "Cancel Flow",
		Steps: []models.StepPortable{
			{Name: "Work", Position: 0, IsStartStep: true, CancelTriggersTurnComplete: true},
			{Name: "Review", Position: 1},
		},
	}
	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", []SyncFileExport{{Path: "flows/cancel.yml", Export: exportOf(first)}})
	require.NoError(t, err)
	require.Equal(t, []string{"Cancel Flow"}, result.Created)

	steps, err := svc.ListStepsByWorkflow(ctx, "imported-Cancel Flow")
	require.NoError(t, err)
	require.Len(t, steps, 2)
	assert.True(t, steps[0].CancelTriggersTurnComplete)

	updated := first
	updated.Steps = append([]models.StepPortable(nil), first.Steps...)
	updated.Steps[0].CancelTriggersTurnComplete = false
	result, err = svc.ApplySyncedWorkflows(ctx, "ws-1", []SyncFileExport{{Path: "flows/cancel.yml", Export: exportOf(updated)}})
	require.NoError(t, err)
	require.Equal(t, []string{"Cancel Flow"}, result.Updated)
	steps, err = svc.ListStepsByWorkflow(ctx, "imported-Cancel Flow")
	require.NoError(t, err)
	assert.False(t, steps[0].CancelTriggersTurnComplete)
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
	assert.NoError(t, svc.EnsureWorkflowMutable(ctx, "wf-missing"),
		"guard fails open on lookup errors; the mutation itself surfaces not-found")
}

func TestEnsureWorkflowMutable_BlocksImproveWorkspace(t *testing.T) {
	svc, provider, _ := setupSyncService(t)
	ctx := context.Background()
	provider.addWorkflow("wf-improve", "ws-improve", "Improve Kandev Workflow")
	svc.SetWorkspaceProvider(&fakeWorkspaceProvider{
		workspaces: map[string]*taskmodels.Workspace{
			"ws-improve": {ID: "ws-improve", Name: "Improve Kandev"},
			"ws-normal":  {ID: "ws-normal", Name: "Default Workspace"},
		},
	})
	provider.addWorkflow("wf-normal", "ws-normal", "Normal Flow")

	assert.ErrorIs(t, svc.EnsureWorkflowMutable(ctx, "wf-improve"), ErrWorkflowWorkspaceReadOnly)
	assert.NoError(t, svc.EnsureWorkflowMutable(ctx, "wf-normal"))
}

type fakeWorkspaceProvider struct {
	workspaces map[string]*taskmodels.Workspace
}

func (f *fakeWorkspaceProvider) GetWorkspace(_ context.Context, id string) (*taskmodels.Workspace, error) {
	if workspace, ok := f.workspaces[id]; ok {
		return workspace, nil
	}
	return nil, fmt.Errorf("workspace not found: %s", id)
}

func TestReleaseSyncedWorkflows(t *testing.T) {
	svc, provider, _ := setupSyncService(t)
	ctx := context.Background()
	addSyncedWorkflow(provider, "wf-a", "ws-1", "Flow A", "flows/a.yml")
	addSyncedWorkflow(provider, "wf-b", "ws-1", "Flow B", "flows/b.yml")
	provider.addWorkflow("wf-manual", "ws-1", "Manual Flow")

	released, err := svc.ReleaseSyncedWorkflows(ctx, "ws-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"Flow A", "Flow B"}, released)

	for _, id := range []string{"wf-a", "wf-b"} {
		wf, err := provider.GetWorkflow(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, taskmodels.WorkflowSourceManual, wf.Source, "released workflows become manual")
		assert.Empty(t, wf.SourcePath)
		assert.NoError(t, svc.EnsureWorkflowMutable(ctx, id), "released workflows are editable again")
	}
}

// TestApplySyncedWorkflows_RebindingStepAgentProfileLogsWarning covers the
// visibility half of the profile-rebinding defect: a sync round that changes
// an existing step's AgentProfileID must warn (naming the workflow, step,
// and both profile IDs) since the workflow is read-only to the user and the
// change would otherwise be silent. A second sync that resolves to the same
// profile must not add another warning.
func TestApplySyncedWorkflows_RebindingStepAgentProfileLogsWarning(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)

	svc, provider, _ := setupSyncServiceWithLogger(t, log)
	ctx := context.Background()
	wf := addSyncedWorkflow(provider, "wf-1", "ws-1", "Dev Flow", "flows/dev.yml")
	createStep(t, svc, &models.WorkflowStep{
		ID: "step-todo", WorkflowID: wf.ID, Name: "Todo", Position: 0, IsStartStep: true,
		AgentProfileID: "profile-old",
	})

	pw := portableWorkflow("Dev Flow", "Todo")
	pw.Steps[0].AgentProfile = &models.AgentProfilePortable{AgentName: "Claude", Model: "opus[1m]", Mode: "auto"}
	files := []SyncFileExport{{Path: "flows/dev.yml", Export: exportOf(pw)}}

	svc.SetAgentProfileFuncs(nil, func(string, string, string, string) string { return "profile-new" })
	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)
	assert.Equal(t, []string{"Dev Flow"}, result.Updated)

	entries := logs.FilterMessage("workflow sync rebinding step's agent profile").All()
	require.Len(t, entries, 1, "rebind must be logged exactly once")
	fields := entries[0].ContextMap()
	assert.Equal(t, "step-todo", fields["step_id"])
	assert.Equal(t, "Todo", fields["step_name"])
	assert.Equal(t, "profile-old", fields["old_agent_profile_id"])
	assert.Equal(t, "profile-new", fields["new_agent_profile_id"])

	// A second sync resolving to the same profile must not warn again.
	result, err = svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)
	assert.Empty(t, result.Updated, "no drift means nothing is rewritten")
	assert.Len(t, logs.FilterMessage("workflow sync rebinding step's agent profile").All(), 1,
		"an unchanged profile must not log another warning")
}

// TestApplySyncedWorkflows_RebindWarningDoesNotFireOnFailedUpdate covers
// R1-F1: the rebind Warn must only fire once the change is actually
// persisted. It forces the step's UpdateStep write to fail (via PRAGMA
// query_only, which makes every write on this connection fail while reads —
// like the ListStepsByWorkflow that already ran — keep working) and asserts
// no Warn is logged for a rebind that never landed.
func TestApplySyncedWorkflows_RebindWarningDoesNotFireOnFailedUpdate(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)

	svc, db := setupTestService(t, log)
	provider := &mockWorkflowProvider{}
	svc.SetWorkflowProvider(provider)
	ops := newMockSyncOps(provider)
	svc.SetSyncWorkflowOps(ops)

	ctx := context.Background()
	wf := addSyncedWorkflow(provider, "wf-1", "ws-1", "Dev Flow", "flows/dev.yml")
	createStep(t, svc, &models.WorkflowStep{
		ID: "step-todo", WorkflowID: wf.ID, Name: "Todo", Position: 0, IsStartStep: true,
		AgentProfileID: "profile-old",
	})

	pw := portableWorkflow("Dev Flow", "Todo")
	pw.Steps[0].AgentProfile = &models.AgentProfilePortable{AgentName: "Claude", Model: "opus[1m]", Mode: "auto"}
	files := []SyncFileExport{{Path: "flows/dev.yml", Export: exportOf(pw)}}
	svc.SetAgentProfileFuncs(nil, func(string, string, string, string) string { return "profile-new" })

	_, err = db.Exec("PRAGMA query_only = ON")
	t.Cleanup(func() { _, _ = db.Exec("PRAGMA query_only = OFF") })
	require.NoError(t, err)

	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)
	require.Len(t, result.Warnings, 1, "the failed step write surfaces a warning")
	assert.Contains(t, result.Warnings[0], "Todo")
	assert.Empty(t, result.Updated, "a step update that failed to persist must not be reported as applied")
	assert.Empty(t, logs.FilterMessage("workflow sync rebinding step's agent profile").All(),
		"a rebind Warn must never fire for a change that was not actually persisted")
}

// TestApplySyncedWorkflows_RebindingWorkflowAgentProfileLogsWarning covers
// R1-F2: the workflow-level AgentProfileID (the fallback steps with no
// profile of their own resolve to) is rebound by the same matcher and must
// be logged just like a step-level rebind.
func TestApplySyncedWorkflows_RebindingWorkflowAgentProfileLogsWarning(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)

	svc, provider, _ := setupSyncServiceWithLogger(t, log)
	ctx := context.Background()
	wf := addSyncedWorkflow(provider, "wf-1", "ws-1", "Dev Flow", "flows/dev.yml")
	wf.AgentProfileID = "profile-old"
	createStep(t, svc, &models.WorkflowStep{ID: "step-todo", WorkflowID: wf.ID, Name: "Todo", Position: 0, IsStartStep: true})

	pw := portableWorkflow("Dev Flow", "Todo")
	pw.AgentProfile = &models.AgentProfilePortable{AgentName: "Claude", Model: "opus[1m]", Mode: "auto"}
	files := []SyncFileExport{{Path: "flows/dev.yml", Export: exportOf(pw)}}

	svc.SetAgentProfileFuncs(nil, func(string, string, string, string) string { return "profile-new" })
	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)
	assert.Equal(t, []string{"Dev Flow"}, result.Updated)

	got, err := provider.GetWorkflow(ctx, wf.ID)
	require.NoError(t, err)
	assert.Equal(t, "profile-new", got.AgentProfileID)

	entries := logs.FilterMessage("workflow sync rebinding workflow's agent profile").All()
	require.Len(t, entries, 1, "workflow-level rebind must be logged exactly once")
	fields := entries[0].ContextMap()
	assert.Equal(t, "wf-1", fields["workflow_id"])
	assert.Equal(t, "Dev Flow", fields["workflow_name"])
	assert.Equal(t, "profile-old", fields["old_agent_profile_id"])
	assert.Equal(t, "profile-new", fields["new_agent_profile_id"])

	// A second sync resolving to the same profile must not warn again.
	result, err = svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)
	assert.Empty(t, result.Updated, "no drift means nothing is rewritten")
	assert.Len(t, logs.FilterMessage("workflow sync rebinding workflow's agent profile").All(), 1,
		"an unchanged profile must not log another warning")
}

// TestApplySyncedWorkflows_WorkflowRebindWarningDoesNotFireOnFailedUpdate is
// the workflow-level counterpart to
// TestApplySyncedWorkflows_RebindWarningDoesNotFireOnFailedUpdate (R1-F1): a
// rebind Warn must never fire for a workflow-level profile change that failed
// to persist.
func TestApplySyncedWorkflows_WorkflowRebindWarningDoesNotFireOnFailedUpdate(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)

	svc, provider, _ := setupSyncServiceWithLogger(t, log)
	ctx := context.Background()
	wf := addSyncedWorkflow(provider, "wf-1", "ws-1", "Dev Flow", "flows/dev.yml")
	wf.AgentProfileID = "profile-old"
	createStep(t, svc, &models.WorkflowStep{ID: "step-todo", WorkflowID: wf.ID, Name: "Todo", Position: 0, IsStartStep: true})

	pw := portableWorkflow("Dev Flow", "Todo")
	pw.AgentProfile = &models.AgentProfilePortable{AgentName: "Claude", Model: "opus[1m]", Mode: "auto"}
	files := []SyncFileExport{{Path: "flows/dev.yml", Export: exportOf(pw)}}
	svc.SetAgentProfileFuncs(nil, func(string, string, string, string) string { return "profile-new" })

	provider.forceUpdateWorkflowErr = fmt.Errorf("simulated UpdateWorkflow failure")

	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)
	require.Len(t, result.Warnings, 1, "the failed workflow write surfaces a warning")
	assert.Contains(t, result.Warnings[0], "Dev Flow")
	assert.Empty(t, result.Updated, "a workflow update that failed to persist must not be reported as applied")
	assert.Empty(t, logs.FilterMessage("workflow sync rebinding workflow's agent profile").All(),
		"a rebind Warn must never fire for a workflow change that was not actually persisted")
}

// TestApplySyncedWorkflows_StepMatcherReceivesCurrentBinding proves
// updateSyncedWorkflow passes the step's existing AgentProfileID to the
// matcher as currentID. A real matcher (buildAgentProfileMatcher) uses that
// to preserve a binding that was disabled after the fact instead of treating
// disablement as a rebind (docs/specs/agents/requirements/profile-disable.md). This test
// stubs that same contract: the matcher echoes currentID back when given
// one, standing in for "the bound profile still matches, keep it".
func TestApplySyncedWorkflows_StepMatcherReceivesCurrentBinding(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)

	svc, provider, _ := setupSyncServiceWithLogger(t, log)
	ctx := context.Background()
	wf := addSyncedWorkflow(provider, "wf-1", "ws-1", "Dev Flow", "flows/dev.yml")
	wf.Description = "synced Dev Flow" // must already match pw's description below, or the workflow itself counts as changed
	createStep(t, svc, &models.WorkflowStep{
		ID: "step-todo", WorkflowID: wf.ID, Name: "Todo", Position: 0, Color: "#aabbcc", IsStartStep: true,
		AgentProfileID: "profile-disabled",
	})

	pw := portableWorkflow("Dev Flow", "Todo")
	pw.Steps[0].AgentProfile = &models.AgentProfilePortable{AgentName: "Claude", Model: "opus[1m]", Mode: "auto"}
	files := []SyncFileExport{{Path: "flows/dev.yml", Export: exportOf(pw)}}

	svc.SetAgentProfileFuncs(nil, func(_, _, _, currentID string) string {
		if currentID != "" {
			return currentID
		}
		return "profile-new"
	})
	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)
	assert.Empty(t, result.Updated, "a preserved binding must not be reported as a change")
	assert.Empty(t, logs.FilterMessage("workflow sync rebinding step's agent profile").All(),
		"a preserved binding must not log a rebind")
}

// TestApplySyncedWorkflows_StepProfileUnbindWarnsWithEmptyNewID covers a
// genuine unbind: the matcher finds no candidate at all (not the
// still-matches-but-disabled case above), so the step's AgentProfileID
// resolves to "". The rebind Warn must still fire and its
// new_agent_profile_id field must be observably empty, so a reader of the
// log can tell the step lost its profile rather than gained a different one.
func TestApplySyncedWorkflows_StepProfileUnbindWarnsWithEmptyNewID(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)

	svc, provider, _ := setupSyncServiceWithLogger(t, log)
	ctx := context.Background()
	wf := addSyncedWorkflow(provider, "wf-1", "ws-1", "Dev Flow", "flows/dev.yml")
	createStep(t, svc, &models.WorkflowStep{
		ID: "step-todo", WorkflowID: wf.ID, Name: "Todo", Position: 0, IsStartStep: true,
		AgentProfileID: "profile-old",
	})

	pw := portableWorkflow("Dev Flow", "Todo")
	pw.Steps[0].AgentProfile = &models.AgentProfilePortable{AgentName: "Claude", Model: "opus[1m]", Mode: "auto"}
	files := []SyncFileExport{{Path: "flows/dev.yml", Export: exportOf(pw)}}

	svc.SetAgentProfileFuncs(nil, func(string, string, string, string) string { return "" })
	result, err := svc.ApplySyncedWorkflows(ctx, "ws-1", files)
	require.NoError(t, err)
	assert.Equal(t, []string{"Dev Flow"}, result.Updated)

	entries := logs.FilterMessage("workflow sync rebinding step's agent profile").All()
	require.Len(t, entries, 1, "an unbind must still be logged")
	fields := entries[0].ContextMap()
	assert.Equal(t, "profile-old", fields["old_agent_profile_id"])
	assert.Equal(t, "", fields["new_agent_profile_id"])
}
