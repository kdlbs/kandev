package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func seedOwnedWorkspaceForCreate(t *testing.T, ctx context.Context, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
}, wsID, ownerID string) string {
	t.Helper()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: wsID, OwnerID: ownerID}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	wfID := wsID + "-wf"
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: wfID, WorkspaceID: wsID, Name: "WF"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	return wfID
}

// TestCreateTaskWithExternalIDDeniesUnauthorizedWorkspace covers the
// permissions scenario: a caller who does not own the workspace gets the
// same not-found error an unauthorized create always gets, revealing
// nothing about the task the external_id resolves to — settledness (or even
// existence) must not be observable across an authorization boundary.
func TestCreateTaskWithExternalIDDeniesUnauthorizedWorkspace(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedOwnedWorkspaceForCreate(t, ctx, repo, "ws-owned-a", "user-a")

	settled, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-owned-a", WorkflowID: wfID, Title: "Settled", ExternalID: "ext-1",
	})
	if err != nil {
		t.Fatalf("seed settled task: %v", err)
	}
	mustSettle(t, ctx, repo, settled.Task.ID, "ext-1")

	unsettledResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-owned-a", WorkflowID: wfID, Title: "Unsettled", ExternalID: "ext-2",
	})
	if err != nil {
		t.Fatalf("seed unsettled task: %v", err)
	}
	if unsettledResult.Task.ExternalIDSettledAt != nil {
		t.Fatal("expected the second seeded task to remain unsettled")
	}

	// User B does not own ws-owned-a: both the settled and unsettled retries
	// must fail identically, with no distinguishing signal.
	if _, err := svc.CreateTask(ctxAs("user-b"), &CreateTaskRequest{
		WorkspaceID: "ws-owned-a", WorkflowID: wfID, Title: "Attempt", ExternalID: "ext-1",
	}); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("err = %v, want ErrWorkspaceNotFound (settled target)", err)
	}
	if _, err := svc.CreateTask(ctxAs("user-b"), &CreateTaskRequest{
		WorkspaceID: "ws-owned-a", WorkflowID: wfID, Title: "Attempt", ExternalID: "ext-2",
	}); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("err = %v, want ErrWorkspaceNotFound (unsettled target)", err)
	}
}

// TestCreateTaskWithExternalIDAuthorizationPrecedesValidation covers the
// ordering requirement: an unauthorized caller gets the not-found error even
// when the payload also fails external_id validation (a 300-byte value) —
// authorization must run first, so the caller never learns their payload was
// invalid for a workspace they cannot see.
func TestCreateTaskWithExternalIDAuthorizationPrecedesValidation(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedOwnedWorkspaceForCreate(t, ctx, repo, "ws-owned-a2", "user-a")

	oversized := make([]byte, 300)
	for i := range oversized {
		oversized[i] = 'x'
	}

	_, err := svc.CreateTask(ctxAs("user-b"), &CreateTaskRequest{
		WorkspaceID: "ws-owned-a2", WorkflowID: wfID, Title: "Attempt", ExternalID: string(oversized),
	})
	if !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("err = %v, want ErrWorkspaceNotFound — authorization must precede validation", err)
	}
	if errors.Is(err, ErrExternalIDInvalid) {
		t.Fatal("must not surface the validation error to an unauthorized caller")
	}
}

// TestLookupAndReleaseExternalIDDenyUnauthorizedWorkspace covers the
// permissions scenario for the two new by-external-id routes directly: a
// caller who does not own the workspace gets ErrWorkspaceNotFound from both
// GetTaskByExternalID (the REST lookup route) and ReleaseTaskExternalID (the
// REST release route) — neither leaks whether the identity exists.
func TestLookupAndReleaseExternalIDDenyUnauthorizedWorkspace(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wfID := seedOwnedWorkspaceForCreate(t, ctx, repo, "ws-owned-a3", "user-a")

	settled, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-owned-a3", WorkflowID: wfID, Title: "Settled", ExternalID: "ext-1",
	})
	if err != nil {
		t.Fatalf("seed settled task: %v", err)
	}
	mustSettle(t, ctx, repo, settled.Task.ID, "ext-1")

	if _, err := svc.GetTaskByExternalID(ctxAs("user-b"), "ws-owned-a3", "ext-1"); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("GetTaskByExternalID err = %v, want ErrWorkspaceNotFound", err)
	}
	if _, err := svc.ReleaseTaskExternalID(ctxAs("user-b"), "ws-owned-a3", "ext-1"); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("ReleaseTaskExternalID err = %v, want ErrWorkspaceNotFound", err)
	}

	// The identity must survive the denied release attempt untouched.
	survivor, err := svc.GetTaskByExternalID(ctx, "ws-owned-a3", "ext-1")
	if err != nil {
		t.Fatalf("owner lookup after denied release: %v", err)
	}
	if survivor.ID != settled.Task.ID {
		t.Fatalf("survivor id = %s, want %s", survivor.ID, settled.Task.ID)
	}
}
