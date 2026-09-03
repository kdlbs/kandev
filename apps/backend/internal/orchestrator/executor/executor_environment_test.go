package executor

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func newEnvTestExecutor(t *testing.T) *Executor {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return &Executor{logger: log}
}

func TestReuseExistingEnvironment_NilEnv(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}

	e.reuseExistingEnvironment(context.Background(), req, nil)

	if req.Metadata != nil {
		t.Error("expected nil metadata for nil env")
	}
	if req.PreviousExecutionID != "" {
		t.Error("expected empty PreviousExecutionID for nil env")
	}
}

func TestReuseExistingEnvironment_WorktreeReuse(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1", RepositoryID: "repo-1", UseWorktree: true}
	env := &models.TaskEnvironment{
		Repos: []*models.TaskEnvironmentRepo{
			{RepositoryID: "repo-1", WorktreeID: "wt-1", BranchSlug: ""},
		},
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if req.WorktreeID != "wt-1" {
		t.Errorf("expected WorktreeID=wt-1, got %s", req.WorktreeID)
	}
}

func TestCanonicalInventoryMatches_AcceptsLegacyUnscopedRowWhenNoScopedRowsExist(t *testing.T) {
	spec := RepoSpec{RepositoryID: "repo-1", BranchIdentitySlug: "main"}
	rows := []*models.TaskEnvironmentRepo{{
		RepositoryID: "repo-1",
		BranchSlug:   "",
		WorktreeID:   "worktree-1",
	}}

	if got := canonicalInventoryMatches(spec, rows, true); got != 1 {
		t.Fatalf("canonicalInventoryMatches() = %d, want legacy inventory match", got)
	}
}

func TestReuseExistingEnvironment_WorktreeReuseKeepsTaskDirName(t *testing.T) {
	repo := newMockRepository()
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:      "task-1",
		UseWorktree: true,
		TaskDirName: "fresh-task-dir",
		Repositories: []RepoSpec{
			{RepositoryID: "repo-kandev", BranchIdentitySlug: "main"},
			{RepositoryID: "repo-docs", BranchIdentitySlug: "main"},
		},
	}
	env := &models.TaskEnvironment{
		ID:          "env-existing",
		TaskDirName: "persisted-task-dir",
		Repos: []*models.TaskEnvironmentRepo{
			{TaskEnvironmentID: "env-existing", RepositoryID: "repo-kandev", BranchSlug: "main", WorktreeID: "wt-kandev"},
		},
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if req.TaskDirName != "persisted-task-dir" {
		t.Fatalf("TaskDirName = %q, want persisted-task-dir", req.TaskDirName)
	}
	if req.Repositories[0].WorktreeID != "wt-kandev" {
		t.Fatalf("first repo WorktreeID = %q, want wt-kandev", req.Repositories[0].WorktreeID)
	}
	if req.Repositories[1].WorktreeID != "" {
		t.Fatalf("second repo WorktreeID = %q, want empty for new checkout", req.Repositories[1].WorktreeID)
	}
}

func TestReuseExistingEnvironment_SkipsReuseOnExecutorTypeMismatch(t *testing.T) {
	// Switching the task's executor profile to a different type must invalidate
	// reuse: stale PreviousExecutionID/ContainerID/sprite_name from the old
	// backend would otherwise leak into the new launch and overwrite the
	// persisted env with mixed resource IDs on the next save.
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{
		TaskID:       "task-1",
		ExecutorType: "local_docker",
		UseWorktree:  true,
	}
	env := &models.TaskEnvironment{
		ExecutorType: "sprites",
		ContainerID:  "container-abc",
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if req.WorktreeID != "" {
		t.Errorf("expected WorktreeID to be empty on executor mismatch, got %q", req.WorktreeID)
	}
	if req.PreviousExecutionID != "" {
		t.Errorf("expected PreviousExecutionID empty on mismatch, got %q", req.PreviousExecutionID)
	}
	if req.Metadata != nil {
		t.Errorf("expected nil metadata on mismatch, got %v", req.Metadata)
	}
}

func TestReuseExistingEnvironment_WorktreeSkippedWhenNotRequested(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1", UseWorktree: false}
	env := &models.TaskEnvironment{}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if req.WorktreeID != "" {
		t.Errorf("expected empty WorktreeID when UseWorktree=false, got %s", req.WorktreeID)
	}
}

func TestWorkspaceReuseAllowedRequiresMatchingExecutorType(t *testing.T) {
	env := &models.TaskEnvironment{
		ExecutorType: string(models.ExecutorTypeLocal),
		Repos:        []*models.TaskEnvironmentRepo{{RepositoryID: "repo-1"}},
	}
	if workspaceReuseAllowed(env, string(models.ExecutorTypeWorktree), true, true) {
		t.Fatal("workspace reuse should be disabled when the executor type changes")
	}
	if !workspaceReuseAllowed(env, string(models.ExecutorTypeLocal), true, true) {
		t.Fatal("workspace reuse should remain enabled for the owning executor type")
	}
	if workspaceReuseAllowed(&models.TaskEnvironment{}, string(models.ExecutorTypeWorktree), true, true) {
		t.Fatal("legacy environments without a physical worktree should not be reused by worktree launches")
	}
	if !workspaceReuseAllowed(&models.TaskEnvironment{
		Repos: []*models.TaskEnvironmentRepo{{WorktreeID: "legacy-worktree", Status: "active"}},
	}, string(models.ExecutorTypeWorktree), true, true) {
		t.Fatal("legacy environments with a physical worktree should remain reusable")
	}
}

func TestWorkspaceReuseAllowedRequiresInventoryForRepoBackedExecutors(t *testing.T) {
	for _, executorType := range []string{
		string(models.ExecutorTypeLocal),
		string(models.ExecutorTypeLocalDocker),
		string(models.ExecutorTypeSSH),
		string(models.ExecutorTypeWorktree),
	} {
		t.Run(executorType, func(t *testing.T) {
			env := &models.TaskEnvironment{ExecutorType: executorType}
			if workspaceReuseAllowed(env, executorType, true, true) {
				t.Fatalf("workspace reuse allowed for %s with empty repository inventory", executorType)
			}
		})
	}
}

func TestWaitForTaskEnvironmentReadyWaitsForMaterializer(t *testing.T) {
	repo := newMockRepository()
	env := &models.TaskEnvironment{ID: "env-wait", Status: models.TaskEnvironmentStatusCreating}
	lookups := 0
	repo.getTaskEnvironmentFunc = func(_ context.Context, id string) (*models.TaskEnvironment, error) {
		if id != env.ID {
			t.Fatalf("environment lookup id = %q, want %q", id, env.ID)
		}
		lookups++
		if lookups > 1 {
			env.Status = models.TaskEnvironmentStatusReady
		}
		return env, nil
	}

	e := newTestExecutor(t, &mockAgentManager{}, repo)
	got, err := e.waitForTaskEnvironmentReady(context.Background(), env.ID)
	if err != nil {
		t.Fatalf("waitForTaskEnvironmentReady: %v", err)
	}
	if got.Status != models.TaskEnvironmentStatusReady || lookups < 2 {
		t.Fatalf("ready environment = %+v after %d lookup(s)", got, lookups)
	}
}

func TestEnvironmentReposForLaunch_PersistsRemoteWorkspaceIdentity(t *testing.T) {
	req := &LaunchAgentRequest{
		RepositoryID:       "repo-1",
		BranchIdentitySlug: "feature/task",
		WorkspacePath:      "/remote/tasks/task-1",
	}
	resp := &LaunchAgentResponse{WorkspacePath: "/remote/tasks/task-1"}

	repos := environmentReposForLaunch(req, resp)
	if len(repos) != 1 {
		t.Fatalf("environment repos = %#v, want one remote repository row", repos)
	}
	got := repos[0]
	if got.RepositoryID != "repo-1" || got.BranchSlug != "feature-task" || got.WorktreePath != "" || got.WorktreeID != "" {
		t.Fatalf("remote environment repo = %#v", got)
	}
}

func TestEnvironmentReposForLaunch_LocalInventoryDoesNotPointAtSeedCheckout(t *testing.T) {
	req := &LaunchAgentRequest{
		ExecutorType:       string(models.ExecutorTypeLocal),
		RepositoryID:       "repo-1",
		RepositoryPath:     "/tmp/e2e-repo",
		BranchIdentitySlug: "feature/task",
	}
	resp := &LaunchAgentResponse{WorkspacePath: req.RepositoryPath}

	repos := environmentReposForLaunch(req, resp)
	if len(repos) != 1 {
		t.Fatalf("environment repos = %#v, want one inventory row", repos)
	}
	got := repos[0]
	if got.WorktreeID != "" || got.WorktreePath != "" || got.WorktreeBranch != "" {
		t.Fatalf("local inventory row points at a physical checkout: %#v", got)
	}
}

func TestEnvironmentReposForLaunch_PersistsEveryRemoteRepository(t *testing.T) {
	req := &LaunchAgentRequest{
		RepositoryID:  "repo-primary",
		WorkspacePath: "/remote/tasks/task-1",
		Repositories: []RepoSpec{
			{RepositoryID: "repo-primary", BranchIdentitySlug: "main"},
			{RepositoryID: "repo-secondary", BranchIdentitySlug: "release/v2"},
		},
	}

	repos := environmentReposForLaunch(req, &LaunchAgentResponse{WorkspacePath: req.WorkspacePath})
	if len(repos) != 2 {
		t.Fatalf("environment repos = %#v, want one row per remote repository", repos)
	}
	if repos[0].RepositoryID != "repo-primary" || repos[0].BranchSlug != "main" {
		t.Fatalf("primary remote environment repo = %#v", repos[0])
	}
	if repos[1].RepositoryID != "repo-secondary" || repos[1].BranchSlug != "release-v2" {
		t.Fatalf("secondary remote environment repo = %#v", repos[1])
	}
	for _, repo := range repos {
		if repo.WorktreePath != "" || repo.WorktreeBranch != "" {
			t.Fatalf("remote repository without a concrete result recorded shared launch fields: %#v", repo)
		}
	}
}
func TestReuseExistingEnvironment_ContainerReuse(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1", WorkspaceReuseRequired: true}
	env := &models.TaskEnvironment{
		ContainerID:                       "container-abc",
		ContainerBootstrapNonceSecretID:   "bootstrap-secret-abc",
		ContainerControlAuthTokenSecretID: "container-control-secret-abc",
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if req.PreviousExecutionID != "" {
		t.Errorf("expected empty PreviousExecutionID, got %s", req.PreviousExecutionID)
	}
	if req.Metadata["container_id"] != "container-abc" {
		t.Errorf("expected metadata container_id=container-abc, got %v", req.Metadata["container_id"])
	}
	if req.Metadata[lifecycle.MetadataKeyBootstrapNonceSecret] != "bootstrap-secret-abc" {
		t.Errorf("expected canonical bootstrap nonce reference, got %v", req.Metadata[lifecycle.MetadataKeyBootstrapNonceSecret])
	}
	if req.Metadata[lifecycle.MetadataKeyContainerControlAuthSecret] != "container-control-secret-abc" {
		t.Errorf("expected canonical container control secret reference, got %v", req.Metadata[lifecycle.MetadataKeyContainerControlAuthSecret])
	}
}

func TestPersistTaskEnvironment_DockerBootstrapNonceBecomesEnvironmentHandle(t *testing.T) {
	repo := newMockRepository()
	env := &models.TaskEnvironment{
		ID:           "env-1",
		TaskID:       "task-1",
		ExecutorType: string(models.ExecutorTypeLocalDocker),
		Status:       models.TaskEnvironmentStatusCreating,
	}
	repo.taskEnvironments[env.ID] = env
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	session := &models.TaskSession{ID: "session-1", TaskID: "task-1", TaskEnvironmentID: env.ID}

	e.persistTaskEnvironment(context.Background(), "task-1", session, env,
		&LaunchAgentRequest{TaskID: "task-1", ExecutorType: string(models.ExecutorTypeLocalDocker)},
		&LaunchAgentResponse{ContainerID: "container-1", Metadata: map[string]interface{}{
			lifecycle.MetadataKeyBootstrapNonceSecret:       "bootstrap-secret-1",
			lifecycle.MetadataKeyContainerControlAuthSecret: "container-control-secret-1",
		}},
		executorConfig{ExecutorID: "docker"})

	persisted := repo.taskEnvironments[env.ID]
	if persisted.ContainerID != "container-1" {
		t.Fatalf("ContainerID = %q, want container-1", persisted.ContainerID)
	}
	if persisted.ContainerBootstrapNonceSecretID != "bootstrap-secret-1" {
		t.Fatalf("ContainerBootstrapNonceSecretID = %q, want bootstrap-secret-1", persisted.ContainerBootstrapNonceSecretID)
	}
	if persisted.ContainerControlAuthTokenSecretID != "container-control-secret-1" {
		t.Fatalf("ContainerControlAuthTokenSecretID = %q, want container-control-secret-1", persisted.ContainerControlAuthTokenSecretID)
	}
	attach := &LaunchAgentRequest{TaskID: "task-1", ExecutorType: string(models.ExecutorTypeLocalDocker), WorkspaceReuseRequired: true}
	e.reuseExistingEnvironment(context.Background(), attach, persisted)
	if attach.PreviousExecutionID != "" {
		t.Fatalf("PreviousExecutionID = %q, want empty for sibling attach", attach.PreviousExecutionID)
	}
	if got := attach.Metadata[lifecycle.MetadataKeyBootstrapNonceSecret]; got != "bootstrap-secret-1" {
		t.Fatalf("bootstrap nonce reference = %v, want canonical environment handle", got)
	}
	if got := attach.Metadata[lifecycle.MetadataKeyContainerControlAuthSecret]; got != "container-control-secret-1" {
		t.Fatalf("container control secret reference = %v, want canonical environment handle", got)
	}
}

func TestPersistTaskEnvironment_FinalizesCreatingEnvironmentWithInventory(t *testing.T) {
	repo := newMockRepository()
	env := &models.TaskEnvironment{
		ID:                       "env-1",
		TaskID:                   "task-1",
		ExecutorType:             string(models.ExecutorTypeWorktree),
		Status:                   models.TaskEnvironmentStatusCreating,
		MaterializationSessionID: "session-1",
	}
	repo.taskEnvironments[env.ID] = env
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	session := &models.TaskSession{ID: "session-1", TaskID: "task-1", TaskEnvironmentID: env.ID}

	e.persistTaskEnvironment(context.Background(), "task-1", session, env,
		&LaunchAgentRequest{TaskID: "task-1", ExecutorType: string(models.ExecutorTypeWorktree)},
		&LaunchAgentResponse{WorktreePath: "/tasks/task-1/repo", Worktrees: []RepoWorktreeResult{{
			RepositoryID: "repo-1", WorktreeID: "wt-1", WorktreePath: "/tasks/task-1/repo", WorktreeBranch: "feature/task-1",
		}}},
		executorConfig{ExecutorID: models.ExecutorIDWorktree})

	if len(repo.finalizeTaskEnvironmentCalls) != 1 {
		t.Fatalf("FinalizeTaskEnvironmentMaterialization calls = %d, want 1", len(repo.finalizeTaskEnvironmentCalls))
	}
	if len(repo.updateTaskEnvironmentCalls) != 0 {
		t.Fatalf("UpdateTaskEnvironment calls = %d, want 0 before atomic finalization", len(repo.updateTaskEnvironmentCalls))
	}
	persisted := repo.taskEnvironments[env.ID]
	if persisted.Status != models.TaskEnvironmentStatusReady || persisted.MaterializationSessionID != "" {
		t.Fatalf("environment publication = status %q owner %q, want ready with cleared owner", persisted.Status, persisted.MaterializationSessionID)
	}
	if got := repo.taskEnvironmentRepos[env.ID]; len(got) != 1 || got[0].WorktreeID != "wt-1" {
		t.Fatalf("canonical inventory = %#v, want one finalized worktree", got)
	}
}

func TestPersistTaskEnvironment_RestoresMaterializationClaimOnFinalizeFailure(t *testing.T) {
	repo := newMockRepository()
	persistErr := errors.New("finalize failed")
	repo.finalizeTaskEnvironmentErr = persistErr
	env := &models.TaskEnvironment{
		ID:                       "env-1",
		TaskID:                   "task-1",
		ExecutorType:             string(models.ExecutorTypeWorktree),
		Status:                   models.TaskEnvironmentStatusCreating,
		MaterializationSessionID: "session-1",
	}
	repo.taskEnvironments[env.ID] = env
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	session := &models.TaskSession{ID: "session-1", TaskID: "task-1", TaskEnvironmentID: env.ID}

	err := e.persistTaskEnvironment(context.Background(), "task-1", session, env,
		&LaunchAgentRequest{TaskID: "task-1", ExecutorType: string(models.ExecutorTypeWorktree)},
		&LaunchAgentResponse{WorktreePath: "/tasks/task-1/repo", Worktrees: []RepoWorktreeResult{{
			RepositoryID: "repo-1", WorktreeID: "wt-1", WorktreePath: "/tasks/task-1/repo", WorktreeBranch: "feature/task-1",
		}}},
		executorConfig{ExecutorID: models.ExecutorIDWorktree})

	if !errors.Is(err, persistErr) {
		t.Fatalf("persistTaskEnvironment error = %v, want %v", err, persistErr)
	}
	if env.Status != models.TaskEnvironmentStatusCreating || env.MaterializationSessionID != session.ID {
		t.Fatalf("materialization claim = status %q owner %q, want creating with original owner", env.Status, env.MaterializationSessionID)
	}
}

// TestPersistTaskEnvironment_NonMaterializerSiblingPersistsReposBeforeReady
// pins the fix for the "ready status precedes inventory commit" hazard: a
// non-initial-materializer sibling must write its per-repo rows before
// flipping the environment to ready, so a crash or concurrent read between
// the two writes can never observe a ready environment whose canonical
// inventory is still empty or stale.
func TestPersistTaskEnvironment_NonMaterializerSiblingPersistsReposBeforeReady(t *testing.T) {
	repo := newMockRepository()
	env := &models.TaskEnvironment{
		ID:           "env-1",
		TaskID:       "task-1",
		ExecutorType: string(models.ExecutorTypeWorktree),
		Status:       models.TaskEnvironmentStatusReady,
	}
	repo.taskEnvironments[env.ID] = env
	repo.taskRepositories["task-repo-1"] = &models.TaskRepository{ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1"}
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	session := &models.TaskSession{ID: "session-2", TaskID: "task-1", TaskEnvironmentID: env.ID}

	if err := e.persistTaskEnvironment(context.Background(), "task-1", session, env,
		&LaunchAgentRequest{TaskID: "task-1", ExecutorType: string(models.ExecutorTypeWorktree)},
		&LaunchAgentResponse{WorktreePath: "/tasks/task-1/repo", Worktrees: []RepoWorktreeResult{{
			RepositoryID: "repo-1", WorktreeID: "wt-2", WorktreePath: "/tasks/task-1/repo", WorktreeBranch: "feature/task-1",
		}}},
		executorConfig{ExecutorID: models.ExecutorIDWorktree}); err != nil {
		t.Fatalf("persistTaskEnvironment: %v", err)
	}

	if len(repo.writeCallLog) < 2 {
		t.Fatalf("write call log = %v, want at least a repo write followed by an env update", repo.writeCallLog)
	}
	repoIdx, envIdx := -1, -1
	for i, call := range repo.writeCallLog {
		switch call {
		case "create_repo":
			if repoIdx == -1 {
				repoIdx = i
			}
		case "update_env":
			if envIdx == -1 {
				envIdx = i
			}
		}
	}
	if repoIdx == -1 || envIdx == -1 || repoIdx > envIdx {
		t.Fatalf("write order = %v, want create_repo before update_env", repo.writeCallLog)
	}
	if got := repo.taskEnvironments[env.ID].Status; got != models.TaskEnvironmentStatusReady {
		t.Fatalf("status = %q, want ready once inventory is persisted", got)
	}
}

func TestPersistTaskEnvironment_LeavesEnvironmentUnchangedOnRepoWriteFailure(t *testing.T) {
	repo := newMockRepository()
	persistErr := errors.New("inventory write failed")
	repo.createTaskEnvironmentRepoErr = persistErr
	env := &models.TaskEnvironment{
		ID:           "env-1",
		TaskID:       "task-1",
		ExecutorType: string(models.ExecutorTypeWorktree),
		Status:       models.TaskEnvironmentStatusReady,
	}
	repo.taskEnvironments[env.ID] = env
	repo.taskRepositories["task-repo-1"] = &models.TaskRepository{ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1"}
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	session := &models.TaskSession{ID: "session-2", TaskID: "task-1", TaskEnvironmentID: env.ID}

	err := e.persistTaskEnvironment(context.Background(), "task-1", session, env,
		&LaunchAgentRequest{TaskID: "task-1", ExecutorType: string(models.ExecutorTypeWorktree)},
		&LaunchAgentResponse{WorktreePath: "/tasks/task-1/repo", Worktrees: []RepoWorktreeResult{{
			RepositoryID: "repo-1", WorktreeID: "wt-2", WorktreePath: "/tasks/task-1/repo", WorktreeBranch: "feature/task-1",
		}}},
		executorConfig{ExecutorID: models.ExecutorIDWorktree})

	if !errors.Is(err, persistErr) {
		t.Fatalf("persistTaskEnvironment error = %v, want %v", err, persistErr)
	}
	if session.TaskEnvironmentID != env.ID {
		t.Fatalf("session TaskEnvironmentID = %q, want unchanged %q", session.TaskEnvironmentID, env.ID)
	}
	if len(repo.updateTaskEnvironmentCalls) != 0 {
		t.Fatalf("UpdateTaskEnvironment calls = %d, want none after inventory failure", len(repo.updateTaskEnvironmentCalls))
	}
}

// TestPersistTaskEnvironment_NonMaterializerSiblingWithEmptyReposDoesNotSetReady
// covers the companion invariant: when this sibling's own launch produced no
// repos and the environment had none recorded either, a repo-backed task's
// environment must not be forced to ready — that would publish an empty
// canonical inventory that permanently bricks reuse.
func TestPersistTaskEnvironment_NonMaterializerSiblingWithEmptyReposDoesNotSetReady(t *testing.T) {
	repo := newMockRepository()
	env := &models.TaskEnvironment{
		ID:           "env-1",
		TaskID:       "task-1",
		ExecutorType: string(models.ExecutorTypeSSH),
		Status:       models.TaskEnvironmentStatusCreating,
	}
	repo.taskEnvironments[env.ID] = env
	repo.taskRepositories["task-repo-1"] = &models.TaskRepository{ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1"}
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	session := &models.TaskSession{ID: "session-2", TaskID: "task-1", TaskEnvironmentID: env.ID}

	if err := e.persistTaskEnvironment(context.Background(), "task-1", session, env,
		&LaunchAgentRequest{TaskID: "task-1", ExecutorType: string(models.ExecutorTypeSSH)},
		&LaunchAgentResponse{},
		executorConfig{ExecutorID: "ssh"}); err != nil {
		t.Fatalf("persistTaskEnvironment: %v", err)
	}

	for _, call := range repo.writeCallLog {
		if call == "create_repo" {
			t.Fatalf("write call log = %v, want no repo rows created for an empty prepare result", repo.writeCallLog)
		}
	}
	if got := repo.taskEnvironments[env.ID].Status; got != models.TaskEnvironmentStatusCreating {
		t.Fatalf("status = %q, want status left untouched instead of forced to ready with empty inventory", got)
	}
}

func TestReuseExistingEnvironment_DockerBranchReuse(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1", ExecutorType: "local_docker"}
	env := &models.TaskEnvironment{
		ExecutorType: "local_docker",
		ContainerID:  "container-abc",
		Repos: []*models.TaskEnvironmentRepo{
			{WorktreeBranch: "feature/existing-task-abc"},
		},
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if req.Metadata[lifecycle.MetadataKeyWorktreeBranch] != "feature/existing-task-abc" {
		t.Fatalf("metadata worktree_branch = %v, want existing branch", req.Metadata[lifecycle.MetadataKeyWorktreeBranch])
	}
}

func TestReuseExistingEnvironment_RuntimeMetadata_CarriesPersistentSecrets(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	repo := newMockRepository()
	now := time.Now().UTC()
	repo.sessions["session-old"] = &models.TaskSession{
		ID:                "session-old",
		TaskID:            "task-1",
		TaskEnvironmentID: "env-1",
		StartedAt:         now,
		UpdatedAt:         now,
	}
	repo.executorsRunning["session-old"] = &models.ExecutorRunning{
		SessionID:        "session-old",
		AgentExecutionID: "exec-old",
		ContainerID:      "container-old",
		Metadata: map[string]interface{}{
			lifecycle.MetadataKeyAuthTokenSecret:      "secret-token",
			lifecycle.MetadataKeyBootstrapNonceSecret: "secret-nonce",
			"task_description":                        "drop me",
		},
	}
	e := &Executor{logger: log, repo: repo}
	req := &LaunchAgentRequest{TaskID: "task-1"}

	e.reuseExistingEnvironment(context.Background(), req, &models.TaskEnvironment{
		ID: "env-1",
	})

	if req.PreviousExecutionID != "exec-old" {
		t.Fatalf("PreviousExecutionID = %q, want exec-old", req.PreviousExecutionID)
	}
	if req.Metadata[lifecycle.MetadataKeyContainerID] != "container-old" {
		t.Fatalf("container metadata = %v, want container-old", req.Metadata[lifecycle.MetadataKeyContainerID])
	}
	if req.Metadata[lifecycle.MetadataKeyAuthTokenSecret] != "secret-token" {
		t.Fatalf("auth token secret missing: %v", req.Metadata)
	}
	if req.Metadata[lifecycle.MetadataKeyBootstrapNonceSecret] != "secret-nonce" {
		t.Fatalf("bootstrap nonce secret missing: %v", req.Metadata)
	}
	if _, ok := req.Metadata["task_description"]; ok {
		t.Fatalf("launch-only metadata should be filtered out: %v", req.Metadata)
	}
}

func TestReuseExistingEnvironment_RuntimeMetadata_FallsBackToMatchingContainer(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	repo := newMockRepository()
	now := time.Now().UTC()
	repo.sessions["session-old"] = &models.TaskSession{
		ID:        "session-old",
		TaskID:    "task-1",
		StartedAt: now,
		UpdatedAt: now,
	}
	repo.executorsRunning["session-old"] = &models.ExecutorRunning{
		SessionID:        "session-old",
		AgentExecutionID: "exec-old",
		ContainerID:      "container-old",
		Metadata: map[string]interface{}{
			lifecycle.MetadataKeyAuthTokenSecret:      "secret-token",
			lifecycle.MetadataKeyBootstrapNonceSecret: "secret-nonce",
		},
	}
	e := &Executor{logger: log, repo: repo}
	req := &LaunchAgentRequest{TaskID: "task-1"}

	e.reuseExistingEnvironment(context.Background(), req, &models.TaskEnvironment{
		ID:          "env-1",
		ContainerID: "container-old",
	})

	if req.PreviousExecutionID != "exec-old" {
		t.Fatalf("PreviousExecutionID = %q, want exec-old", req.PreviousExecutionID)
	}
	if req.Metadata[lifecycle.MetadataKeyContainerID] != "container-old" {
		t.Fatalf("container metadata = %v, want container-old", req.Metadata[lifecycle.MetadataKeyContainerID])
	}
	if req.Metadata[lifecycle.MetadataKeyAuthTokenSecret] != "secret-token" {
		t.Fatalf("auth token secret missing: %v", req.Metadata)
	}
	if req.Metadata[lifecycle.MetadataKeyBootstrapNonceSecret] != "secret-nonce" {
		t.Fatalf("bootstrap nonce secret missing: %v", req.Metadata)
	}
}

func TestBuildResumeRequest_ReusesDockerEnvironmentHandleWithoutSiblingRuntime(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{}
	exec := newTestExecutor(t, agentManager, repo)
	now := time.Now().UTC()
	task := &v1.Task{
		ID:          "task-1",
		WorkspaceID: "workspace-1",
		Title:       "Task 1",
	}
	session := &models.TaskSession{
		ID:                "session-new",
		TaskID:            "task-1",
		AgentProfileID:    "profile-1",
		ExecutorID:        models.ExecutorIDLocalDocker,
		TaskEnvironmentID: "env-1",
		State:             models.TaskSessionStateWaitingForInput,
		StartedAt:         now,
		UpdatedAt:         now,
	}
	repo.executors[models.ExecutorIDLocalDocker] = &models.Executor{
		ID:        models.ExecutorIDLocalDocker,
		Type:      models.ExecutorTypeLocalDocker,
		Status:    models.ExecutorStatusActive,
		Resumable: true,
	}
	repo.taskEnvironments["env-1"] = &models.TaskEnvironment{
		ID:                              "env-1",
		TaskID:                          "task-1",
		ExecutorType:                    string(models.ExecutorTypeLocalDocker),
		ContainerID:                     "container-old",
		ContainerBootstrapNonceSecretID: "bootstrap-secret",
		Status:                          models.TaskEnvironmentStatusReady,
	}
	repo.sessions["session-old"] = &models.TaskSession{
		ID:                "session-old",
		TaskID:            "task-1",
		TaskEnvironmentID: "env-1",
		StartedAt:         now.Add(-time.Minute),
		UpdatedAt:         now.Add(-time.Minute),
	}
	repo.executorsRunning["session-old"] = &models.ExecutorRunning{
		SessionID:        "session-old",
		TaskID:           "task-1",
		AgentExecutionID: "exec-old",
		ContainerID:      "container-old",
		Runtime:          models.ExecutorTypeLocalDocker.Runtime(),
		Metadata: map[string]interface{}{
			lifecycle.MetadataKeyAuthTokenSecret: "secret-token",
			"task_description":                   "drop me",
		},
	}

	req, _, _, _, running, err := exec.buildResumeRequest(context.Background(), task, session, true)
	if err != nil {
		t.Fatalf("buildResumeRequest returned error: %v", err)
	}

	if running != nil {
		t.Fatalf("current session should not have an ExecutorRunning row")
	}
	if req.TaskEnvironmentID != "env-1" {
		t.Fatalf("TaskEnvironmentID = %q, want env-1", req.TaskEnvironmentID)
	}
	if req.PreviousExecutionID != "" {
		t.Fatalf("PreviousExecutionID = %q, want empty for sibling session", req.PreviousExecutionID)
	}
	if req.Metadata[lifecycle.MetadataKeyContainerID] != "container-old" {
		t.Fatalf("container metadata = %v, want container-old", req.Metadata[lifecycle.MetadataKeyContainerID])
	}
	if _, found := req.Metadata[lifecycle.MetadataKeyAuthTokenSecret]; found {
		t.Fatalf("sibling auth token leaked into attach request: %v", req.Metadata)
	}
	if req.Metadata[lifecycle.MetadataKeyBootstrapNonceSecret] != "bootstrap-secret" {
		t.Fatalf("bootstrap nonce reference = %v, want canonical environment handle", req.Metadata[lifecycle.MetadataKeyBootstrapNonceSecret])
	}
	if _, ok := req.Metadata["task_description"]; ok {
		t.Fatalf("launch-only metadata should be filtered out: %v", req.Metadata)
	}
}

func TestBuildResumeRequest_UsesExecutionProfileAndKeepsOfficeIdentity(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetGitLabCredentialResolver(&fakeGitLabCredentialResolver{byWorkspace: map[string]struct{ host, token string }{
		"workspace-1": {host: "https://gitlab.example", token: "resume-token"},
	}})
	task := &v1.Task{ID: "task-1", WorkspaceID: "workspace-1", Title: "Task 1"}
	session := &models.TaskSession{
		ID:                 "session-1",
		TaskID:             task.ID,
		AgentProfileID:     "office-cto",
		ExecutionProfileID: "claude-opus",
		ExecutorID:         models.ExecutorIDLocal,
		State:              models.TaskSessionStateWaitingForInput,
	}
	repo.executorsRunning[session.ID] = &models.ExecutorRunning{
		SessionID:          session.ID,
		TaskID:             task.ID,
		ExecutionProfileID: "claude-opus",
		ResumeToken:        "claude-session",
	}

	req, _, _, _, _, err := exec.buildResumeRequest(context.Background(), task, session, true)
	if err != nil {
		t.Fatalf("buildResumeRequest: %v", err)
	}
	if req.AgentProfileID != "claude-opus" {
		t.Fatalf("AgentProfileID = %q, want concrete execution profile", req.AgentProfileID)
	}
	if req.OfficeAgentProfileID != "office-cto" {
		t.Fatalf("OfficeAgentProfileID = %q, want stable Office identity", req.OfficeAgentProfileID)
	}
	if req.ACPSessionID != "claude-session" {
		t.Fatalf("ACPSessionID = %q, want matching profile token", req.ACPSessionID)
	}
	if req.WorkspaceID != "workspace-1" || req.Env[envGitLabToken] != "resume-token" {
		t.Fatalf("resume credentials not workspace scoped: workspace=%q env=%#v", req.WorkspaceID, req.Env)
	}
}

func TestReuseExistingEnvironment_SandboxReuse(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}
	env := &models.TaskEnvironment{
		SandboxID: "kandev-sprite-abc",
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if req.PreviousExecutionID != "" {
		t.Errorf("expected empty PreviousExecutionID, got %s", req.PreviousExecutionID)
	}
	if req.Metadata["sprite_name"] != "kandev-sprite-abc" {
		t.Errorf("expected metadata sprite_name=kandev-sprite-abc, got %v", req.Metadata["sprite_name"])
	}
}

func TestReuseExistingEnvironment_WorktreeAndContainer(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1", RepositoryID: "repo-1", UseWorktree: true}
	env := &models.TaskEnvironment{
		ContainerID: "container-abc",
		Repos: []*models.TaskEnvironmentRepo{
			{RepositoryID: "repo-1", WorktreeID: "wt-1"},
		},
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if req.WorktreeID != "wt-1" {
		t.Errorf("expected WorktreeID=wt-1, got %s", req.WorktreeID)
	}
	if req.Metadata["container_id"] != "container-abc" {
		t.Errorf("expected metadata container_id=container-abc, got %v", req.Metadata["container_id"])
	}
	if req.PreviousExecutionID != "" {
		t.Errorf("expected empty PreviousExecutionID, got %s", req.PreviousExecutionID)
	}
}

func TestReuseExistingEnvironment_AttachOnlyDoesNotAdoptSiblingExecution(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{ID: "session-1", TaskID: "task-1", TaskEnvironmentID: "environment-1"}
	repo.executorsRunning["session-1"] = &models.ExecutorRunning{AgentExecutionID: "execution-1"}
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{TaskID: "task-1", WorkspaceReuseRequired: true}

	e.reuseExistingEnvironment(context.Background(), req, &models.TaskEnvironment{ID: "environment-1"})

	if req.PreviousExecutionID != "" {
		t.Fatalf("attach-only reuse adopted sibling execution %q", req.PreviousExecutionID)
	}
}

func TestReuseExistingEnvironment_EmptyEnvFieldsDoNothing(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}
	env := &models.TaskEnvironment{}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if req.Metadata != nil {
		t.Error("expected nil metadata when no container/sandbox IDs")
	}
	if req.PreviousExecutionID != "" {
		t.Error("expected empty PreviousExecutionID when no container/sandbox IDs")
	}
}

func TestReuseExistingEnvironment_ReuseRequiredSSHUsesCanonicalRemoteTaskDir(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{
		TaskID:                 "task-1",
		ExecutorType:           string(models.ExecutorTypeSSH),
		WorkspaceReuseRequired: true,
	}
	env := &models.TaskEnvironment{
		ID:            "environment-1",
		ExecutorType:  string(models.ExecutorTypeSSH),
		WorkspacePath: "/home/kandev/.kandev/tasks/task-1",
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if got := req.Metadata[lifecycle.MetadataKeySSHRemoteTaskDir]; got != env.WorkspacePath {
		t.Fatalf("remote task directory = %v, want %q", got, env.WorkspacePath)
	}
	if req.PreviousExecutionID != "" {
		t.Fatalf("PreviousExecutionID = %q, want empty for attach-only reuse", req.PreviousExecutionID)
	}
}

// TestReuseExistingEnvironment_ReuseNotRequiredSSHSkipsStaleRemoteTaskDir
// pins the fix for the empty-inventory SSH reuse hazard: when
// workspaceReuseAllowed has already forced a fresh materialization (reuse
// not required), reuseExistingEnvironment must not forward the old
// env.WorkspacePath, or the SSH executor's attach-only path would trust a
// possibly incomplete or stale checkout instead of preparing a fresh one.
func TestReuseExistingEnvironment_ReuseNotRequiredSSHSkipsStaleRemoteTaskDir(t *testing.T) {
	e := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	req := &LaunchAgentRequest{
		TaskID:                 "task-1",
		ExecutorType:           string(models.ExecutorTypeSSH),
		WorkspaceReuseRequired: false,
	}
	env := &models.TaskEnvironment{
		ID:            "environment-1",
		ExecutorType:  string(models.ExecutorTypeSSH),
		WorkspacePath: "/home/kandev/.kandev/tasks/task-1",
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if got, ok := req.Metadata[lifecycle.MetadataKeySSHRemoteTaskDir]; ok {
		t.Fatalf("remote task directory = %v, want unset for a non-reuse launch", got)
	}
}

func TestReuseExistingEnvironment_FreshRepoRecoveryDoesNotAdoptSiblingExecution(t *testing.T) {
	repo := newMockRepository()
	repo.taskRepositories["task-repo-1"] = &models.TaskRepository{
		ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1",
	}
	repo.sessions["session-old"] = &models.TaskSession{
		ID: "session-old", TaskID: "task-1", TaskEnvironmentID: "env-1",
	}
	repo.executorsRunning["session-old"] = &models.ExecutorRunning{
		SessionID:        "session-old",
		AgentExecutionID: "exec-old",
	}
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:                 "task-1",
		ExecutorType:           string(models.ExecutorTypeSSH),
		WorkspaceReuseRequired: false,
	}
	env := &models.TaskEnvironment{ID: "env-1", TaskID: "task-1", ExecutorType: string(models.ExecutorTypeSSH)}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if req.PreviousExecutionID != "" {
		t.Fatalf("PreviousExecutionID = %q, want empty during fresh repo recovery", req.PreviousExecutionID)
	}
}

func TestReuseExistingEnvironment_FreshRepoRecoveryDropsContainerHandle(t *testing.T) {
	repo := newMockRepository()
	repo.taskRepositories["task-repo-1"] = &models.TaskRepository{
		ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1",
	}
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{
		TaskID:                 "task-1",
		ExecutorType:           string(models.ExecutorTypeLocalDocker),
		WorkspaceReuseRequired: false,
	}
	env := &models.TaskEnvironment{
		ID:           "env-1",
		TaskID:       "task-1",
		ExecutorType: string(models.ExecutorTypeLocalDocker),
		ContainerID:  "container-stale",
	}

	e.reuseExistingEnvironment(context.Background(), req, env)

	if req.PreviousExecutionID != "" {
		t.Fatalf("PreviousExecutionID = %q, want empty during fresh repo recovery", req.PreviousExecutionID)
	}
	if req.Metadata != nil {
		t.Fatalf("metadata = %#v, want no stale container handle during fresh recovery", req.Metadata)
	}
}

// TestApplyExecutorRunningMetadata_SkipsSessionScopedKeys pins the guard
// that prevents a SECOND session on the same task from inheriting the FIRST
// session's session-scoped runtime resources — agentctl PID/port, remote
// session dir, local forward port. Without this filter, the SSH executor's
// ResumeRemoteInstance would interpret those keys as a resume hint and
// reattach to session-1's agentctl process, so session 2 would end up
// sharing session 1's ACP session and instance port and never finish its
// own initialize.
//
// Connection / task-environment-wide keys (host, port, user, fingerprint,
// remote task dir, workdir root, proxy jump) MUST still propagate so the
// second session connects to the same host and reuses the task dir.
func TestApplyExecutorRunningMetadata_SkipsSessionScopedKeys(t *testing.T) {
	req := &LaunchAgentRequest{TaskID: "task-1"}
	running := &models.ExecutorRunning{
		AgentExecutionID: "exec-prev",
		Metadata: map[string]interface{}{
			// Connection config — should propagate.
			lifecycle.MetadataKeySSHHost:            "example.com",
			lifecycle.MetadataKeySSHPort:            "2200",
			lifecycle.MetadataKeySSHUser:            "deploy",
			lifecycle.MetadataKeySSHHostFingerprint: "SHA256:aaa",
			lifecycle.MetadataKeySSHRemoteTaskDir:   "/home/deploy/.kandev/tasks/task-1",
			lifecycle.MetadataKeySSHWorkdirRoot:     "/home/deploy/.kandev",
			lifecycle.MetadataKeySSHProxyJump:       "bastion",
			// Session-scoped runtime resources — must NOT propagate.
			lifecycle.MetadataKeySSHRemoteSessionDir:   "/home/deploy/.kandev/tasks/task-1/.kandev/sessions/sess-1",
			lifecycle.MetadataKeySSHRemoteAgentctlPort: "41001",
			lifecycle.MetadataKeySSHRemoteAgentctlPID:  "12345",
			lifecycle.MetadataKeySSHLocalForwardPort:   "59123",
			lifecycle.MetadataKeySSHRemoteAgentctlURL:  "http://127.0.0.1:59123",
			// Non-persistent key — must NOT propagate (not in persistentMetadataKeys).
			"task_description": "session 1 prompt",
		},
	}

	applyExecutorRunningMetadata(req, running)

	if req.PreviousExecutionID != "exec-prev" {
		t.Errorf("PreviousExecutionID = %q, want exec-prev", req.PreviousExecutionID)
	}
	if req.Metadata == nil {
		t.Fatal("req.Metadata is nil; expected propagated keys")
	}

	propagated := []string{
		lifecycle.MetadataKeySSHHost,
		lifecycle.MetadataKeySSHPort,
		lifecycle.MetadataKeySSHUser,
		lifecycle.MetadataKeySSHHostFingerprint,
		lifecycle.MetadataKeySSHRemoteTaskDir,
		lifecycle.MetadataKeySSHWorkdirRoot,
		lifecycle.MetadataKeySSHProxyJump,
	}
	for _, k := range propagated {
		if _, ok := req.Metadata[k]; !ok {
			t.Errorf("expected connection key %q to propagate", k)
		}
	}

	sessionScoped := []string{
		lifecycle.MetadataKeySSHRemoteSessionDir,
		lifecycle.MetadataKeySSHRemoteAgentctlPort,
		lifecycle.MetadataKeySSHRemoteAgentctlPID,
		lifecycle.MetadataKeySSHLocalForwardPort,
		lifecycle.MetadataKeySSHRemoteAgentctlURL,
	}
	for _, k := range sessionScoped {
		if v, ok := req.Metadata[k]; ok {
			t.Errorf("session-scoped key %q leaked into sibling-session request (value=%v)", k, v)
		}
		if !lifecycle.IsSessionScopedMetadataKey(k) {
			t.Errorf("IsSessionScopedMetadataKey(%q) = false; expected true", k)
		}
	}

	// task_description is not in persistentMetadataKeys at all, so the
	// pre-existing ShouldPersistMetadataKey gate already drops it.
	if _, ok := req.Metadata["task_description"]; ok {
		t.Error("task_description (non-persistent) leaked into sibling-session request")
	}
}

func TestApplyExecutorRunningMetadata_DoesNotResumeSiblingKubernetesSession(t *testing.T) {
	req := &LaunchAgentRequest{
		TaskID: "task-1", SessionID: "session-new", ExecutorType: string(models.ExecutorTypeKubernetes),
	}
	running := &models.ExecutorRunning{
		SessionID: "session-old", TaskID: "task-1", AgentExecutionID: "execution-old",
		Runtime:  agentruntime.RuntimeKubernetes,
		Metadata: recordedKubernetesResumeMetadataFor("task-1", "session-old", "execution-old"),
	}

	applyExecutorRunningMetadata(req, running)

	if req.PreviousExecutionID != "" {
		t.Fatalf("PreviousExecutionID = %q, want empty for fresh sibling Kubernetes launch", req.PreviousExecutionID)
	}
}

func TestApplyExecutorRunningMetadata_KubernetesResumeRequiresExactSessionAuthority(t *testing.T) {
	tests := []struct {
		name           string
		requestType    string
		requestSession string
		runtime        agentruntime.Runtime
		runningSession string
		wantPrevious   string
	}{
		{
			name: "exact non-empty Kubernetes session resumes", requestType: string(models.ExecutorTypeKubernetes),
			requestSession: "session-1", runtime: agentruntime.RuntimeKubernetes,
			runningSession: "session-1", wantPrevious: "execution-old",
		},
		{
			name: "sibling Kubernetes session starts fresh", requestType: string(models.ExecutorTypeKubernetes),
			requestSession: "session-new", runtime: agentruntime.RuntimeKubernetes,
			runningSession: "session-old",
		},
		{
			name: "missing recorded Kubernetes session starts fresh", requestType: string(models.ExecutorTypeKubernetes),
			requestSession: "session-new", runtime: agentruntime.RuntimeKubernetes,
		},
		{
			name: "missing request Kubernetes session starts fresh", requestType: string(models.ExecutorTypeKubernetes),
			runtime: agentruntime.RuntimeKubernetes, runningSession: "session-old",
		},
		{
			name: "Kubernetes request rejects a mismatched recorded runtime", requestType: string(models.ExecutorTypeKubernetes),
			requestSession: "session-1", runtime: agentruntime.RuntimeSSH,
			runningSession: "session-1",
		},
		{
			name: "non-Kubernetes request rejects a recorded Kubernetes runtime", requestType: string(models.ExecutorTypeLocal),
			requestSession: "session-1", runtime: agentruntime.RuntimeKubernetes,
			runningSession: "session-1",
		},
		{
			name: "legacy runtime keeps cross-session environment reuse", requestType: string(models.ExecutorTypeLocal),
			requestSession: "session-new", runtime: agentruntime.RuntimeStandalone,
			runningSession: "session-old", wantPrevious: "execution-old",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := &LaunchAgentRequest{
				TaskID: "task-1", SessionID: test.requestSession, ExecutorType: test.requestType,
			}
			running := &models.ExecutorRunning{
				SessionID: test.runningSession, TaskID: "task-1",
				AgentExecutionID: "execution-old", Runtime: test.runtime,
			}

			applyExecutorRunningMetadata(req, running)

			if req.PreviousExecutionID != test.wantPrevious {
				t.Fatalf("PreviousExecutionID = %q, want %q", req.PreviousExecutionID, test.wantPrevious)
			}
		})
	}
}

// TestApplyExecutorRunningMetadata_RequestKeysWin documents that an explicit
// value already on the request (e.g. set by the caller from launch options)
// is not overwritten by the previous ExecutorRunning record. This applies to
// every persistent key, not just the connection ones.
func TestApplyExecutorRunningMetadata_RequestKeysWin(t *testing.T) {
	req := &LaunchAgentRequest{
		TaskID: "task-1",
		Metadata: map[string]interface{}{
			lifecycle.MetadataKeySSHHost: "user-override.example.com",
		},
	}
	running := &models.ExecutorRunning{
		Metadata: map[string]interface{}{
			lifecycle.MetadataKeySSHHost: "stale.example.com",
		},
	}

	applyExecutorRunningMetadata(req, running)

	if got := req.Metadata[lifecycle.MetadataKeySSHHost]; got != "user-override.example.com" {
		t.Errorf("ssh_host = %q, want user-override.example.com (request value should win)", got)
	}
}

func TestExtractSandboxID(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]interface{}
		want     string
	}{
		{"nil metadata", nil, ""},
		{"no sprite_name", map[string]interface{}{"other": "val"}, ""},
		{"with sprite_name", map[string]interface{}{"sprite_name": "kandev-abc"}, "kandev-abc"},
		{"non-string sprite_name", map[string]interface{}{"sprite_name": 42}, ""},
		{"empty sprite_name", map[string]interface{}{"sprite_name": ""}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSandboxID(tt.metadata)
			if got != tt.want {
				t.Errorf("extractSandboxID() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestApplyRepositoryConfig_PropagatesRepositoryID asserts that
// applyRepositoryConfig copies RepositoryID from the resolved repoInfo onto
// the launch request. The lifecycle layer carries this field through to the
// worktree manager's runWorktreeSetupScript, which uses it to look up the
// repository's setup script. When the field is empty the manager silently
// skips the script — manifesting as "the start script is not run" for the
// user who configured one on their repo.
func TestApplyRepositoryConfig_PropagatesRepositoryID(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}
	task := &v1.Task{ID: "task-1", WorkspaceID: "workspace-1", Title: "Some task"}
	info := &repoInfo{
		RepositoryID:   "repo-abc",
		RepositoryPath: "/repos/myrepo",
		BaseBranch:     "main",
		Repository: &models.Repository{
			ID:          "repo-abc",
			Name:        "myrepo",
			SetupScript: "npm install",
		},
	}
	execCfg := executorConfig{ExecutorID: "exec-1", ExecutorType: string(models.ExecutorTypeLocal)}

	if _, err := e.applyRepositoryConfig(req, task, info, execCfg, nil); err != nil {
		t.Fatalf("applyRepositoryConfig: %v", err)
	}

	if req.RepositoryID != "repo-abc" {
		t.Errorf("req.RepositoryID = %q, want %q", req.RepositoryID, "repo-abc")
	}
}

func TestApplyRepositoryConfig_ReuseRequiredDoesNotRequireCloneURL(t *testing.T) {
	e := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	req := &LaunchAgentRequest{TaskID: "task-1", WorkspaceReuseRequired: true}
	task := &v1.Task{ID: "task-1", WorkspaceID: "workspace-1", Title: "Some task"}
	info := &repoInfo{
		RepositoryID:   "repo-abc",
		RepositoryPath: "/repos/myrepo",
		Repository:     &models.Repository{ID: "repo-abc", Name: "myrepo"},
	}
	execCfg := executorConfig{ExecutorID: "docker", ExecutorType: string(models.ExecutorTypeLocalDocker)}

	if _, err := e.applyRepositoryConfig(req, task, info, execCfg, nil); err != nil {
		t.Fatalf("reuse attach rejected an unavailable clone URL: %v", err)
	}
}

func TestApplyRepositoryConfig_DockerUsesRepositoryPathWhenCloneURLIsUnavailable(t *testing.T) {
	e := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	req := &LaunchAgentRequest{TaskID: "task-1"}
	task := &v1.Task{ID: "task-1", WorkspaceID: "workspace-1", Title: "Some task"}
	source := t.TempDir()
	info := &repoInfo{RepositoryID: "repo-abc", RepositoryPath: source, Repository: &models.Repository{ID: "repo-abc", Name: "myrepo"}}
	execCfg := executorConfig{ExecutorID: "docker", ExecutorType: string(models.ExecutorTypeLocalDocker)}

	metadata, err := e.applyRepositoryConfig(req, task, info, execCfg, nil)
	if err != nil {
		t.Fatalf("applyRepositoryConfig: %v", err)
	}
	if req.RepositoryURL != source || metadata["repository_clone_url"] != source {
		t.Fatalf("Docker clone source = %q / %#v, want repository path", req.RepositoryURL, metadata)
	}
}

func TestApplyRepositoryConfig_DockerRejectsMissingRepositoryPathFallback(t *testing.T) {
	e := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	req := &LaunchAgentRequest{TaskID: "task-1"}
	task := &v1.Task{ID: "task-1", WorkspaceID: "workspace-1", Title: "Some task"}
	source := filepath.Join(t.TempDir(), "docker-host-only-source")
	info := &repoInfo{
		RepositoryID:   "repo-abc",
		RepositoryPath: source,
		Repository:     &models.Repository{ID: "repo-abc", Name: "myrepo"},
	}
	execCfg := executorConfig{ExecutorID: "docker", ExecutorType: string(models.ExecutorTypeLocalDocker)}

	if _, err := e.applyRepositoryConfig(req, task, info, execCfg, nil); !errors.Is(err, ErrNoCloneURL) {
		t.Fatalf("applyRepositoryConfig error = %v, want ErrNoCloneURL", err)
	}
}

func TestApplyRepositoryConfig_DockerRejectsRemoteRepositoryPathFallback(t *testing.T) {
	e := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	req := &LaunchAgentRequest{TaskID: "task-1"}
	task := &v1.Task{ID: "task-1", WorkspaceID: "workspace-1", Title: "Some task"}
	info := &repoInfo{
		RepositoryID:   "repo-abc",
		RepositoryPath: "https://example.invalid/owner/repository.git",
		Repository:     &models.Repository{ID: "repo-abc", Name: "myrepo"},
	}
	execCfg := executorConfig{ExecutorID: "docker", ExecutorType: string(models.ExecutorTypeLocalDocker)}

	if _, err := e.applyRepositoryConfig(req, task, info, execCfg, nil); !errors.Is(err, ErrNoCloneURL) {
		t.Fatalf("applyRepositoryConfig error = %v, want ErrNoCloneURL", err)
	}
}

func TestApplyRepositoryConfig_DirectLocalSetsFilesystemSafeRepoName(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}
	task := &v1.Task{ID: "task-1", WorkspaceID: "workspace-1", Title: "Some task"}
	info := &repoInfo{
		RepositoryID:   "repo-abc",
		RepositoryPath: "/repos/widget",
		Repository: &models.Repository{
			ID:   "repo-abc",
			Name: "acme/widget",
		},
	}
	execCfg := executorConfig{ExecutorID: "exec-1", ExecutorType: string(models.ExecutorTypeLocal)}

	if _, err := e.applyRepositoryConfig(req, task, info, execCfg, nil); err != nil {
		t.Fatalf("applyRepositoryConfig: %v", err)
	}

	if req.RepoName != "acme-widget" {
		t.Errorf("req.RepoName = %q, want %q", req.RepoName, "acme-widget")
	}
	if req.TaskDirName != "" {
		t.Errorf("req.TaskDirName = %q, want empty for direct-local launch", req.TaskDirName)
	}
}

func TestApplyRepositoryConfig_DirectLocalFallsBackToRepositoryIDForUnsafeRepoName(t *testing.T) {
	e := newEnvTestExecutor(t)
	req := &LaunchAgentRequest{TaskID: "task-1"}
	task := &v1.Task{ID: "task-1", WorkspaceID: "workspace-1", Title: "Some task"}
	info := &repoInfo{
		RepositoryID:   "repo_123",
		RepositoryPath: "/repos/widget",
		Repository: &models.Repository{
			ID:   "repo_123",
			Name: "日本語!!!",
		},
	}
	execCfg := executorConfig{ExecutorID: "exec-1", ExecutorType: string(models.ExecutorTypeLocal)}

	if _, err := e.applyRepositoryConfig(req, task, info, execCfg, nil); err != nil {
		t.Fatalf("applyRepositoryConfig: %v", err)
	}

	if req.RepoName != "repo_123" {
		t.Errorf("req.RepoName = %q, want stable repository ID fallback %q", req.RepoName, "repo_123")
	}
	if req.TaskDirName != "" {
		t.Errorf("req.TaskDirName = %q, want empty for direct-local launch", req.TaskDirName)
	}
}
