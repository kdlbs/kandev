package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// TestResolveResumeTaskEnvironment_ReusesInheritedEnvironment is a regression
// test for "UNIQUE constraint failed: task_environments.id" on resume. A child
// task created by an office task-handoff (inherit_parent / shared_group) has its
// session.TaskEnvironmentID rewritten to point at the parent's / group's env
// row, which is owned by a *different* task. GetTaskEnvironmentByTaskID(childID)
// therefore returns nil, and without the inherited-env fallback the resume path
// treated the env as absent and re-created a row using the inherited ID, which
// already exists. The resolver must instead return the inherited row so the
// persist path takes its update branch.
func TestResolveResumeTaskEnvironment_ReusesInheritedEnvironment(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	repo.taskEnvironments["env-parent"] = &models.TaskEnvironment{
		ID:           "env-parent",
		TaskID:       "task-parent",
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}
	repo.tasks["task-parent"] = &models.Task{ID: "task-parent"}
	session := &models.TaskSession{
		ID:                "sess-child",
		TaskID:            "task-child",
		TaskEnvironmentID: "env-parent",
	}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), "task-child", session)
	if err != nil {
		t.Fatalf("resolveResumeTaskEnvironment: %v", err)
	}
	if env == nil || env.ID != "env-parent" {
		t.Fatalf("resolved env = %+v, want inherited env-parent", env)
	}
	// Provenance: the returned row must be the parent's, not a freshly created
	// child-owned row that happened to reuse the inherited ID.
	if env.TaskID != "task-parent" {
		t.Fatalf("env.TaskID = %q, want task-parent (should be the inherited parent row)", env.TaskID)
	}
	if len(repo.createTaskEnvironmentCalls) != 0 {
		t.Fatalf("expected no CreateTaskEnvironment calls, got %d", len(repo.createTaskEnvironmentCalls))
	}
	if session.TaskEnvironmentID != "env-parent" {
		t.Fatalf("session.TaskEnvironmentID = %q, want env-parent", session.TaskEnvironmentID)
	}
}

// TestResolveResumeTaskEnvironment_MissingReferenceFallsThroughToCreate covers a
// session that references an env ID with no matching row (deleted env, or an env
// that was never persisted). The SQLite repo signals this with the production
// ErrTaskEnvironmentNotFound sentinel (not a nil,nil miss), so the resolver must
// treat that sentinel as absent, return nil, and let the caller create a fresh
// row using the free ID — preserving resume rather than aborting it.
func TestResolveResumeTaskEnvironment_MissingReferenceFallsThroughToCreate(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	// Mirror the sqlite repository, which returns ErrTaskEnvironmentNotFound
	// (wrapped) rather than (nil, nil) when the referenced row is absent.
	repo.getTaskEnvironmentFunc = func(_ context.Context, _ string) (*models.TaskEnvironment, error) {
		return nil, repoerrors.ErrTaskEnvironmentNotFound
	}

	session := &models.TaskSession{
		ID:                "sess-child",
		TaskID:            "task-child",
		TaskEnvironmentID: "env-missing",
	}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), "task-child", session)
	if err != nil {
		t.Fatalf("resolveResumeTaskEnvironment: %v", err)
	}
	if env != nil {
		t.Fatalf("resolved env = %+v, want nil so the create path runs", env)
	}
}

func TestResolveResumeTaskEnvironment_InheritedMissingReferenceFailsClosed(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	repo.getTaskEnvironmentFunc = func(_ context.Context, _ string) (*models.TaskEnvironment, error) {
		return nil, repoerrors.ErrTaskEnvironmentNotFound
	}

	session := &models.TaskSession{
		ID:                "sess-child",
		TaskID:            "task-child",
		TaskEnvironmentID: "env-missing",
	}
	repo.tasks[session.TaskID] = &models.Task{
		ID:       session.TaskID,
		Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
	}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), session.TaskID, session)
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("resolveResumeTaskEnvironmentForTask() error = %v, want ErrWorkspaceReuseUnsafe", err)
	}
	if env != nil {
		t.Fatalf("resolved env = %+v, want nil", env)
	}
}

func TestResolveResumeTaskEnvironment_RejectsArchivedInheritedOwner(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	archivedAt := time.Now()
	repo.tasks["task-parent"] = &models.Task{ID: "task-parent", ArchivedAt: &archivedAt}
	repo.taskEnvironments["env-parent"] = &models.TaskEnvironment{
		ID:     "env-parent",
		TaskID: "task-parent",
		Status: models.TaskEnvironmentStatusReady,
	}

	session := &models.TaskSession{
		ID:                "sess-child",
		TaskID:            "task-child",
		TaskEnvironmentID: "env-parent",
	}
	repo.tasks[session.TaskID] = &models.Task{
		ID:       session.TaskID,
		Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
	}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), session.TaskID, session)
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("resolveResumeTaskEnvironmentForTask() error = %v, want ErrWorkspaceReuseUnsafe", err)
	}
	if env != nil {
		t.Fatalf("resolved env = %+v, want nil", env)
	}
}

func TestResolveResumeTaskEnvironment_RejectsUnverifiableInheritedOwner(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	repo.taskEnvironments["env-parent"] = &models.TaskEnvironment{
		ID:     "env-parent",
		TaskID: "task-parent",
		Status: models.TaskEnvironmentStatusReady,
	}
	repo.getTaskFunc = func(_ context.Context, id string) (*models.Task, error) {
		if id == "task-child" {
			return &models.Task{
				ID:       id,
				Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
			}, nil
		}
		return nil, errors.New("owner lookup failed")
	}

	session := &models.TaskSession{
		ID:                "sess-child",
		TaskID:            "task-child",
		TaskEnvironmentID: "env-parent",
	}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), session.TaskID, session)
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("resolveResumeTaskEnvironment() error = %v, want ErrWorkspaceReuseUnsafe", err)
	}
	if env != nil {
		t.Fatalf("resolved env = %+v, want nil", env)
	}
}

// TestResolveResumeTaskEnvironment_OwnedEnvironmentUnchanged locks in the
// pre-existing behavior for a normal (non-inherited) session: the env is found
// by task id and no inherited fallback lookup occurs.
func TestResolveResumeTaskEnvironment_OwnedEnvironmentUnchanged(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	repo.taskEnvironments["env-own"] = &models.TaskEnvironment{
		ID:           "env-own",
		TaskID:       "task-1",
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}
	session := &models.TaskSession{ID: "sess-1", TaskID: "task-1"}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), "task-1", session)
	if err != nil {
		t.Fatalf("resolveResumeTaskEnvironment: %v", err)
	}
	if env == nil || env.ID != "env-own" {
		t.Fatalf("resolved env = %+v, want env-own", env)
	}
	if session.TaskEnvironmentID != "env-own" {
		t.Fatalf("session.TaskEnvironmentID = %q, want env-own", session.TaskEnvironmentID)
	}
}

// TestResolveResumeTaskEnvironment_NoEnvironmentAndNoReference returns nil so the
// caller can create a fresh environment (the ordinary cold-resume case).
func TestResolveResumeTaskEnvironment_NoEnvironmentAndNoReference(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	session := &models.TaskSession{ID: "sess-1", TaskID: "task-1"}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), "task-1", session)
	if err != nil {
		t.Fatalf("resolveResumeTaskEnvironment: %v", err)
	}
	if env != nil {
		t.Fatalf("resolved env = %+v, want nil", env)
	}
}

// TestPersistTaskEnvironment_GuestDoesNotMutateOwnerEnvironment is a regression
// test for "persist task environment: ready status requires repository
// inventory" on resume. A guest session (office inherit_parent / shared_group)
// attaches to a canonical environment owned by a *different* task. The guest's
// resume request carries no repo specs and the owner's inventory can be empty,
// so the repo-backed ready-status guard used to fail a resume the guest has no
// authority to fix. The guest must attach without touching the owner's row.
func TestPersistTaskEnvironment_GuestDoesNotMutateOwnerEnvironment(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	// The child is repo-backed, which is what previously tripped the guard.
	repo.taskRepositories["tr-child"] = &models.TaskRepository{
		ID: "tr-child", TaskID: "task-child", RepositoryID: "repo-a",
	}
	owner := &models.TaskEnvironment{
		ID:           "env-parent",
		TaskID:       "task-parent",
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}
	repo.taskEnvironments["env-parent"] = owner
	session := &models.TaskSession{ID: "sess-child", TaskID: "task-child", TaskEnvironmentID: "env-parent"}
	req := &LaunchAgentRequest{TaskID: "task-child", ExecutorType: string(models.ExecutorTypeLocal)}
	resp := &LaunchAgentResponse{}

	if err := exec.persistTaskEnvironment(context.Background(), "task-child", session, owner, req, resp, executorConfig{}); err != nil {
		t.Fatalf("persistTaskEnvironment (guest): %v", err)
	}
	if len(repo.updateTaskEnvironmentCalls) != 0 {
		t.Fatalf("guest must not update the owner env, got %d UpdateTaskEnvironment calls", len(repo.updateTaskEnvironmentCalls))
	}
	if len(repo.createTaskEnvironmentCalls) != 0 {
		t.Fatalf("guest must not create an env, got %d CreateTaskEnvironment calls", len(repo.createTaskEnvironmentCalls))
	}
	if len(repo.finalizeTaskEnvironmentCalls) != 0 {
		t.Fatalf("guest is not the materializer, got %d finalize calls", len(repo.finalizeTaskEnvironmentCalls))
	}
	if len(repo.writeCallLog) != 0 {
		t.Fatalf("guest must not write repo rows, write log = %v", repo.writeCallLog)
	}
	if session.TaskEnvironmentID != "env-parent" {
		t.Fatalf("session.TaskEnvironmentID = %q, want env-parent", session.TaskEnvironmentID)
	}
}

// TestPersistTaskEnvironment_GuestMaterializerRunsFinalizePath proves the guest
// short-circuit does not swallow the shared_group case where a member session is
// elected to materialize a still-CREATING canonical environment. Even though the
// environment is owned by a different task, the elected materializer must run the
// normal finalize path so the group's inventory publishes atomically.
func TestPersistTaskEnvironment_GuestMaterializerRunsFinalizePath(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	owner := &models.TaskEnvironment{
		ID:                       "env-group",
		TaskID:                   "task-owner",
		ExecutorType:             string(models.ExecutorTypeLocal),
		Status:                   models.TaskEnvironmentStatusCreating,
		MaterializationSessionID: "sess-member",
	}
	repo.taskEnvironments["env-group"] = owner
	session := &models.TaskSession{ID: "sess-member", TaskID: "task-member", TaskEnvironmentID: "env-group"}
	req := &LaunchAgentRequest{TaskID: "task-member", ExecutorType: string(models.ExecutorTypeLocal), RepositoryID: "repo-a"}
	resp := &LaunchAgentResponse{WorktreeID: "wt-a", WorktreePath: "/tasks/group/repo-a", WorktreeBranch: "feat/x"}

	if err := exec.persistTaskEnvironment(context.Background(), "task-member", session, owner, req, resp, executorConfig{}); err != nil {
		t.Fatalf("persistTaskEnvironment (guest materializer): %v", err)
	}
	if len(repo.finalizeTaskEnvironmentCalls) != 1 {
		t.Fatalf("elected materializer must finalize, got %d finalize calls", len(repo.finalizeTaskEnvironmentCalls))
	}
}
