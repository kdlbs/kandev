package sqlite

import (
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// seedDuplicateWorktreeEnvironments drops the active-task unique index and
// seeds an older loser and a newer ready winner for one worktree task.
func seedDuplicateWorktreeEnvironments(t *testing.T, repo *Repository, taskID string) (string, string) {
	t.Helper()
	if _, err := repo.db.Exec(`DROP INDEX IF EXISTS uniq_task_environments_task_id`); err != nil {
		t.Fatalf("drop index: %v", err)
	}
	insertTask(t, repo.db, taskID)
	older := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	loser, winner := taskID+"-env-old", taskID+"-env-new"
	insertEnv(t, repo.db, &models.TaskEnvironment{
		ID: loser, TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		WorkspacePath: "/work/" + taskID, CreatedAt: older, UpdatedAt: older,
	})
	insertEnv(t, repo.db, &models.TaskEnvironment{
		ID: winner, TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		WorkspacePath: "/work/" + taskID, CreatedAt: older.Add(time.Second), UpdatedAt: older.Add(time.Second),
	})
	// insertEnv leaves executor_type empty; the preservation proof compares
	// the stored worktree executor identity, so record it explicitly.
	if _, err := repo.db.Exec(`UPDATE task_environments SET executor_type = ?, executor_id = '' WHERE task_id = ?`,
		string(models.ExecutorTypeWorktree), taskID); err != nil {
		t.Fatalf("set executor identity: %v", err)
	}
	return loser, winner
}

func insertEnvironmentRepo(t *testing.T, repo *Repository, id, environmentID, worktreePath string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(`
		INSERT INTO task_environment_repos (id, task_environment_id, repository_id, worktree_path, created_at, updated_at)
		VALUES (?, ?, 'repo-1', ?, ?, ?)
	`, id, environmentID, worktreePath, now, now); err != nil {
		t.Fatalf("insert environment repo: %v", err)
	}
}

func countEnvironmentRows(t *testing.T, repo *Repository, taskID string) int {
	t.Helper()
	var n int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM task_environments WHERE task_id = ?`, taskID).Scan(&n); err != nil {
		t.Fatalf("count environments: %v", err)
	}
	return n
}

func TestHealDuplicateTaskEnvironments_TransfersCompatibleLoserInventory(t *testing.T) {
	repo := newRepoForHealTests(t)
	loser, winner := seedDuplicateWorktreeEnvironments(t, repo, "task-dup-inventory")
	insertEnvironmentRepo(t, repo, "repo-row-old", loser, "/work/task-dup-inventory")

	if err := repo.healDuplicateTaskEnvironments(); err != nil {
		t.Fatalf("heal: %v", err)
	}
	if got := countEnvironmentRows(t, repo, "task-dup-inventory"); got != 1 {
		t.Fatalf("environments after heal = %d, want 1", got)
	}
	var owner string
	if err := repo.db.QueryRow(`SELECT task_environment_id FROM task_environment_repos WHERE id = 'repo-row-old'`).Scan(&owner); err != nil {
		t.Fatalf("read transferred inventory: %v", err)
	}
	if owner != winner {
		t.Fatalf("inventory owner = %q, want the canonical environment %q with its row ID kept", owner, winner)
	}
}

func TestHealDuplicateTaskEnvironments_StopsOnLoserResourceHandle(t *testing.T) {
	repo := newRepoForHealTests(t)
	loser, _ := seedDuplicateWorktreeEnvironments(t, repo, "task-dup-resource")
	if _, err := repo.db.Exec(`UPDATE task_environments SET container_id = 'container-1' WHERE id = ?`, loser); err != nil {
		t.Fatalf("seed resource handle: %v", err)
	}

	err := repo.healDuplicateTaskEnvironments()
	if !errors.Is(err, errDuplicateEnvironmentPreservation) {
		t.Fatalf("heal error = %v, want the explicit preservation stop", err)
	}
	if got := countEnvironmentRows(t, repo, "task-dup-resource"); got != 2 {
		t.Fatalf("environments after refused heal = %d, want both rows kept by the rollback", got)
	}
}

func TestHealDuplicateTaskEnvironments_StopsOnLoserRecoveryClaim(t *testing.T) {
	repo := newRepoForHealTests(t)
	loser, _ := seedDuplicateWorktreeEnvironments(t, repo, "task-dup-claim")
	now := time.Now().UTC()
	if _, err := repo.db.Exec(`
		INSERT INTO task_environment_recovery_claims
			(task_environment_id, owner_task_id, ownership_generation, session_id, operation_id, executor_type, created_at, updated_at)
		VALUES (?, 'task-dup-claim', 1, 'session-1', 'operation-1', 'worktree', ?, ?)
	`, loser, now, now); err != nil {
		t.Fatalf("seed recovery claim: %v", err)
	}

	if err := repo.healDuplicateTaskEnvironments(); !errors.Is(err, errDuplicateEnvironmentPreservation) {
		t.Fatalf("heal error = %v, want the explicit preservation stop", err)
	}
	var claims int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM task_environment_recovery_claims WHERE task_environment_id = ?`, loser).Scan(&claims); err != nil {
		t.Fatalf("count claims: %v", err)
	}
	if claims != 1 || countEnvironmentRows(t, repo, "task-dup-claim") != 2 {
		t.Fatalf("claims = %d, environments = %d; want the claim and both rows kept", claims, countEnvironmentRows(t, repo, "task-dup-claim"))
	}
}

func TestHealDuplicateTaskEnvironments_StopsWhenTargetAlreadyHasInventory(t *testing.T) {
	repo := newRepoForHealTests(t)
	loser, winner := seedDuplicateWorktreeEnvironments(t, repo, "task-dup-target")
	insertEnvironmentRepo(t, repo, "repo-row-old", loser, "/work/task-dup-target")
	insertEnvironmentRepo(t, repo, "repo-row-new", winner, "/work/task-dup-target")

	if err := repo.healDuplicateTaskEnvironments(); !errors.Is(err, errDuplicateEnvironmentPreservation) {
		t.Fatalf("heal error = %v, want the explicit preservation stop", err)
	}
	var owner string
	if err := repo.db.QueryRow(`SELECT task_environment_id FROM task_environment_repos WHERE id = 'repo-row-old'`).Scan(&owner); err != nil {
		t.Fatalf("read loser inventory: %v", err)
	}
	if owner != loser {
		t.Fatalf("loser inventory owner = %q, want it untouched after the rollback", owner)
	}
}

func TestHealDuplicateTaskEnvironments_RelinksBorrowingSessionOfAnotherTask(t *testing.T) {
	repo := newRepoForHealTests(t)
	loser, winner := seedDuplicateWorktreeEnvironments(t, repo, "task-dup-borrow")
	insertTask(t, repo.db, "task-borrower")
	insertTask(t, repo.db, "task-unrelated")
	now := time.Now().UTC()
	for _, row := range []struct{ id, taskID, env string }{
		{"session-borrower", "task-borrower", loser},
		{"session-unrelated", "task-unrelated", "env-elsewhere"},
	} {
		if _, err := repo.db.Exec(`
			INSERT INTO task_sessions (id, task_id, task_environment_id, state, started_at, updated_at)
			VALUES (?, ?, ?, 'CREATED', ?, ?)
		`, row.id, row.taskID, row.env, now, now); err != nil {
			t.Fatalf("insert session %s: %v", row.id, err)
		}
	}

	if err := repo.healDuplicateTaskEnvironments(); err != nil {
		t.Fatalf("heal: %v", err)
	}
	for id, want := range map[string]string{"session-borrower": winner, "session-unrelated": "env-elsewhere"} {
		var got string
		if err := repo.db.QueryRow(`SELECT task_environment_id FROM task_sessions WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("read session %s: %v", id, err)
		}
		if got != want {
			t.Fatalf("session %s environment = %q, want %q", id, got, want)
		}
	}
}
