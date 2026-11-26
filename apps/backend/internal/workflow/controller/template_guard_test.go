package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/workflow/service"
)

func TestCreateStepsFromTemplateReadOnly(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	svc := service.NewService(nil, log)
	t.Cleanup(func() { require.NoError(t, svc.Close()) })
	svc.SetWorkflowAuthorizationRequiredChecker(func(context.Context) bool { return true })
	svc.SetWorkflowAccessChecker(func(_ context.Context, workflowID string) error {
		if workflowID == "foreign-workflow" {
			return service.ErrNotVisible
		}
		return nil
	})
	svc.SetWorkflowMutationReadOnlyChecker(func(_ context.Context, workflowID string) (bool, bool, error) {
		switch workflowID {
		case "synced-workflow":
			return true, false, nil
		case "improve-workflow":
			return false, true, nil
		default:
			return false, false, nil
		}
	})
	ctrl := NewController(svc)

	t.Run("managed workspace remains read-only", func(t *testing.T) {
		err := ctrl.CreateStepsFromTemplate(context.Background(), CreateStepsFromTemplateRequest{
			WorkflowID: "improve-workflow",
			TemplateID: "simple",
		})
		require.ErrorIs(t, err, service.ErrWorkflowWorkspaceReadOnly)
	})

	t.Run("synced workflow remains read-only", func(t *testing.T) {
		err := ctrl.CreateStepsFromTemplate(context.Background(), CreateStepsFromTemplateRequest{
			WorkflowID: "synced-workflow",
			TemplateID: "simple",
		})
		require.ErrorIs(t, err, service.ErrWorkflowReadOnly)
	})

	t.Run("foreign workflow remains concealed", func(t *testing.T) {
		err := ctrl.CreateStepsFromTemplate(context.Background(), CreateStepsFromTemplateRequest{
			WorkflowID: "foreign-workflow",
			TemplateID: "simple",
		})
		require.True(t, errors.Is(err, service.ErrNotVisible))
	})
}
