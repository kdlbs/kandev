package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

// @covers AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.4
func TestAutomationCredentialRunCreatorFallbackProtectsUnboundTask(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	seedTeamWorkspace(t, repo, true)
	task, err := repo.GetTask(ctx, "task-team")
	require.NoError(t, err)
	task.Metadata = map[string]interface{}{models.MetaKeyAutomationRunID: "run-before-bind"}
	require.NoError(t, repo.UpdateTask(ctx, task))
	svc.SetAutomationCredentialRunAccessChecker(credentialRunAccessFixture{}.CheckAutomationCredentialRunAccess)
	svc.SetAutomationCredentialRunCreatorLookup(func(_ context.Context, runID string) (string, error) {
		require.Equal(t, "run-before-bind", runID)
		return "user-bruno", nil
	})

	protected, err := svc.IsProtectedAutomationTask(ctx, task.ID)
	require.NoError(t, err)
	require.True(t, protected, "task metadata links protection before automation_runs.task_id is bound")
	require.ErrorIs(t, svc.AuthorizeTaskAccess(ctxAsRole("user-ana", "member"), task.ID), repoerrors.ErrTaskNotFound)
	require.NoError(t, svc.AuthorizeTaskAccess(ctxAsRole("user-bruno", "member"), task.ID))
}
