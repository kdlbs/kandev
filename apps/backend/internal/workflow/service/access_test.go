package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/workflow/models"
)

// scopedCtx is a request context carrying a real (non-synthetic) identity.
func scopedCtx() context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-a", Role: authn.RoleMember})
}

// syntheticCtx is what the auth middleware injects while authentication is
// disabled.
func syntheticCtx() context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-a", Synthetic: true})
}

// denyAll stands in for a task service that answers "not yours" for everything.
func denyAll(context.Context, string) error { return repoerrors.ErrWorkspaceNotFound }

// callRecorder records the IDs a checker was asked about.
type callRecorder struct {
	ids []string
	err error
}

func (r *callRecorder) check(_ context.Context, id string) error {
	r.ids = append(r.ids, id)
	return r.err
}

func TestAuthorizeWorkflowAndWorkspaceFailClosedOnAnEmptyID(t *testing.T) {
	svc, _ := setupTestService(t)
	recorder := &callRecorder{}
	svc.SetWorkflowAccessChecker(recorder.check)
	svc.SetWorkspaceAccessChecker(recorder.check)
	svc.SetTaskAccessChecker(recorder.check)

	// An empty ID means the owner could not be resolved. The task service
	// reads workspaceID=="" as "no scoping applies" and would allow it, which
	// is why this never reaches the checker (see backendapp/auth.go's
	// runSubscriptionCheck for the same trap).
	require.ErrorIs(t, svc.AuthorizeWorkflow(scopedCtx(), ""), ErrNotVisible)
	require.ErrorIs(t, svc.AuthorizeWorkspace(scopedCtx(), ""), ErrNotVisible)
	require.ErrorIs(t, svc.AuthorizeTask(scopedCtx(), ""), ErrNotVisible)
	require.Empty(t, recorder.ids, "an unresolvable owner was passed to the task service")
}

func TestAuthorizeHelpersNoOpWhenAuthIsDisabled(t *testing.T) {
	svc, _ := setupTestService(t)
	recorder := &callRecorder{err: repoerrors.ErrWorkspaceNotFound}
	svc.SetWorkflowAccessChecker(recorder.check)
	svc.SetWorkspaceAccessChecker(recorder.check)
	svc.SetTaskAccessChecker(recorder.check)

	for name, ctx := range map[string]context.Context{
		"synthetic identity": syntheticCtx(),
		"no identity":        context.Background(),
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, svc.AuthorizeWorkflow(ctx, "wf-1"))
			require.NoError(t, svc.AuthorizeWorkspace(ctx, "ws-1"))
			require.NoError(t, svc.AuthorizeStep(ctx, "step-1"))
			require.NoError(t, svc.AuthorizeTask(ctx, "task-1"))
			require.NoError(t, svc.AuthorizeWorkflow(ctx, ""), "an empty ID must not fail closed for an unscoped caller")
		})
	}
	require.Empty(t, recorder.ids, "unscoped callers must not reach the task service at all")
}

func TestAuthorizeStepResolvesItsOwningWorkflow(t *testing.T) {
	t.Run("a step in a visible workflow is allowed", func(t *testing.T) {
		svc, _ := setupTestService(t)
		recorder := &callRecorder{}
		svc.SetWorkflowAccessChecker(recorder.check)
		step := &models.WorkflowStep{WorkflowID: "wf-1", Name: "Backlog", Color: "#111111"}
		require.NoError(t, svc.CreateStep(context.Background(), step))

		require.NoError(t, svc.AuthorizeStep(scopedCtx(), step.ID))
		require.Equal(t, []string{"wf-1"}, recorder.ids)
	})

	t.Run("a step in a foreign workflow is not found", func(t *testing.T) {
		svc, _ := setupTestService(t)
		svc.SetWorkflowAccessChecker(denyAll)
		step := &models.WorkflowStep{WorkflowID: "wf-1", Name: "Backlog", Color: "#111111"}
		require.NoError(t, svc.CreateStep(context.Background(), step))

		require.ErrorIs(t, svc.AuthorizeStep(scopedCtx(), step.ID), ErrNotVisible)
	})

	// A step row with no workflow has no resolvable owner. Failing open here
	// would hand every scoped caller a step nobody can be shown to own, so the
	// branch is pinned with a row inserted straight into the table (the model
	// validator rejects one through the service).
	t.Run("a step with no owning workflow fails closed", func(t *testing.T) {
		svc, db := setupTestService(t)
		svc.SetWorkflowAccessChecker(func(context.Context, string) error { return nil })
		_, err := db.Exec(
			`INSERT INTO workflow_steps (id, workflow_id, name, position, wip_limit, created_at, updated_at)
			 VALUES ('orphan-step', '', 'Orphan', 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
		require.NoError(t, err)

		require.ErrorIs(t, svc.AuthorizeStep(scopedCtx(), "orphan-step"), ErrNotVisible)
	})

	t.Run("a nonexistent step is the same error", func(t *testing.T) {
		svc, _ := setupTestService(t)
		svc.SetWorkflowAccessChecker(denyAll)
		require.ErrorIs(t, svc.AuthorizeStep(scopedCtx(), "no-such-step"), ErrNotVisible)
	})
}

// TestAuthorizeWorkflowNormalizesAMissingWorkflow pins the classification the
// no-existence-leak rule depends on. The task repository reports a missing
// workflow as a formatted error rather than a sentinel, so treating only
// sentinels as denials would answer 500 for "does not exist" and 404 for "not
// yours" — telling the two apart.
func TestAuthorizeWorkflowNormalizesAMissingWorkflow(t *testing.T) {
	svc, _ := setupTestService(t)
	svc.SetWorkflowAccessChecker(func(context.Context, string) error {
		return fmt.Errorf("workflow not found: %s", "wf-1")
	})
	require.ErrorIs(t, svc.AuthorizeWorkflow(scopedCtx(), "wf-1"), ErrNotVisible)
}

// TestAuthorizeWorkflowPropagatesRealFailures is the other side of that coin: a
// database failure must not be laundered into a 404, which would hide an
// outage behind an empty board.
func TestAuthorizeWorkflowPropagatesRealFailures(t *testing.T) {
	svc, _ := setupTestService(t)
	broken := errors.New("database is locked")
	svc.SetWorkflowAccessChecker(func(context.Context, string) error { return broken })

	err := svc.AuthorizeWorkflow(scopedCtx(), "wf-1")
	require.ErrorIs(t, err, broken)
	require.NotErrorIs(t, err, ErrNotVisible)
}

// TestServiceEntryPointsAuthorizeThemselves covers the methods reached without
// passing through the workflow controller: the boot-payload builder calls
// ListStepsByWorkspaceID directly, and the MCP config tools call
// ExportWorkflow/ImportWorkflows directly.
func TestServiceEntryPointsAuthorizeThemselves(t *testing.T) {
	newDeniedService := func(t *testing.T) *Service {
		t.Helper()
		svc, _ := setupTestService(t)
		svc.SetWorkflowAccessChecker(denyAll)
		svc.SetWorkspaceAccessChecker(denyAll)
		// A provider that would panic-free serve data if the guard let the call
		// through, so a missing guard shows up as a nil error, not a crash.
		svc.SetWorkflowProvider(&stubWorkflowProvider{})
		return svc
	}

	t.Run("list steps by workspace", func(t *testing.T) {
		svc := newDeniedService(t)
		_, err := svc.ListStepsByWorkspaceID(scopedCtx(), "ws-a")
		require.ErrorIs(t, err, ErrNotVisible)
	})

	t.Run("export one workflow", func(t *testing.T) {
		svc := newDeniedService(t)
		_, err := svc.ExportWorkflow(scopedCtx(), "wf-a")
		require.ErrorIs(t, err, ErrNotVisible)
	})

	t.Run("export a workspace", func(t *testing.T) {
		svc := newDeniedService(t)
		_, err := svc.ExportWorkflows(scopedCtx(), "ws-a", nil)
		require.ErrorIs(t, err, ErrNotVisible)
	})

	t.Run("import into a workspace", func(t *testing.T) {
		svc := newDeniedService(t)
		_, err := svc.ImportWorkflows(scopedCtx(), "ws-a", &models.WorkflowExport{
			Version:   models.ExportVersion,
			Type:      models.ExportType,
			Workflows: []models.WorkflowPortable{{Name: "Smuggled"}},
		})
		require.ErrorIs(t, err, ErrNotVisible)
		require.Empty(t, svc.workflowProvider.(*stubWorkflowProvider).created)
	})

	// ReorderSteps is the layer that writes the positions, so it authorizes
	// the workflow itself rather than trusting a caller to have done it. The
	// membership check below cannot stand in for this one: steps that really
	// do belong to a workflow the caller cannot see would otherwise reorder.
	t.Run("reorder authorizes the workflow it is asked to reorder", func(t *testing.T) {
		svc, _ := setupTestService(t)
		step := &models.WorkflowStep{WorkflowID: "wf-a", Name: "Backlog", Color: "#111111", Position: 7}
		require.NoError(t, svc.CreateStep(context.Background(), step))
		svc.SetWorkflowAccessChecker(denyAll)

		require.ErrorIs(t, svc.ReorderSteps(scopedCtx(), "wf-a", []string{step.ID}), ErrNotVisible)
		stored, err := svc.GetStep(context.Background(), step.ID)
		require.NoError(t, err)
		require.Equal(t, 7, stored.Position, "a denied reorder wrote anyway")
	})

	t.Run("reorder rejects a step from another workflow", func(t *testing.T) {
		svc, _ := setupTestService(t)
		// Position 7, so the reorder this test denies would visibly move the
		// step to 0 if the guard were missing. wf-b is authorized: the step
		// belonging to wf-a is the only reason to refuse.
		step := &models.WorkflowStep{WorkflowID: "wf-a", Name: "Backlog", Color: "#111111", Position: 7}
		require.NoError(t, svc.CreateStep(context.Background(), step))
		svc.SetWorkflowAccessChecker(func(context.Context, string) error { return nil })

		require.ErrorIs(t, svc.ReorderSteps(scopedCtx(), "wf-b", []string{step.ID}), ErrNotVisible)

		stored, err := svc.GetStep(context.Background(), step.ID)
		require.NoError(t, err)
		require.Equal(t, 7, stored.Position, "a denied reorder wrote anyway")
	})
}

// stubWorkflowProvider is a permissive task-domain stand-in: every method
// succeeds, so any assertion that fails is the guard's absence rather than a
// missing fixture.
type stubWorkflowProvider struct {
	created []string
}

func (p *stubWorkflowProvider) ListWorkflows(context.Context, string, bool) ([]*taskmodels.Workflow, error) {
	return nil, nil
}

func (p *stubWorkflowProvider) GetWorkflow(_ context.Context, id string) (*taskmodels.Workflow, error) {
	return &taskmodels.Workflow{ID: id, WorkspaceID: "ws-a", Name: "A Flow"}, nil
}

func (p *stubWorkflowProvider) CreateWorkflow(
	_ context.Context, workspaceID, name, description string,
) (*taskmodels.Workflow, error) {
	p.created = append(p.created, name)
	return &taskmodels.Workflow{ID: "created-" + name, WorkspaceID: workspaceID, Name: name, Description: description}, nil
}

func (p *stubWorkflowProvider) UpdateWorkflow(context.Context, *taskmodels.Workflow) error {
	return nil
}
