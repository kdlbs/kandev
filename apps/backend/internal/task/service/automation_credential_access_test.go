package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

type credentialRunAccessFixture struct {
	creator string
}

type credentialRunUseFixture struct {
	creator string
	allowed bool
}

func (f credentialRunAccessFixture) CheckAutomationCredentialRunAccess(
	_ context.Context,
	_ string,
	userID string,
) (bool, bool, error) {
	if f.creator == "" {
		return false, false, nil
	}
	return true, f.creator == userID, nil
}

func (f *credentialRunUseFixture) CheckAutomationCredentialRunUseAccess(
	_ context.Context,
	_ string,
	userID string,
) (bool, bool, error) {
	if f.creator == "" {
		return false, false, nil
	}
	return true, f.allowed && f.creator == userID, nil
}

func TestAutomationCredentialRunAccess(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	seedTeamWorkspace(t, repo, true)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "task-team-session", TaskID: "task-team", State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "task-team-environment", TaskID: "task-team", ExecutorType: "worktree",
		WorkspacePath: t.TempDir(), Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("create environment: %v", err)
	}
	protected := credentialRunAccessFixture{creator: "user-bruno"}
	svc.SetAutomationCredentialRunAccessChecker(protected.CheckAutomationCredentialRunAccess)

	owner := ctxAsRole("user-ana", "member")
	creator := ctxAsRole("user-bruno", "member")
	for name, authorize := range map[string]func(context.Context) error{
		"task":        func(ctx context.Context) error { return svc.AuthorizeTaskAccess(ctx, "task-team") },
		"session":     func(ctx context.Context) error { return svc.AuthorizeSessionAccess(ctx, "task-team-session") },
		"environment": func(ctx context.Context) error { return svc.AuthorizeEnvironmentAccess(ctx, "task-team-environment") },
		"prompt":      func(ctx context.Context) error { return svc.AuthorizeTaskPromptScope(ctx, "task-team") },
	} {
		t.Run(name, func(t *testing.T) {
			if err := authorize(owner); !errors.Is(err, repoerrors.ErrTaskNotFound) {
				t.Fatalf("workspace owner access = %v, want task not found", err)
			}
			if err := authorize(creator); err != nil {
				t.Fatalf("creator access = %v, want success", err)
			}
		})
	}
	if err := svc.AuthorizeTaskEnvironmentAccess(owner, "task-team", "task-team-environment"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("owner task/environment access = %v, want task not found", err)
	}
	if err := svc.AuthorizeTaskEnvironmentAccess(creator, "task-team", "task-team-environment"); err != nil {
		t.Fatalf("creator task/environment access = %v, want success", err)
	}

	unprotected := credentialRunAccessFixture{}
	svc.SetAutomationCredentialRunAccessChecker(unprotected.CheckAutomationCredentialRunAccess)
	if err := svc.AuthorizeTaskAccess(owner, "task-team"); err != nil {
		t.Fatalf("ordinary task access = %v, want existing workspace access", err)
	}
}

func TestAutomationCredentialRevocationClosesLivePromptAndRuntimeAccess(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	seedTeamWorkspace(t, repo, true)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "task-team-session", TaskID: "task-team", State: models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "task-team-environment", TaskID: "task-team", ExecutorType: "worktree",
		WorkspacePath: t.TempDir(), Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("create environment: %v", err)
	}
	runAccess := credentialRunAccessFixture{creator: "user-bruno"}
	useAccess := &credentialRunUseFixture{creator: "user-bruno", allowed: true}
	svc.SetAutomationCredentialRunAccessChecker(runAccess.CheckAutomationCredentialRunAccess)
	svc.SetAutomationCredentialRunUseChecker(useAccess.CheckAutomationCredentialRunUseAccess)
	creator := ctxAsRole("user-bruno", "member")
	require.NoError(t, svc.AuthorizeTaskPromptScope(creator, "task-team"))
	require.NoError(t, svc.AuthorizeEnvironmentAccess(creator, "task-team-environment"))

	useAccess.allowed = false

	require.ErrorIs(t, svc.AuthorizeTaskPromptScope(creator, "task-team"), repoerrors.ErrTaskNotFound)
	require.ErrorIs(t, svc.AuthorizeSessionScope(creator, "task-team-session", authz.ScopeSessionExec), repoerrors.ErrTaskNotFound)
	require.ErrorIs(t, svc.AuthorizeEnvironmentAccess(creator, "task-team-environment"), repoerrors.ErrTaskNotFound)
	// Reading the task's safe status remains available to the fixed creator.
	require.NoError(t, svc.AuthorizeTaskAccess(creator, "task-team"))
}
