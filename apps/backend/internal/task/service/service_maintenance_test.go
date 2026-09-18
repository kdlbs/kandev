package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAssistantMaintenanceNoContributionEffects(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	workflowID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-maintenance")
	wf, err := repo.GetWorkflow(ctx, workflowID)
	require.NoError(t, err)
	template := "improve-kandev"
	wf.WorkflowTemplateID = &template
	require.NoError(t, repo.UpdateWorkflow(ctx, wf))
	preparer := &fakeContributionDestinationPreparer{}
	svc.SetContributionDestinationPreparer(preparer)
	_, err = svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-maintenance", WorkflowID: workflowID, Title: "Local repair", LocalPreparationOnly: true})
	require.Error(t, err)
	require.Zero(t, preparer.calls, "deny before the contribution provider can publish a fork")
}
