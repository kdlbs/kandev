package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// The SSR terminal-list routes (GET /api/v1/environments/:id/terminals and
// GET /api/v1/tasks/:id/terminals) call these two authorizers through the
// lifecycle manager's CheckTaskAccess / CheckEnvironmentAccess. Their guard
// only holds if the authorizers themselves deny a foreign scope and, just as
// importantly, no-op when auth is disabled: the route must behave exactly as
// it did pre-auth for the single-user install.
func TestTerminalRouteAuthorizersScopeByOwner(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-b", TaskID: "task-b", ExecutorType: "worktree",
		WorkspacePath: "/tmp/b", Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("create task environment: %v", err)
	}

	// Foreign caller: denied, with the sentinel that reads as "no such thing".
	if err := svc.AuthorizeTaskAccess(ctxAs("user-a"), "task-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Errorf("foreign task access = %v, want ErrTaskNotFound", err)
	}
	if err := svc.AuthorizeEnvironmentAccess(ctxAs("user-a"), "env-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Errorf("foreign environment access = %v, want ErrTaskNotFound", err)
	}

	// Owner: allowed.
	if err := svc.AuthorizeTaskAccess(ctxAs("user-b"), "task-b"); err != nil {
		t.Errorf("owner task access = %v, want nil", err)
	}
	if err := svc.AuthorizeEnvironmentAccess(ctxAs("user-b"), "env-b"); err != nil {
		t.Errorf("owner environment access = %v, want nil", err)
	}

	// Auth disabled (synthetic identity) and internal callers stay unscoped,
	// so the guard is invisible on a single-user install.
	for name, callerCtx := range map[string]context.Context{
		"internal":  context.Background(),
		"synthetic": ctxSynthetic(),
	} {
		t.Run(name, func(t *testing.T) {
			if err := svc.AuthorizeTaskAccess(callerCtx, "task-b"); err != nil {
				t.Errorf("task access = %v, want nil", err)
			}
			if err := svc.AuthorizeEnvironmentAccess(callerCtx, "env-b"); err != nil {
				t.Errorf("environment access = %v, want nil", err)
			}
		})
	}
}
