package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Session-keyed reads and mutations must apply the same per-user scoping as the
// task surface. They did not: GetTaskSession, ListTaskSessions and
// GetPrimarySession returned rows for any ID, and DismissLastAgentError /
// ApproveSession mutated any session — so knowing another user's session or
// task ID was enough. The session DTO these feed carries Metadata, the host
// worktree path, and branch names, and ApproveSession advances a workflow step.

type sessionScopeRepo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
	CreateTaskSession(context.Context, *models.TaskSession) error
}

// seedSessionScopeFixture gives every owner a task with a primary session so
// both halves of the matrix (denied for foreign, allowed for own) are covered,
// plus a pre-auth unowned workspace that must stay visible to everyone.
func seedSessionScopeFixture(t *testing.T, repo sessionScopeRepo) {
	t.Helper()
	ctx := context.Background()
	owners := []struct{ suffix, owner string }{
		{"a", "user-a"},
		{"b", "user-b"},
		{"legacy", ""},
	}
	for _, o := range owners {
		if err := repo.CreateWorkspace(ctx, &models.Workspace{
			ID: "ws-" + o.suffix, Name: "ws " + o.suffix, OwnerID: o.owner,
		}); err != nil {
			t.Fatalf("create workspace %s: %v", o.suffix, err)
		}
		if err := repo.CreateWorkflow(ctx, &models.Workflow{
			ID: "wf-" + o.suffix, WorkspaceID: "ws-" + o.suffix, Name: "flow " + o.suffix,
		}); err != nil {
			t.Fatalf("create workflow %s: %v", o.suffix, err)
		}
		if err := repo.CreateTask(ctx, &models.Task{
			ID: "task-" + o.suffix, WorkspaceID: "ws-" + o.suffix, WorkflowID: "wf-" + o.suffix,
			WorkflowStepID: "step-1", Title: "task " + o.suffix,
			State: v1.TaskStateCreated, Priority: "medium",
		}); err != nil {
			t.Fatalf("create task %s: %v", o.suffix, err)
		}
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: "sess-" + o.suffix, TaskID: "task-" + o.suffix,
			IsPrimary: true, State: models.TaskSessionStateCreated,
		}); err != nil {
			t.Fatalf("create session %s: %v", o.suffix, err)
		}
	}
}

func TestSessionScopingGetTaskSession(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSessionScopeFixture(t, repo)

	if _, err := svc.GetTaskSession(ctxAs("user-a"), "sess-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("get foreign session = %v, want ErrTaskNotFound", err)
	}
	if _, err := svc.GetTaskSession(ctxAs("user-a"), "sess-a"); err != nil {
		t.Fatalf("owner must still read own session: %v", err)
	}
	if _, err := svc.GetTaskSession(ctxAs("user-a"), "sess-legacy"); err != nil {
		t.Fatalf("pre-auth unowned session must stay visible: %v", err)
	}
}

func TestSessionScopingListTaskSessions(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSessionScopeFixture(t, repo)

	if _, err := svc.ListTaskSessions(ctxAs("user-a"), "task-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("list foreign task's sessions = %v, want ErrTaskNotFound", err)
	}
	own, err := svc.ListTaskSessions(ctxAs("user-a"), "task-a")
	if err != nil {
		t.Fatalf("owner must still list own sessions: %v", err)
	}
	if len(own) != 1 || own[0].ID != "sess-a" {
		t.Fatalf("owner list = %+v, want [sess-a]", own)
	}
}

func TestSessionScopingGetPrimarySession(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSessionScopeFixture(t, repo)

	if _, err := svc.GetPrimarySession(ctxAs("user-a"), "task-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("get foreign primary session = %v, want ErrTaskNotFound", err)
	}
	own, err := svc.GetPrimarySession(ctxAs("user-a"), "task-a")
	if err != nil {
		t.Fatalf("owner must still read own primary session: %v", err)
	}
	if own.ID != "sess-a" {
		t.Fatalf("owner primary = %s, want sess-a", own.ID)
	}
}

// TestSessionScopingDismissLastAgentError covers a mutation: dismissing the
// banner on someone else's session.
func TestSessionScopingDismissLastAgentError(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSessionScopeFixture(t, repo)

	if _, err := svc.DismissLastAgentError(ctxAs("user-a"), "sess-b", ""); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("dismiss on foreign session = %v, want ErrTaskNotFound", err)
	}
	if _, err := svc.DismissLastAgentError(ctxAs("user-a"), "sess-a", ""); err != nil {
		t.Fatalf("owner must still dismiss on own session: %v", err)
	}
}

// TestSessionScopingApproveSession covers the highest-impact mutation: approving
// a workflow review step on another user's session. The owner path is asserted
// only as "not denied" — a full approval needs workflow-step fixtures this test
// deliberately does not build, so any other error is fine here.
func TestSessionScopingApproveSession(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSessionScopeFixture(t, repo)

	if _, err := svc.ApproveSession(ctxAs("user-a"), "sess-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("approve foreign session = %v, want ErrTaskNotFound", err)
	}
	if _, err := svc.ApproveSession(ctxAs("user-a"), "sess-a"); errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatal("owner must not be denied access to their own session")
	}
}

// TestSessionScopingGetMessage covers the by-ID message read behind the
// shell-output route. ListMessages was already scoped; GetMessage was not, so a
// caller holding someone else's (session_id, message_id) pair could read their
// command output.
func TestSessionScopingGetMessage(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSessionScopeFixture(t, repo)
	ctx := context.Background()
	foreign, err := svc.CreateMessage(ctx, &CreateMessageRequest{
		TaskSessionID: "sess-b", TaskID: "task-b",
		AuthorType: "agent", Content: "secret shell output",
	})
	if err != nil {
		t.Fatalf("seed message: %v", err)
	}
	own, err := svc.CreateMessage(ctx, &CreateMessageRequest{
		TaskSessionID: "sess-a", TaskID: "task-a",
		AuthorType: "agent", Content: "my shell output",
	})
	if err != nil {
		t.Fatalf("seed own message: %v", err)
	}

	if _, err := svc.GetMessage(ctxAs("user-a"), foreign.ID); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("get foreign message = %v, want ErrTaskNotFound", err)
	}
	if _, err := svc.GetMessage(ctxAs("user-a"), own.ID); err != nil {
		t.Fatalf("owner must still read own message: %v", err)
	}
	if _, err := svc.GetMessage(context.Background(), foreign.ID); err != nil {
		t.Fatalf("internal caller must stay unscoped: %v", err)
	}
}

// TestSessionScopingInternalAndSyntheticCallersUnscoped pins the compatibility
// contract: identity-less internal callers (pollers, event bus, office
// schedulers) and the synthetic identity used while auth is disabled keep
// seeing everything.
func TestSessionScopingInternalAndSyntheticCallersUnscoped(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSessionScopeFixture(t, repo)

	for name, ctx := range map[string]context.Context{
		"internal":  context.Background(),
		"synthetic": ctxSynthetic(),
	} {
		if _, err := svc.GetTaskSession(ctx, "sess-b"); err != nil {
			t.Errorf("%s caller GetTaskSession: %v", name, err)
		}
		if _, err := svc.ListTaskSessions(ctx, "task-b"); err != nil {
			t.Errorf("%s caller ListTaskSessions: %v", name, err)
		}
		if _, err := svc.GetPrimarySession(ctx, "task-b"); err != nil {
			t.Errorf("%s caller GetPrimarySession: %v", name, err)
		}
		if _, err := svc.DismissLastAgentError(ctx, "sess-b", ""); err != nil {
			t.Errorf("%s caller DismissLastAgentError: %v", name, err)
		}
	}
}

// TestSessionScopingAttachWorkspaceSources covers POST
// /tasks/:id/workspace-sources and the add_workspace_sources_kandev MCP tool.
// The route arrived on main while this branch was in review with no ownership
// check, so a caller could attach repositories and folders to another user's
// task and have them materialized into that task's workspace.
func TestSessionScopingAttachWorkspaceSources(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedSessionScopeFixture(t, repo)

	_, err := svc.AttachWorkspaceSources(ctxAs("user-a"), AttachWorkspaceSourcesRequest{
		TaskID:  "task-b",
		Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceFolder, LocalPath: t.TempDir()}},
	})
	if !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("attach to foreign task = %v, want ErrTaskNotFound", err)
	}

	// The owner is not denied: it gets past the guard and fails (or succeeds)
	// on the source-materialization logic this test does not set up.
	_, ownErr := svc.AttachWorkspaceSources(ctxAs("user-a"), AttachWorkspaceSourcesRequest{
		TaskID:  "task-a",
		Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceFolder, LocalPath: t.TempDir()}},
	})
	if errors.Is(ownErr, repoerrors.ErrTaskNotFound) {
		t.Error("owner was denied access to their own task")
	}
}
