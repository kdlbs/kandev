package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateStepsFromTemplateKeepsExecutorAuthorization(t *testing.T) {
	svc, db := setupTestService(t)
	insertWorkflow(t, db, "wf-template-executor", "Managed workflow")
	wantErr := errors.New("executor is not eligible")
	svc.SetWorkflowMutationReadOnlyChecker(func(context.Context, string) (bool, bool, error) {
		return false, true, nil
	})
	svc.SetExecutorMutationChecker(func(context.Context, string) error {
		return wantErr
	})

	err := svc.CreateStepsFromTemplate(context.Background(), "wf-template-executor", "simple")
	require.ErrorIs(t, err, wantErr)
	steps, err := svc.repo.ListStepsByWorkflow(context.Background(), "wf-template-executor")
	require.NoError(t, err)
	require.Empty(t, steps)
}

func TestCreateStepsFromTemplateRequiresWorkflowAccess(t *testing.T) {
	svc, db := setupTestService(t)
	insertWorkflow(t, db, "wf-template-access", "Private workflow")
	svc.SetWorkflowAuthorizationRequiredChecker(func(context.Context) bool { return true })
	svc.SetWorkflowAccessChecker(func(context.Context, string) error { return ErrNotVisible })

	err := svc.CreateStepsFromTemplate(context.Background(), "wf-template-access", "simple")
	require.ErrorIs(t, err, ErrNotVisible)
}

func TestCreateStepsFromTemplateAllowsInternalReadOnlyWorkspaceInitialization(t *testing.T) {
	svc, db := setupTestService(t)
	insertWorkflow(t, db, "wf-template-read-only", "Managed workflow")
	svc.SetWorkflowMutationReadOnlyChecker(func(context.Context, string) (bool, bool, error) {
		return false, true, nil
	})

	require.NoError(t, svc.CreateStepsFromTemplate(context.Background(), "wf-template-read-only", "simple"))
	steps, err := svc.repo.ListStepsByWorkflow(context.Background(), "wf-template-read-only")
	require.NoError(t, err)
	require.Len(t, steps, 4)
}
