package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

func scopedCtx() context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-a", Role: authn.RoleMember})
}

type panicWorkflowProvider struct{}

func (panicWorkflowProvider) ListWorkflows(context.Context, string, bool) ([]*taskmodels.Workflow, error) {
	panic("unexpected ListWorkflows call")
}

func (panicWorkflowProvider) GetWorkflow(context.Context, string) (*taskmodels.Workflow, error) {
	panic("unexpected GetWorkflow call")
}

func (panicWorkflowProvider) CreateWorkflow(context.Context, string, string, string) (*taskmodels.Workflow, error) {
	panic("unexpected CreateWorkflow call")
}

func (panicWorkflowProvider) UpdateWorkflow(context.Context, *taskmodels.Workflow) error {
	panic("unexpected UpdateWorkflow call")
}

func newTestService(t *testing.T) *workflowservice.Service {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	svc := workflowservice.NewService(nil, log)
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func TestGetCoordinatorMonitoring_AuthorizesWorkflow(t *testing.T) {
	svc := newTestService(t)
	authorized := 0
	svc.SetWorkflowAccessChecker(func(context.Context, string) error {
		authorized++
		return workflowservice.ErrNotVisible
	})
	controller := NewController(svc)

	_, err := controller.GetCoordinatorMonitoring(scopedCtx(), "wf-1")
	require.ErrorIs(t, err, workflowservice.ErrNotVisible)
	require.Equal(t, 1, authorized)
}

func TestSetCoordinatorMonitoring_AuthorizesWorkflowBeforeMutabilityCheck(t *testing.T) {
	svc := newTestService(t)
	authorized := 0
	svc.SetWorkflowAccessChecker(func(context.Context, string) error {
		authorized++
		return workflowservice.ErrNotVisible
	})
	svc.SetWorkflowProvider(panicWorkflowProvider{})
	controller := NewController(svc)

	_, err := controller.SetCoordinatorMonitoring(scopedCtx(), SetCoordinatorMonitoringRequest{WorkflowID: "wf-1"})
	require.ErrorIs(t, err, workflowservice.ErrNotVisible)
	require.Equal(t, 1, authorized)
}
