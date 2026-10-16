package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

func seedMaterializationClaimTask(t *testing.T, repo *Repository, taskID string) {
	t.Helper()
	seedWorkspace(t, repo, "workspace-"+taskID)
	if err := repo.CreateTask(context.Background(), &models.Task{ID: taskID, WorkspaceID: "workspace-" + taskID, Title: taskID}); err != nil {
		t.Fatal(err)
	}
}

// seedCreatingOwner elects ownerID as the task's first materializer and leaves
// its session in state.
func seedCreatingOwner(t *testing.T, repo *Repository, taskID, ownerID string, state models.TaskSessionState) *models.TaskSession {
	t.Helper()
	ctx := context.Background()
	owner := &models.TaskSession{ID: ownerID, TaskID: taskID}
	if err := repo.CreateTaskSessionWithWorkspaceBinding(ctx, owner, &models.TaskEnvironment{
		TaskID: taskID, ExecutorType: string(models.ExecutorTypeLocal),
	}); err != nil {
		t.Fatalf("elect owner: %v", err)
	}
	if state != models.TaskSessionStateCreated {
		if err := repo.UpdateTaskSessionState(ctx, ownerID, state, ""); err != nil {
			t.Fatal(err)
		}
	}
	return owner
}

func ageTaskEnvironment(t *testing.T, repo *Repository, environmentID string, age time.Duration) {
	t.Helper()
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_environments SET updated_at = ? WHERE id = ?`),
		time.Now().UTC().Add(-age), environmentID); err != nil {
		t.Fatal(err)
	}
}

func bindRetry(repo *Repository, taskID, sessionID string) (*models.TaskSession, error) {
	session := &models.TaskSession{ID: sessionID, TaskID: taskID}
	err := repo.CreateTaskSessionWithWorkspaceBinding(context.Background(), session, &models.TaskEnvironment{
		TaskID: taskID, ExecutorType: string(models.ExecutorTypeLocal),
	})
	return session, err
}

func assertCreatingOwner(t *testing.T, repo *Repository, environmentID, ownerID string) {
	t.Helper()
	environment, err := repo.GetTaskEnvironment(context.Background(), environmentID)
	if err != nil {
		t.Fatal(err)
	}
	if environment.Status != models.TaskEnvironmentStatusCreating || environment.MaterializationSessionID != ownerID {
		t.Fatalf("environment = status %q owner %q, want creating owned by %q",
			environment.Status, environment.MaterializationSessionID, ownerID)
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.2
func TestWorkspaceBindingTakesOverStaleClaimOfSilentOwner(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedMaterializationClaimTask(t, repo, "task-stale-claim")
	owner := seedCreatingOwner(t, repo, "task-stale-claim", "session-silent-owner", models.TaskSessionStateCreated)
	ageTaskEnvironment(t, repo, owner.TaskEnvironmentID, 24*time.Hour)

	retry, err := bindRetry(repo, "task-stale-claim", "session-after-silent-owner")
	if err != nil {
		t.Fatalf("bind after silent owner: %v", err)
	}
	assertTakenOverMaterialization(t, repo, owner.TaskEnvironmentID, retry)
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.3
func TestWorkspaceBindingKeepsYoungClaimOfSilentOwner(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedMaterializationClaimTask(t, repo, "task-young-claim")
	owner := seedCreatingOwner(t, repo, "task-young-claim", "session-young-owner", models.TaskSessionStateCreated)

	_, err := bindRetry(repo, "task-young-claim", "session-after-young-owner")
	if !errors.Is(err, models.ErrWorkspacePreparing) {
		t.Fatalf("bind against young claim error = %v, want workspace preparing", err)
	}
	assertCreatingOwner(t, repo, owner.TaskEnvironmentID, owner.ID)
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.3
func TestWorkspaceBindingKeepsStaleClaimOfLiveOwner(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedMaterializationClaimTask(t, repo, "task-live-claim")
	owner := seedCreatingOwner(t, repo, "task-live-claim", "session-live-owner", models.TaskSessionStateRunning)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running-live-owner", SessionID: owner.ID, TaskID: owner.TaskID,
		ExecutorID: "executor-live-owner", AgentExecutionID: "execution-live-owner", Status: "running",
	}); err != nil {
		t.Fatal(err)
	}
	ageTaskEnvironment(t, repo, owner.TaskEnvironmentID, 24*time.Hour)

	_, err := bindRetry(repo, "task-live-claim", "session-after-live-owner")
	if !errors.Is(err, models.ErrWorkspacePreparing) {
		t.Fatalf("bind against live owner error = %v, want workspace preparing", err)
	}
	assertCreatingOwner(t, repo, owner.TaskEnvironmentID, owner.ID)
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.2
func TestWorkspaceBindingFailsAbandonedClaimWithPhysicalEvidence(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedMaterializationClaimTask(t, repo, "task-evidence-claim")
	owner := seedCreatingOwner(t, repo, "task-evidence-claim", "session-evidence-owner", models.TaskSessionStateFailed)
	if err := repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		TaskEnvironmentID: owner.TaskEnvironmentID, RepositoryID: "repository-evidence",
		WorktreeID: "worktree-evidence", WorktreePath: "/tasks/evidence/repo", Status: "active",
	}); err != nil {
		t.Fatal(err)
	}

	_, err := bindRetry(repo, "task-evidence-claim", "session-after-evidence-owner")
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("bind against abandoned claim with a worktree error = %v, want reuse unsafe", err)
	}
	environment, err := repo.GetTaskEnvironment(ctx, owner.TaskEnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	if environment.Status != models.TaskEnvironmentStatusFailed || environment.MaterializationSessionID != "" {
		t.Fatalf("environment = status %q owner %q, want failed without owner", environment.Status, environment.MaterializationSessionID)
	}
	if _, err := repo.GetTaskSession(ctx, "session-after-evidence-owner"); err == nil {
		t.Fatal("refused binding persisted a session")
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.6
func TestWorkspaceBindingConcurrentTakeOverHasOneOwner(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedMaterializationClaimTask(t, repo, "task-takeover-race")
	owner := seedCreatingOwner(t, repo, "task-takeover-race", "session-race-owner", models.TaskSessionStateFailed)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"session-takeover-a", "session-takeover-b"} {
		wg.Add(1)
		go func(sessionID string) {
			defer wg.Done()
			<-start
			_, err := bindRetry(repo, "task-takeover-race", sessionID)
			errs <- err
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
			t.Fatalf("concurrent take-over error = %v", err)
		}
	}
	if succeeded != 1 || preparing != 1 {
		t.Fatalf("take-over results: success=%d preparing=%d, want one each", succeeded, preparing)
	}
	environment, err := repo.GetTaskEnvironment(context.Background(), owner.TaskEnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	if environment.MaterializationSessionID != "session-takeover-a" && environment.MaterializationSessionID != "session-takeover-b" {
		t.Fatalf("environment owner = %q, want one of the racing sessions", environment.MaterializationSessionID)
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.9
func TestWorkspaceBindingDoesNotTakeOverClaimHeldByRecovery(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const (
		taskID        = "task-claim-recovery"
		environmentID = "environment-claim-recovery"
	)
	seedRecoveryClaimEnvironment(t, repo, taskID, environmentID)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-claim-recovery", TaskID: taskID, TaskEnvironmentID: environmentID,
		State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatal(err)
	}
	claim, err := repo.AcquireTaskEnvironmentRecoveryClaim(ctx, recoveryClaimRequest(
		environmentID, taskID, "session-claim-recovery", "operation-claim-recovery", 1,
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim) })

	_, err = bindRetry(repo, taskID, "session-after-claim-recovery")
	if !errors.Is(err, recoveryclaim.ErrBusy) {
		t.Fatalf("bind during recovery claim error = %v, want recovery claim busy", err)
	}
	assertCreatingOwner(t, repo, environmentID, "")
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.2
func TestSharedGroupBindingTakesOverAbandonedGroupClaim(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-group-takeover")
	for _, taskID := range []string{"task-group-takeover-a", "task-group-takeover-b"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "workspace-group-takeover", Title: taskID}); err != nil {
			t.Fatal(err)
		}
	}
	seedSharedWorkspaceGroup(t, repo, "group-takeover", "task-group-takeover-a", "task-group-takeover-b")
	owner := &models.TaskSession{ID: "session-group-owner", TaskID: "task-group-takeover-a"}
	if err := repo.CreateTaskSessionWithSharedGroupWorkspaceBinding(ctx, owner, &models.TaskEnvironment{
		TaskID: owner.TaskID, ExecutorType: string(models.ExecutorTypeLocal),
	}, "group-takeover"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateTaskSessionState(ctx, owner.ID, models.TaskSessionStateFailed, ""); err != nil {
		t.Fatal(err)
	}

	member := &models.TaskSession{ID: "session-group-member", TaskID: "task-group-takeover-b"}
	if err := repo.CreateTaskSessionWithSharedGroupWorkspaceBinding(ctx, member, &models.TaskEnvironment{
		TaskID: member.TaskID, ExecutorType: string(models.ExecutorTypeLocal),
	}, "group-takeover"); err != nil {
		t.Fatalf("bind group member after abandoned claim: %v", err)
	}
	if member.TaskEnvironmentID != owner.TaskEnvironmentID {
		t.Fatalf("member environment = %q, want group environment %q", member.TaskEnvironmentID, owner.TaskEnvironmentID)
	}
	assertCreatingOwner(t, repo, owner.TaskEnvironmentID, member.ID)
	var pointer string
	if err := repo.db.Get(&pointer, `SELECT materialized_environment_id FROM task_workspace_groups WHERE id = 'group-takeover'`); err != nil {
		t.Fatal(err)
	}
	if pointer != owner.TaskEnvironmentID {
		t.Fatalf("group pointer = %q, want %q", pointer, owner.TaskEnvironmentID)
	}
}

func seedSharedWorkspaceGroup(t *testing.T, repo *Repository, groupID string, taskIDs ...string) {
	t.Helper()
	if _, err := repo.db.Exec(`
		CREATE TABLE IF NOT EXISTS task_workspace_groups (
			id TEXT PRIMARY KEY,
			materialized_environment_id TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE IF NOT EXISTS task_workspace_group_members (
			workspace_group_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			released_at TIMESTAMP NULL
		);
	`); err != nil {
		t.Fatalf("create workspace group tables: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO task_workspace_groups (id, updated_at) VALUES (?, CURRENT_TIMESTAMP)`), groupID); err != nil {
		t.Fatal(err)
	}
	for _, taskID := range taskIDs {
		if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO task_workspace_group_members (workspace_group_id, task_id) VALUES (?, ?)`), groupID, taskID); err != nil {
			t.Fatal(err)
		}
	}
}
