package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestProtectedAutomationCredentialAuthorityOnMissingWorkspaceFallback(t *testing.T) {
	svc, _, repo := createTestService(t)
	const taskID = "task-protected-orphan"
	require.NoError(t, repo.CreateTask(context.Background(), &models.Task{
		ID: taskID, WorkspaceID: "workspace-missing", Title: "Protected",
		State: v1.TaskStateCreated, Priority: "medium",
	}))

	const creatorID = "automation-creator"
	useAllowed := false
	runChecks, useChecks := 0, 0
	svc.SetAutomationCredentialRunAccessChecker(func(_ context.Context, gotTaskID, userID string) (bool, bool, error) {
		runChecks++
		require.Equal(t, taskID, gotTaskID)
		return true, userID == creatorID, nil
	})
	svc.SetAutomationCredentialRunUseChecker(func(_ context.Context, gotTaskID, userID string) (bool, bool, error) {
		useChecks++
		require.Equal(t, taskID, gotTaskID)
		return true, userID == creatorID && useAllowed, nil
	})

	creator := ctxAsRole(creatorID, authn.RoleMember)
	otherMember := ctxAsRole("other-member", authn.RoleMember)
	read := func(ctx context.Context) error {
		return svc.AuthorizeTaskScope(ctx, taskID, authz.ScopeWorkspaceRead)
	}
	prompt := func() error {
		return svc.AuthorizeTaskScope(creator, taskID, authz.ScopeSessionPrompt)
	}
	exec := func() error {
		return svc.AuthorizeTaskScope(creator, taskID, authz.ScopeSessionExec)
	}

	require.ErrorIs(t, read(otherMember), repoerrors.ErrTaskNotFound)
	require.Equal(t, 1, runChecks, "creator authorization must run before the fallback allows task access")
	require.Equal(t, 0, useChecks, "read scope must not revalidate credential use")
	require.NoError(t, read(creator), "the protected creator can read safe task state")
	require.Equal(t, 2, runChecks)
	require.Equal(t, 0, useChecks)

	require.ErrorIs(t, prompt(), repoerrors.ErrTaskNotFound, "prompt admission must reject revoked authority")
	require.ErrorIs(t, exec(), repoerrors.ErrTaskNotFound, "exec admission must reject revoked authority")
	require.Equal(t, 4, runChecks)
	require.Equal(t, 2, useChecks)

	useAllowed = true
	require.NoError(t, prompt(), "prompt admission resumes when current authority is restored")
	require.NoError(t, exec(), "exec admission resumes when current authority is restored")
	require.Equal(t, 6, runChecks)
	require.Equal(t, 4, useChecks)
}
