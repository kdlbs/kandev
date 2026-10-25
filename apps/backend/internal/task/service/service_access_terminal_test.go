package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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
	// Deliberately a torn-down environment: stopped, with a materialization
	// session that no longer exists. Authorization reads task and workspace
	// rows, never the in-memory execution, so a dead agent must not turn into
	// a denial for the owner (the SSR panel renders a denial as empty).
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-b", TaskID: "task-b", ExecutorType: "worktree",
		WorkspacePath: "/tmp/b", Status: models.TaskEnvironmentStatusStopped,
		MaterializationSessionID: "sess-deleted",
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

// TestAuthorizeTaskEnvironmentAccessPairsBinding covers the pairing the two
// independent checks miss: authorizing a task and authorizing an environment
// separately does not establish that they belong together, so a caller could
// combine the ordinary terminals of one of their tasks with the unmanaged
// shells of an unrelated environment.
//
// The relationship checked is deliberately NOT `env.TaskID == taskID`. Two
// shipping workspace modes bind a task's session to an environment row owned
// by a different task: inherit_parent gives a subtask the parent's environment
// (orchestrator.resolveInheritedEnvironment) and shared_group gives every
// member of the group one canonical environment
// (orchestrator.inheritFromSharedGroup). The frontend sends the session's
// environment alongside the task ID, so an owner-of-the-row check would 404
// the terminal panel for both. What matters is whether a session of this task
// actually points at this environment.
func TestAuthorizeTaskEnvironmentAccessPairsBinding(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)

	// task-b owns env-b; sess-b is bound to it.
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-b", TaskID: "task-b", ExecutorType: "worktree",
		WorkspacePath: "/tmp/b", Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("create env-b: %v", err)
	}

	// task-c is a second task of the SAME owner, with its own environment.
	// Nothing links it to task-b: this is the pair that must be refused.
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-c", WorkspaceID: "ws-b", WorkflowID: "wf-b", WorkflowStepID: "step-1",
		Title: "B's other task", State: v1.TaskStateCreated, Priority: "medium",
	}); err != nil {
		t.Fatalf("create task-c: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-c", TaskID: "task-c", ExecutorType: "worktree",
		WorkspacePath: "/tmp/c", Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("create env-c: %v", err)
	}

	// task-d owns no environment; its session is bound to task-b's, the shape
	// inherit_parent and shared_group produce.
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-d", WorkspaceID: "ws-b", WorkflowID: "wf-b", WorkflowStepID: "step-1",
		Title: "B's subtask", State: v1.TaskStateCreated, Priority: "medium",
	}); err != nil {
		t.Fatalf("create task-d: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-d", TaskID: "task-d", State: models.TaskSessionStateCreated,
		TaskEnvironmentID: "env-b",
	}); err != nil {
		t.Fatalf("create sess-d: %v", err)
	}

	owner := ctxAs("user-b")
	if err := svc.AuthorizeTaskEnvironmentAccess(owner, "task-b", "env-b"); err != nil {
		t.Errorf("task with its own environment = %v, want nil", err)
	}
	if err := svc.AuthorizeTaskEnvironmentAccess(owner, "task-d", "env-b"); err != nil {
		t.Errorf("inherited environment = %v, want nil", err)
	}
	if err := svc.AuthorizeTaskEnvironmentAccess(owner, "task-b", "env-c"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Errorf("unrelated environment of the same owner = %v, want ErrTaskNotFound", err)
	}
	if err := svc.AuthorizeTaskEnvironmentAccess(owner, "task-d", "env-c"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Errorf("unrelated environment for a subtask = %v, want ErrTaskNotFound", err)
	}

	// A foreign caller is refused on the pair as well, with the same sentinel.
	if err := svc.AuthorizeTaskEnvironmentAccess(ctxAs("user-a"), "task-b", "env-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Errorf("foreign pair = %v, want ErrTaskNotFound", err)
	}

	// Auth disabled: the pairing is an authorization check, so it must not
	// start rejecting combinations the single-user install served before.
	for name, callerCtx := range map[string]context.Context{
		"internal":  context.Background(),
		"synthetic": ctxSynthetic(),
	} {
		t.Run(name, func(t *testing.T) {
			if err := svc.AuthorizeTaskEnvironmentAccess(callerCtx, "task-b", "env-c"); err != nil {
				t.Errorf("unscoped pair = %v, want nil", err)
			}
		})
	}
}
