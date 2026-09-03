package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-MISSING-WORKSPACE-REHOME-001.3
func TestClaimTaskEnvironmentRehomeIsAtomicUnderConcurrency(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-rehome")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-rehome", WorkspaceID: "workspace-rehome", Title: "Rehome"}); err != nil {
		t.Fatal(err)
	}
	env := &models.TaskEnvironment{ID: "env-rehome", TaskID: "task-rehome", ExecutorType: string(models.ExecutorTypeSSH), Status: models.TaskEnvironmentStatusReady, WorkspacePath: "/remote/missing", TaskDirName: "task-rehome"}
	if err := repo.CreateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}

	const callers = 8
	var wg sync.WaitGroup
	results := make(chan bool, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := repo.ClaimTaskEnvironmentRehome(ctx, env.TaskID, env.ID, "session-rehome", true)
			if err != nil {
				t.Errorf("ClaimTaskEnvironmentRehome: %v", err)
				return
			}
			results <- claimed
		}()
	}
	wg.Wait()
	close(results)
	claims := 0
	for claimed := range results {
		if claimed {
			claims++
		}
	}
	if claims != 1 {
		t.Fatalf("claims = %d, want exactly 1", claims)
	}
	got, err := repo.GetTaskEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.TaskEnvironmentStatusCreating || got.MaterializationSessionID != "session-rehome" || got.WorkspacePath != "" {
		t.Fatalf("rehome environment = %+v", got)
	}
}

// @covers AC-TASKS-MISSING-WORKSPACE-REHOME-002.1
func TestClaimTaskEnvironmentRehomeRequiresLossAuthorizationWithoutSafeSnapshot(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-rehome-loss")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-rehome-loss", WorkspaceID: "workspace-rehome-loss", Title: "Loss"}); err != nil {
		t.Fatal(err)
	}
	env := &models.TaskEnvironment{ID: "env-rehome-loss", TaskID: "task-rehome-loss", ExecutorType: string(models.ExecutorTypeSSH), Status: models.TaskEnvironmentStatusReady, WorkspacePath: "/remote/missing"}
	if err := repo.CreateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}

	claimed, err := repo.ClaimTaskEnvironmentRehome(ctx, env.TaskID, env.ID, "session-loss", false)
	if claimed || !errors.Is(err, models.ErrWorkspaceRehomeNeedsAuthorization) {
		t.Fatalf("claim = %v, error = %v; want authorization required", claimed, err)
	}
	got, getErr := repo.GetTaskEnvironment(ctx, env.ID)
	if getErr != nil || got.Status != models.TaskEnvironmentStatusReady || got.WorkspacePath != env.WorkspacePath {
		t.Fatalf("blocked claim mutated environment: %+v, %v", got, getErr)
	}
}

func TestClaimTaskEnvironmentRehomeDoesNotAutomaticallyDiscardUniqueWork(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-rehome-unique")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-rehome-unique", WorkspaceID: "workspace-rehome-unique", Title: "Unique"}); err != nil {
		t.Fatal(err)
	}
	env := &models.TaskEnvironment{ID: "env-rehome-unique", TaskID: "task-rehome-unique", ExecutorType: string(models.ExecutorTypeSSH), Status: models.TaskEnvironmentStatusReady, WorkspacePath: "/remote/missing"}
	if err := repo.CreateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{TaskEnvironmentID: env.ID, SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "feature/task", RemoteBranch: "origin/feature/task", HeadCommit: "abc", Ahead: 1, Files: map[string]interface{}{"new.txt": "untracked"}}); err != nil {
		t.Fatal(err)
	}

	claimed, err := repo.ClaimTaskEnvironmentRehome(ctx, env.TaskID, env.ID, "session-unique", false)
	if claimed || !errors.Is(err, models.ErrWorkspaceRehomeNeedsAuthorization) {
		t.Fatalf("claim = %v, error = %v; want explicit data-loss authorization", claimed, err)
	}
}

// @covers AC-TASKS-MISSING-WORKSPACE-REHOME-001.1
func TestClaimTaskEnvironmentRehomeAllowsAutomaticRecoveryWithCleanPushedSnapshot(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-rehome-safe")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-rehome-safe", WorkspaceID: "workspace-rehome-safe", Title: "Safe"}); err != nil {
		t.Fatal(err)
	}
	env := &models.TaskEnvironment{ID: "env-rehome-safe", TaskID: "task-rehome-safe", ExecutorType: string(models.ExecutorTypeSSH), Status: models.TaskEnvironmentStatusReady, WorkspacePath: "/remote/missing"}
	if err := repo.CreateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	seedRehomeSession(t, repo, env.TaskID, env.ID, "session-safe")
	seedRehomeRepository(t, repo, "workspace-rehome-safe", env.ID, "repo-safe", 0)
	if err := repo.CreateGitSnapshot(ctx, cleanCompletionSnapshot(env.ID, "session-safe", "repo-safe")); err != nil {
		t.Fatal(err)
	}

	claimed, err := repo.ClaimTaskEnvironmentRehome(ctx, env.TaskID, env.ID, "session-safe", false)
	if err != nil || !claimed {
		t.Fatalf("claim = %v, error = %v; want automatic recovery", claimed, err)
	}
}

func TestClaimTaskEnvironmentRehomeChecksEveryRepository(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-rehome-multi")
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-rehome-multi", WorkspaceID: "workspace-rehome-multi", Title: "Multi"}); err != nil {
		t.Fatal(err)
	}
	env := &models.TaskEnvironment{ID: "env-rehome-multi", TaskID: "task-rehome-multi", Status: models.TaskEnvironmentStatusReady}
	if err := repo.CreateTaskEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	seedRehomeSession(t, repo, env.TaskID, env.ID, "session-multi")
	seedRehomeRepository(t, repo, "workspace-rehome-multi", env.ID, "repo-clean", 0)
	seedRehomeRepository(t, repo, "workspace-rehome-multi", env.ID, "repo-dirty", 1)
	if err := repo.CreateGitSnapshot(ctx, cleanCompletionSnapshot(env.ID, "session-multi", "repo-clean")); err != nil {
		t.Fatal(err)
	}
	dirty := cleanCompletionSnapshot(env.ID, "session-multi", "repo-dirty")
	dirty.Ahead = 1
	dirty.Files = map[string]interface{}{"unique.txt": "untracked"}
	if err := repo.CreateGitSnapshot(ctx, dirty); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimTaskEnvironmentRehome(ctx, env.TaskID, env.ID, "session-multi", false)
	if claimed || !errors.Is(err, models.ErrWorkspaceRehomeNeedsAuthorization) {
		t.Fatalf("claim = %v, error = %v; want authorization for dirty sibling repository", claimed, err)
	}
}

func TestClaimTaskEnvironmentRehomeFailsClosedOnIncompleteOrStaleInventory(t *testing.T) {
	for _, tc := range []struct {
		name      string
		sessionID string
		live      bool
	}{
		{name: "snapshot belongs to previous session", sessionID: "previous-session"},
		{name: "newest observation is incomplete live snapshot", sessionID: "current-session", live: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			ctx := context.Background()
			workspaceID := "workspace-" + tc.sessionID
			taskID := "task-" + tc.sessionID
			envID := "env-" + tc.sessionID
			seedWorkspace(t, repo, workspaceID)
			if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Inventory"}); err != nil {
				t.Fatal(err)
			}
			env := &models.TaskEnvironment{ID: envID, TaskID: taskID, Status: models.TaskEnvironmentStatusReady}
			if err := repo.CreateTaskEnvironment(ctx, env); err != nil {
				t.Fatal(err)
			}
			seedRehomeSession(t, repo, taskID, envID, tc.sessionID)
			if tc.sessionID != "current-session" {
				seedRehomeSession(t, repo, taskID, envID, "current-session")
			}
			seedRehomeRepository(t, repo, workspaceID, env.ID, "repo", 0)
			if err := repo.CreateGitSnapshot(ctx, cleanCompletionSnapshot(env.ID, tc.sessionID, "repo")); err != nil {
				t.Fatal(err)
			}
			if tc.live {
				if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{TaskEnvironmentID: env.ID, SessionID: tc.sessionID, TriggeredBy: TriggeredByLiveMonitor, Metadata: map[string]interface{}{"repository_name": "repo"}, Files: map[string]interface{}{}}); err != nil {
					t.Fatal(err)
				}
			}
			claimed, err := repo.ClaimTaskEnvironmentRehome(ctx, taskID, envID, "current-session", false)
			if claimed || !errors.Is(err, models.ErrWorkspaceRehomeNeedsAuthorization) {
				t.Fatalf("claim = %v, error = %v; want authorization", claimed, err)
			}
		})
	}
}

func seedRehomeSession(t *testing.T, repo *Repository, taskID, environmentID, sessionID string) {
	t.Helper()
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateCompleted,
	}); err != nil {
		t.Fatal(err)
	}
}

func seedRehomeRepository(t *testing.T, repo *Repository, workspaceID, environmentID, repositoryID string, position int) {
	t.Helper()
	if err := repo.CreateRepository(context.Background(), &models.Repository{ID: repositoryID, WorkspaceID: workspaceID, Name: repositoryID}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironmentRepo(context.Background(), &models.TaskEnvironmentRepo{
		TaskEnvironmentID: environmentID, RepositoryID: repositoryID, Position: position,
	}); err != nil {
		t.Fatal(err)
	}
}

func cleanCompletionSnapshot(environmentID, sessionID, repositoryName string) *models.GitSnapshot {
	return &models.GitSnapshot{
		TaskEnvironmentID: environmentID, SessionID: sessionID, TriggeredBy: triggeredByAgentCompleted,
		SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "feature/task", RemoteBranch: "origin/feature/task",
		HeadCommit: "abc", Files: map[string]interface{}{}, Metadata: map[string]interface{}{"repository_name": repositoryName},
	}
}
