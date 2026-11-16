package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type workflowTemplateCreatorFunc func(context.Context, string, string) error

func (f workflowTemplateCreatorFunc) CreateStepsFromTemplate(ctx context.Context, workflowID, templateID string) error {
	return f(ctx, workflowID, templateID)
}

func TestCreateWorkflowTemplateInitialization(t *testing.T) {
	t.Run("reports a missing template creator", func(t *testing.T) {
		svc, eventBus, repo := createTestService(t)
		ctx := context.Background()
		require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-template-no-creator", Name: "Workspace"}))
		templateID := "improve-kandev"

		created, err := svc.CreateWorkflow(ctx, &CreateWorkflowRequest{
			WorkspaceID:        "ws-template-no-creator",
			Name:               "Improve Kandev",
			WorkflowTemplateID: &templateID,
		})

		require.ErrorContains(t, err, "workflow step creator is not configured")
		require.Nil(t, created)
		require.Empty(t, eventBus.GetPublishedEvents())
	})

	t.Run("reports template errors without publishing workflow success", func(t *testing.T) {
		svc, eventBus, repo := createTestService(t)
		ctx := context.Background()
		require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-template-error", Name: "Workspace"}))
		wantErr := errors.New("template validation failed")
		svc.SetWorkflowStepCreator(workflowTemplateCreatorFunc(func(context.Context, string, string) error {
			return wantErr
		}))
		templateID := "improve-kandev"

		created, err := svc.CreateWorkflow(ctx, &CreateWorkflowRequest{
			WorkspaceID:        "ws-template-error",
			Name:               "Improve Kandev",
			WorkflowTemplateID: &templateID,
		})

		require.ErrorIs(t, err, wantErr)
		require.Nil(t, created)
		workflows, listErr := svc.ListWorkflows(ctx, "ws-template-error", true)
		require.NoError(t, listErr)
		require.Len(t, workflows, 1, "the empty workflow row remains available for bootstrap retry")
		require.Equal(t, templateID, *workflows[0].WorkflowTemplateID)
		for _, event := range eventBus.GetPublishedEvents() {
			require.NotEqual(t, events.WorkflowCreated, event.Type)
		}
	})

	t.Run("publishes workflow success after template initialization", func(t *testing.T) {
		svc, eventBus, repo := createTestService(t)
		ctx := context.Background()
		require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-template-success", Name: "Workspace"}))
		svc.SetWorkflowStepCreator(workflowTemplateCreatorFunc(func(context.Context, string, string) error {
			return nil
		}))
		templateID := "improve-kandev"

		created, err := svc.CreateWorkflow(ctx, &CreateWorkflowRequest{
			WorkspaceID:        "ws-template-success",
			Name:               "Improve Kandev",
			WorkflowTemplateID: &templateID,
		})

		require.NoError(t, err)
		require.NotNil(t, created)
		require.Contains(t, eventTypes(eventBus.GetPublishedEvents()), events.WorkflowCreated)
	})
}

func TestEnsureWorkflowTemplateStepsRejectsUnauthorizedAndSyncedWorkflows(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-template-policy", Name: "Workspace", OwnerID: "owner"}))
	managedTemplateID := "improve-kandev"
	syncedTemplateID := "report-kandev-issue"
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-foreign-template", WorkspaceID: "ws-template-policy", WorkflowTemplateID: &managedTemplateID,
	}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-synced-template", WorkspaceID: "ws-template-policy", WorkflowTemplateID: &syncedTemplateID,
		Source: models.WorkflowSourceGitHub,
	}))

	err := svc.EnsureWorkflowTemplateSteps(ctxAs("stranger"), "wf-foreign-template", managedTemplateID)
	require.ErrorIs(t, err, repoerrors.ErrWorkspaceNotFound)

	err = svc.EnsureWorkflowTemplateSteps(ctx, "wf-synced-template", syncedTemplateID)
	require.ErrorContains(t, err, "managed by GitHub sync")
}

func TestEnsureWorkflowTemplateStepsRequiresStepLister(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-template-lister", Name: "Workspace"}))
	templateID := "improve-kandev"
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-template-lister", WorkspaceID: "ws-template-lister", WorkflowTemplateID: &templateID,
	}))

	err := svc.EnsureWorkflowTemplateSteps(ctx, "wf-template-lister", templateID)
	require.ErrorContains(t, err, "workflow step lister is not configured")
}
