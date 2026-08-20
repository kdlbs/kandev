package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
)

func TestService_CoordinatorMonitoring_SaveThenLoad(t *testing.T) {
	svc, db := setupTestService(t)
	insertWorkflow(t, db, "wf-1", "Workflow 1")
	step := &models.WorkflowStep{WorkflowID: "wf-1", Name: "Step A", Position: 0}
	createStep(t, svc, step)

	ctx := context.Background()

	entries, err := svc.GetCoordinatorMonitoring(ctx, "wf-1")
	require.NoError(t, err)
	assert.Empty(t, entries)

	saved, err := svc.SetCoordinatorMonitoring(ctx, "ws-1", "wf-1", []models.CoordinatorStepMonitor{
		{WorkflowStepID: step.ID, Selected: true, Prompt: "watch closely"},
	})
	require.NoError(t, err)
	require.Len(t, saved, 1)
	assert.Equal(t, step.ID, saved[0].WorkflowStepID)
	assert.True(t, saved[0].Selected)
	assert.Equal(t, "watch closely", saved[0].Prompt)

	loaded, err := svc.GetCoordinatorMonitoring(ctx, "wf-1")
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	assert.Equal(t, "watch closely", loaded[0].Prompt)
}

func TestService_CoordinatorMonitoring_ReplaceRemovesStaleRows(t *testing.T) {
	svc, db := setupTestService(t)
	insertWorkflow(t, db, "wf-1", "Workflow 1")
	step := &models.WorkflowStep{WorkflowID: "wf-1", Name: "Step A", Position: 0}
	createStep(t, svc, step)

	ctx := context.Background()
	_, err := svc.SetCoordinatorMonitoring(ctx, "ws-1", "wf-1", []models.CoordinatorStepMonitor{
		{WorkflowStepID: step.ID, Selected: true, Prompt: "first"},
	})
	require.NoError(t, err)

	saved, err := svc.SetCoordinatorMonitoring(ctx, "ws-1", "wf-1", []models.CoordinatorStepMonitor{
		{WorkflowStepID: step.ID, Selected: false, Prompt: ""},
	})
	require.NoError(t, err)
	assert.Empty(t, saved)

	loaded, err := svc.GetCoordinatorMonitoring(ctx, "wf-1")
	require.NoError(t, err)
	assert.Empty(t, loaded)
}

func TestService_CoordinatorMonitoring_RejectsForeignStepID(t *testing.T) {
	svc, db := setupTestService(t)
	insertWorkflow(t, db, "wf-1", "Workflow 1")
	insertWorkflow(t, db, "wf-2", "Workflow 2")
	stepInOther := &models.WorkflowStep{WorkflowID: "wf-2", Name: "Other Step", Position: 0}
	createStep(t, svc, stepInOther)

	ctx := context.Background()
	_, err := svc.SetCoordinatorMonitoring(ctx, "ws-1", "wf-1", []models.CoordinatorStepMonitor{
		{WorkflowStepID: stepInOther.ID, Selected: true, Prompt: "should be rejected"},
	})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "does not belong to workflow"))

	loaded, err := svc.GetCoordinatorMonitoring(ctx, "wf-1")
	require.NoError(t, err)
	assert.Empty(t, loaded)
}

// The workspace stored on a coordinator monitoring row must come from the
// workflow, not from the caller: workspace_id arrives in the request body and
// a row whose workspace disagrees with its workflow is unusable provenance.
func TestService_CoordinatorMonitoring_StoresWorkflowOwnWorkspace(t *testing.T) {
	svc, db, provider := setupTestServiceWithProvider(t)
	insertWorkflow(t, db, "wf-1", "Workflow 1")
	provider.workflows = []*taskmodels.Workflow{{ID: "wf-1", WorkspaceID: "real-workspace"}}
	step := &models.WorkflowStep{WorkflowID: "wf-1", Name: "Step A", Position: 0}
	createStep(t, svc, step)

	_, err := svc.SetCoordinatorMonitoring(context.Background(), "attacker-supplied-workspace", "wf-1",
		[]models.CoordinatorStepMonitor{{WorkflowStepID: step.ID, Selected: true, Prompt: "watch"}})
	require.NoError(t, err)

	var stored string
	require.NoError(t, db.Get(&stored,
		"SELECT workspace_id FROM workflow_coordinator_monitoring WHERE workflow_id = ?", "wf-1"))
	assert.Equal(t, "real-workspace", stored)
}

// Without a wired workflow provider (a bare service) the supplied value is
// still used, so this resolution never turns into a hard dependency.
func TestService_CoordinatorMonitoring_FallsBackWithoutProvider(t *testing.T) {
	svc, db := setupTestService(t)
	insertWorkflow(t, db, "wf-1", "Workflow 1")
	step := &models.WorkflowStep{WorkflowID: "wf-1", Name: "Step A", Position: 0}
	createStep(t, svc, step)

	_, err := svc.SetCoordinatorMonitoring(context.Background(), "ws-fallback", "wf-1",
		[]models.CoordinatorStepMonitor{{WorkflowStepID: step.ID, Selected: true, Prompt: "watch"}})
	require.NoError(t, err)

	var stored string
	require.NoError(t, db.Get(&stored,
		"SELECT workspace_id FROM workflow_coordinator_monitoring WHERE workflow_id = ?", "wf-1"))
	assert.Equal(t, "ws-fallback", stored)
}

func TestService_CoordinatorMonitoring_ProviderFailureDoesNotPersistCallerWorkspace(t *testing.T) {
	svc, db, provider := setupTestServiceWithProvider(t)
	insertWorkflow(t, db, "wf-1", "Workflow 1")
	step := &models.WorkflowStep{WorkflowID: "wf-1", Name: "Step A", Position: 0}
	createStep(t, svc, step)
	provider.forceGetWorkflowErr = errors.New("workflow provider unavailable")

	_, err := svc.SetCoordinatorMonitoring(context.Background(), "attacker-supplied-workspace", "wf-1",
		[]models.CoordinatorStepMonitor{{WorkflowStepID: step.ID, Selected: true, Prompt: "watch"}})
	require.Error(t, err)

	var count int
	require.NoError(t, db.Get(&count, "SELECT COUNT(*) FROM workflow_coordinator_monitoring WHERE workflow_id = ?", "wf-1"))
	assert.Zero(t, count, "provider failure must not persist caller-controlled workspace provenance")
}
