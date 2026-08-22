package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDeleteTaskEnvironmentMissingReturnsSentinel(t *testing.T) {
	repo := newRepoForHealTests(t)

	err := repo.DeleteTaskEnvironment(context.Background(), "missing-environment")

	if !errors.Is(err, ErrTaskEnvironmentNotFound) {
		t.Fatalf("DeleteTaskEnvironment error = %v, want ErrTaskEnvironmentNotFound", err)
	}
}

func TestTaskEnvironmentRepoUpdateDeleteAndBulkCleanup(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-task-environment-crud")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-environment-crud", WorkspaceID: "workspace-task-environment-crud", Title: "Task"}); err != nil {
		t.Fatal(err)
	}
	for _, repositoryID := range []string{"environment-repo-one", "environment-repo-two"} {
		if err := repo.CreateRepository(ctx, &models.Repository{ID: repositoryID, WorkspaceID: "workspace-task-environment-crud", Name: repositoryID}); err != nil {
			t.Fatal(err)
		}
	}
	environment := &models.TaskEnvironment{ID: "task-environment-crud", TaskID: "task-environment-crud", ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady}
	if err := repo.CreateTaskEnvironment(ctx, environment); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	seedForMsgTest(t, repo, "task-environment-crud", "session-environment-crud", "turn-environment-crud")
	if _, err := repo.db.Exec(`UPDATE task_sessions SET task_environment_id = ? WHERE id = ?`, environment.ID, "session-environment-crud"); err != nil {
		t.Fatal(err)
	}
	for index, repositoryID := range []string{"environment-repo-one", "environment-repo-two"} {
		if err := repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{ID: "env-link-" + repositoryID, TaskEnvironmentID: environment.ID, RepositoryID: repositoryID, Position: index, WorktreeBranch: "main"}); err != nil {
			t.Fatalf("CreateTaskEnvironmentRepo: %v", err)
		}
	}
	links, err := repo.ListTaskEnvironmentRepos(ctx, environment.ID)
	if err != nil || len(links) != 2 {
		t.Fatalf("ListTaskEnvironmentRepos = %+v, %v", links, err)
	}
	mergedAt := time.Date(2026, time.July, 8, 9, 10, 11, 0, time.UTC)
	links[0].BranchSlug = "feature"
	links[0].WorktreeBranch = "feature/updated"
	links[0].Status = "merged"
	links[0].MergedAt = &mergedAt
	if err := repo.UpdateTaskEnvironmentRepo(ctx, links[0]); err != nil {
		t.Fatalf("UpdateTaskEnvironmentRepo: %v", err)
	}
	if err := repo.UpdateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{ID: "missing"}); err == nil {
		t.Fatal("UpdateTaskEnvironmentRepo accepted missing row")
	}
	links, err = repo.ListTaskEnvironmentRepos(ctx, environment.ID)
	if err != nil || links[0].WorktreeBranch != "feature/updated" || links[0].MergedAt == nil {
		t.Fatalf("updated environment repo = %+v, %v", links, err)
	}
	if err := repo.UpdateTaskSessionWorktreeBranch(ctx, "session-environment-crud", "feature/all"); err != nil {
		t.Fatalf("UpdateTaskSessionWorktreeBranch: %v", err)
	}
	branches, err := repo.ListSessionsWithBranches(ctx)
	if err != nil || len(branches) != 1 || branches[0].SessionID != "session-environment-crud" || branches[0].Branch != "feature/all" {
		t.Fatalf("ListSessionsWithBranches = %+v, %v", branches, err)
	}
	if err := repo.DeleteTaskEnvironmentRepo(ctx, links[0].ID); err != nil {
		t.Fatalf("DeleteTaskEnvironmentRepo: %v", err)
	}
	if err := repo.DeleteTaskEnvironmentRepo(ctx, links[0].ID); err == nil {
		t.Fatal("second DeleteTaskEnvironmentRepo returned nil")
	}
	if err := repo.DeleteTaskEnvironmentReposByEnv(ctx, environment.ID); err != nil {
		t.Fatalf("DeleteTaskEnvironmentReposByEnv: %v", err)
	}
	links, err = repo.ListTaskEnvironmentRepos(ctx, environment.ID)
	if err != nil || len(links) != 0 {
		t.Fatalf("links after bulk cleanup = %+v, %v", links, err)
	}
	if err := repo.DeleteTaskEnvironmentsByTask(ctx, environment.TaskID); err != nil {
		t.Fatalf("DeleteTaskEnvironmentsByTask: %v", err)
	}
	if got, err := repo.GetTaskEnvironmentByTaskID(ctx, environment.TaskID); err != nil || got != nil {
		t.Fatalf("environment after bulk delete = %+v, %v", got, err)
	}
}

func TestGetTaskEnvironmentMissingReturnsSentinel(t *testing.T) {
	repo := newRepoForHealTests(t)

	_, err := repo.GetTaskEnvironment(context.Background(), "missing-environment")

	if !errors.Is(err, ErrTaskEnvironmentNotFound) {
		t.Fatalf("GetTaskEnvironment error = %v, want ErrTaskEnvironmentNotFound", err)
	}
}

func TestTaskEnvironment_PersistsDockerBootstrapNonceReference(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-docker-bootstrap")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-docker-bootstrap", WorkspaceID: "workspace-docker-bootstrap", Title: "Docker bootstrap"}); err != nil {
		t.Fatal(err)
	}
	env := &models.TaskEnvironment{
		ID:                                "env-docker-bootstrap",
		TaskID:                            "task-docker-bootstrap",
		ExecutorType:                      string(models.ExecutorTypeLocalDocker),
		Status:                            models.TaskEnvironmentStatusReady,
		ContainerID:                       "container-1",
		ContainerBootstrapNonceSecretID:   "bootstrap-secret-1",
		ContainerControlAuthTokenSecretID: "container-control-secret-1",
	}
	if err := repo.CreateTaskEnvironment(ctx, env); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}

	got, err := repo.GetTaskEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatalf("GetTaskEnvironment: %v", err)
	}
	if got.ContainerBootstrapNonceSecretID != "bootstrap-secret-1" {
		t.Fatalf("bootstrap nonce secret = %q, want bootstrap-secret-1", got.ContainerBootstrapNonceSecretID)
	}
	if got.ContainerControlAuthTokenSecretID != "container-control-secret-1" {
		t.Fatalf("container control secret = %q, want container-control-secret-1", got.ContainerControlAuthTokenSecretID)
	}

	got.ContainerBootstrapNonceSecretID = "bootstrap-secret-2"
	got.ContainerControlAuthTokenSecretID = "container-control-secret-2"
	if err := repo.UpdateTaskEnvironment(ctx, got); err != nil {
		t.Fatalf("UpdateTaskEnvironment: %v", err)
	}
	got, err = repo.GetTaskEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatalf("GetTaskEnvironment after update: %v", err)
	}
	if got.ContainerBootstrapNonceSecretID != "bootstrap-secret-2" {
		t.Fatalf("updated bootstrap nonce secret = %q, want bootstrap-secret-2", got.ContainerBootstrapNonceSecretID)
	}
	if got.ContainerControlAuthTokenSecretID != "container-control-secret-2" {
		t.Fatalf("updated container control secret = %q, want container-control-secret-2", got.ContainerControlAuthTokenSecretID)
	}
}

func TestFinalizeTaskEnvironmentMaterializationPublishesInventoryAtomically(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-finalize-environment")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-finalize-environment", WorkspaceID: "workspace-finalize-environment", Title: "Finalize environment"}); err != nil {
		t.Fatal(err)
	}
	env := &models.TaskEnvironment{
		ID:                       "env-finalize",
		TaskID:                   "task-finalize-environment",
		ExecutorType:             string(models.ExecutorTypeWorktree),
		Status:                   models.TaskEnvironmentStatusCreating,
		MaterializationSessionID: "session-materializer",
	}
	if err := repo.CreateTaskEnvironment(ctx, env); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	env.Status = models.TaskEnvironmentStatusReady
	env.MaterializationSessionID = ""
	env.WorkspacePath = "/tasks/task-finalize/repo"
	if err := repo.FinalizeTaskEnvironmentMaterialization(ctx, env, []*models.TaskEnvironmentRepo{{
		RepositoryID: "repository-1", WorktreeID: "worktree-1", WorktreePath: env.WorkspacePath, WorktreeBranch: "feature/finalize",
	}}, "session-materializer"); err != nil {
		t.Fatalf("FinalizeTaskEnvironmentMaterialization: %v", err)
	}

	persisted, err := repo.GetTaskEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatalf("GetTaskEnvironment: %v", err)
	}
	if persisted.Status != models.TaskEnvironmentStatusReady || persisted.MaterializationSessionID != "" {
		t.Fatalf("environment = status %q owner %q, want ready with no owner", persisted.Status, persisted.MaterializationSessionID)
	}
	if len(persisted.Repos) != 1 || persisted.Repos[0].WorktreeID != "worktree-1" {
		t.Fatalf("canonical inventory = %#v, want one finalized repository", persisted.Repos)
	}
}

func TestCreateTaskSessionWithWorkspaceBindingElectsAndAttaches(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-binding")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-binding", WorkspaceID: "workspace-binding", Title: "binding"}); err != nil {
		t.Fatal(err)
	}

	first := &models.TaskSession{ID: "session-owner", TaskID: "task-binding"}
	candidate := &models.TaskEnvironment{TaskID: "task-binding", ExecutorType: string(models.ExecutorTypeLocal)}
	if err := repo.CreateTaskSessionWithWorkspaceBinding(ctx, first, candidate); err != nil {
		t.Fatalf("bind first session: %v", err)
	}
	if first.TaskEnvironmentID == "" || first.TaskEnvironmentID != candidate.ID {
		t.Fatalf("first binding = %q, candidate = %q", first.TaskEnvironmentID, candidate.ID)
	}
	env, err := repo.GetTaskEnvironment(ctx, first.TaskEnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	if env.Status != models.TaskEnvironmentStatusCreating || env.MaterializationSessionID != first.ID {
		t.Fatalf("creating environment = %+v", env)
	}

	blocked := &models.TaskSession{ID: "session-blocked", TaskID: "task-binding"}
	err = repo.CreateTaskSessionWithWorkspaceBinding(ctx, blocked, &models.TaskEnvironment{TaskID: "task-binding"})
	if !errors.Is(err, models.ErrWorkspacePreparing) {
		t.Fatalf("second bind error = %v, want preparing", err)
	}
	sessions, err := repo.ListTaskSessions(ctx, "task-binding")
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions after blocked bind = %+v, %v", sessions, err)
	}

	env.Status = models.TaskEnvironmentStatusReady
	env.MaterializationSessionID = ""
	if err := repo.UpdateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	attached := &models.TaskSession{ID: "session-attached", TaskID: "task-binding"}
	if err := repo.CreateTaskSessionWithWorkspaceBinding(ctx, attached, &models.TaskEnvironment{TaskID: "task-binding"}); err != nil {
		t.Fatalf("attach ready environment: %v", err)
	}
	if attached.TaskEnvironmentID != env.ID {
		t.Fatalf("attached environment = %q, want %q", attached.TaskEnvironmentID, env.ID)
	}
}

func TestCreateTaskSessionWithWorkspaceBindingConcurrentElectionHasOneOwner(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-binding-race")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-binding-race", WorkspaceID: "workspace-binding-race", Title: "binding"}); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"session-race-a", "session-race-b"} {
		wg.Add(1)
		go func(sessionID string) {
			defer wg.Done()
			<-start
			errs <- repo.CreateTaskSessionWithWorkspaceBinding(ctx,
				&models.TaskSession{ID: sessionID, TaskID: "task-binding-race"},
				&models.TaskEnvironment{TaskID: "task-binding-race", ExecutorType: string(models.ExecutorTypeLocal)})
		}(id)
	}
	close(start)
	wg.Wait()
	close(errs)

	var succeeded, preparing int
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, models.ErrWorkspacePreparing):
			preparing++
		default:
			t.Fatalf("concurrent binding error = %v", err)
		}
	}
	if succeeded != 1 || preparing != 1 {
		t.Fatalf("election results: success=%d preparing=%d, want one each", succeeded, preparing)
	}
	sessions, err := repo.ListTaskSessions(ctx, "task-binding-race")
	if err != nil || len(sessions) != 1 {
		t.Fatalf("persisted sessions = %+v, %v", sessions, err)
	}
}

func TestCreateTaskSessionWithSharedGroupWorkspaceBindingElectsOneMaterializer(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-group-binding")
	for _, taskID := range []string{"task-group-a", "task-group-b"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "workspace-group-binding", Title: taskID}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.db.Exec(`
		CREATE TABLE task_workspace_groups (
			id TEXT PRIMARY KEY,
			materialized_environment_id TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE task_workspace_group_members (
			workspace_group_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			released_at TIMESTAMP NULL
		);
		INSERT INTO task_workspace_groups (id, updated_at) VALUES ('group-binding', CURRENT_TIMESTAMP);
		INSERT INTO task_workspace_group_members (workspace_group_id, task_id) VALUES
			('group-binding', 'task-group-a'), ('group-binding', 'task-group-b');
	`); err != nil {
		t.Fatalf("seed workspace group: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, taskID := range []string{"task-group-a", "task-group-b"} {
		wg.Add(1)
		go func(taskID string) {
			defer wg.Done()
			<-start
			errs <- repo.CreateTaskSessionWithSharedGroupWorkspaceBinding(ctx,
				&models.TaskSession{ID: "session-" + taskID, TaskID: taskID},
				&models.TaskEnvironment{TaskID: taskID, ExecutorType: string(models.ExecutorTypeLocal)},
				"group-binding")
		}(taskID)
	}
	close(start)
	wg.Wait()
	close(errs)

	var succeeded, preparing int
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, models.ErrWorkspacePreparing):
			preparing++
		default:
			t.Fatalf("concurrent group binding error = %v", err)
		}
	}
	if succeeded != 1 || preparing != 1 {
		t.Fatalf("group election results: success=%d preparing=%d, want one each", succeeded, preparing)
	}
	var environmentID string
	if err := repo.db.Get(&environmentID, `SELECT materialized_environment_id FROM task_workspace_groups WHERE id = 'group-binding'`); err != nil {
		t.Fatal(err)
	}
	if environmentID == "" {
		t.Fatal("shared group has no elected environment")
	}
	var sessionCount int
	if err := repo.db.Get(&sessionCount, `SELECT COUNT(*) FROM task_sessions WHERE task_id IN ('task-group-a', 'task-group-b')`); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 1 {
		t.Fatalf("group sessions = %d, want 1", sessionCount)
	}
	environment, err := repo.GetTaskEnvironment(ctx, environmentID)
	if err != nil {
		t.Fatal(err)
	}
	environment.Status = models.TaskEnvironmentStatusReady
	environment.MaterializationSessionID = ""
	if err := repo.UpdateTaskEnvironment(ctx, environment); err != nil {
		t.Fatal(err)
	}
	follower := &models.TaskSession{ID: "session-group-follower", TaskID: "task-group-b"}
	if err := repo.CreateTaskSessionWithSharedGroupWorkspaceBinding(ctx, follower, &models.TaskEnvironment{
		TaskID: "task-group-b", ExecutorType: string(models.ExecutorTypeLocal),
	}, "group-binding"); err != nil {
		t.Fatalf("bind shared group follower: %v", err)
	}
	if follower.TaskEnvironmentID != environmentID {
		t.Fatalf("follower environment = %q, want %q", follower.TaskEnvironmentID, environmentID)
	}
}

func TestCreateTaskSessionWithWorkspaceBindingFailsClosedForAbandonedCreatingClaim(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-binding-abandoned")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-binding-abandoned", WorkspaceID: "workspace-binding-abandoned", Title: "binding"}); err != nil {
		t.Fatal(err)
	}

	environment := &models.TaskEnvironment{
		ID:                       "environment-abandoned",
		TaskID:                   "task-binding-abandoned",
		ExecutorType:             string(models.ExecutorTypeLocal),
		Status:                   models.TaskEnvironmentStatusCreating,
		MaterializationSessionID: "missing-owner",
	}
	if err := repo.CreateTaskEnvironment(ctx, environment); err != nil {
		t.Fatalf("create abandoned environment: %v", err)
	}

	err := repo.CreateTaskSessionWithWorkspaceBinding(ctx,
		&models.TaskSession{ID: "session-retry", TaskID: "task-binding-abandoned"},
		&models.TaskEnvironment{TaskID: "task-binding-abandoned", ExecutorType: string(models.ExecutorTypeLocal)})
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("bind after abandoned claim error = %v, want reuse unsafe", err)
	}

	got, err := repo.GetTaskEnvironment(ctx, environment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.TaskEnvironmentStatusFailed || got.MaterializationSessionID != "" {
		t.Fatalf("abandoned environment = %+v, want failed without owner", got)
	}
	sessions, err := repo.ListTaskSessions(ctx, environment.TaskID)
	if err != nil || len(sessions) != 0 {
		t.Fatalf("sessions after failed recovery = %+v, %v", sessions, err)
	}
}

func TestCreateTaskSessionWithWorkspaceBindingFailsClosedForOwnerlessCreatingClaim(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-binding-ownerless")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-binding-ownerless", WorkspaceID: "workspace-binding-ownerless", Title: "binding"}); err != nil {
		t.Fatal(err)
	}

	environment := &models.TaskEnvironment{
		ID:           "environment-ownerless",
		TaskID:       "task-binding-ownerless",
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusCreating,
	}
	if err := repo.CreateTaskEnvironment(ctx, environment); err != nil {
		t.Fatalf("create ownerless environment: %v", err)
	}

	err := repo.CreateTaskSessionWithWorkspaceBinding(ctx,
		&models.TaskSession{ID: "session-after-ownerless", TaskID: environment.TaskID},
		&models.TaskEnvironment{TaskID: environment.TaskID, ExecutorType: string(models.ExecutorTypeLocal)})
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("bind after ownerless claim error = %v, want reuse unsafe", err)
	}

	got, err := repo.GetTaskEnvironment(ctx, environment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.TaskEnvironmentStatusFailed || got.MaterializationSessionID != "" {
		t.Fatalf("ownerless environment = %+v, want failed without owner", got)
	}
}

func TestCreateTaskSessionWithWorkspaceBindingFailsClosedForTerminalCreatingOwner(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-binding-terminal-owner")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-binding-terminal-owner", WorkspaceID: "workspace-binding-terminal-owner", Title: "binding"}); err != nil {
		t.Fatal(err)
	}

	owner := &models.TaskSession{ID: "terminal-owner", TaskID: "task-binding-terminal-owner"}
	if err := repo.CreateTaskSessionWithWorkspaceBinding(ctx, owner, &models.TaskEnvironment{
		TaskID:       owner.TaskID,
		ExecutorType: string(models.ExecutorTypeLocal),
	}); err != nil {
		t.Fatalf("create materialization owner: %v", err)
	}
	if err := repo.UpdateTaskSessionState(ctx, owner.ID, models.TaskSessionStateFailed, "launch failed"); err != nil {
		t.Fatal(err)
	}

	err := repo.CreateTaskSessionWithWorkspaceBinding(ctx,
		&models.TaskSession{ID: "session-after-terminal-owner", TaskID: owner.TaskID},
		&models.TaskEnvironment{TaskID: owner.TaskID, ExecutorType: string(models.ExecutorTypeLocal)})
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("bind after terminal owner error = %v, want reuse unsafe", err)
	}
	environment, err := repo.GetTaskEnvironment(ctx, owner.TaskEnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	if environment.Status != models.TaskEnvironmentStatusFailed || environment.MaterializationSessionID != "" {
		t.Fatalf("terminal-owner environment = %+v, want failed without owner", environment)
	}
}

func TestUpdateTaskEnvironmentMissingReturnsSentinel(t *testing.T) {
	repo := newRepoForHealTests(t)

	err := repo.UpdateTaskEnvironment(context.Background(), &models.TaskEnvironment{ID: "missing-environment"})

	if !errors.Is(err, ErrTaskEnvironmentNotFound) {
		t.Fatalf("UpdateTaskEnvironment error = %v, want ErrTaskEnvironmentNotFound", err)
	}
}

func TestTransferTaskEnvironmentMissingReturnsSentinel(t *testing.T) {
	repo := newRepoForHealTests(t)

	err := repo.TransferTaskEnvironmentToTask(context.Background(), "missing-environment", "task-1")

	if !errors.Is(err, ErrTaskEnvironmentNotFound) {
		t.Fatalf("TransferTaskEnvironmentToTask error = %v, want ErrTaskEnvironmentNotFound", err)
	}
}
